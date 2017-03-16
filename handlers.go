package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/boltdb/bolt"
	"github.com/echlebek/ranger"
)

// for xml encoding
type bucket struct {
	Keys []string `xml:"key"`
}

func splitPath(path string) [][]byte {
	parts := [][]byte{{'/'}}
	for _, p := range bytes.Split([]byte(path), []byte{'/'}) {
		if len(p) > 0 {
			parts = append(parts, p)
		}
	}
	return parts
}

// returns whether or not the ETag matched any of the If-None-Match values
func checkIfNoneMatch(storedHeader http.Header, req *http.Request) bool {
	if nm := req.Header["If-None-Match"]; len(nm) > 0 {
		for _, m := range nm {
			if m == "*" || m == storedHeader.Get("ETag") {
				return true
			}
		}
	}
	return false
}

func getBucketOrValue(ctx context, w http.ResponseWriter, req *http.Request) {
	var (
		keys []string
		err  error
	)

	parts := splitPath(req.URL.EscapedPath())

	err = ctx.db.View(func(tx *bolt.Tx) error {
		header, err := getHeaderValue(tx, req.URL.EscapedPath())
		if err != nil {
			return fmt.Errorf("couldn't get header: %s", err)
		}

		if header != nil {
			if checkIfNoneMatch(header, req) {
				hdr := make(http.Header)
				hdr.Set("ETag", header.Get("ETag"))
				writeHeader(hdr, w)
				w.WriteHeader(http.StatusNotModified)
				return nil
			}
		}

		p := parts
		if len(parts) > 1 {
			p = parts[:len(parts)-1]
		}
		bucket := getBoltBucket(tx, p) // get the enclosing bucket
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		if len(parts) == 1 {
			keys, err = listKeys(bucket)
			return err
		}

		var value []byte
		bucket, value = getBoltBucketOrValue(bucket, parts[len(parts)-1])
		if bucket == nil && value == nil {
			return bolt.ErrBucketNotFound
		} else if bucket != nil {
			keys, err = listKeys(bucket)
			return err
		} else if value != nil {
			if _, ok := req.Header["Range"]; ok {
				ranges, err := ranger.ParseHeader(req.Header, len(value))
				if err != nil {
					if err == ranger.Error {
						return err
					}
					return errBadRequest
				}
				header.Del("ETag")
				header.Del("Content-Length")
				w.WriteHeader(http.StatusPartialContent)
				writeHeader(header, w)
				for _, r := range ranges {
					_, err := w.Write(value[r.Start : r.Stop+1])
					if err != nil {
						return err
					}
				}
				return nil
			} else {
				writeHeader(header, w)
				_, err := w.Write(value)
				return err
			}
		}
		return nil
	})

	if err == bolt.ErrBucketNotFound {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	} else if err == ranger.Error {
		http.Error(w, "Requested range not satisfiable.", http.StatusRequestedRangeNotSatisfiable)
		return
	} else if err == errBadRequest {
		http.Error(w, "Bad request.", http.StatusBadRequest)
		return
	} else if err != nil {
		log.Println(err)
		http.Error(w, "Out of cheese.", http.StatusInternalServerError)
		return
	}

	if keys != nil {
		writeKeys(w, req, keys)
	}
}

func isText(hdr string) bool {
	return (hdr == "" ||
		strings.HasPrefix(hdr, "text/*") ||
		strings.HasPrefix(hdr, "text/plain") ||
		strings.HasPrefix(hdr, "*/*"))
}

func writeKeys(w http.ResponseWriter, req *http.Request, keys []string) {
	accept := req.Header.Get("Accept")
	if isText(accept) {
		for _, k := range keys {
			if _, err := fmt.Fprintln(w, k); err != nil {
				log.Println(err)
			}
		}
		return
	}
	if strings.HasPrefix(accept, "application/json") {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(keys); err != nil {
			log.Println(err)
		}
		return
	}
	if strings.HasPrefix(accept, "application/xml") {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		if _, err := w.Write([]byte(xml.Header)); err != nil {
			log.Println(err)
		}
		enc := xml.NewEncoder(w)
		enc.Indent("", "  ")
		if err := enc.Encode(bucket{keys}); err != nil {
			log.Println(err)
		}
		return
	}
	if strings.HasPrefix(accept, "text/html") {
		pkg := &KeyPkg{
			Path: req.URL.EscapedPath(),
			Keys: keys,
		}
		if err := keysTmpl.Execute(w, pkg); err != nil {
			log.Println(err)
		}
		return
	}
}

func badPutOrDeleteHeaders(w http.ResponseWriter, req *http.Request) bool {
	if req.ContentLength > 1<<24 {
		http.Error(w, "Request too large.", http.StatusBadRequest)
		return true
	}
	if req.Header.Get("If-None-Match") != "" {
		http.Error(w, "Precondition failed.", http.StatusPreconditionFailed)
		return true
	}
	return false
}

func checkIfMatch(header http.Header, req *http.Request) bool {
	if header == nil {
		for _, m := range req.Header["If-Match"] {
			if m == "*" {
				return false
			}
		}
		return true
	}
	matches, ok := req.Header["If-Match"]
	if !ok {
		return true
	}
	if eTag := header.Get("ETag"); eTag != "" {
		for _, m := range matches {
			if eTag == m || m == "*" {
				return true
			}
		}
	}
	return false
}

func putBucketOrValue(ctx context, w http.ResponseWriter, req *http.Request) {
	if badPutOrDeleteHeaders(w, req) {
		return
	}
	parts := splitPath(req.URL.EscapedPath())
	key := parts[len(parts)-1]
	msg := "Out of cheese."
	status := 500
	err := ctx.db.Update(func(tx *bolt.Tx) error {
		alreadyExists := false
		if req.ContentLength > 0 {
			header, err := getHeaderValue(tx, req.URL.EscapedPath())
			if err != nil {
				log.Printf("couldn't get header: %s", err)
				return err
			}
			if header != nil {
				alreadyExists = true
			}
			if !checkIfMatch(header, req) {
				msg, status = "Precondition failed.", http.StatusPreconditionFailed
				return errors.New("precondition failed")
			}
			parts = parts[:len(parts)-1]
			if len(parts) == 0 {
				msg, status = "Cannot PUT a value in the root bucket.", http.StatusBadRequest
				return errors.New("request to put value in root bucket")
			}
		}
		bucket, err := getOrCreateBoltBucket(tx, parts)
		if err != nil {
			msg, status = "Error processing request.", http.StatusInternalServerError
			log.Println(err)
			return err
		}
		if req.ContentLength > 0 {
			buf := make([]byte, req.ContentLength)
			_, err := io.ReadFull(req.Body, buf)
			if err != nil && err == io.ErrUnexpectedEOF {
				msg, status = "Bad request.", http.StatusBadRequest
			}
			if err != nil {
				log.Println(err)
				return err
			}
			if err := bucket.Put(key, buf); err != nil {
				log.Println(err)
				return err
			}
			header := extractHeader(req.Header)
			eTag := etag(buf)
			header.Set("ETag", eTag)
			lastModified := time.Now().UTC().Format(time.RFC1123Z)
			header.Set("Last-Modified", lastModified)
			if err := writeHeaderValue(tx, req.URL.EscapedPath(), header); err != nil {
				return tx.Rollback()
			}
			w.Header().Set("ETag", eTag)
			w.Header().Set("Last-Modified", lastModified)
			if !alreadyExists {
				w.Header().Set("Location", req.URL.EscapedPath())
				w.WriteHeader(http.StatusCreated)
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
			return nil
		}
		return nil
	})
	if err != nil {
		http.Error(w, msg, status)
		return
	}
}

func writeHeaderValue(tx *bolt.Tx, path string, header http.Header) error {
	bucket := tx.Bucket(headerBucket)
	value, _ := json.Marshal(header)
	return bucket.Put([]byte(path), value)

}

func deleteBucketOrKey(ctx context, w http.ResponseWriter, req *http.Request) {
	if badPutOrDeleteHeaders(w, req) {
		return
	}
	parts := [][]byte{{'/'}}
	escapedPath := []byte(req.URL.EscapedPath())
	for _, p := range bytes.Split(escapedPath, []byte{'/'}) {
		if len(p) > 0 {
			parts = append(parts, p)
		}
	}
	if len(parts) < 2 {
		http.Error(w, "Invalid path.", http.StatusBadRequest)
		return
	}
	var msg, status = "Out of cheese.", http.StatusInternalServerError
	err := ctx.db.Update(func(tx *bolt.Tx) error {
		header, err := getHeaderValue(tx, req.URL.EscapedPath())
		if err != nil {
			log.Printf("couldn't get header: %s", err)
			return err
		}
		if !checkIfMatch(header, req) {
			msg, status = "Precondition failed.", http.StatusPreconditionFailed
			return errors.New("precondition failed")
		}
		if header == nil {
			msg, status = "Not found.", http.StatusNotFound
			return errors.New("Not found")
		}
		bucket := getBoltBucket(tx, parts[:len(parts)-1])
		if bucket == nil {
			// We got the header, but not the content. Something is seriously
			// wrong.
			msg, status = "Internal server error.", http.StatusInternalServerError
			log.Printf("Can't find content for valid header: %+v", header)
			return bolt.ErrBucketNotFound
		}
		if err := tx.Bucket(headerBucket).Delete(escapedPath); err != nil {
			return err
		}
		if err := bucket.Delete(parts[len(parts)-1]); err != nil {
			log.Printf("error: %s (rolling back tx)", err)
			return tx.Rollback()
		}
		return nil
	})
	if err != nil {
		http.Error(w, msg, status)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// base64 encoded etag
func etag(b []byte) string {
	h := fnv.New64a()
	h.Write(b)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func extractHeader(h http.Header) http.Header {
	result := make(http.Header)
	for _, k := range headerFieldsToExtract {
		values := h[k]
		for _, v := range values {
			result.Add(k, v)
		}
	}
	return result
}

func writeHeader(header http.Header, w http.ResponseWriter) {
	for key, values := range header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
}

func getHeader(ctx context, w http.ResponseWriter, req *http.Request) {
	var header http.Header
	err := ctx.db.View(func(tx *bolt.Tx) error {
		var err error
		header, err = getHeaderValue(tx, req.URL.EscapedPath())
		return err
	})
	if err == bolt.ErrBucketNotFound {
		log.Println(err)
		http.Error(w, "Header bucket not found.", http.StatusInternalServerError)
		return
	} else if err != nil {
		log.Println(err)
		http.Error(w, "Out of cheese.", http.StatusInternalServerError)
		return
	}
	if header == nil {
		http.Error(w, "Not found.", http.StatusNotFound)
		return
	}

	writeHeader(header, w)
}

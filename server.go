/*
Copyright 2017 Eric Chlebek

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
the Software, and to permit persons to whom the Software is furnished to do so,
subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/boltdb/bolt"
)

var (
	DBName                = flag.String("db", "bolt.db", "Bolt database to use")
	Port                  = flag.Int("port", 8080, "Port to serve from")
	headerBucket          = append([]byte{0}, []byte("headers")...)
	headerFieldsToExtract = []string{
		"Content-Type",
		"Content-Length",
	}
)

type context struct {
	db *bolt.DB
}

type router struct {
	ctx context
}

func logRequest(req *http.Request) {
	log.Println(req.Method, req.URL.Path)
}

func createHeaderBucketIfNotExists(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(headerBucket)
		if err != nil {
			return err
		}
		return bucket.Put([]byte("/"), []byte("{}"))
	})
}

func createRootBucketIfNotExists(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("/"))
		return err
	})
}

func getBoltBucketOrValue(bucket *bolt.Bucket, key []byte) (*bolt.Bucket, []byte) {
	b := bucket.Bucket(key)
	if b == nil {
		return nil, bucket.Get(key)
	}
	return b, nil
}

func listKeys(bucket *bolt.Bucket) (keys []string, err error) {
	keys = []string{}
	err = bucket.ForEach(func(k, _ []byte) error {
		keys = append(keys, string(k))
		return nil
	})
	return
}

func (r router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logRequest(req)
	switch req.Method {
	case "HEAD":
		getHeader(r.ctx, w, req)
	case "OPTIONS":
		w.Header().Set("Allow", "GET,PUT,DELETE,HEAD")
	case "GET":
		getBucketOrValue(r.ctx, w, req)
	case "PUT":
		putBucketOrValue(r.ctx, w, req)
	case "DELETE":
		deleteBucketOrKey(r.ctx, w, req)
	case "POST", "PATCH", "TRACE", "CONNECT":
		http.Error(w, "Method not allowed.", http.StatusMethodNotAllowed)
	default:
		http.Error(w, "Bad request.", http.StatusBadRequest)
	}
}

// returns the ETag that matched the If-None-Match
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
		header, err := getHeaderValue(tx, req)
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
			writeHeader(header, w)
			_, err := w.Write(value)
			return err
		}
		return nil
	})

	if err == bolt.ErrBucketNotFound {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	} else if err != nil {
		log.Println(err)
		http.Error(w, "Out of cheese.", http.StatusInternalServerError)
		return
	}

	if keys != nil {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(keys); err != nil {
			log.Println(err)
		}
		return
	}

}

func getBoltBucket(tx *bolt.Tx, parts [][]byte) *bolt.Bucket {
	b := tx.Bucket(parts[0])
	if b == nil {
		panic("nil root bucket")
	}
	for _, p := range parts[1:] {
		b = b.Bucket(p)
		if b == nil {
			return b
		}
	}
	return b
}

func getOrCreateBoltBucket(tx *bolt.Tx, parts [][]byte) (*bolt.Bucket, error) {
	b := tx.Bucket(parts[0])
	if b == nil {
		panic("nil root bucket")
	}
	var err error
	for _, p := range parts[1:] {
		b, err = b.CreateBucketIfNotExists(p)
		if err != nil {
			return nil, err
		}
	}
	return b, nil
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
			header, err := getHeaderValue(tx, req)
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
		header, err := getHeaderValue(tx, req)
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

func getHeaderValue(tx *bolt.Tx, req *http.Request) (http.Header, error) {
	var header http.Header

	bucket := tx.Bucket(headerBucket)
	if bucket == nil {
		return nil, bolt.ErrBucketNotFound
	}
	h := bucket.Get([]byte(req.URL.EscapedPath()))
	if h == nil {
		return nil, nil
	}
	err := json.Unmarshal(h, &header)
	return header, err
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
		header, err = getHeaderValue(tx, req)
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

func main() {
	flag.Parse()
	db, err := bolt.Open(*DBName, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	if err := createHeaderBucketIfNotExists(db); err != nil {
		log.Fatal(err)
	}
	if err := createRootBucketIfNotExists(db); err != nil {
		log.Fatal(err)
	}
	ctx := context{db}
	router := router{ctx}
	http.ListenAndServe(fmt.Sprintf(":%d", *Port), router)
}

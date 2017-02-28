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
	"hash/fnv"
	"io"
	"log"
	"net/http"

	"github.com/boltdb/bolt"
)

var (
	DBName       = flag.String("db", "bolt.db", "Bolt database to use")
	headerBucket = append([]byte{0}, []byte("headers")...)
	headerKeys   = []string{
		"Content-Type",
		"Content-Length",
		"ETag",
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

func getBucketOrValue(ctx context, w http.ResponseWriter, req *http.Request) {
	var (
		keys []string
		err  error
	)

	parts := [][]byte{{'/'}}
	for _, p := range bytes.Split([]byte(req.URL.EscapedPath()), []byte{'/'}) {
		if len(p) > 0 {
			parts = append(parts, p)
		}
	}

	err = ctx.db.View(func(tx *bolt.Tx) error {
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
			getHeader(ctx, w, req)
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

func putBucketOrValue(ctx context, w http.ResponseWriter, req *http.Request) {
	parts := [][]byte{{'/'}}
	for _, p := range bytes.Split([]byte(req.URL.EscapedPath()), []byte{'/'}) {
		if len(p) > 0 {
			parts = append(parts, p)
		}
	}

	if req.ContentLength > 1<<24 {
		http.Error(w, "Request too large.", http.StatusBadRequest)
		return
	}
	key := parts[len(parts)-1]
	msg := "Out of cheese."
	status := 500
	err := ctx.db.Update(func(tx *bolt.Tx) error {
		if req.ContentLength > 0 {
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
			w.Header().Set("ETag", eTag)
			return writeHeader(tx, req.URL.EscapedPath(), header)

		} else {
			// Try to read exactly one byte to see if the request
			// has a non-empty body.
			buf := make([]byte, 1)
			if _, err := io.ReadFull(req.Body, buf); err == io.ErrUnexpectedEOF {
				// We got some bytes in the body without a Content-Length header.
				msg, status = "Content-Length required.", http.StatusLengthRequired
				return errors.New("No Content-Length")
			} else if err != io.EOF {
				msg, status = "Error processing request.", http.StatusInternalServerError
				log.Println(err)
				return err
			} else {
				return nil
			}
		}
	})
	if err != nil {
		http.Error(w, msg, status)
		return
	}
}

func writeHeader(tx *bolt.Tx, path string, header http.Header) error {
	bucket := tx.Bucket(headerBucket)
	value, _ := json.Marshal(header)
	return bucket.Put([]byte(path), value)

}

func deleteBucketOrKey(ctx context, w http.ResponseWriter, req *http.Request) {
	parts := [][]byte{{'/'}}
	for _, p := range bytes.Split([]byte(req.URL.EscapedPath()), []byte{'/'}) {
		if len(p) > 0 {
			parts = append(parts, p)
		}
	}
	if len(parts) < 2 {
		http.Error(w, "Invalid path.", http.StatusBadRequest)
		return
	}
	err := ctx.db.Update(func(tx *bolt.Tx) error {
		bucket := getBoltBucket(tx, parts[:len(parts)-1])
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		return bucket.Delete(parts[len(parts)-1])
	})
	if err == bolt.ErrBucketNotFound {
		http.Error(w, "Not found.", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "Out of cheese.", http.StatusInternalServerError)
		log.Println(err)
		return
	}
}

// base64 encoded etag
func etag(b []byte) string {
	h := fnv.New64a()
	return base64.StdEncoding.EncodeToString(h.Sum(b))
}

func extractHeader(h http.Header) http.Header {
	result := make(http.Header)
	for _, k := range headerKeys {
		values := h[k]
		for _, v := range values {
			result.Add(k, v)
		}
	}
	return result
}

func getHeader(ctx context, w http.ResponseWriter, req *http.Request) {
	var header http.Header
	err := ctx.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(headerBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		h := bucket.Get([]byte(req.URL.EscapedPath()))
		if h == nil {
			return bolt.ErrBucketNotFound
		}
		return json.Unmarshal(h, &header)
	})
	if err == bolt.ErrBucketNotFound {
		http.Error(w, "Not found.", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "Out of cheese.", http.StatusInternalServerError)
		return
	}

	for key, values := range header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

}

func main() {
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
	http.ListenAndServe(":8080", router)
}

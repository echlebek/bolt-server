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

package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/boltdb/bolt"
)

var (
	headerBucket          = append([]byte{0}, []byte("headers")...)
	headerFieldsToExtract = []string{
		"Content-Type",
		"Content-Length",
	}
	errBadRequest = errors.New("bad request")
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

func New(dbName string) (http.Handler, error) {
	db, err := bolt.Open(dbName, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't open bolt db: %s", err)
	}
	if err := createHeaderBucketIfNotExists(db); err != nil {
		return nil, fmt.Errorf("couldn't create header bucket: %s", err)
	}
	if err := createRootBucketIfNotExists(db); err != nil {
		return nil, fmt.Errorf("couldn't create root bucket: %s", err)
	}

	return router{ctx: context{db: db}}, nil
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
		w.Header().Set("Allow", "GET,PUT,DELETE,HEAD")
		http.Error(w, "Method not allowed.", http.StatusMethodNotAllowed)
	default:
		http.Error(w, "Bad request.", http.StatusBadRequest)
	}
}

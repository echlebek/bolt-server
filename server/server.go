// Copyright 2017 Eric Chlebek. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/boltdb/bolt"
	"github.com/echlebek/bolt-server/config"
	"github.com/gorilla/csrf"
)

var (
	headerBucket          = append([]byte{0}, []byte("headers")...)
	headerFieldsToExtract = []string{
		"Content-Type",
		"Content-Length",
	}
	errBadRequest = errors.New("bad request")
)

const (
	boltDBContextKey = "bolt"
)

func withDB(ctx context.Context, db *bolt.DB) context.Context {
	return context.WithValue(ctx, boltDBContextKey, db)
}

type router struct {
	ctx  context.Context
	csrf bool
}

func logRequest(req *http.Request) {
	log.Println(req.Method, req.URL.Path)
}

func New(dbName string, cfg config.Data) (http.Handler, error) {
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

	ctx := withDB(context.Background(), db)
	var handler http.Handler = router{ctx: ctx, csrf: len(cfg.CSRF.Key) == 32}

	if len(cfg.CSRF.Key) == 32 {
		handler = csrf.Protect([]byte(cfg.CSRF.Key))(handler)
	}

	return handler, nil
}

func (r router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logRequest(req)

	if r.csrf {
		switch req.Method {
		case "HEAD", "OPTIONS", "GET":
			w.Header().Set("X-CSRF-Token", csrf.Token(req))
		}
	}

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

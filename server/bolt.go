// Copyright 2017 Eric Chlebek. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package server

import (
	"encoding/json"
	"net/http"

	"github.com/boltdb/bolt"
)

func createHeaderBucketIfNotExists(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(headerBucket)
		if err != nil {
			return err
		}
		if bucket.Get([]byte("/")) == nil {
			return bucket.Put([]byte("/"), []byte("{}"))
		}
		return nil
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

func getHeaderValue(tx *bolt.Tx, path string) (http.Header, error) {
	var header http.Header

	bucket := tx.Bucket(headerBucket)
	if bucket == nil {
		return nil, bolt.ErrBucketNotFound
	}
	h := bucket.Get([]byte(path))
	if h == nil {
		return nil, nil
	}
	err := json.Unmarshal(h, &header)
	return header, err
}

// Copyright 2017 Eric Chlebek. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/echlebek/bolt-server/config"
	"github.com/echlebek/bolt-server/server"
)

var (
	DBName = flag.String("db", "bolt.db", "Bolt database to use")
	Port   = flag.Int("port", 8080, "Port to serve from")
	Config = flag.String("config", "", "Config file (JSON)")
)

func main() {
	flag.Parse()
	var cfg config.Data
	if len(*Config) > 0 {
		var err error
		cfg, err = config.New(*Config)
		if err != nil {
			log.Fatalf("fatal: %s", err)
		}
	}
	handler, err := server.New(*DBName, cfg)
	if err != nil {
		log.Fatalf("fatal : %s", err)
	}
	http.ListenAndServe(fmt.Sprintf(":%d", *Port), handler)
}

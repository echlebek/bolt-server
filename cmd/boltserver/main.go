package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/echlebek/bolt-server/server"
)

var (
	DBName = flag.String("db", "bolt.db", "Bolt database to use")
	Port   = flag.Int("port", 8080, "Port to serve from")
)

func main() {
	flag.Parse()
	handler, err := server.New(*DBName)
	if err != nil {
		log.Fatalf("fatal error: %s", err)
	}
	http.ListenAndServe(fmt.Sprintf(":%d", *Port), handler)
}

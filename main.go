package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"time"

	"go.universe.tf/mixtape/db"
	"go.universe.tf/mixtape/scanner"
)

func main() {
	go http.ListenAndServe("[::]:1234", nil)

	db, err := db.Open("mixtape.db")
	if err != nil {
		log.Fatal(err)
	}

	roots, err := roots(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	for {
		log.Print("Scanning roots ", roots)
		if err := scanner.Scan(db, os.DirFS("/"), roots); err != nil {
			log.Print("Error during scan: ", err)
		}

		log.Print("Updating sums")
		if err := scanner.Sum(db, os.DirFS("/")); err != nil {
			log.Print("Error during sum updating: ", err)
		}

		time.Sleep(10 * time.Second)
	}

	db.Close()
}

func roots(rs []string) ([]string, error) {
	for i := range rs {
		root, err := filepath.Abs(rs[i])
		if err != nil {
			return nil, err
		}
		rs[i] = root[1:]
	}
	return rs, nil
}

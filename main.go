package main

import (
	"log"

	"go.universe.tf/mixtape/db"
)

func main() {
	db, err := db.Open("mixtape.db")
	if err != nil {
		log.Fatal(err)
	}

	db.Close()
}

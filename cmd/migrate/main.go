package main

import (
	"fmt"
	"log"
	"os"

	"keyavi-videos/internal/db"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "up" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/migrate up")
		os.Exit(2)
	}

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL est obligatoire")
	}

	if err := db.MigrerHaut(url, "migrations"); err != nil {
		log.Fatal(err)
	}
}

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

type metadata struct {
	App       string `json:"app"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
	Now       string `json:"now"`
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("Hello from a signed Go container image.\n"))
	})

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	mux.HandleFunc("/metadata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metadata{
			App:       "notation-go-handson",
			Version:   version,
			Commit:    commit,
			BuildTime: buildTime,
			Now:       time.Now().UTC().Format(time.RFC3339),
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	addr := ":" + port
	log.Printf("starting server on %s version=%s commit=%s buildTime=%s", addr, version, commit, buildTime)
	log.Fatal(http.ListenAndServe(addr, mux))
}

package main

import (
	"log"
	"net/http"
	"os"
	"sync/atomic"
)

var ready atomic.Bool

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	// The example validates presence only; it deliberately does not claim to connect.
	_ = databaseURL

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	metricsEnabled := os.Getenv("ENABLE_METRICS") == "true"
	apiToken, _ := os.LookupEnv("API_TOKEN")
	_ = apiToken

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if !ready.Load() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
	if metricsEnabled {
		mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("requests_total 0\n"))
		})
	}

	ready.Store(true)
	log.Printf("payment-service listening on %s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

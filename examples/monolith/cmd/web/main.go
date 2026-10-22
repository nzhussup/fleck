package main

import (
	"log"
	"net/http"
	"os"

	"example.com/inventory-monolith/internal/catalog"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/products", catalog.Products)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

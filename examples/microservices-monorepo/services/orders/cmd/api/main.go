package main

import (
	"log"
	"net/http"
	"os"

	"example.com/commerce/shared/logging"
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

	logging.Started("orders")
	log.Fatal(http.ListenAndServe(":"+port, http.NewServeMux()))
}

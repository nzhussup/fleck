package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	redisAddress := os.Getenv("REDIS_ADDRESS")
	if redisAddress == "" {
		log.Fatal("REDIS_ADDRESS is required")
	}

	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("using cache %s", redisAddress)
	log.Fatal(http.ListenAndServe(":"+port, http.NewServeMux()))
}

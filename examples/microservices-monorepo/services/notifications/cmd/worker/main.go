package main

import (
	"log"
	"os"

	"example.com/commerce/shared/logging"
)

func main() {
	queueURL := os.Getenv("QUEUE_URL")
	if queueURL == "" {
		log.Fatal("QUEUE_URL is required")
	}

	concurrency := os.Getenv("WORKER_CONCURRENCY")
	if concurrency == "" {
		concurrency = "4"
	}

	logging.Started("notifications")
	log.Printf("consuming %s with concurrency %s", queueURL, concurrency)
}

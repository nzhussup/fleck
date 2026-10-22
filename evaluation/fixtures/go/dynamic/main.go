package main

import "os"

func read(prefix, name string) string {
	return os.Getenv(prefix + name)
}

func main() { _ = read("APP_", "TOKEN") }

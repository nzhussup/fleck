package logging

import "log"

func Started(service string) {
	log.Printf("%s started", service)
}

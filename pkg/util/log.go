package util

import (
	"log"
	"os"
)

// LogDebugf logs a message if the DEBUG environment variable is set.
func LogDebugf(msg string, args ...interface{}) {
	if os.Getenv("DEBUG") != "" {
		log.Printf(msg, args...)
	}
}

func LogDebugln(msg string) {
	if os.Getenv("DEBUG") != "" {
		log.Println(msg)
	}
}

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

func LogWarnf(msg string, args ...interface{}) {
	log.Printf("(WARNING) "+msg, args...)
}

func LogWarnln(msg string) {
	log.Println("(WARNING) " + msg)
}

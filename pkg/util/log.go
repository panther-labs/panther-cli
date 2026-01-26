package util

import (
	"fmt"
	"log"
	"os"
)

const (
	red = "\033[0;31m"
	end = "\033[0m"
)

func LogRedf(format string, args ...any) {
	formattedMsg := fmt.Sprintf(format, args...)
	coloredMsg := fmt.Sprintf("%s%s%s", red, formattedMsg, end)
	log.Print(coloredMsg)
}

func LogRedln(msg string) {
	coloredMsg := fmt.Sprintf("%s%s%s", red, msg, end)
	log.Println(coloredMsg)
}

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

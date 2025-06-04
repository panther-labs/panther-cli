package main

import (
	"fmt"
	"log"
	"os"

	"github.com/panther-labs/panther-cli/pkg/rsapem"
)

func main() {
	privateKeyAsStr, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatalf("failed to read RSA private key from PEM file: %v", err)
	}

	privateKey, err := rsapem.ParseRSAPEMPrivateKey(string(privateKeyAsStr))
	if err != nil {
		log.Fatalf("failed to read RSA private key from PEM file: %v", err)
	}
	fmt.Printf("Public Key for %s:\n\n", os.Args[1])
	fmt.Println(rsapem.MustFormatPublicKeyForSnowflake(privateKey))
	fmt.Println()
}

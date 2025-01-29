package util

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"log"

	"github.com/pkg/errors"
)

// ParseRSAPEMPrivateKey parses a string expected as a PEM block encoded RSA PRIVATE KEY
func ParseRSAPEMPrivateKey(raw string) (*rsa.PrivateKey, error) {
	if raw == "" {
		return nil, nil
	}
	block, remainder := pem.Decode([]byte(raw))
	if block == nil || len(remainder) > 0 {
		return nil, errors.New("could not decode private key PEM block into single block with no remainder")
	}
	privateKeyEncoded, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.Wrapf(err, "parse private key as PKCS #8")
	}
	rsaKey, ok := privateKeyEncoded.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.Errorf("cast x509 public key as RSA PRIVATE KEY")
	}
	return rsaKey, nil
}

// FormatPublicKeyFromPrivateKey takes an rsa.PrivateKey and returns a PEM-encoded public key string
func FormatPublicKeyFromPrivateKey(privateKey *rsa.PrivateKey) (string, error) {
	// Extract the public key from the private key
	publicKey := &privateKey.PublicKey

	// Marshal the public key into ASN.1 DER encoded format
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal public key")
	}

	// Encode the public key bytes into PEM format
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	})

	// Return the PEM-encoded public key as a string
	return string(pubKeyPEM), nil
}

func MustFormatPublicKeyFromPrivateKey(privateKey *rsa.PrivateKey) string {
	pubKey, err := FormatPublicKeyFromPrivateKey(privateKey)
	if err != nil {
		log.Fatalf("failed to format public key from private key: %s", err.Error())
	}
	return pubKey
}

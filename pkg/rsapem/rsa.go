package rsapem

/**
 * Copyright (C) 2022 Panther Labs, Inc.
 *
 * The Panther SaaS is licensed under the terms of the Panther Enterprise Subscription
 * Agreement available at https://panther.com/enterprise-subscription-agreement/.
 * All intellectual property rights in and to the Panther SaaS, including any and all
 * rights to access the Panther SaaS, are governed by the Panther Enterprise Subscription Agreement.
 */

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"log"

	"github.com/pkg/errors"
)

const (
	PantherRSABitSize = 4096
)

// RSA Key Functions to interoperate with Snowflake key pair auth https://docs.snowflake.com/en/user-guide/key-pair-auth

// KeyPair contains a PEM-encoded (base64 DER) RSA keypair.
type KeyPair struct {
	PrivateKey *rsa.PrivateKey
	PublicKey  *rsa.PublicKey
}

func GenerateKeyPair() (*KeyPair, error) {
	key, err := rsa.GenerateKey(rand.Reader, PantherRSABitSize)
	if err != nil {
		return nil, errors.Wrapf(err, "generating %d bit RSA keypair", PantherRSABitSize)
	}
	return &KeyPair{
		PrivateKey: key,
		PublicKey:  &key.PublicKey,
	}, nil
}

// EncodeRSAPEMPrivateKey packages the private key as a PEM block,
func EncodeRSAPEMPrivateKey(r *rsa.PrivateKey) (string, error) {
	if r == nil {
		return "", nil
	}
	bytes, err := x509.MarshalPKCS8PrivateKey(r)
	if err != nil {
		return "", err
	}
	pemBytes := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: bytes,
		},
	)
	return string(pemBytes), nil
}

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

// EncodeRSAPEMPublicKey packages the public key as base64 encoded DER ASN.1
func EncodeRSAPEMPublicKey(r *rsa.PublicKey) (string, error) {
	if r == nil {
		return "", nil
	}
	publicKeyMarshalled, err := x509.MarshalPKIXPublicKey(r)
	if err != nil {
		return "", err
	}
	publicKeyString := base64.StdEncoding.EncodeToString(publicKeyMarshalled)
	return publicKeyString, nil
}

// ParseRSAPEMPublicKey parses string expected as a base 64 encoded DER ASN.1 RSA key
// Will return an empty key rather than errors in cases with empty string
func ParseRSAPEMPublicKey(raw string) (*rsa.PublicKey, error) {
	if raw == "" {
		return nil, nil
	}
	bytes, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.Wrap(err, "base 64 decode on public key string")
	}
	publicKeyEncoded, err := x509.ParsePKIXPublicKey(bytes)
	if err != nil {
		return nil, errors.Wrap(err, "parse public key in PKIX format")
	}
	rsaKey, ok := publicKeyEncoded.(*rsa.PublicKey)
	if !ok {
		return nil, errors.Errorf("cast x509 public key as RSA PUBLIC KEY")
	}
	return rsaKey, nil
}

// FormatPublicKey takes an rsa.PrivateKey and returns a PEM-encoded public key string
func FormatPublicKey(privateKey *rsa.PrivateKey) (string, error) {
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

func MustFormatPublicKey(privateKey *rsa.PrivateKey) string {
	pubKey, err := FormatPublicKey(privateKey)
	if err != nil {
		log.Fatalf("failed to format public key from private key: %s", err.Error())
	}
	return pubKey
}

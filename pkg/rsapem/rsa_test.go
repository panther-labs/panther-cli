package rsapem

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_RSAPEMKeys(t *testing.T) {
	// generate once, as it's slow in a unit test
	keyPair, err := GenerateKeyPair()
	require.NoError(t, err)
	require.NotNil(t, keyPair.PrivateKey)
	require.NotNil(t, keyPair.PublicKey)
	prvKeyStr, err := EncodeRSAPEMPrivateKey(keyPair.PrivateKey)
	require.NoError(t, err)

	t.Run("sanity check public key", func(t *testing.T) {
		// encode
		pubkey := keyPair.PrivateKey.PublicKey
		pubBytes, err := x509.MarshalPKIXPublicKey(&pubkey)
		require.NoError(t, err)
		pubkeyPEM := pem.EncodeToMemory(
			&pem.Block{
				Type:  "RSA PUBLIC KEY",
				Bytes: pubBytes,
			},
		)
		// decode
		block, remainder := pem.Decode(pubkeyPEM)
		require.Len(t, remainder, 0)
		assert.Equal(t, "RSA PUBLIC KEY", block.Type)
		decodedKey, err := x509.ParsePKIXPublicKey(block.Bytes)
		require.NoError(t, err)
		decodedRSA, ok := decodedKey.(*rsa.PublicKey)
		require.True(t, ok)
		assert.Equal(t, pubkey, *decodedRSA)
	})

	t.Run("KeyPair expected format", func(t *testing.T) {
		// Test private key decodes back to rsa.PrivateKey.
		block, rest := pem.Decode([]byte(prvKeyStr))
		require.Empty(t, rest)
		privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		require.NoError(t, err)
		require.IsType(t, &rsa.PrivateKey{}, privateKey)
		rsaPrivateKey, ok := privateKey.(*rsa.PrivateKey)
		require.True(t, ok)
		// check the public component is the correct type
		require.IsType(t, rsa.PublicKey{}, rsaPrivateKey.PublicKey)
	})

	t.Run("deserialize private key", func(t *testing.T) {
		key, err := ParseRSAPEMPrivateKey(prvKeyStr)
		require.NoError(t, err)
		assert.Equal(t, 512, key.Size())
	})

	t.Run("publickey parsed", func(t *testing.T) {
		pubStr, err := EncodeRSAPEMPublicKey(keyPair.PublicKey)
		require.NoError(t, err)
		parsedPubKey, err := ParseRSAPEMPublicKey(pubStr)
		require.NoError(t, err)
		assert.True(t, keyPair.PublicKey.Equal(parsedPubKey))
	})
}

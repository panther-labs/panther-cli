package util

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadContent_FileLoading(t *testing.T) {
	// Create a temporary file for testing
	content := "test content"
	tmpFile, err := os.CreateTemp("", "test-*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	err = tmpFile.Close()
	require.NoError(t, err)

	// Test with file path
	t.Run("Local file path", func(t *testing.T) {
		result, err := LoadContent(context.Background(), tmpFile.Name())
		require.NoError(t, err)
		assert.Equal(t, content, result)
	})

	// Test with file:// scheme
	t.Run("file:// scheme", func(t *testing.T) {
		fileURI := "file://" + tmpFile.Name()
		result, err := LoadContent(context.Background(), fileURI)
		require.NoError(t, err)
		assert.Equal(t, content, result)
	})

	// Test with non-existent file
	t.Run("Non-existent file", func(t *testing.T) {
		nonExistentPath := filepath.Join(os.TempDir(), "non-existent-file.txt")
		_, err := LoadContent(context.Background(), nonExistentPath)
		assert.Error(t, err)
	})

	// Test with unsupported scheme
	t.Run("Unsupported scheme", func(t *testing.T) {
		_, err := LoadContent(context.Background(), "ftp://example.com/file.txt")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported URI scheme")
	})
}

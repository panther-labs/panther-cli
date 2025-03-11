package util

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetURLAsString(t *testing.T) {
	// Create a test server
	testContent := "test http content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/success" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(testContent))
		} else if r.URL.Path == "/notfound" {
			w.WriteHeader(http.StatusNotFound)
		} else if r.URL.Path == "/error" {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	// Test successful HTTP request
	t.Run("Successful HTTP GET", func(t *testing.T) {
		result, err := getURLAsString(context.Background(), server.URL+"/success")
		require.NoError(t, err)
		assert.Equal(t, testContent, result)
	})

	// Test 404 Not Found
	t.Run("404 Not Found", func(t *testing.T) {
		_, err := getURLAsString(context.Background(), server.URL+"/notfound")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})

	// Test 500 Internal Server Error
	t.Run("500 Internal Server Error", func(t *testing.T) {
		_, err := getURLAsString(context.Background(), server.URL+"/error")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})

	// Test invalid URL
	t.Run("Invalid URL", func(t *testing.T) {
		_, err := getURLAsString(context.Background(), "http://invalid-domain-that-should-not-exist.example")
		require.Error(t, err)
	})

	// Test HTTP loader through LoadContent
	t.Run("LoadContent with HTTP URL", func(t *testing.T) {
		result, err := LoadContent(context.Background(), server.URL+"/success")
		require.NoError(t, err)
		assert.Equal(t, testContent, result)
	})
}

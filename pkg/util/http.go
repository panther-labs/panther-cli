package util

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// if the HTTP request takes longer than 30 seconds, something terrible has happened
const httpTimeout = 30 * time.Second

func getURLAsString(ctx context.Context, url string) (string, error) {
	ctxCancellable, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctxCancellable, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	// Make a GET request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("failed to fetch URL: %s, status code: %d", url, resp.StatusCode)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

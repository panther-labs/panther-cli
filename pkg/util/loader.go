package util

import (
	"context"
	"io"
	"log"
	"os"
	"strings"

	"github.com/pkg/errors"
)

// contentLoader is an interface to load content based on different URI schemas.
type contentLoader interface {
	load(ctx context.Context, uri string) (string, error)
}

// httpLoader handles HTTP/HTTPS URIs.
type httpLoader struct{}

func (h httpLoader) load(ctx context.Context, uri string) (string, error) {
	return getURLAsString(ctx, uri)
}

// fileLoader handles file:// URIs or local file paths.
type fileLoader struct{}

func (f fileLoader) load(_ context.Context, uri string) (string, error) {
	filePath := strings.TrimPrefix(uri, "file://")

	file, err := os.Open(filePath)
	if err != nil {
		return "", errors.Errorf("failed to open file: %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Fatalf("failed to close file: %v\n", err)
		}
	}()

	content, err := io.ReadAll(file)
	if err != nil {
		return "", errors.Errorf("failed to read file content: %v", err)
	}

	return string(content), nil
}

// LoadContent determines the correct loader based on the URI scheme and loads the content.
// The only supported URI schemas are http(s):// and file://.
func LoadContent(ctx context.Context, uri string) (string, error) {
	var loader contentLoader

	switch {
	case strings.HasPrefix(uri, "http://"), strings.HasPrefix(uri, "https://"):
		loader = httpLoader{}
	case strings.HasPrefix(uri, "file://"):
		loader = fileLoader{}
	case !strings.Contains(uri, "://"): // Assume it's a local file path
		loader = fileLoader{}
	default:
		return "", errors.Errorf("unsupported URI scheme: %s", uri)
	}

	return loader.load(ctx, uri)
}

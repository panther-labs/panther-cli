package util

import (
	"time"

	"github.com/cenkalti/backoff/v4"
)

func GetDefaultExponentialBackoffRetrier() *backoff.ExponentialBackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 5 * time.Second
	b.MaxInterval = 30 * time.Second
	b.MaxElapsedTime = 5 * time.Minute
	return b
}

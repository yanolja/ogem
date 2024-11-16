package state

import (
	"context"
	"time"
)

type Manager interface {
	// Checks if the model in the region of the provider is allowed to be used.
	// If not, returns false and the duration to wait before retrying.
	Allow(ctx context.Context, provider string, region string, model string, interval time.Duration) (bool, time.Duration, error)

	// Disables the model in the region of the provider for a given duration.
	Disable(ctx context.Context, provider string, region string, model string, duration time.Duration) error

	// Saves the cache for a given key with a given duration.
	SaveCache(ctx context.Context, key string, value []byte, duration time.Duration) error

	// Loads the cache for a given key.
	LoadCache(ctx context.Context, key string) ([]byte, error)
}

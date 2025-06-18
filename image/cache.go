package image

import (
	"context"
	"time"

	"github.com/yanolja/ogem/state"
)

type StateCacheManager struct {
	stateManager state.Manager
}

func NewStateCacheManager(stateManager state.Manager) *StateCacheManager {
	return &StateCacheManager{
		stateManager: stateManager,
	}
}

func (s *StateCacheManager) Get(ctx context.Context, key string) ([]byte, error) {
	return s.stateManager.LoadCache(ctx, key)
}

func (s *StateCacheManager) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return s.stateManager.SaveCache(ctx, key, value, ttl)
}
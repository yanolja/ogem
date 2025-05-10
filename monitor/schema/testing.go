package schema

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"
)

type MockManager struct {
	mock.Mock
}

func (m *MockManager) Allow(ctx context.Context, provider string, region string, model string, interval time.Duration) (bool, time.Duration, error) {
	args := m.Called(ctx, provider, region, model, interval)
	return args.Bool(0), args.Get(1).(time.Duration), args.Error(2)
}

func (m *MockManager) Disable(ctx context.Context, provider string, region string, model string, duration time.Duration) error {
	args := m.Called(ctx, provider, region, model, duration)
	return args.Error(0)
}

func (m *MockManager) SaveCache(ctx context.Context, key string, value []byte, duration time.Duration) error {
	args := m.Called(ctx, key, value, duration)
	return args.Error(0)
}

func (m *MockManager) LoadCache(ctx context.Context, key string) ([]byte, error) {
	args := m.Called(ctx, key)
	return args.Get(0).([]byte), args.Error(1)
}

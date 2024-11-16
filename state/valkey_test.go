package state

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	valkeymock "github.com/valkey-io/valkey-go/mock"
	"go.uber.org/mock/gomock"
)

func TestValkeyManager(t *testing.T) {
	t.Run("Allow method", func(t *testing.T) {
		t.Run("success when not disabled", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := valkeymock.NewClient(ctrl)
			manager := NewValkeyManager(mockClient)
			ctx := context.Background()

			mockResponse := valkeymock.Result(valkeymock.ValkeyArray(valkeymock.ValkeyInt64(1)))
			mockClient.EXPECT().
				Do(ctx, valkeymock.MatchFn(func(cmd []string) bool {
					return cmd[0] == "EVAL" &&
						cmd[len(cmd)-2] == "ogem:disabled:openai:us-east1:gpt4" &&
						cmd[len(cmd)-1] == "100"
				}, "EVAL script with correct key and interval")).
				Return(mockResponse)

			allowed, wait, err := manager.Allow(
				ctx, "openai", "us-east1", "gpt4", 100*time.Millisecond)

			assert.NoError(t, err)
			assert.True(t, allowed)
			assert.Equal(t, time.Duration(0), wait)
		})

		t.Run("not allowed when disabled", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := valkeymock.NewClient(ctrl)
			manager := NewValkeyManager(mockClient)
			ctx := context.Background()

			mockResponse := valkeymock.Result(valkeymock.ValkeyArray(
				valkeymock.ValkeyInt64(0),
				valkeymock.ValkeyInt64(50000),
			))

			mockClient.EXPECT().
				Do(ctx, valkeymock.MatchFn(func(cmd []string) bool {
					return cmd[0] == "EVAL" &&
						cmd[len(cmd)-2] == "ogem:disabled:openai:us-east1:gpt4" &&
						cmd[len(cmd)-1] == "100"
				}, "EVAL script with correct key and interval")).
				Return(mockResponse)

			// Parameters here do not matter because the mock response is always
			// the same.
			allowed, wait, err := manager.Allow(
				ctx, "openai", "us-east1", "gpt4", 100*time.Millisecond)

			assert.NoError(t, err)
			assert.False(t, allowed)
			assert.Equal(t, 50*time.Millisecond, wait)
		})

		t.Run("handles error", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := valkeymock.NewClient(ctrl)
			manager := NewValkeyManager(mockClient)
			ctx := context.Background()

			mockClient.EXPECT().
				Do(ctx, gomock.Any()).
				Return(valkeymock.ErrorResult(fmt.Errorf("redis error")))

			allowed, wait, err := manager.Allow(ctx, "openai", "us-east1", "gpt4", 100*time.Millisecond)

			assert.Error(t, err)
			assert.False(t, allowed)
			assert.Equal(t, time.Duration(0), wait)
		})
	})

	t.Run("Cache operations", func(t *testing.T) {
		t.Run("SaveCache success", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := valkeymock.NewClient(ctrl)
			manager := NewValkeyManager(mockClient)
			ctx := context.Background()

			mockClient.EXPECT().
				Do(ctx, valkeymock.Match("SET", "test-key", "test-value", "EX", "1")).
				Return(valkeymock.Result(valkeymock.ValkeyString("OK")))

			err := manager.SaveCache(ctx, "test-key", []byte("test-value"), time.Second)
			assert.NoError(t, err)
		})

		t.Run("LoadCache success", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := valkeymock.NewClient(ctrl)
			manager := NewValkeyManager(mockClient)
			ctx := context.Background()

			expectedValue := []byte("test-value")
			mockClient.EXPECT().
				Do(ctx, valkeymock.Match("GET", "test-key")).
				Return(valkeymock.Result(valkeymock.ValkeyBlobString(string(expectedValue))))

			value, err := manager.LoadCache(ctx, "test-key")
			assert.NoError(t, err)
			assert.Equal(t, expectedValue, value)
		})

		t.Run("LoadCache handles nil value", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := valkeymock.NewClient(ctrl)
			manager := NewValkeyManager(mockClient)
			ctx := context.Background()

			mockClient.EXPECT().
				Do(ctx, valkeymock.Match("GET", "test-key")).
				Return(valkeymock.Result(valkeymock.ValkeyNil()))

			value, err := manager.LoadCache(ctx, "test-key")
			assert.NoError(t, err)
			assert.Nil(t, value)
		})
	})

	t.Run("Edge cases", func(t *testing.T) {
		t.Run("context cancellation", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := valkeymock.NewClient(ctrl)
			manager := NewValkeyManager(mockClient)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			mockClient.EXPECT().
				Do(ctx, gomock.Any()).
				Return(valkeymock.ErrorResult(context.Canceled))

			err := manager.SaveCache(ctx, "test-key", []byte("test-value"), time.Second)
			assert.Error(t, err)
			assert.Equal(t, context.Canceled, err)
		})

		t.Run("zero duration", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := valkeymock.NewClient(ctrl)
			manager := NewValkeyManager(mockClient)
			ctx := context.Background()

			mockClient.EXPECT().
				Do(ctx, valkeymock.Match("SET", "test-key", "test-value", "EX", "0")).
				Return(valkeymock.Result(valkeymock.ValkeyString("OK")))

			err := manager.SaveCache(ctx, "test-key", []byte("test-value"), 0)
			assert.NoError(t, err)
		})

		t.Run("large values", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := valkeymock.NewClient(ctrl)
			manager := NewValkeyManager(mockClient)
			ctx := context.Background()

			largeValue := make([]byte, 1024*1024) // 1MB
			mockClient.EXPECT().
				Do(ctx, valkeymock.MatchFn(func(cmd []string) bool {
					return cmd[0] == "SET" &&
						cmd[1] == "test-key" &&
						len(cmd[2]) == 1024*1024 &&
						cmd[3] == "EX" &&
						cmd[4] == "1"
				}, "SET with large value")).
				Return(valkeymock.Result(valkeymock.ValkeyString("OK")))

			err := manager.SaveCache(ctx, "test-key", largeValue, time.Second)
			assert.NoError(t, err)
		})
	})
}

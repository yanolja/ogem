package imagedownloader

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockStateManager struct {
	mock.Mock
}

func (m *MockStateManager) Allow(ctx context.Context, provider string, region string, model string, interval time.Duration) (bool, time.Duration, error) {
	args := m.Called(ctx, provider, region, model, interval)
	return args.Bool(0), args.Get(1).(time.Duration), args.Error(2)
}

func (m *MockStateManager) Disable(ctx context.Context, provider string, region string, model string, duration time.Duration) error {
	args := m.Called(ctx, provider, region, model, duration)
	return args.Error(0)
}

func (m *MockStateManager) SaveCache(ctx context.Context, key string, value []byte, duration time.Duration) error {
	args := m.Called(ctx, key, value, duration)
	return args.Error(0)
}

func (m *MockStateManager) LoadCache(ctx context.Context, key string) ([]byte, error) {
	args := m.Called(ctx, key)
	return args.Get(0).([]byte), args.Error(1)
}

func TestFetchImageAsBase64(t *testing.T) {
	ctx := context.Background()
	mockStateManager := new(MockStateManager)

	t.Run("Cached Image", func(t *testing.T) {
		cacheKey := fmt.Sprintf("imgcache:%x", sha256.Sum256([]byte("testkey")))
		expectedBase64 := "dGVzdA==" // "test" in base64
		mockStateManager.On("LoadCache", ctx, cacheKey).Return([]byte(expectedBase64), nil)

		downloader := NewImageDownloader(mockStateManager)
		result, err := downloader.FetchImageAsBase64(ctx, "testkey")

		assert.NoError(t, err)
		assert.Equal(t, expectedBase64, result)
		mockStateManager.AssertExpectations(t)
	})

	t.Run("Download And Cache", func(t *testing.T) {
		testImageData := []byte("testdata")
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write(testImageData)
		}))
		defer testServer.Close()

		cacheKey := fmt.Sprintf("imgcache:%x", sha256.Sum256([]byte(testServer.URL)))
		expectedBase64 := base64.StdEncoding.EncodeToString(testImageData)

		mockStateManager.On("LoadCache", ctx, cacheKey).Return([]byte(nil), fmt.Errorf("cache miss"))
		mockStateManager.On("SaveCache", ctx, cacheKey, []byte(expectedBase64), time.Hour).Return(nil)

		downloader := NewImageDownloader(mockStateManager)
		result, err := downloader.FetchImageAsBase64(ctx, testServer.URL)
		assert.NoError(t, err)
		assert.Equal(t, expectedBase64, result)
		mockStateManager.AssertExpectations(t)
	})

	t.Run("HTTP Failure", func(t *testing.T) {
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer testServer.Close()

		cacheKey := fmt.Sprintf("imgcache:%x", sha256.Sum256([]byte(testServer.URL)))
		mockStateManager.On("LoadCache", ctx, cacheKey).Return([]byte(nil), fmt.Errorf("cache miss"))

		downloader := NewImageDownloader(mockStateManager)
		_, err := downloader.FetchImageAsBase64(ctx, testServer.URL)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to download image")
		mockStateManager.AssertExpectations(t)
	})
}

func TestGetImageType(t *testing.T) {
	downloader := NewImageDownloader(nil)

	t.Run("PNG Image", func(t *testing.T) {
		result, err := downloader.GetImageType("https://example.com/image.png")
		assert.NoError(t, err)
		assert.Equal(t, "image/png", result)
	})

	t.Run("JPG Image", func(t *testing.T) {
		result, err := downloader.GetImageType("https://example.com/photo.jpg")
		assert.NoError(t, err)
		assert.Equal(t, "image/jpeg", result)
	})

	t.Run("JPEG Image", func(t *testing.T) {
		result, err := downloader.GetImageType("https://example.com/picture.jpeg")
		assert.NoError(t, err)
		assert.Equal(t, "image/jpeg", result)
	})

	t.Run("GIF Image", func(t *testing.T) {
		result, err := downloader.GetImageType("https://example.com/animation.gif")
		assert.NoError(t, err)
		assert.Equal(t, "image/gif", result)
	})

	t.Run("WEBP Image", func(t *testing.T) {
		result, err := downloader.GetImageType("https://example.com/graphic.webp")
		assert.NoError(t, err)
		assert.Equal(t, "image/webp", result)
	})

	t.Run("Unsupported Image Type", func(t *testing.T) {
		_, err := downloader.GetImageType("https://example.com/unknown.bmp")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported image type")
	})
}

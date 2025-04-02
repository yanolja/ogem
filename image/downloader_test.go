package image

import (
	"context"
	"crypto/sha256"
	"encoding/json"
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

func (m *MockStateManager) LoadCache(ctx context.Context, key string) ([]byte, error) {
	args := m.Called(ctx, key)
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockStateManager) SaveCache(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	args := m.Called(ctx, key, value, expiration)
	return args.Error(0)
}

func (m *MockStateManager) Allow(ctx context.Context, provider string, region string, model string, duration time.Duration) (bool, time.Duration, error) {
	args := m.Called(ctx, provider, region, model, duration)
	return args.Get(0).(bool), args.Get(1).(time.Duration), args.Error(2)
}

func (m *MockStateManager) Disable(ctx context.Context, provider string, region string, model string, duration time.Duration) error {
	args := m.Called(ctx, provider, region, model, duration)
	return args.Error(0)
}

func TestFetchImage(t *testing.T) {
	ctx := context.Background()
	mockState := new(MockStateManager)
	imageURL := "https://example.com/image.jpg"
	cacheKey := fmt.Sprintf("imgcache:%x", sha256.Sum256([]byte(imageURL)))

	t.Run("Cached Image", func(t *testing.T) {
		cachedImage := Image{Data: []byte("fake-data"), Type: ImageTypeJPEG}
		cachedBytes, _ := json.Marshal(cachedImage)
		mockState.On("LoadCache", ctx, cacheKey).Return(cachedBytes, nil)
		
		downloader := NewDownloader(mockState)
		img, err := downloader.FetchImage(ctx, imageURL)

		assert.NoError(t, err)
		assert.NotNil(t, img)
		assert.Equal(t, cachedImage.Data, img.Data)
		assert.Equal(t, cachedImage.Type, img.Type)
		mockState.AssertExpectations(t)
	})

	t.Run("Download and Cache Image", func(t *testing.T) {
		fakeImageData := []byte("fake-image-data")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			w.WriteHeader(http.StatusOK)
			w.Write(fakeImageData)
		}))
		defer server.Close()
		dynamicCacheKey := fmt.Sprintf("imgcache:%x", sha256.Sum256([]byte(server.URL)))
		mockState.On("LoadCache", ctx, dynamicCacheKey).Return([]byte{}, fmt.Errorf("cache miss"))
		mockState.On("SaveCache", ctx, dynamicCacheKey, mock.Anything, cacheExpiry).Return(nil)

		downloader := NewDownloader(mockState)
		img, err := downloader.FetchImage(ctx, server.URL)

		assert.NoError(t, err)
		assert.NotNil(t, img)
		assert.Equal(t, fakeImageData, img.Data)
		assert.Equal(t, ImageTypeJPEG, img.Type)
		mockState.AssertExpectations(t)
	})

	t.Run("Invalid Content Type", func(t *testing.T) {
		fakeImageData := []byte("fake-image-data")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write(fakeImageData)
		}))
		defer server.Close()
		dynamicCacheKey := fmt.Sprintf("imgcache:%x", sha256.Sum256([]byte(server.URL)))
		mockState.On("LoadCache", ctx, dynamicCacheKey).Return([]byte{}, fmt.Errorf("cache miss"))

		downloader := NewDownloader(mockState)
		img, err := downloader.FetchImage(ctx, server.URL)

		assert.Error(t, err)
		assert.Nil(t, img)
		assert.Contains(t, err.Error(), "unsupported image type")
		mockState.AssertExpectations(t)
	})

	t.Run("Failed To Download Image", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()
		dynamicCacheKey := fmt.Sprintf("imgcache:%x", sha256.Sum256([]byte(server.URL)))
		mockState.On("LoadCache", ctx, dynamicCacheKey).Return([]byte{}, fmt.Errorf("cache miss"))

		downloader := NewDownloader(mockState)
		img, err := downloader.FetchImage(ctx, server.URL)

		assert.Error(t, err)
		assert.Nil(t, img)
		assert.Contains(t, err.Error(), "failed to download image")
		mockState.AssertExpectations(t)
	})

	t.Run("Invalid URL", func(t *testing.T) {
		invalidURL := "ht@tp://invalid-url"
		dynamicCacheKey := fmt.Sprintf("imgcache:%x", sha256.Sum256([]byte(invalidURL)))
		mockState.On("LoadCache", ctx, dynamicCacheKey).Return([]byte{}, fmt.Errorf("cache miss"))

		downloader := NewDownloader(mockState)
		img, err := downloader.FetchImage(ctx, invalidURL)

		assert.Error(t, err)
		assert.Nil(t, img)
		assert.Contains(t, err.Error(), "failed to create request")
		mockState.AssertExpectations(t)
	})

	t.Run("Failed To Save Image To Cache", func(t *testing.T) {
		fakeImageData := []byte("fake-image-data")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			w.WriteHeader(http.StatusOK)
			w.Write(fakeImageData)
		}))
		defer server.Close()
		dynamicCacheKey := fmt.Sprintf("imgcache:%x", sha256.Sum256([]byte(server.URL)))
		mockState.On("LoadCache", ctx, dynamicCacheKey).Return([]byte{}, fmt.Errorf("cache miss"))
		mockState.On("SaveCache", ctx, dynamicCacheKey, mock.Anything, cacheExpiry).Return(fmt.Errorf("failed to save image to cache"))

		downloader := NewDownloader(mockState)
		img, err := downloader.FetchImage(ctx, server.URL)

		assert.Error(t, err)
		assert.Nil(t, img)
		assert.Contains(t, err.Error(), "failed to save image to cache")
		mockState.AssertExpectations(t)
	})

	t.Run("HTTP Timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(15 * time.Second) // Simulate timeout
		}))
		defer server.Close()
		dynamicCacheKey := fmt.Sprintf("imgcache:%x", sha256.Sum256([]byte(server.URL)))
		mockState.On("LoadCache", ctx, dynamicCacheKey).Return([]byte{}, fmt.Errorf("cache miss"))

		downloader := NewDownloader(mockState)
		img, err := downloader.FetchImage(ctx, server.URL)

		assert.Error(t, err)
		assert.Nil(t, img)
		assert.Contains(t, err.Error(), "context deadline exceeded")
		mockState.AssertExpectations(t)
	})
}

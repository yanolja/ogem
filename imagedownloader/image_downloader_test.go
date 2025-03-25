package imagedownloader

import (
	"context"
	"crypto/sha1"
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

func TestFetchImageAsBase64_Cached(t *testing.T) {
	ctx := context.Background()
	mockStateManager := new(MockStateManager)
	downloader := NewImageDownloader(mockStateManager)
	cacheKey := fmt.Sprintf("imgcache:%x", sha1.Sum([]byte("testkey")))
	expectedBase64 := "dGVzdA==" // "test" in base64
	mockStateManager.On("LoadCache", ctx, cacheKey).Return([]byte(expectedBase64), nil)

	result, err := downloader.FetchImageAsBase64(ctx, "testkey")

	assert.NoError(t, err)
	assert.Equal(t, expectedBase64, result)
	mockStateManager.AssertExpectations(t)
}

func TestFetchImageAsBase64_DownloadAndCache(t *testing.T) {
	ctx := context.Background()
	mockStateManager := new(MockStateManager)
	testImageData := []byte("testdata")
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(testImageData)
	}))
	defer testServer.Close()

	downloader := NewImageDownloader(mockStateManager)
	cacheKey := fmt.Sprintf("imgcache:%x", sha1.Sum([]byte(testServer.URL)))
	expectedBase64 := base64.StdEncoding.EncodeToString(testImageData)

	mockStateManager.On("LoadCache", ctx, cacheKey).Return([]byte(nil), fmt.Errorf("cache miss"))
	mockStateManager.On("SaveCache", ctx, cacheKey, []byte(expectedBase64), time.Hour).Return(nil)

	result, err := downloader.FetchImageAsBase64(ctx, testServer.URL)
	assert.NoError(t, err)
	assert.Equal(t, expectedBase64, result)
	mockStateManager.AssertExpectations(t)
}

func TestFetchImageAsBase64_HTTPFailure(t *testing.T) {
	ctx := context.Background()
	mockStateManager := new(MockStateManager)
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer testServer.Close()

	downloader := NewImageDownloader(mockStateManager)
	cacheKey := fmt.Sprintf("imgcache:%x", sha1.Sum([]byte(testServer.URL)))

	mockStateManager.On("LoadCache", ctx, cacheKey).Return([]byte(nil), fmt.Errorf("cache miss"))

	_, err := downloader.FetchImageAsBase64(ctx, testServer.URL)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to download image")
	mockStateManager.AssertExpectations(t)
}

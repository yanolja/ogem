package imagedownloader

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/yanolja/ogem/state"
)

type ImageDownloader interface {
	FetchImageAsBase64(ctx context.Context, imageURL string) (string, error)
	GetImageType(url string) (string, error)
}

type imageDownloaderImpl struct {
	stateManager state.Manager
}

func NewImageDownloader(stateManager state.Manager) *imageDownloaderImpl {
	return &imageDownloaderImpl{
		stateManager: stateManager,
	}
}

func (d *imageDownloaderImpl) GetImageType(url string) (string, error) {
	ext := filepath.Ext(url)
	if ext == ".png" {
		return "image/png", nil
	}
	if ext == ".jpg" || ext == ".jpeg" {
		return "image/jpeg", nil
	}
	if ext == ".gif" {
		return "image/gif", nil
	}
	if ext == ".webp" {
		return "image/webp", nil
	}
	return "", fmt.Errorf("unsupported image type: %s", ext)
}

func (d *imageDownloaderImpl) FetchImageAsBase64(ctx context.Context, imageURL string) (string, error) {
	cacheKey := fmt.Sprintf("imgcache:%x", sha1.Sum([]byte(imageURL)))
	cachedImage, err := d.stateManager.LoadCache(ctx, cacheKey)
	if err == nil && cachedImage != nil {
		return string(cachedImage), nil
	}

	resp, err := http.Get(imageURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download image: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	encodedImage := base64.StdEncoding.EncodeToString(data)

	err = d.stateManager.SaveCache(ctx, cacheKey, []byte(encodedImage), time.Hour)
	if err != nil {
		return "", err
	}

	return encodedImage, nil
}

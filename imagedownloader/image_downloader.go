package imagedownloader

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/yanolja/ogem/state"
)

type ImageDownloader struct {
	stateManager state.Manager
}

func NewImageDownloader(stateManager state.Manager) *ImageDownloader {
	return &ImageDownloader{
		stateManager: stateManager,
	}
}

func (d *ImageDownloader) FetchImageAsBase64(ctx context.Context, imageURL string) (string, error) {
	cacheKey := fmt.Sprintf("imgcache:%x", sha1.Sum([]byte(imageURL)))
	cachedImage, err := d.stateManager.LoadCache(ctx, cacheKey)
	if err == nil {
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
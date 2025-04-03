package image

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/yanolja/ogem/state"
)

type Image struct {
	Data []byte    `json:"data"`
	Type ImageType `json:"type"`
}

type ImageType string

const (
	ImageTypeJPEG ImageType = "image/jpeg"
	ImageTypePNG  ImageType = "image/png"
	ImageTypeGIF  ImageType = "image/gif"
	ImageTypeWebP ImageType = "image/webp"
)

var validImageTypes = map[string]ImageType{
	"image/jpeg": ImageTypeJPEG,
	"image/png":  ImageTypePNG,
	"image/gif":  ImageTypeGIF,
	"image/webp": ImageTypeWebP,
}

// TODO(#100): Make timeout and cache expiry configurable.
const (
	cacheExpiry = time.Hour
	timeout     = 10 * time.Second
)

type Downloader interface {
	FetchImage(ctx context.Context, imageURL string) (*Image, error)
}

type downloaderImpl struct {
	stateManager state.Manager
	httpClient   *http.Client
}

func NewDownloader(stateManager state.Manager) Downloader {
	return &downloaderImpl{
		stateManager: stateManager,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (d *downloaderImpl) FetchImage(ctx context.Context, imageURL string) (*Image, error) {
	cacheKey := fmt.Sprintf("imgcache:%x", sha256.Sum256([]byte(imageURL)))

	// Attempt to load from cache
	if cachedVal, err := d.stateManager.LoadCache(ctx, cacheKey); err == nil && cachedVal != nil {
		var cachedImage Image
		if err := json.Unmarshal(cachedVal, &cachedImage); err == nil {
			return &cachedImage, nil
		}
	}

	// Fetch from remote URL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download image: status %d", resp.StatusCode)
	}

	// Validate Content-Type
	contentType := resp.Header.Get("Content-Type")
	imageType, valid := validImageTypes[contentType]
	if !valid {
		return nil, fmt.Errorf("unsupported image type: %s", contentType)
	}

	// Read image data
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}

	image := &Image{
		Data: data,
		Type: imageType,
	}

	// Save to cache
	cacheBytes, err := json.Marshal(image)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal image for cache: %w", err)
	}

	if err := d.stateManager.SaveCache(ctx, cacheKey, cacheBytes, cacheExpiry); err != nil {
		return nil, fmt.Errorf("failed to save image to cache: %w", err)
	}

	return image, nil
}

package image

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type CacheManager interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
}

type Downloader struct {
	cache  CacheManager
	client *http.Client
}

type ImageData struct {
	Data     []byte
	MimeType string
	Size     int64
}

func NewDownloader(cache CacheManager) *Downloader {
	return &Downloader{
		cache: cache,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (d *Downloader) ProcessImageURL(ctx context.Context, imageURL string) (*ImageData, error) {
	if strings.HasPrefix(imageURL, "data:") {
		return d.processDataURL(imageURL)
	}
	
	return d.downloadAndCache(ctx, imageURL)
}

func (d *Downloader) processDataURL(dataURL string) (*ImageData, error) {
	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid data URL format")
	}

	header := parts[0]
	data := parts[1]

	var mimeType string
	if strings.Contains(header, ";") {
		mimeTypePart := strings.Split(header, ";")[0]
		mimeType = strings.TrimPrefix(mimeTypePart, "data:")
	} else {
		mimeType = strings.TrimPrefix(header, "data:")
	}

	var imageData []byte
	var err error

	if strings.Contains(header, "base64") {
		imageData, err = base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 data: %v", err)
		}
	} else {
		imageData = []byte(data)
	}

	return &ImageData{
		Data:     imageData,
		MimeType: mimeType,
		Size:     int64(len(imageData)),
	}, nil
}

func (d *Downloader) downloadAndCache(ctx context.Context, imageURL string) (*ImageData, error) {
	cacheKey := d.generateCacheKey(imageURL)
	
	if d.cache != nil {
		if cached, err := d.cache.Get(ctx, cacheKey); err == nil {
			return d.deserializeImageData(cached)
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download image: HTTP %d", resp.StatusCode)
	}

	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image data: %v", err)
	}

	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	result := &ImageData{
		Data:     imageData,
		MimeType: mimeType,
		Size:     int64(len(imageData)),
	}

	if d.cache != nil {
		if serialized, err := d.serializeImageData(result); err == nil {
			d.cache.Set(ctx, cacheKey, serialized, 24*time.Hour)
		}
	}

	return result, nil
}

func (d *Downloader) generateCacheKey(imageURL string) string {
	hasher := sha256.New()
	hasher.Write([]byte(imageURL))
	hash := hex.EncodeToString(hasher.Sum(nil))
	return fmt.Sprintf("ogem:image:%s", hash)
}

func (d *Downloader) serializeImageData(img *ImageData) ([]byte, error) {
	header := fmt.Sprintf("%s:%d:", img.MimeType, img.Size)
	result := make([]byte, len(header)+len(img.Data))
	copy(result, []byte(header))
	copy(result[len(header):], img.Data)
	return result, nil
}

func (d *Downloader) deserializeImageData(data []byte) (*ImageData, error) {
	headerEnd := -1
	colonCount := 0
	for i, b := range data {
		if b == ':' {
			colonCount++
			if colonCount == 2 {
				headerEnd = i
				break
			}
		}
	}
	
	if headerEnd == -1 {
		return nil, fmt.Errorf("invalid cached image data format")
	}
	
	header := string(data[:headerEnd])
	parts := strings.Split(header, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid cached image header format")
	}
	
	mimeType := parts[0]
	imageData := data[headerEnd+1:]
	
	return &ImageData{
		Data:     imageData,
		MimeType: mimeType,
		Size:     int64(len(imageData)),
	}, nil
}

func IsImageMimeType(mimeType string) bool {
	return strings.HasPrefix(mimeType, "image/")
}

func ConvertToBase64DataURL(img *ImageData) string {
	encoded := base64.StdEncoding.EncodeToString(img.Data)
	return fmt.Sprintf("data:%s;base64,%s", img.MimeType, encoded)
}
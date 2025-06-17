package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

type VirtualKey struct {
	ID          string            `json:"id"`
	Key         string            `json:"key"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Models      []string          `json:"models,omitempty"`
	MaxTokens   *int64            `json:"max_tokens,omitempty"`
	MaxRequests *int64            `json:"max_requests,omitempty"`
	Budget      *float64          `json:"budget,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	IsActive    bool              `json:"is_active"`
	UsageStats  *UsageStats       `json:"usage_stats,omitempty"`
}

type UsageStats struct {
	TotalTokens   int64   `json:"total_tokens"`
	TotalRequests int64   `json:"total_requests"`
	TotalCost     float64 `json:"total_cost"`
	LastUsed      *time.Time `json:"last_used,omitempty"`
}

type KeyRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Models      []string          `json:"models,omitempty"`
	MaxTokens   *int64            `json:"max_tokens,omitempty"`
	MaxRequests *int64            `json:"max_requests,omitempty"`
	Budget      *float64          `json:"budget,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
}

type KeyResponse struct {
	ID          string            `json:"id"`
	Key         string            `json:"key"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Models      []string          `json:"models,omitempty"`
	MaxTokens   *int64            `json:"max_tokens,omitempty"`
	MaxRequests *int64            `json:"max_requests,omitempty"`
	Budget      *float64          `json:"budget,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	IsActive    bool              `json:"is_active"`
	UsageStats  *UsageStats       `json:"usage_stats,omitempty"`
}

type Manager interface {
	CreateKey(ctx context.Context, req *KeyRequest) (*VirtualKey, error)
	GetKey(ctx context.Context, keyID string) (*VirtualKey, error)
	GetKeyByValue(ctx context.Context, keyValue string) (*VirtualKey, error)
	ListKeys(ctx context.Context) ([]*VirtualKey, error)
	UpdateKey(ctx context.Context, keyID string, updates map[string]interface{}) (*VirtualKey, error)
	DeleteKey(ctx context.Context, keyID string) error
	ValidateKey(ctx context.Context, keyValue string, modelName string) (*VirtualKey, error)
	UpdateUsage(ctx context.Context, keyValue string, tokens int64, cost float64) error
}

func GenerateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %v", err)
	}
	return "ogem-" + hex.EncodeToString(bytes), nil
}

func GenerateKeyID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %v", err)
	}
	return "key_" + hex.EncodeToString(bytes), nil
}
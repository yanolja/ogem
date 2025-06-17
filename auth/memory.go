package auth

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type MemoryManager struct {
	keys   map[string]*VirtualKey
	keyMap map[string]string
	mutex  sync.RWMutex
}

func NewMemoryManager() *MemoryManager {
	return &MemoryManager{
		keys:   make(map[string]*VirtualKey),
		keyMap: make(map[string]string),
	}
}

func (m *MemoryManager) CreateKey(ctx context.Context, req *KeyRequest) (*VirtualKey, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	keyID, err := GenerateKeyID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate key ID: %v", err)
	}

	keyValue, err := GenerateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %v", err)
	}

	key := &VirtualKey{
		ID:          keyID,
		Key:         keyValue,
		Name:        req.Name,
		Description: req.Description,
		Models:      req.Models,
		MaxTokens:   req.MaxTokens,
		MaxRequests: req.MaxRequests,
		Budget:      req.Budget,
		Metadata:    req.Metadata,
		CreatedAt:   time.Now(),
		ExpiresAt:   req.ExpiresAt,
		IsActive:    true,
		UsageStats: &UsageStats{
			TotalTokens:   0,
			TotalRequests: 0,
			TotalCost:     0,
		},
	}

	m.keys[keyID] = key
	m.keyMap[keyValue] = keyID

	return key, nil
}

func (m *MemoryManager) GetKey(ctx context.Context, keyID string) (*VirtualKey, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	key, exists := m.keys[keyID]
	if !exists {
		return nil, fmt.Errorf("key not found")
	}

	return key, nil
}

func (m *MemoryManager) GetKeyByValue(ctx context.Context, keyValue string) (*VirtualKey, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	keyID, exists := m.keyMap[keyValue]
	if !exists {
		return nil, fmt.Errorf("key not found")
	}

	key, exists := m.keys[keyID]
	if !exists {
		return nil, fmt.Errorf("key not found")
	}

	return key, nil
}

func (m *MemoryManager) ListKeys(ctx context.Context) ([]*VirtualKey, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	keys := make([]*VirtualKey, 0, len(m.keys))
	for _, key := range m.keys {
		keys = append(keys, key)
	}

	return keys, nil
}

func (m *MemoryManager) UpdateKey(ctx context.Context, keyID string, updates map[string]interface{}) (*VirtualKey, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	key, exists := m.keys[keyID]
	if !exists {
		return nil, fmt.Errorf("key not found")
	}

	if name, ok := updates["name"].(string); ok {
		key.Name = name
	}
	if description, ok := updates["description"].(string); ok {
		key.Description = description
	}
	if models, ok := updates["models"].([]string); ok {
		key.Models = models
	}
	if maxTokens, ok := updates["max_tokens"].(*int64); ok {
		key.MaxTokens = maxTokens
	}
	if maxRequests, ok := updates["max_requests"].(*int64); ok {
		key.MaxRequests = maxRequests
	}
	if budget, ok := updates["budget"].(*float64); ok {
		key.Budget = budget
	}
	if metadata, ok := updates["metadata"].(map[string]string); ok {
		key.Metadata = metadata
	}
	if expiresAt, ok := updates["expires_at"].(*time.Time); ok {
		key.ExpiresAt = expiresAt
	}
	if isActive, ok := updates["is_active"].(bool); ok {
		key.IsActive = isActive
	}

	return key, nil
}

func (m *MemoryManager) DeleteKey(ctx context.Context, keyID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	key, exists := m.keys[keyID]
	if !exists {
		return fmt.Errorf("key not found")
	}

	delete(m.keys, keyID)
	delete(m.keyMap, key.Key)

	return nil
}

func (m *MemoryManager) ValidateKey(ctx context.Context, keyValue string, modelName string) (*VirtualKey, error) {
	key, err := m.GetKeyByValue(ctx, keyValue)
	if err != nil {
		return nil, err
	}

	if !key.IsActive {
		return nil, fmt.Errorf("key is inactive")
	}

	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		return nil, fmt.Errorf("key has expired")
	}

	if len(key.Models) > 0 {
		allowed := false
		for _, allowedModel := range key.Models {
			if allowedModel == modelName || strings.Contains(modelName, allowedModel) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("model %s not allowed for this key", modelName)
		}
	}

	if key.MaxRequests != nil && key.UsageStats.TotalRequests >= *key.MaxRequests {
		return nil, fmt.Errorf("request limit exceeded")
	}

	if key.Budget != nil && key.UsageStats.TotalCost >= *key.Budget {
		return nil, fmt.Errorf("budget limit exceeded")
	}

	return key, nil
}

func (m *MemoryManager) UpdateUsage(ctx context.Context, keyValue string, tokens int64, cost float64) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	keyID, exists := m.keyMap[keyValue]
	if !exists {
		return fmt.Errorf("key not found")
	}

	key, exists := m.keys[keyID]
	if !exists {
		return fmt.Errorf("key not found")
	}

	if key.UsageStats == nil {
		key.UsageStats = &UsageStats{}
	}

	key.UsageStats.TotalTokens += tokens
	key.UsageStats.TotalRequests++
	key.UsageStats.TotalCost += cost
	now := time.Now()
	key.UsageStats.LastUsed = &now

	return nil
}
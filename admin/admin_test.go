package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yanolja/ogem/auth"
)

// KeyInfo matches the structure in handleAPIKeys
type KeyInfo struct {
	VirtualKey   string    `json:"virtual_key"`
	Budget       *float64  `json:"budget"`
	Used         float64   `json:"used"`
	Permissions  []string  `json:"permissions"`
	CreatedAt    time.Time `json:"created_at"`
	LastUsed     *time.Time `json:"last_used"`
}

// Mock Manager implementation
type mockManager struct {
	keys    map[string]*auth.VirtualKey
	counter int
}

func newMockManager() *mockManager {
	return &mockManager{
		keys:    make(map[string]*auth.VirtualKey),
		counter: 0,
	}
}

func (m *mockManager) CreateKey(ctx context.Context, req *auth.KeyRequest) (*auth.VirtualKey, error) {
	m.counter++
	id := fmt.Sprintf("test-id-%d", m.counter)
	keyValue := fmt.Sprintf("sk-test-key-%d", m.counter)

	key := &auth.VirtualKey{
		ID:        id,
		Key:       keyValue,
		Name:      req.Name,
		Models:    req.Models,
		Budget:    req.Budget,
		CreatedAt: time.Now(),
		IsActive:  true,
	}
	m.keys[key.ID] = key
	return key, nil
}

func (m *mockManager) GetKey(ctx context.Context, keyID string) (*auth.VirtualKey, error) {
	if key, ok := m.keys[keyID]; ok {
		return key, nil
	}
	return nil, assert.AnError
}

func (m *mockManager) GetKeyByValue(ctx context.Context, keyValue string) (*auth.VirtualKey, error) {
	for _, key := range m.keys {
		if key.Key == keyValue {
			return key, nil
		}
	}
	return nil, assert.AnError
}

func (m *mockManager) ListKeys(ctx context.Context) ([]*auth.VirtualKey, error) {
	keys := make([]*auth.VirtualKey, 0, len(m.keys))
	for _, key := range m.keys {
		keys = append(keys, key)
	}
	return keys, nil
}

func (m *mockManager) UpdateKey(ctx context.Context, keyID string, updates map[string]interface{}) (*auth.VirtualKey, error) {
	if key, ok := m.keys[keyID]; ok {
		return key, nil
	}
	return nil, assert.AnError
}

func (m *mockManager) DeleteKey(ctx context.Context, keyID string) error {
	if _, ok := m.keys[keyID]; ok {
		delete(m.keys, keyID)
		return nil
	}
	return assert.AnError
}

func (m *mockManager) ValidateKey(ctx context.Context, keyValue string, modelName string) (*auth.VirtualKey, error) {
	return m.GetKeyByValue(ctx, keyValue)
}

func (m *mockManager) UpdateUsage(ctx context.Context, keyValue string, tokens int64, cost float64) error {
	return nil
}

func TestNewAdminServer(t *testing.T) {
	manager := newMockManager()
	server := NewAdminServer(manager)

	assert.NotNil(t, server)
	assert.Equal(t, manager, server.virtualKeyManager)
}

func TestAdminServer_handleDashboard(t *testing.T) {
	manager := newMockManager()
	server := NewAdminServer(manager)

	t.Run("serves dashboard HTML", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		w := httptest.NewRecorder()

		server.handleDashboard(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
		assert.Contains(t, w.Body.String(), "Ogem Admin Dashboard")
	})
}

func TestAdminServer_handleAPIKeys(t *testing.T) {
	manager := newMockManager()
	server := NewAdminServer(manager)

	t.Run("lists API keys", func(t *testing.T) {
		// Create some keys
		budget := 100.0
		_, err := manager.CreateKey(context.Background(), &auth.KeyRequest{
			Name:   "Test Key",
			Models: []string{"gpt-4"},
			Budget: &budget,
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/keys", nil)
		w := httptest.NewRecorder()

		server.handleAPIKeys(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var keys []KeyInfo
		err = json.NewDecoder(w.Body).Decode(&keys)
		require.NoError(t, err)
		assert.Len(t, keys, 1)
		assert.Contains(t, keys[0].VirtualKey, "sk-test-key")
	})

	t.Run("handles empty key list", func(t *testing.T) {
		emptyManager := newMockManager()
		emptyServer := NewAdminServer(emptyManager)

		req := httptest.NewRequest(http.MethodGet, "/api/keys", nil)
		w := httptest.NewRecorder()

		emptyServer.handleAPIKeys(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var keys []KeyInfo
		err := json.NewDecoder(w.Body).Decode(&keys)
		require.NoError(t, err)
		assert.Empty(t, keys)
	})
}

func TestAdminServer_handleCreateKey(t *testing.T) {
	manager := newMockManager()
	server := NewAdminServer(manager)

	t.Run("creates key successfully", func(t *testing.T) {
		budget := 50.0
		requestBody := map[string]interface{}{
			"budget":      budget,
			"permissions": []string{"chat", "embeddings"},
		}
		body, _ := json.Marshal(requestBody)

		req := httptest.NewRequest(http.MethodPost, "/api/keys/create", bytes.NewReader(body))
		w := httptest.NewRecorder()

		server.handleCreateKey(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var response map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Contains(t, response["virtual_key"].(string), "sk-test-key")
		assert.Equal(t, budget, response["budget"])
	})

	t.Run("uses default permissions", func(t *testing.T) {
		requestBody := map[string]interface{}{}
		body, _ := json.Marshal(requestBody)

		req := httptest.NewRequest(http.MethodPost, "/api/keys/create", bytes.NewReader(body))
		w := httptest.NewRecorder()

		server.handleCreateKey(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		permissions := response["permissions"].([]interface{})
		assert.Contains(t, permissions, "chat")
		assert.Contains(t, permissions, "embeddings")
	})

	t.Run("rejects non-POST requests", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/keys/create", nil)
		w := httptest.NewRecorder()

		server.handleCreateKey(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("handles invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/keys/create", bytes.NewReader([]byte("invalid json")))
		w := httptest.NewRecorder()

		server.handleCreateKey(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestAdminServer_handleDeleteKey(t *testing.T) {
	manager := newMockManager()
	server := NewAdminServer(manager)

	t.Run("deletes key successfully", func(t *testing.T) {
		// Create a key first
		key, err := manager.CreateKey(context.Background(), &auth.KeyRequest{
			Name: "Test Key",
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodDelete, "/api/keys/delete?key="+key.ID, nil)
		w := httptest.NewRecorder()

		server.handleDeleteKey(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]string
		err = json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "deleted", response["status"])
	})

	t.Run("rejects non-DELETE requests", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/keys/delete?key=test", nil)
		w := httptest.NewRecorder()

		server.handleDeleteKey(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("requires key parameter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/keys/delete", nil)
		w := httptest.NewRecorder()

		server.handleDeleteKey(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("handles non-existent key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/keys/delete?key=nonexistent", nil)
		w := httptest.NewRecorder()

		server.handleDeleteKey(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestAdminServer_getDashboardData(t *testing.T) {
	manager := newMockManager()
	server := NewAdminServer(manager)

	t.Run("returns dashboard data", func(t *testing.T) {
		// Create some keys
		budget := 100.0
		for i := 0; i < 3; i++ {
			_, err := manager.CreateKey(context.Background(), &auth.KeyRequest{
				Name:   "Test Key",
				Budget: &budget,
			})
			require.NoError(t, err)
		}

		data, err := server.getDashboardData()
		require.NoError(t, err)
		assert.NotNil(t, data)
		assert.Equal(t, 3, data.ActiveKeys)
		assert.NotZero(t, data.TotalRequests)
	})

	t.Run("handles empty key list", func(t *testing.T) {
		emptyManager := newMockManager()
		emptyServer := NewAdminServer(emptyManager)

		data, err := emptyServer.getDashboardData()
		require.NoError(t, err)
		assert.NotNil(t, data)
		assert.Equal(t, 0, data.ActiveKeys)
	})
}

func TestAdminServer_handleAPIStats(t *testing.T) {
	manager := newMockManager()
	server := NewAdminServer(manager)

	t.Run("returns stats JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
		w := httptest.NewRecorder()

		server.handleAPIStats(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var data DashboardData
		err := json.NewDecoder(w.Body).Decode(&data)
		require.NoError(t, err)
		assert.NotNil(t, data.SystemStatus)
	})
}

func TestAdminServer_RegisterRoutes(t *testing.T) {
	manager := newMockManager()
	server := NewAdminServer(manager)

	t.Run("registers routes", func(t *testing.T) {
		mux := http.NewServeMux()
		server.RegisterRoutes(mux)

		// The function should not panic
		assert.NotNil(t, mux)
	})
}

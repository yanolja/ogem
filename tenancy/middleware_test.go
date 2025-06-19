package tenancy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestNewTenantMiddleware(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	tests := []struct {
		name   string
		config *MiddlewareConfig
	}{
		{
			name:   "default config",
			config: nil,
		},
		{
			name: "custom config",
			config: &MiddlewareConfig{
				IdentificationMethod: IdentificationMethodPath,
				TenantHeaderName:     "X-Custom-Tenant",
				RequireTenant:        false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := NewTenantMiddleware(manager, nil, tt.config, logger)
			assert.NotNil(t, middleware)
			assert.NotNil(t, middleware.config)

			if tt.config == nil {
				assert.Equal(t, IdentificationMethodHeader, middleware.config.IdentificationMethod)
				assert.Equal(t, "X-Tenant-ID", middleware.config.TenantHeaderName)
			} else {
				assert.Equal(t, tt.config.IdentificationMethod, middleware.config.IdentificationMethod)
				assert.Equal(t, tt.config.TenantHeaderName, middleware.config.TenantHeaderName)
			}
		})
	}
}

func TestDefaultMiddlewareConfig(t *testing.T) {
	config := DefaultMiddlewareConfig()

	assert.NotNil(t, config)
	assert.Equal(t, IdentificationMethodHeader, config.IdentificationMethod)
	assert.Equal(t, "X-Tenant-ID", config.TenantHeaderName)
	assert.Equal(t, "X-Tenant-Domain", config.TenantDomainHeader)
	assert.Equal(t, "/t", config.TenantPathPrefix)
	assert.False(t, config.TenantSubdomain)
	assert.Empty(t, config.DefaultTenantID)
	assert.False(t, config.AutoProvisionTenants)
	assert.True(t, config.RequireTenant)
	assert.True(t, config.EnableCaching)
	assert.Equal(t, 300, config.CacheTTL)
}

func TestTenantMiddleware_HeaderIdentification(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create a test tenant
	tenant := &Tenant{
		ID:     "test-tenant",
		Name:   "Test Tenant",
		Type:   TenantTypeTeam,
		Status: TenantStatusActive,
	}
	err := manager.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	config := &MiddlewareConfig{
		IdentificationMethod: IdentificationMethodHeader,
		TenantHeaderName:     "X-Tenant-ID",
		RequireTenant:        true,
	}

	middleware := NewTenantMiddleware(manager, nil, config, logger)
	handler := middleware.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantCtx, ok := GetTenantFromContext(r.Context())
		assert.True(t, ok)
		assert.NotNil(t, tenantCtx)
		assert.Equal(t, "test-tenant", tenantCtx.Tenant.ID)
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name         string
		headers      map[string]string
		expectedCode int
		expectTenant bool
	}{
		{
			name: "valid tenant header",
			headers: map[string]string{
				"X-Tenant-ID": "test-tenant",
			},
			expectedCode: http.StatusOK,
			expectTenant: true,
		},
		{
			name: "alternative header name",
			headers: map[string]string{
				"X-Organization-ID": "test-tenant",
			},
			expectedCode: http.StatusOK,
			expectTenant: true,
		},
		{
			name:         "missing tenant header",
			headers:      map[string]string{},
			expectedCode: http.StatusBadRequest,
			expectTenant: false,
		},
		{
			name: "non-existent tenant",
			headers: map[string]string{
				"X-Tenant-ID": "non-existent",
			},
			expectedCode: http.StatusNotFound,
			expectTenant: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, tt.expectedCode, recorder.Code)

			if tt.expectTenant {
				assert.Equal(t, "test-tenant", recorder.Header().Get("X-Tenant-ID"))
				assert.Equal(t, "Test Tenant", recorder.Header().Get("X-Tenant-Name"))
			}
		})
	}
}

func TestTenantMiddleware_PathIdentification(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create a test tenant
	tenant := &Tenant{
		ID:     "path-tenant",
		Name:   "Path Tenant",
		Type:   TenantTypeTeam,
		Status: TenantStatusActive,
	}
	err := manager.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	config := &MiddlewareConfig{
		IdentificationMethod: IdentificationMethodPath,
		TenantPathPrefix:     "/t",
		RequireTenant:        true,
	}

	middleware := NewTenantMiddleware(manager, nil, config, logger)
	handler := middleware.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name         string
		path         string
		expectedCode int
	}{
		{
			name:         "valid tenant path",
			path:         "/t/path-tenant/models",
			expectedCode: http.StatusOK,
		},
		{
			name:         "org prefix path",
			path:         "/org/path-tenant/models",
			expectedCode: http.StatusOK,
		},
		{
			name:         "invalid path format",
			path:         "/invalid/path",
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "non-existent tenant in path",
			path:         "/t/non-existent/models",
			expectedCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, tt.expectedCode, recorder.Code)
		})
	}
}

func TestTenantMiddleware_SubdomainIdentification(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create a test tenant
	tenant := &Tenant{
		ID:     "subdomain-tenant",
		Name:   "Subdomain Tenant",
		Type:   TenantTypeTeam,
		Status: TenantStatusActive,
	}
	err := manager.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	config := &MiddlewareConfig{
		IdentificationMethod: IdentificationMethodSubdomain,
		TenantSubdomain:      true,
		RequireTenant:        true,
	}

	middleware := NewTenantMiddleware(manager, nil, config, logger)
	handler := middleware.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name         string
		host         string
		expectedCode int
	}{
		{
			name:         "valid subdomain",
			host:         "subdomain-tenant.example.com",
			expectedCode: http.StatusOK,
		},
		{
			name:         "subdomain with port",
			host:         "subdomain-tenant.example.com:8080",
			expectedCode: http.StatusOK,
		},
		{
			name:         "www subdomain ignored",
			host:         "www.example.com",
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "api subdomain ignored",
			host:         "api.example.com",
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "non-existent subdomain tenant",
			host:         "nonexistent.example.com",
			expectedCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Host = tt.host
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, tt.expectedCode, recorder.Code)
		})
	}
}

func TestTenantMiddleware_CombinedIdentification(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create a test tenant
	tenant := &Tenant{
		ID:     "combined-tenant",
		Name:   "Combined Tenant",
		Type:   TenantTypeTeam,
		Status: TenantStatusActive,
	}
	err := manager.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	config := &MiddlewareConfig{
		IdentificationMethod: IdentificationMethodCombined,
		TenantHeaderName:     "X-Tenant-ID",
		TenantPathPrefix:     "/t",
		RequireTenant:        true,
	}

	middleware := NewTenantMiddleware(manager, nil, config, logger)
	handler := middleware.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name         string
		headers      map[string]string
		path         string
		expectedCode int
		description  string
	}{
		{
			name: "header takes precedence",
			headers: map[string]string{
				"X-Tenant-ID": "combined-tenant",
			},
			path:         "/different/path",
			expectedCode: http.StatusOK,
			description:  "Header should be preferred over path",
		},
		{
			name:         "fallback to path",
			headers:      map[string]string{},
			path:         "/t/combined-tenant/models",
			expectedCode: http.StatusOK,
			description:  "Should fall back to path when header not present",
		},
		{
			name:         "no identification method works",
			headers:      map[string]string{},
			path:         "/invalid/path",
			expectedCode: http.StatusBadRequest,
			description:  "Should fail when no method identifies tenant",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, tt.expectedCode, recorder.Code, tt.description)
		})
	}
}

func TestTenantMiddleware_DefaultTenant(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create a default tenant
	tenant := &Tenant{
		ID:     "default-tenant",
		Name:   "Default Tenant",
		Type:   TenantTypePersonal,
		Status: TenantStatusActive,
	}
	err := manager.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	config := &MiddlewareConfig{
		IdentificationMethod: IdentificationMethodHeader,
		TenantHeaderName:     "X-Tenant-ID",
		DefaultTenantID:      "default-tenant",
		RequireTenant:        true,
	}

	middleware := NewTenantMiddleware(manager, nil, config, logger)
	handler := middleware.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantCtx, ok := GetTenantFromContext(r.Context())
		assert.True(t, ok)
		assert.NotNil(t, tenantCtx)
		assert.Equal(t, "default-tenant", tenantCtx.Tenant.ID)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	// No tenant header set, should use default
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "default-tenant", recorder.Header().Get("X-Tenant-ID"))
}

func TestTenantMiddleware_AutoProvisioning(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	config := &MiddlewareConfig{
		IdentificationMethod: IdentificationMethodHeader,
		TenantHeaderName:     "X-Tenant-ID",
		AutoProvisionTenants: true,
		RequireTenant:        true,
	}

	middleware := NewTenantMiddleware(manager, nil, config, logger)
	handler := middleware.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantCtx, ok := GetTenantFromContext(r.Context())
		assert.True(t, ok)
		assert.NotNil(t, tenantCtx)
		assert.Equal(t, "auto-tenant", tenantCtx.Tenant.ID)
		assert.Equal(t, "auto-tenant", tenantCtx.Tenant.Name)
		assert.Equal(t, "true", tenantCtx.Tenant.Metadata["auto_provisioned"])
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Tenant-ID", "auto-tenant")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)

	// Verify tenant was actually created
	tenant, err := manager.GetTenant(context.Background(), "auto-tenant")
	assert.NoError(t, err)
	assert.Equal(t, "auto-tenant", tenant.ID)
	assert.Equal(t, "true", tenant.Metadata["auto_provisioned"])
}

func TestTenantMiddleware_TenantValidation(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create test tenants with different statuses
	activeTenant := &Tenant{
		ID:     "active-tenant",
		Name:   "Active Tenant",
		Type:   TenantTypeTeam,
		Status: TenantStatusActive,
	}

	suspendedTenant := &Tenant{
		ID:     "suspended-tenant",
		Name:   "Suspended Tenant",
		Type:   TenantTypeTeam,
		Status: TenantStatusSuspended,
	}

	expiredTenant := &Tenant{
		ID:     "expired-tenant",
		Name:   "Expired Tenant",
		Type:   TenantTypeTeam,
		Status: TenantStatusActive,
		Subscription: &Subscription{
			Status:  "expired",
			EndDate: middlewareTimePtr(time.Now().Add(-24 * time.Hour)),
		},
	}

	err := manager.CreateTenant(context.Background(), activeTenant)
	require.NoError(t, err)
	err = manager.CreateTenant(context.Background(), suspendedTenant)
	require.NoError(t, err)
	err = manager.CreateTenant(context.Background(), expiredTenant)
	require.NoError(t, err)

	config := &MiddlewareConfig{
		IdentificationMethod: IdentificationMethodHeader,
		TenantHeaderName:     "X-Tenant-ID",
		RequireTenant:        true,
	}

	middleware := NewTenantMiddleware(manager, nil, config, logger)
	handler := middleware.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name         string
		tenantID     string
		expectedCode int
		errorType    string
	}{
		{
			name:         "active tenant allowed",
			tenantID:     "active-tenant",
			expectedCode: http.StatusOK,
		},
		{
			name:         "suspended tenant forbidden",
			tenantID:     "suspended-tenant",
			expectedCode: http.StatusForbidden,
			errorType:    "tenant_inactive",
		},
		{
			name:         "expired tenant payment required",
			tenantID:     "expired-tenant",
			expectedCode: http.StatusPaymentRequired,
			errorType:    "tenant_expired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("X-Tenant-ID", tt.tenantID)

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, tt.expectedCode, recorder.Code)

			if tt.expectedCode != http.StatusOK {
				assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))

				var errorResponse map[string]interface{}
				err := json.Unmarshal(recorder.Body.Bytes(), &errorResponse)
				assert.NoError(t, err)

				errorObj := errorResponse["error"].(map[string]interface{})
				assert.Equal(t, tt.errorType, errorObj["type"])
				assert.Equal(t, float64(tt.expectedCode), errorObj["code"])
			}
		})
	}
}

func TestTenantMiddleware_OptionalTenant(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	config := &MiddlewareConfig{
		IdentificationMethod: IdentificationMethodHeader,
		TenantHeaderName:     "X-Tenant-ID",
		RequireTenant:        false, // Tenant is optional
	}

	middleware := NewTenantMiddleware(manager, nil, config, logger)
	handler := middleware.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantCtx, ok := GetTenantFromContext(r.Context())
		if ok && tenantCtx != nil {
			w.Header().Set("Found-Tenant", "true")
		} else {
			w.Header().Set("Found-Tenant", "false")
		}
		w.WriteHeader(http.StatusOK)
	}))

	// Request without tenant header should succeed
	req := httptest.NewRequest("GET", "/test", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "false", recorder.Header().Get("Found-Tenant"))
}

func TestTenantEnforcementMiddleware(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create a tenant with security settings
	tenant := &Tenant{
		ID:     "secure-tenant",
		Name:   "Secure Tenant",
		Type:   TenantTypeEnterprise,
		Status: TenantStatusActive,
		Security: &TenantSecurity{
			AllowedIPs:         []string{"192.168.1.100", "10.0.0.0/8"},
			BlockedIPs:         []string{"192.168.1.200"},
			EncryptionRequired: true,
		},
		Settings: &TenantSettings{
			DataRegion: "us-west-2",
		},
	}
	err := manager.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	enforcement := NewTenantEnforcementMiddleware(manager, logger)
	handler := enforcement.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name         string
		clientIP     string
		expectedCode int
		checkHeaders bool
	}{
		{
			name:         "allowed IP access",
			clientIP:     "192.168.1.100",
			expectedCode: http.StatusOK,
			checkHeaders: true,
		},
		{
			name:         "blocked IP denied",
			clientIP:     "192.168.1.200",
			expectedCode: http.StatusForbidden,
			checkHeaders: false,
		},
		{
			name:         "non-allowed IP denied",
			clientIP:     "203.0.113.1",
			expectedCode: http.StatusForbidden,
			checkHeaders: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("X-Forwarded-For", tt.clientIP)

			// Add tenant context to request
			tenantCtx := &TenantContext{
				Tenant: tenant,
				UserID: "test-user",
			}
			ctx := WithTenantContext(req.Context(), tenantCtx)
			req = req.WithContext(ctx)

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			assert.Equal(t, tt.expectedCode, recorder.Code)

			if tt.checkHeaders && tt.expectedCode == http.StatusOK {
				// Check security headers
				assert.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
				assert.Equal(t, "DENY", recorder.Header().Get("X-Frame-Options"))
				assert.Equal(t, "1; mode=block", recorder.Header().Get("X-XSS-Protection"))
				assert.Equal(t, "max-age=31536000; includeSubDomains", recorder.Header().Get("Strict-Transport-Security"))
				assert.Equal(t, "us-west-2", recorder.Header().Get("X-Data-Region"))
			}
		})
	}
}

func TestTenantMiddleware_ExtractMethods(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	config := DefaultMiddlewareConfig()
	middleware := NewTenantMiddleware(manager, nil, config, logger)

	t.Run("extractFromHeader", func(t *testing.T) {
		tests := []struct {
			name     string
			headers  map[string]string
			expected string
		}{
			{
				name: "configured header",
				headers: map[string]string{
					"X-Tenant-ID": "tenant1",
				},
				expected: "tenant1",
			},
			{
				name: "alternative header",
				headers: map[string]string{
					"X-Organization-ID": "tenant2",
				},
				expected: "tenant2",
			},
			{
				name: "multiple headers, first wins",
				headers: map[string]string{
					"X-Tenant-ID":       "tenant1",
					"X-Organization-ID": "tenant2",
				},
				expected: "tenant1",
			},
			{
				name:     "no headers",
				headers:  map[string]string{},
				expected: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := httptest.NewRequest("GET", "/test", nil)
				for key, value := range tt.headers {
					req.Header.Set(key, value)
				}

				result := middleware.extractFromHeader(req)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("extractFromPath", func(t *testing.T) {
		tests := []struct {
			name     string
			path     string
			expected string
		}{
			{
				name:     "tenant prefix",
				path:     "/t/tenant1/models",
				expected: "tenant1",
			},
			{
				name:     "org prefix",
				path:     "/org/tenant2/users",
				expected: "tenant2",
			},
			{
				name:     "organization prefix",
				path:     "/organization/tenant3/settings",
				expected: "tenant3",
			},
			{
				name:     "no valid prefix",
				path:     "/api/v1/models",
				expected: "",
			},
			{
				name:     "short path",
				path:     "/t",
				expected: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := httptest.NewRequest("GET", tt.path, nil)
				result := middleware.extractFromPath(req)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("extractFromSubdomain", func(t *testing.T) {
		// Enable subdomain extraction
		middleware.config.TenantSubdomain = true

		tests := []struct {
			name     string
			host     string
			expected string
		}{
			{
				name:     "valid subdomain",
				host:     "tenant1.example.com",
				expected: "tenant1",
			},
			{
				name:     "subdomain with port",
				host:     "tenant2.example.com:8080",
				expected: "tenant2",
			},
			{
				name:     "www subdomain ignored",
				host:     "www.example.com",
				expected: "",
			},
			{
				name:     "api subdomain ignored",
				host:     "api.example.com",
				expected: "",
			},
			{
				name:     "app subdomain ignored",
				host:     "app.example.com",
				expected: "",
			},
			{
				name:     "no subdomain",
				host:     "example.com",
				expected: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := httptest.NewRequest("GET", "/test", nil)
				req.Host = tt.host
				result := middleware.extractFromSubdomain(req)
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}

func TestTenantEnforcementMiddleware_ClientIP(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	enforcement := NewTenantEnforcementMiddleware(manager, logger)

	tests := []struct {
		name     string
		headers  map[string]string
		remoteAddr string
		expected string
	}{
		{
			name: "X-Forwarded-For header",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.1, 198.51.100.1",
			},
			remoteAddr: "192.168.1.1:12345",
			expected:   "203.0.113.1",
		},
		{
			name: "X-Real-IP header",
			headers: map[string]string{
				"X-Real-IP": "203.0.113.2",
			},
			remoteAddr: "192.168.1.1:12345",
			expected:   "203.0.113.2",
		},
		{
			name:       "RemoteAddr fallback",
			headers:    map[string]string{},
			remoteAddr: "203.0.113.3:12345",
			expected:   "203.0.113.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tt.remoteAddr
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			result := enforcement.getClientIP(req)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTenantEnforcementMiddleware_IPMatching(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	enforcement := NewTenantEnforcementMiddleware(manager, logger)

	tests := []struct {
		name     string
		ip       string
		pattern  string
		expected bool
	}{
		{
			name:     "exact match",
			ip:       "192.168.1.100",
			pattern:  "192.168.1.100",
			expected: true,
		},
		{
			name:     "wildcard match",
			ip:       "any.ip.address",
			pattern:  "*",
			expected: true,
		},
		{
			name:     "no match",
			ip:       "192.168.1.100",
			pattern:  "192.168.1.200",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := enforcement.matchesIPPattern(tt.ip, tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTenantIdentificationMethod_Constants(t *testing.T) {
	assert.Equal(t, TenantIdentificationMethod("header"), IdentificationMethodHeader)
	assert.Equal(t, TenantIdentificationMethod("path"), IdentificationMethodPath)
	assert.Equal(t, TenantIdentificationMethod("subdomain"), IdentificationMethodSubdomain)
	assert.Equal(t, TenantIdentificationMethod("jwt"), IdentificationMethodJWT)
	assert.Equal(t, TenantIdentificationMethod("combined"), IdentificationMethodCombined)
}

func TestMiddlewareConfig_Structure(t *testing.T) {
	config := &MiddlewareConfig{
		IdentificationMethod: IdentificationMethodCombined,
		TenantHeaderName:     "X-Custom-Tenant",
		TenantDomainHeader:   "X-Custom-Domain",
		TenantPathPrefix:     "/tenant",
		TenantSubdomain:      true,
		DefaultTenantID:      "default",
		AutoProvisionTenants: true,
		RequireTenant:        false,
		EnableCaching:        true,
		CacheTTL:            600,
	}

	assert.Equal(t, IdentificationMethodCombined, config.IdentificationMethod)
	assert.Equal(t, "X-Custom-Tenant", config.TenantHeaderName)
	assert.Equal(t, "X-Custom-Domain", config.TenantDomainHeader)
	assert.Equal(t, "/tenant", config.TenantPathPrefix)
	assert.True(t, config.TenantSubdomain)
	assert.Equal(t, "default", config.DefaultTenantID)
	assert.True(t, config.AutoProvisionTenants)
	assert.False(t, config.RequireTenant)
	assert.True(t, config.EnableCaching)
	assert.Equal(t, 600, config.CacheTTL)
}

func TestTenantMiddleware_NoTenantContext(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	enforcement := NewTenantEnforcementMiddleware(manager, logger)
	handler := enforcement.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request without tenant context should pass through
	req := httptest.NewRequest("GET", "/test", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
}

// Helper function to create a time pointer
func middlewareTimePtr(t time.Time) *time.Time {
	return &t
}
package tenancy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestNewTenantAPI(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	api := NewTenantAPI(manager, logger)
	assert.NotNil(t, api)
	assert.Equal(t, manager, api.tenantManager)
	assert.Equal(t, logger, api.logger)
}

func TestTenantAPI_CreateTenant(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	api := NewTenantAPI(manager, logger)
	router := mux.NewRouter()
	api.RegisterRoutes(router)

	tests := []struct {
		name         string
		request      interface{}
		expectedCode int
		expectTenant bool
		errorType    string
	}{
		{
			name: "valid tenant creation",
			request: CreateTenantRequest{
				ID:          "test-tenant-1",
				Name:        "Test Tenant 1",
				DisplayName: "Test Tenant Display",
				Description: "A test tenant",
				Type:        TenantTypeTeam,
				Metadata: map[string]string{
					"environment": "test",
				},
			},
			expectedCode: http.StatusCreated,
			expectTenant: true,
		},
		{
			name: "missing tenant ID",
			request: CreateTenantRequest{
				Name: "Test Tenant 2",
				Type: TenantTypePersonal,
			},
			expectedCode: http.StatusBadRequest,
			expectTenant: false,
			errorType:    "validation_error",
		},
		{
			name: "missing tenant name",
			request: CreateTenantRequest{
				ID:   "test-tenant-3",
				Type: TenantTypePersonal,
			},
			expectedCode: http.StatusBadRequest,
			expectTenant: false,
			errorType:    "validation_error",
		},
		{
			name: "tenant with custom settings",
			request: CreateTenantRequest{
				ID:   "test-tenant-4",
				Name: "Test Tenant 4",
				Type: TenantTypeEnterprise,
				Settings: &TenantSettings{
					AllowedModels: []string{"gpt-4o", "claude-3-5-sonnet-20241022"},
					DefaultModel:  "gpt-4o",
					EnableMetrics: true,
					LogLevel:      "DEBUG",
					DataRegion:    "eu-west-1",
				},
				Limits: &TenantLimits{
					RequestsPerHour: 5000,
					TokensPerHour:   500000,
					MaxUsers:        100,
				},
			},
			expectedCode: http.StatusCreated,
			expectTenant: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody, err := json.Marshal(tt.request)
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "/tenants", bytes.NewBuffer(reqBody))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, req)

			assert.Equal(t, tt.expectedCode, recorder.Code)
			assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))

			if tt.expectTenant {
				var tenant Tenant
				err := json.Unmarshal(recorder.Body.Bytes(), &tenant)
				assert.NoError(t, err)

				createReq := tt.request.(CreateTenantRequest)
				assert.Equal(t, createReq.ID, tenant.ID)
				assert.Equal(t, createReq.Name, tenant.Name)
				assert.Equal(t, createReq.Type, tenant.Type)
				assert.Equal(t, TenantStatusActive, tenant.Status)
				assert.NotZero(t, tenant.CreatedAt)
				assert.NotZero(t, tenant.UpdatedAt)
			} else {
				var errorResponse map[string]interface{}
				err := json.Unmarshal(recorder.Body.Bytes(), &errorResponse)
				assert.NoError(t, err)

				errorObj := errorResponse["error"].(map[string]interface{})
				if tt.errorType != "" {
					assert.Equal(t, tt.errorType, errorObj["type"])
				}
			}
		})
	}
}

func TestTenantAPI_CreateTenant_InvalidJSON(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	api := NewTenantAPI(manager, logger)
	router := mux.NewRouter()
	api.RegisterRoutes(router)

	req := httptest.NewRequest("POST", "/tenants", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)

	var errorResponse map[string]interface{}
	err := json.Unmarshal(recorder.Body.Bytes(), &errorResponse)
	assert.NoError(t, err)

	errorObj := errorResponse["error"].(map[string]interface{})
	assert.Equal(t, "invalid_request", errorObj["type"])
}

func TestTenantAPI_GetTenant(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create a test tenant
	tenant := &Tenant{
		ID:          "get-tenant-test",
		Name:        "Get Tenant Test",
		DisplayName: "Get Tenant Display",
		Type:        TenantTypeTeam,
		Status:      TenantStatusActive,
	}
	err := manager.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	api := NewTenantAPI(manager, logger)
	router := mux.NewRouter()
	api.RegisterRoutes(router)

	tests := []struct {
		name         string
		tenantID     string
		expectedCode int
		expectTenant bool
	}{
		{
			name:         "existing tenant",
			tenantID:     "get-tenant-test",
			expectedCode: http.StatusOK,
			expectTenant: true,
		},
		{
			name:         "non-existent tenant",
			tenantID:     "non-existent",
			expectedCode: http.StatusNotFound,
			expectTenant: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/tenants/"+tt.tenantID, nil)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, req)

			assert.Equal(t, tt.expectedCode, recorder.Code)

			if tt.expectTenant {
				var retrievedTenant Tenant
				err := json.Unmarshal(recorder.Body.Bytes(), &retrievedTenant)
				assert.NoError(t, err)
				assert.Equal(t, tt.tenantID, retrievedTenant.ID)
			}
		})
	}
}

func TestTenantAPI_UpdateTenant(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create a test tenant
	tenant := &Tenant{
		ID:     "update-tenant-test",
		Name:   "Original Name",
		Type:   TenantTypeTeam,
		Status: TenantStatusActive,
	}
	err := manager.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	api := NewTenantAPI(manager, logger)
	router := mux.NewRouter()
	api.RegisterRoutes(router)

	tests := []struct {
		name         string
		tenantID     string
		request      UpdateTenantRequest
		expectedCode int
		checkUpdates func(*testing.T, *Tenant)
	}{
		{
			name:     "update name and display name",
			tenantID: "update-tenant-test",
			request: UpdateTenantRequest{
				Name:        stringPtr("Updated Name"),
				DisplayName: stringPtr("Updated Display"),
				Description: stringPtr("Updated Description"),
			},
			expectedCode: http.StatusOK,
			checkUpdates: func(t *testing.T, tenant *Tenant) {
				assert.Equal(t, "Updated Name", tenant.Name)
				assert.Equal(t, "Updated Display", tenant.DisplayName)
				assert.Equal(t, "Updated Description", tenant.Description)
			},
		},
		{
			name:     "update type and status",
			tenantID: "update-tenant-test",
			request: UpdateTenantRequest{
				Type:   tenantTypePtr(TenantTypeEnterprise),
				Status: tenantStatusPtr(TenantStatusSuspended),
			},
			expectedCode: http.StatusOK,
			checkUpdates: func(t *testing.T, tenant *Tenant) {
				assert.Equal(t, TenantTypeEnterprise, tenant.Type)
				assert.Equal(t, TenantStatusSuspended, tenant.Status)
			},
		},
		{
			name:     "update metadata",
			tenantID: "update-tenant-test",
			request: UpdateTenantRequest{
				Metadata: map[string]string{
					"environment": "production",
					"region":      "us-west-2",
				},
			},
			expectedCode: http.StatusOK,
			checkUpdates: func(t *testing.T, tenant *Tenant) {
				assert.Equal(t, "production", tenant.Metadata["environment"])
				assert.Equal(t, "us-west-2", tenant.Metadata["region"])
			},
		},
		{
			name:     "non-existent tenant",
			tenantID: "non-existent",
			request: UpdateTenantRequest{
				Name: stringPtr("Updated Name"),
			},
			expectedCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody, err := json.Marshal(tt.request)
			require.NoError(t, err)

			req := httptest.NewRequest("PUT", "/tenants/"+tt.tenantID, bytes.NewBuffer(reqBody))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, req)

			assert.Equal(t, tt.expectedCode, recorder.Code)

			if tt.expectedCode == http.StatusOK && tt.checkUpdates != nil {
				var updatedTenant Tenant
				err := json.Unmarshal(recorder.Body.Bytes(), &updatedTenant)
				assert.NoError(t, err)
				tt.checkUpdates(t, &updatedTenant)
			}
		})
	}
}

func TestTenantAPI_DeleteTenant(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create a test tenant
	tenant := &Tenant{
		ID:     "delete-tenant-test",
		Name:   "Delete Test",
		Type:   TenantTypePersonal,
		Status: TenantStatusActive,
	}
	err := manager.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	api := NewTenantAPI(manager, logger)
	router := mux.NewRouter()
	api.RegisterRoutes(router)

	tests := []struct {
		name         string
		tenantID     string
		expectedCode int
	}{
		{
			name:         "delete existing tenant",
			tenantID:     "delete-tenant-test",
			expectedCode: http.StatusNoContent,
		},
		{
			name:         "delete non-existent tenant",
			tenantID:     "non-existent",
			expectedCode: http.StatusInternalServerError, // Manager will return error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("DELETE", "/tenants/"+tt.tenantID, nil)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, req)

			assert.Equal(t, tt.expectedCode, recorder.Code)

			if tt.expectedCode == http.StatusNoContent {
				// Verify tenant is soft deleted
				deletedTenant, err := manager.GetTenant(context.Background(), tt.tenantID)
				assert.NoError(t, err)
				assert.Equal(t, TenantStatusDeleted, deletedTenant.Status)
				assert.NotNil(t, deletedTenant.DeletedAt)
			}
		})
	}
}

func TestTenantAPI_ListTenants(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create test tenants
	tenants := []*Tenant{
		{
			ID:     "list-tenant-1",
			Name:   "List Tenant 1",
			Type:   TenantTypeTeam,
			Status: TenantStatusActive,
		},
		{
			ID:     "list-tenant-2",
			Name:   "List Tenant 2",
			Type:   TenantTypePersonal,
			Status: TenantStatusActive,
		},
		{
			ID:       "list-tenant-3",
			Name:     "List Tenant 3",
			Type:     TenantTypeTeam,
			Status:   TenantStatusSuspended,
			ParentID: "list-tenant-1",
		},
	}

	for _, tenant := range tenants {
		err := manager.CreateTenant(context.Background(), tenant)
		require.NoError(t, err)
	}

	api := NewTenantAPI(manager, logger)
	router := mux.NewRouter()
	api.RegisterRoutes(router)

	tests := []struct {
		name           string
		queryParams    string
		expectedCode   int
		expectedCount  int
		validateResult func(*testing.T, *ListTenantsResponse)
	}{
		{
			name:          "list all tenants",
			queryParams:   "",
			expectedCode:  http.StatusOK,
			expectedCount: 3,
			validateResult: func(t *testing.T, resp *ListTenantsResponse) {
				assert.Equal(t, 3, resp.Pagination.Total)
				assert.Equal(t, 3, resp.Pagination.Count)
				assert.Equal(t, 0, resp.Pagination.Offset)
				assert.Equal(t, 50, resp.Pagination.Limit)
			},
		},
		{
			name:          "filter by status",
			queryParams:   "?status=active",
			expectedCode:  http.StatusOK,
			expectedCount: 2,
			validateResult: func(t *testing.T, resp *ListTenantsResponse) {
				for _, tenant := range resp.Tenants {
					assert.Equal(t, TenantStatusActive, tenant.Status)
				}
			},
		},
		{
			name:          "filter by type",
			queryParams:   "?type=team",
			expectedCode:  http.StatusOK,
			expectedCount: 2,
			validateResult: func(t *testing.T, resp *ListTenantsResponse) {
				for _, tenant := range resp.Tenants {
					assert.Equal(t, TenantTypeTeam, tenant.Type)
				}
			},
		},
		{
			name:          "filter by parent_id",
			queryParams:   "?parent_id=list-tenant-1",
			expectedCode:  http.StatusOK,
			expectedCount: 1,
			validateResult: func(t *testing.T, resp *ListTenantsResponse) {
				assert.Equal(t, "list-tenant-3", resp.Tenants[0].ID)
				assert.Equal(t, "list-tenant-1", resp.Tenants[0].ParentID)
			},
		},
		{
			name:          "pagination with limit",
			queryParams:   "?limit=2",
			expectedCode:  http.StatusOK,
			expectedCount: 2,
			validateResult: func(t *testing.T, resp *ListTenantsResponse) {
				assert.Equal(t, 3, resp.Pagination.Total)
				assert.Equal(t, 2, resp.Pagination.Count)
				assert.Equal(t, 2, resp.Pagination.Limit)
			},
		},
		{
			name:          "pagination with offset",
			queryParams:   "?limit=2&offset=1",
			expectedCode:  http.StatusOK,
			expectedCount: 2,
			validateResult: func(t *testing.T, resp *ListTenantsResponse) {
				assert.Equal(t, 3, resp.Pagination.Total)
				assert.Equal(t, 2, resp.Pagination.Count)
				assert.Equal(t, 1, resp.Pagination.Offset)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/tenants"+tt.queryParams, nil)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, req)

			assert.Equal(t, tt.expectedCode, recorder.Code)

			if tt.expectedCode == http.StatusOK {
				var response ListTenantsResponse
				err := json.Unmarshal(recorder.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Len(t, response.Tenants, tt.expectedCount)

				if tt.validateResult != nil {
					tt.validateResult(t, &response)
				}
			}
		})
	}
}

func TestTenantAPI_ActivateAndSuspendTenant(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create a test tenant
	tenant := &Tenant{
		ID:     "status-tenant-test",
		Name:   "Status Test",
		Type:   TenantTypeTeam,
		Status: TenantStatusSuspended,
	}
	err := manager.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	api := NewTenantAPI(manager, logger)
	router := mux.NewRouter()
	api.RegisterRoutes(router)

	// Test activation
	t.Run("activate tenant", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/tenants/status-tenant-test/activate", nil)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var activatedTenant Tenant
		err := json.Unmarshal(recorder.Body.Bytes(), &activatedTenant)
		assert.NoError(t, err)
		assert.Equal(t, TenantStatusActive, activatedTenant.Status)
	})

	// Test suspension
	t.Run("suspend tenant", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/tenants/status-tenant-test/suspend", nil)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var suspendedTenant Tenant
		err := json.Unmarshal(recorder.Body.Bytes(), &suspendedTenant)
		assert.NoError(t, err)
		assert.Equal(t, TenantStatusSuspended, suspendedTenant.Status)
	})

	// Test with non-existent tenant
	t.Run("activate non-existent tenant", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/tenants/non-existent/activate", nil)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})
}

func TestTenantAPI_GetTenantUsage(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create a test tenant
	tenant := &Tenant{
		ID:     "usage-tenant-test",
		Name:   "Usage Test",
		Type:   TenantTypeTeam,
		Status: TenantStatusActive,
	}
	err := manager.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	// Record some usage
	usageRecord := &UsageRecord{
		Type:      "request",
		Count:     10,
		Timestamp: time.Now(),
	}
	err = manager.RecordUsage(context.Background(), "usage-tenant-test", usageRecord)
	require.NoError(t, err)

	api := NewTenantAPI(manager, logger)
	router := mux.NewRouter()
	api.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/tenants/usage-tenant-test/usage", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)

	var usage UsageMetrics
	err = json.Unmarshal(recorder.Body.Bytes(), &usage)
	assert.NoError(t, err)
	assert.Equal(t, int64(10), usage.RequestsThisHour)
}

func TestTenantAPI_GetAndUpdateTenantLimits(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create a test tenant
	tenant := &Tenant{
		ID:     "limits-tenant-test",
		Name:   "Limits Test",
		Type:   TenantTypeTeam,
		Status: TenantStatusActive,
	}
	err := manager.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	api := NewTenantAPI(manager, logger)
	router := mux.NewRouter()
	api.RegisterRoutes(router)

	// Test getting limits
	t.Run("get tenant limits", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/tenants/limits-tenant-test/limits", nil)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var limits TenantLimits
		err := json.Unmarshal(recorder.Body.Bytes(), &limits)
		assert.NoError(t, err)
		assert.Greater(t, limits.RequestsPerHour, int64(0))
	})

	// Test updating limits
	t.Run("update tenant limits", func(t *testing.T) {
		newLimits := TenantLimits{
			RequestsPerHour: 5000,
			TokensPerHour:   500000,
			MaxUsers:        100,
		}

		reqBody, err := json.Marshal(newLimits)
		require.NoError(t, err)

		req := httptest.NewRequest("PUT", "/tenants/limits-tenant-test/limits", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var updatedLimits TenantLimits
		err = json.Unmarshal(recorder.Body.Bytes(), &updatedLimits)
		assert.NoError(t, err)
		assert.Equal(t, int64(5000), updatedLimits.RequestsPerHour)
		assert.Equal(t, int64(500000), updatedLimits.TokensPerHour)
		assert.Equal(t, 100, updatedLimits.MaxUsers)
	})
}

func TestTenantAPI_GetAndUpdateTenantSettings(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create a test tenant
	tenant := &Tenant{
		ID:     "settings-tenant-test",
		Name:   "Settings Test",
		Type:   TenantTypeTeam,
		Status: TenantStatusActive,
	}
	err := manager.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	api := NewTenantAPI(manager, logger)
	router := mux.NewRouter()
	api.RegisterRoutes(router)

	// Test getting settings
	t.Run("get tenant settings", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/tenants/settings-tenant-test/settings", nil)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var settings TenantSettings
		err := json.Unmarshal(recorder.Body.Bytes(), &settings)
		assert.NoError(t, err)
		assert.NotNil(t, settings.AllowedModels)
	})

	// Test updating settings
	t.Run("update tenant settings", func(t *testing.T) {
		newSettings := TenantSettings{
			AllowedModels: []string{"gpt-4o", "claude-3-5-sonnet-20241022"},
			DefaultModel:  "gpt-4o",
			EnableMetrics: true,
			LogLevel:      "DEBUG",
			DataRegion:    "eu-west-1",
			Features: map[string]bool{
				"advanced_analytics": true,
				"custom_models":      true,
			},
		}

		reqBody, err := json.Marshal(newSettings)
		require.NoError(t, err)

		req := httptest.NewRequest("PUT", "/tenants/settings-tenant-test/settings", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var updatedSettings TenantSettings
		err = json.Unmarshal(recorder.Body.Bytes(), &updatedSettings)
		assert.NoError(t, err)
		assert.Equal(t, []string{"gpt-4o", "claude-3-5-sonnet-20241022"}, updatedSettings.AllowedModels)
		assert.Equal(t, "gpt-4o", updatedSettings.DefaultModel)
		assert.Equal(t, "DEBUG", updatedSettings.LogLevel)
		assert.True(t, updatedSettings.Features["advanced_analytics"])
	})
}

func TestTenantAPI_GetAndUpdateTenantSecurity(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create a test tenant
	tenant := &Tenant{
		ID:     "security-tenant-test",
		Name:   "Security Test",
		Type:   TenantTypeEnterprise,
		Status: TenantStatusActive,
	}
	err := manager.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	api := NewTenantAPI(manager, logger)
	router := mux.NewRouter()
	api.RegisterRoutes(router)

	// Test getting security settings
	t.Run("get tenant security", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/tenants/security-tenant-test/security", nil)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var security TenantSecurity
		err := json.Unmarshal(recorder.Body.Bytes(), &security)
		assert.NoError(t, err)
		assert.NotNil(t, security.AllowedAuthMethods)
	})

	// Test updating security settings
	t.Run("update tenant security", func(t *testing.T) {
		newSecurity := TenantSecurity{
			RequireSSO:         true,
			AllowedAuthMethods: []string{"saml", "oauth2"},
			SessionTimeout:     8 * time.Hour,
			AllowedIPs:         []string{"192.168.1.0/24"},
			PIIMaskingEnabled:  true,
			RequireMFA:         true,
			PasswordPolicy: &PasswordPolicy{
				MinLength:        12,
				RequireUppercase: true,
				RequireLowercase: true,
				RequireNumbers:   true,
				RequireSymbols:   true,
			},
		}

		reqBody, err := json.Marshal(newSecurity)
		require.NoError(t, err)

		req := httptest.NewRequest("PUT", "/tenants/security-tenant-test/security", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var updatedSecurity TenantSecurity
		err = json.Unmarshal(recorder.Body.Bytes(), &updatedSecurity)
		assert.NoError(t, err)
		assert.True(t, updatedSecurity.RequireSSO)
		assert.Equal(t, []string{"saml", "oauth2"}, updatedSecurity.AllowedAuthMethods)
		assert.Equal(t, 8*time.Hour, updatedSecurity.SessionTimeout)
		assert.True(t, updatedSecurity.RequireMFA)
		assert.Equal(t, 12, updatedSecurity.PasswordPolicy.MinLength)
	})
}

func TestTenantAPI_GetAndUpdateTenantSubscription(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create a test tenant with subscription
	now := time.Now()
	tenant := &Tenant{
		ID:     "subscription-tenant-test",
		Name:   "Subscription Test",
		Type:   TenantTypeEnterprise,
		Status: TenantStatusActive,
		Subscription: &Subscription{
			PlanID:      "enterprise-plan",
			PlanName:    "Enterprise Plan",
			Status:      "active",
			StartDate:   now,
			RenewalDate: now.Add(30 * 24 * time.Hour),
			Amount:      299.99,
			Currency:    "USD",
		},
	}
	err := manager.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	api := NewTenantAPI(manager, logger)
	router := mux.NewRouter()
	api.RegisterRoutes(router)

	// Test getting subscription
	t.Run("get tenant subscription", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/tenants/subscription-tenant-test/subscription", nil)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var subscription Subscription
		err := json.Unmarshal(recorder.Body.Bytes(), &subscription)
		assert.NoError(t, err)
		assert.Equal(t, "enterprise-plan", subscription.PlanID)
		assert.Equal(t, "active", subscription.Status)
		assert.Equal(t, 299.99, subscription.Amount)
	})

	// Test updating subscription
	t.Run("update tenant subscription", func(t *testing.T) {
		updatedSubscription := Subscription{
			PlanID:       "enterprise-plus",
			PlanName:     "Enterprise Plus Plan",
			Status:       "active",
			StartDate:    now,
			RenewalDate:  now.Add(30 * 24 * time.Hour),
			Amount:       499.99,
			Currency:     "USD",
			BillingCycle: "monthly",
		}

		reqBody, err := json.Marshal(updatedSubscription)
		require.NoError(t, err)

		req := httptest.NewRequest("PUT", "/tenants/subscription-tenant-test/subscription", bytes.NewBuffer(reqBody))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var newSubscription Subscription
		err = json.Unmarshal(recorder.Body.Bytes(), &newSubscription)
		assert.NoError(t, err)
		assert.Equal(t, "enterprise-plus", newSubscription.PlanID)
		assert.Equal(t, 499.99, newSubscription.Amount)
		assert.Equal(t, "monthly", newSubscription.BillingCycle)
	})
}

func TestTenantAPI_CheckTenantAccess(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create a test tenant
	tenant := &Tenant{
		ID:     "access-tenant-test",
		Name:   "Access Test",
		Type:   TenantTypeTeam,
		Status: TenantStatusActive,
	}
	err := manager.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	api := NewTenantAPI(manager, logger)
	router := mux.NewRouter()
	api.RegisterRoutes(router)

	accessCheckReq := AccessCheckRequest{
		UserID:   "user123",
		Resource: "models",
		Action:   "list",
	}

	reqBody, err := json.Marshal(accessCheckReq)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/tenants/access-tenant-test/access-check", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)

	var accessResult AccessResult
	err = json.Unmarshal(recorder.Body.Bytes(), &accessResult)
	assert.NoError(t, err)
	assert.True(t, accessResult.Allowed)
	assert.Equal(t, "access-tenant-test", accessResult.TenantID)
	assert.Equal(t, "user123", accessResult.UserID)
	assert.Equal(t, "models", accessResult.Resource)
	assert.Equal(t, "list", accessResult.Action)
}

func TestTenantAPI_GetTenantStats(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create test tenants
	tenants := []*Tenant{
		{
			ID:     "stats-tenant-1",
			Name:   "Stats Tenant 1",
			Type:   TenantTypeTeam,
			Status: TenantStatusActive,
		},
		{
			ID:     "stats-tenant-2",
			Name:   "Stats Tenant 2",
			Type:   TenantTypePersonal,
			Status: TenantStatusActive,
		},
		{
			ID:     "stats-tenant-3",
			Name:   "Stats Tenant 3",
			Type:   TenantTypeTeam,
			Status: TenantStatusSuspended,
		},
	}

	for _, tenant := range tenants {
		err := manager.CreateTenant(context.Background(), tenant)
		require.NoError(t, err)
	}

	api := NewTenantAPI(manager, logger)
	router := mux.NewRouter()
	api.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/tenants/stats", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", recorder.Code, recorder.Body.String())
	}

	var stats map[string]interface{}
	err := json.Unmarshal(recorder.Body.Bytes(), &stats)
	assert.NoError(t, err)
	assert.Equal(t, float64(3), stats["total_tenants"])
	if trackingEnabled, ok := stats["usage_tracking_enabled"]; ok {
		assert.True(t, trackingEnabled.(bool))
	} else {
		t.Fatal("usage_tracking_enabled field not found in stats")
	}
}

func TestTenantAPI_GetTenantAnalytics(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	// Create a test tenant
	tenant := &Tenant{
		ID:     "analytics-tenant-test",
		Name:   "Analytics Test",
		Type:   TenantTypeTeam,
		Status: TenantStatusActive,
	}
	err := manager.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	// Record some usage
	usageRecord := &UsageRecord{
		Type:      "request",
		Count:     50,
		Timestamp: time.Now(),
	}
	err = manager.RecordUsage(context.Background(), "analytics-tenant-test", usageRecord)
	require.NoError(t, err)

	api := NewTenantAPI(manager, logger)
	router := mux.NewRouter()
	api.RegisterRoutes(router)

	req := httptest.NewRequest("GET", "/tenants/analytics-tenant-test/analytics", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)

	var analytics TenantAnalytics
	err = json.Unmarshal(recorder.Body.Bytes(), &analytics)
	assert.NoError(t, err)
	assert.Equal(t, "analytics-tenant-test", analytics.TenantID)
	assert.Equal(t, "Analytics Test", analytics.TenantName)
	assert.Equal(t, TenantTypeTeam, analytics.TenantType)
	assert.Equal(t, TenantStatusActive, analytics.Status)
	assert.NotNil(t, analytics.Usage)
	assert.NotNil(t, analytics.Limits)
	assert.NotNil(t, analytics.Utilization)
	assert.Equal(t, int64(50), analytics.Usage.RequestsThisHour)
}

func TestTenantAPI_CalculateUtilization(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestTenantManager(t)
	defer manager.Stop()

	api := NewTenantAPI(manager, logger)

	usage := &UsageMetrics{
		RequestsThisHour:  50,
		RequestsThisDay:   500,
		RequestsThisMonth: 5000,
		TokensThisHour:    5000,
		TokensThisDay:     50000,
		TokensThisMonth:   500000,
		CostThisHour:      25.0,
		CostThisDay:       250.0,
		CostThisMonth:     2500.0,
		StorageUsedGB:     25,
		FilesCount:        2500,
	}

	limits := &TenantLimits{
		RequestsPerHour:  100,
		RequestsPerDay:   1000,
		RequestsPerMonth: 10000,
		TokensPerHour:    10000,
		TokensPerDay:     100000,
		TokensPerMonth:   1000000,
		CostPerHour:      50.0,
		CostPerDay:       500.0,
		CostPerMonth:     5000.0,
		MaxStorageGB:     100,
		MaxFiles:         10000,
	}

	utilization := api.calculateUtilization(usage, limits)

	assert.Equal(t, 50.0, utilization["requests_per_hour"])
	assert.Equal(t, 50.0, utilization["requests_per_day"])
	assert.Equal(t, 50.0, utilization["requests_per_month"])
	assert.Equal(t, 50.0, utilization["tokens_per_hour"])
	assert.Equal(t, 50.0, utilization["tokens_per_day"])
	assert.Equal(t, 50.0, utilization["tokens_per_month"])
	assert.Equal(t, 50.0, utilization["cost_per_hour"])
	assert.Equal(t, 50.0, utilization["cost_per_day"])
	assert.Equal(t, 50.0, utilization["cost_per_month"])
	assert.Equal(t, 25.0, utilization["storage"])
	assert.Equal(t, 25.0, utilization["files"])
}

func TestTenantAPI_RequestResponseTypes(t *testing.T) {
	// Test CreateTenantRequest
	createReq := CreateTenantRequest{
		ID:          "test-tenant",
		Name:        "Test Tenant",
		DisplayName: "Test Display",
		Type:        TenantTypeTeam,
		Metadata: map[string]string{
			"env": "test",
		},
	}

	assert.Equal(t, "test-tenant", createReq.ID)
	assert.Equal(t, "Test Tenant", createReq.Name)
	assert.Equal(t, TenantTypeTeam, createReq.Type)

	// Test UpdateTenantRequest
	updateReq := UpdateTenantRequest{
		Name:        stringPtr("Updated Name"),
		DisplayName: stringPtr("Updated Display"),
		Type:        tenantTypePtr(TenantTypeEnterprise),
	}

	assert.Equal(t, "Updated Name", *updateReq.Name)
	assert.Equal(t, "Updated Display", *updateReq.DisplayName)
	assert.Equal(t, TenantTypeEnterprise, *updateReq.Type)

	// Test ListTenantsResponse
	tenants := []*Tenant{
		{ID: "tenant1", Name: "Tenant 1"},
		{ID: "tenant2", Name: "Tenant 2"},
	}

	listResp := ListTenantsResponse{
		Tenants: tenants,
		Pagination: PaginationInfo{
			Total:  10,
			Limit:  2,
			Offset: 0,
			Count:  2,
		},
	}

	assert.Len(t, listResp.Tenants, 2)
	assert.Equal(t, 10, listResp.Pagination.Total)

	// Test AccessCheckRequest
	accessReq := AccessCheckRequest{
		UserID:   "user123",
		Resource: "models",
		Action:   "list",
	}

	assert.Equal(t, "user123", accessReq.UserID)
	assert.Equal(t, "models", accessReq.Resource)
	assert.Equal(t, "list", accessReq.Action)

	// Test TenantAnalytics
	analytics := TenantAnalytics{
		TenantID:   "tenant123",
		TenantName: "Test Tenant",
		TenantType: TenantTypeTeam,
		Status:     TenantStatusActive,
		CreatedAt:  time.Now(),
		Utilization: map[string]float64{
			"requests": 75.5,
			"tokens":   50.0,
		},
		LastUpdated: time.Now(),
	}

	assert.Equal(t, "tenant123", analytics.TenantID)
	assert.Equal(t, "Test Tenant", analytics.TenantName)
	assert.Equal(t, 75.5, analytics.Utilization["requests"])
}

// Helper functions

func stringPtr(s string) *string {
	return &s
}

func tenantTypePtr(t TenantType) *TenantType {
	return &t
}

func tenantStatusPtr(s TenantStatus) *TenantStatus {
	return &s
}

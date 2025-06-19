package tenancy

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestNewTenantManager(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	
	tests := []struct {
		name    string
		config  *TenantConfig
		wantErr bool
	}{
		{
			name:    "default config",
			config:  nil,
			wantErr: false,
		},
		{
			name: "custom config",
			config: &TenantConfig{
				Enabled:           true,
				DefaultTenantType: TenantTypeTeam,
				TrackUsage:        true,
				StorageType:       "memory",
			},
			wantErr: false,
		},
		{
			name: "disabled tenancy",
			config: &TenantConfig{
				Enabled: false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewTenantManager(tt.config, nil, nil, logger)
			
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, manager)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, manager)
				
				if tt.config == nil {
					assert.NotNil(t, manager.config)
					assert.True(t, manager.config.Enabled)
				} else {
					assert.Equal(t, tt.config.Enabled, manager.config.Enabled)
				}
			}
			
			if manager != nil {
				manager.Stop()
			}
		})
	}
}

func TestDefaultTenantConfig(t *testing.T) {
	config := DefaultTenantConfig()
	
	assert.NotNil(t, config)
	assert.True(t, config.Enabled)
	assert.Equal(t, TenantTypePersonal, config.DefaultTenantType)
	assert.True(t, config.TrackUsage)
	assert.Equal(t, time.Hour, config.UsageResetInterval)
	assert.Equal(t, 90, config.UsageRetentionDays)
	assert.True(t, config.AllowAutoProvisioning)
	assert.Equal(t, TenantTypePersonal, config.AutoProvisioningType)
	assert.True(t, config.EnableHierarchy)
	assert.Equal(t, 3, config.MaxDepth)
	assert.True(t, config.StrictIsolation)
	assert.False(t, config.SharedResources)
	assert.Equal(t, "memory", config.StorageType)
	assert.Equal(t, 24*time.Hour, config.CleanupInterval)
	assert.True(t, config.EnforceLimits)
	assert.Equal(t, 0.8, config.SoftLimitThreshold)
	assert.False(t, config.BillingEnabled)
}

func TestTenantManager_CreateTenant(t *testing.T) {
	manager := createTestTenantManager(t)
	defer manager.Stop()
	
	ctx := context.Background()
	
	tests := []struct {
		name    string
		tenant  *Tenant
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid tenant",
			tenant: &Tenant{
				ID:          "tenant-1",
				Name:        "Test Tenant",
				DisplayName: "Test Tenant Display",
				Type:        TenantTypeTeam,
				Status:      TenantStatusActive,
			},
			wantErr: false,
		},
		{
			name: "tenant without ID",
			tenant: &Tenant{
				Name: "Test Tenant",
				Type: TenantTypeTeam,
			},
			wantErr: true,
			errMsg:  "tenant ID is required",
		},
		{
			name: "tenant without name",
			tenant: &Tenant{
				ID:   "tenant-2",
				Type: TenantTypeTeam,
			},
			wantErr: true,
			errMsg:  "tenant name is required",
		},
		{
			name: "duplicate tenant ID",
			tenant: &Tenant{
				ID:   "tenant-1", // Already exists from first test
				Name: "Another Tenant",
				Type: TenantTypePersonal,
			},
			wantErr: true,
			errMsg:  "already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.CreateTenant(ctx, tt.tenant)
			
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				
				// Verify tenant was created with defaults
				assert.NotZero(t, tt.tenant.CreatedAt)
				assert.NotZero(t, tt.tenant.UpdatedAt)
				assert.NotNil(t, tt.tenant.Settings)
				assert.NotNil(t, tt.tenant.Limits)
				assert.NotNil(t, tt.tenant.Security)
				
				// Verify tenant can be retrieved
				retrieved, err := manager.GetTenant(ctx, tt.tenant.ID)
				assert.NoError(t, err)
				assert.Equal(t, tt.tenant.ID, retrieved.ID)
				assert.Equal(t, tt.tenant.Name, retrieved.Name)
			}
		})
	}
}

func TestTenantManager_CreateTenantDisabled(t *testing.T) {
	config := &TenantConfig{Enabled: false}
	logger := zaptest.NewLogger(t).Sugar()
	manager, err := NewTenantManager(config, nil, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()
	
	ctx := context.Background()
	tenant := &Tenant{
		ID:   "tenant-1",
		Name: "Test Tenant",
		Type: TenantTypePersonal,
	}
	
	err = manager.CreateTenant(ctx, tenant)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "multi-tenancy is disabled")
}

func TestTenantManager_GetTenant(t *testing.T) {
	manager := createTestTenantManager(t)
	defer manager.Stop()
	
	ctx := context.Background()
	
	// Create a test tenant
	tenant := &Tenant{
		ID:          "tenant-1",
		Name:        "Test Tenant",
		DisplayName: "Test Tenant Display",
		Type:        TenantTypeTeam,
		Status:      TenantStatusActive,
	}
	
	err := manager.CreateTenant(ctx, tenant)
	require.NoError(t, err)
	
	// Test getting existing tenant
	retrieved, err := manager.GetTenant(ctx, "tenant-1")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "tenant-1", retrieved.ID)
	assert.Equal(t, "Test Tenant", retrieved.Name)
	
	// Test getting non-existent tenant
	_, err = manager.GetTenant(ctx, "non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	
	// Verify returned tenant is a copy (modifications don't affect original)
	retrieved.Name = "Modified Name"
	original, err := manager.GetTenant(ctx, "tenant-1")
	assert.NoError(t, err)
	assert.Equal(t, "Test Tenant", original.Name)
}

func TestTenantManager_UpdateTenant(t *testing.T) {
	manager := createTestTenantManager(t)
	defer manager.Stop()
	
	ctx := context.Background()
	
	// Create a test tenant
	tenant := &Tenant{
		ID:          "tenant-1",
		Name:        "Original Name",
		DisplayName: "Original Display",
		Type:        TenantTypeTeam,
		Status:      TenantStatusActive,
	}
	
	err := manager.CreateTenant(ctx, tenant)
	require.NoError(t, err)
	
	originalCreatedAt := tenant.CreatedAt
	
	// Update the tenant
	tenant.Name = "Updated Name"
	tenant.DisplayName = "Updated Display"
	tenant.Description = "Updated Description"
	
	time.Sleep(10 * time.Millisecond) // Ensure different timestamp
	err = manager.UpdateTenant(ctx, tenant)
	assert.NoError(t, err)
	
	// Verify update
	updated, err := manager.GetTenant(ctx, "tenant-1")
	assert.NoError(t, err)
	assert.Equal(t, "Updated Name", updated.Name)
	assert.Equal(t, "Updated Display", updated.DisplayName)
	assert.Equal(t, "Updated Description", updated.Description)
	assert.Equal(t, originalCreatedAt, updated.CreatedAt) // Should preserve creation time
	assert.True(t, updated.UpdatedAt.After(originalCreatedAt)) // Should update timestamp
	
	// Test updating non-existent tenant
	nonExistent := &Tenant{
		ID:   "non-existent",
		Name: "Non Existent",
		Type: TenantTypePersonal,
	}
	
	err = manager.UpdateTenant(ctx, nonExistent)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestTenantManager_DeleteTenant(t *testing.T) {
	manager := createTestTenantManager(t)
	defer manager.Stop()
	
	ctx := context.Background()
	
	// Create a test tenant
	tenant := &Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Type:   TenantTypeTeam,
		Status: TenantStatusActive,
	}
	
	err := manager.CreateTenant(ctx, tenant)
	require.NoError(t, err)
	
	// Delete the tenant
	err = manager.DeleteTenant(ctx, "tenant-1")
	assert.NoError(t, err)
	
	// Verify tenant is soft deleted
	deleted, err := manager.GetTenant(ctx, "tenant-1")
	assert.NoError(t, err)
	assert.Equal(t, TenantStatusDeleted, deleted.Status)
	assert.NotNil(t, deleted.DeletedAt)
	
	// Test deleting non-existent tenant
	err = manager.DeleteTenant(ctx, "non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestTenantManager_ListTenants(t *testing.T) {
	manager := createTestTenantManager(t)
	defer manager.Stop()
	
	ctx := context.Background()
	
	// Create test tenants
	tenants := []*Tenant{
		{
			ID:     "tenant-1",
			Name:   "Team Tenant",
			Type:   TenantTypeTeam,
			Status: TenantStatusActive,
		},
		{
			ID:     "tenant-2",
			Name:   "Personal Tenant",
			Type:   TenantTypePersonal,
			Status: TenantStatusActive,
		},
		{
			ID:     "tenant-3",
			Name:   "Enterprise Tenant",
			Type:   TenantTypeEnterprise,
			Status: TenantStatusActive,
		},
		{
			ID:     "tenant-4",
			Name:   "Suspended Tenant",
			Type:   TenantTypeTeam,
			Status: TenantStatusSuspended,
		},
	}
	
	for _, tenant := range tenants {
		err := manager.CreateTenant(ctx, tenant)
		require.NoError(t, err)
	}
	
	// Test listing all tenants
	allTenants, err := manager.ListTenants(ctx, nil)
	assert.NoError(t, err)
	assert.Len(t, allTenants, 4)
	
	// Test filtering by status
	activeStatus := TenantStatusActive
	filter := &TenantFilter{Status: &activeStatus}
	activeTenants, err := manager.ListTenants(ctx, filter)
	assert.NoError(t, err)
	assert.Len(t, activeTenants, 3)
	
	// Test filtering by type
	teamType := TenantTypeTeam
	filter = &TenantFilter{Type: &teamType}
	teamTenants, err := manager.ListTenants(ctx, filter)
	assert.NoError(t, err)
	assert.Len(t, teamTenants, 2)
	
	// Test filtering by creation time
	now := time.Now()
	future := now.Add(time.Hour)
	filter = &TenantFilter{CreatedBefore: &future}
	recentTenants, err := manager.ListTenants(ctx, filter)
	assert.NoError(t, err)
	assert.Len(t, recentTenants, 4) // All should be before future time
	
	past := now.Add(-time.Hour)
	filter = &TenantFilter{CreatedAfter: &past}
	newTenants, err := manager.ListTenants(ctx, filter)
	assert.NoError(t, err)
	assert.Len(t, newTenants, 4) // All should be after past time
}

func TestTenantManager_CheckAccess(t *testing.T) {
	manager := createTestTenantManager(t)
	defer manager.Stop()
	
	ctx := context.Background()
	
	// Create test tenants
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
			EndDate: managerTimePtr(time.Now().Add(-24 * time.Hour)),
		},
	}
	
	err := manager.CreateTenant(ctx, activeTenant)
	require.NoError(t, err)
	err = manager.CreateTenant(ctx, suspendedTenant)
	require.NoError(t, err)
	err = manager.CreateTenant(ctx, expiredTenant)
	require.NoError(t, err)
	
	tests := []struct {
		name      string
		tenantID  string
		userID    string
		resource  string
		action    string
		allowed   bool
		reasonContains string
	}{
		{
			name:     "active tenant access",
			tenantID: "active-tenant",
			userID:   "user-1",
			resource: "models",
			action:   "list",
			allowed:  true,
		},
		{
			name:           "suspended tenant access",
			tenantID:       "suspended-tenant",
			userID:         "user-1",
			resource:       "models",
			action:         "list",
			allowed:        false,
			reasonContains: "suspended",
		},
		{
			name:           "expired tenant access",
			tenantID:       "expired-tenant",
			userID:         "user-1",
			resource:       "models",
			action:         "list",
			allowed:        false,
			reasonContains: "expired",
		},
		{
			name:           "non-existent tenant",
			tenantID:       "non-existent",
			userID:         "user-1",
			resource:       "models",
			action:         "list",
			allowed:        false,
			reasonContains: "not found",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := manager.CheckAccess(ctx, tt.tenantID, tt.userID, tt.resource, tt.action)
			
			if tt.tenantID == "non-existent" {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			assert.NotNil(t, result)
			assert.Equal(t, tt.allowed, result.Allowed)
			assert.Equal(t, tt.tenantID, result.TenantID)
			assert.Equal(t, tt.userID, result.UserID)
			assert.Equal(t, tt.resource, result.Resource)
			assert.Equal(t, tt.action, result.Action)
			
			if tt.reasonContains != "" {
				assert.Contains(t, result.Reason, tt.reasonContains)
			}
		})
	}
}

func TestTenantManager_RecordUsage(t *testing.T) {
	manager := createTestTenantManager(t)
	defer manager.Stop()
	
	ctx := context.Background()
	
	// Create a test tenant
	tenant := &Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Type:   TenantTypeTeam,
		Status: TenantStatusActive,
	}
	
	err := manager.CreateTenant(ctx, tenant)
	require.NoError(t, err)
	
	// Record various types of usage
	usageRecords := []*UsageRecord{
		{
			Type:      "request",
			Count:     5,
			Model:     "gpt-4o",
			Provider:  "openai",
			Timestamp: time.Now(),
		},
		{
			Type:      "token",
			Count:     1000,
			Model:     "gpt-4o",
			Provider:  "openai",
			Timestamp: time.Now(),
		},
		{
			Type:      "cost",
			Amount:    2.50,
			Model:     "gpt-4o",
			Provider:  "openai",
			Timestamp: time.Now(),
		},
	}
	
	for _, usage := range usageRecords {
		err := manager.RecordUsage(ctx, "tenant-1", usage)
		assert.NoError(t, err)
	}
	
	// Verify usage metrics
	metrics, err := manager.GetUsageMetrics(ctx, "tenant-1")
	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(5), metrics.RequestsThisHour)
	assert.Equal(t, int64(5), metrics.RequestsThisDay)
	assert.Equal(t, int64(5), metrics.RequestsThisMonth)
	assert.Equal(t, int64(1000), metrics.TokensThisHour)
	assert.Equal(t, int64(1000), metrics.TokensThisDay)
	assert.Equal(t, int64(1000), metrics.TokensThisMonth)
	assert.Equal(t, 2.50, metrics.CostThisHour)
	assert.Equal(t, 2.50, metrics.CostThisDay)
	assert.Equal(t, 2.50, metrics.CostThisMonth)
}

func TestTenantManager_RecordUsageDisabled(t *testing.T) {
	config := &TenantConfig{
		Enabled:    true,
		TrackUsage: false,
	}
	logger := zaptest.NewLogger(t).Sugar()
	manager, err := NewTenantManager(config, nil, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()
	
	ctx := context.Background()
	
	usage := &UsageRecord{
		Type:  "request",
		Count: 5,
	}
	
	err = manager.RecordUsage(ctx, "tenant-1", usage)
	assert.NoError(t, err) // Should not error, just do nothing
}

func TestTenantManager_GetUsageMetrics(t *testing.T) {
	manager := createTestTenantManager(t)
	defer manager.Stop()
	
	ctx := context.Background()
	
	// Test getting metrics for non-existent tenant
	metrics, err := manager.GetUsageMetrics(ctx, "non-existent")
	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(0), metrics.RequestsThisHour)
	
	// Create a tenant and record usage
	tenant := &Tenant{
		ID:     "tenant-1",
		Name:   "Test Tenant",
		Type:   TenantTypeTeam,
		Status: TenantStatusActive,
	}
	
	err = manager.CreateTenant(ctx, tenant)
	require.NoError(t, err)
	
	usage := &UsageRecord{
		Type:      "request",
		Count:     10,
		Timestamp: time.Now(),
	}
	
	err = manager.RecordUsage(ctx, "tenant-1", usage)
	require.NoError(t, err)
	
	// Get metrics and verify they're a copy
	metrics, err = manager.GetUsageMetrics(ctx, "tenant-1")
	assert.NoError(t, err)
	assert.Equal(t, int64(10), metrics.RequestsThisHour)
	
	// Modify returned metrics and verify original is unchanged
	metrics.RequestsThisHour = 999
	
	metrics2, err := manager.GetUsageMetrics(ctx, "tenant-1")
	assert.NoError(t, err)
	assert.Equal(t, int64(10), metrics2.RequestsThisHour)
}

func TestTenantManager_GetTenantStats(t *testing.T) {
	manager := createTestTenantManager(t)
	defer manager.Stop()
	
	ctx := context.Background()
	
	// Create test tenants
	tenants := []*Tenant{
		{
			ID:     "tenant-1",
			Name:   "Team Tenant 1",
			Type:   TenantTypeTeam,
			Status: TenantStatusActive,
		},
		{
			ID:     "tenant-2",
			Name:   "Team Tenant 2",
			Type:   TenantTypeTeam,
			Status: TenantStatusSuspended,
		},
		{
			ID:     "tenant-3",
			Name:   "Enterprise Tenant",
			Type:   TenantTypeEnterprise,
			Status: TenantStatusActive,
		},
		{
			ID:     "tenant-4",
			Name:   "Personal Tenant",
			Type:   TenantTypePersonal,
			Status: TenantStatusActive,
		},
	}
	
	for _, tenant := range tenants {
		err := manager.CreateTenant(ctx, tenant)
		require.NoError(t, err)
	}
	
	stats := manager.GetTenantStats()
	assert.NotNil(t, stats)
	
	assert.Equal(t, 4, stats["total_tenants"])
	assert.True(t, stats["usage_tracking_enabled"].(bool))
	assert.Equal(t, 4, stats["tracked_usage_entries"]) // One per tenant
	
	statusCounts := stats["tenants_by_status"].(map[TenantStatus]int)
	assert.Equal(t, 3, statusCounts[TenantStatusActive])
	assert.Equal(t, 1, statusCounts[TenantStatusSuspended])
	
	typeCounts := stats["tenants_by_type"].(map[TenantType]int)
	assert.Equal(t, 2, typeCounts[TenantTypeTeam])
	assert.Equal(t, 1, typeCounts[TenantTypeEnterprise])
	assert.Equal(t, 1, typeCounts[TenantTypePersonal])
}

func TestTenantManager_ValidateTenant(t *testing.T) {
	manager := createTestTenantManager(t)
	defer manager.Stop()
	
	tests := []struct {
		name    string
		tenant  *Tenant
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid tenant",
			tenant: &Tenant{
				ID:   "valid-tenant",
				Name: "Valid Tenant",
				Type: TenantTypeTeam,
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			tenant: &Tenant{
				Name: "Missing ID Tenant",
			},
			wantErr: true,
			errMsg:  "tenant ID is required",
		},
		{
			name: "missing name",
			tenant: &Tenant{
				ID: "missing-name",
			},
			wantErr: true,
			errMsg:  "tenant name is required",
		},
		{
			name: "tenant without type gets default",
			tenant: &Tenant{
				ID:   "no-type",
				Name: "No Type Tenant",
			},
			wantErr: false,
		},
		{
			name: "tenant without status gets default",
			tenant: &Tenant{
				ID:   "no-status",
				Name: "No Status Tenant",
				Type: TenantTypePersonal,
			},
			wantErr: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.validateTenant(tt.tenant)
			
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				
				// Check defaults were set
				if tt.tenant.Type == "" {
					assert.Equal(t, manager.config.DefaultTenantType, tt.tenant.Type)
				}
				if tt.tenant.Status == "" {
					assert.Equal(t, TenantStatusActive, tt.tenant.Status)
				}
			}
		})
	}
}

func TestTenantManager_DefaultSettings(t *testing.T) {
	manager := createTestTenantManager(t)
	defer manager.Stop()
	
	tests := []struct {
		tenantType       TenantType
		expectedFeatures []string
	}{
		{
			tenantType: TenantTypeEnterprise,
			expectedFeatures: []string{
				"advanced_analytics",
				"custom_models",
				"priority_support",
				"sso",
				"audit_logs",
			},
		},
		{
			tenantType: TenantTypeTeam,
			expectedFeatures: []string{
				"team_collaboration",
				"shared_workspaces",
				"basic_analytics",
			},
		},
		{
			tenantType: TenantTypeTrial,
			expectedFeatures: []string{
				"basic_features",
			},
		},
		{
			tenantType: TenantTypePersonal,
			expectedFeatures: []string{
				"personal_workspace",
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(string(tt.tenantType), func(t *testing.T) {
			settings := manager.getDefaultTenantSettings(tt.tenantType)
			assert.NotNil(t, settings)
			
			assert.Equal(t, []string{"*"}, settings.AllowedModels)
			assert.Equal(t, []string{"*"}, settings.AllowedProviders)
			assert.True(t, settings.EnableMetrics)
			assert.True(t, settings.EnableAuditLogging)
			assert.Equal(t, "info", settings.LogLevel)
			assert.Equal(t, "us-east-1", settings.DataRegion)
			
			for _, feature := range tt.expectedFeatures {
				assert.True(t, settings.Features[feature], "Feature %s should be enabled", feature)
			}
		})
	}
}

func TestTenantManager_DefaultSecurity(t *testing.T) {
	manager := createTestTenantManager(t)
	defer manager.Stop()
	
	security := manager.getDefaultTenantSecurity()
	assert.NotNil(t, security)
	
	assert.False(t, security.RequireSSO)
	assert.Equal(t, []string{"password", "oauth2"}, security.AllowedAuthMethods)
	assert.Equal(t, 24*time.Hour, security.SessionTimeout)
	assert.True(t, security.PIIMaskingEnabled)
	assert.Equal(t, "replace", security.PIIMaskingStrategy)
	assert.False(t, security.EncryptionRequired)
	assert.Equal(t, 90, security.DataRetentionDays)
	assert.Equal(t, 365, security.AuditRetentionDays)
	assert.False(t, security.RequireMFA)
	
	assert.NotNil(t, security.PasswordPolicy)
	assert.Equal(t, 8, security.PasswordPolicy.MinLength)
	assert.True(t, security.PasswordPolicy.RequireUppercase)
	assert.True(t, security.PasswordPolicy.RequireLowercase)
	assert.True(t, security.PasswordPolicy.RequireNumbers)
	assert.False(t, security.PasswordPolicy.RequireSymbols)
	assert.Equal(t, 90*24*time.Hour, security.PasswordPolicy.MaxAge)
	assert.Equal(t, 5, security.PasswordPolicy.PreventReuse)
}

func TestTenantManager_TenantFilter_MatchesFilter(t *testing.T) {
	manager := createTestTenantManager(t)
	defer manager.Stop()
	
	now := time.Now()
	tenant := &Tenant{
		ID:        "test-tenant",
		Name:      "Test Tenant",
		Type:      TenantTypeTeam,
		Status:    TenantStatusActive,
		ParentID:  "parent-tenant",
		CreatedAt: now,
	}
	
	tests := []struct {
		name    string
		filter  *TenantFilter
		matches bool
	}{
		{
			name:    "nil filter matches all",
			filter:  nil,
			matches: true,
		},
		{
			name: "matching status",
			filter: &TenantFilter{
				Status: &[]TenantStatus{TenantStatusActive}[0],
			},
			matches: true,
		},
		{
			name: "non-matching status",
			filter: &TenantFilter{
				Status: &[]TenantStatus{TenantStatusSuspended}[0],
			},
			matches: false,
		},
		{
			name: "matching type",
			filter: &TenantFilter{
				Type: &[]TenantType{TenantTypeTeam}[0],
			},
			matches: true,
		},
		{
			name: "non-matching type",
			filter: &TenantFilter{
				Type: &[]TenantType{TenantTypeEnterprise}[0],
			},
			matches: false,
		},
		{
			name: "matching parent ID",
			filter: &TenantFilter{
				ParentID: &[]string{"parent-tenant"}[0],
			},
			matches: true,
		},
		{
			name: "non-matching parent ID",
			filter: &TenantFilter{
				ParentID: &[]string{"other-parent"}[0],
			},
			matches: false,
		},
		{
			name: "created after past time",
			filter: &TenantFilter{
				CreatedAfter: &[]time.Time{now.Add(-time.Hour)}[0],
			},
			matches: true,
		},
		{
			name: "created after future time",
			filter: &TenantFilter{
				CreatedAfter: &[]time.Time{now.Add(time.Hour)}[0],
			},
			matches: false,
		},
		{
			name: "created before future time",
			filter: &TenantFilter{
				CreatedBefore: &[]time.Time{now.Add(time.Hour)}[0],
			},
			matches: true,
		},
		{
			name: "created before past time",
			filter: &TenantFilter{
				CreatedBefore: &[]time.Time{now.Add(-time.Hour)}[0],
			},
			matches: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.matchesFilter(tenant, tt.filter)
			assert.Equal(t, tt.matches, result)
		})
	}
}

func TestUsageRecord_Structure(t *testing.T) {
	usage := &UsageRecord{
		Type:      "request",
		Count:     100,
		Amount:    15.50,
		Model:     "gpt-4o",
		Provider:  "openai",
		Timestamp: time.Now(),
	}
	
	assert.Equal(t, "request", usage.Type)
	assert.Equal(t, int64(100), usage.Count)
	assert.Equal(t, 15.50, usage.Amount)
	assert.Equal(t, "gpt-4o", usage.Model)
	assert.Equal(t, "openai", usage.Provider)
	assert.False(t, usage.Timestamp.IsZero())
}

func TestAccessResult_Structure(t *testing.T) {
	now := time.Now()
	result := &AccessResult{
		Allowed:   true,
		TenantID:  "tenant-123",
		UserID:    "user-456",
		Resource:  "models",
		Action:    "list",
		Reason:    "",
		CheckedAt: now,
		LimitResult: &LimitResult{
			Allowed:    true,
			Reason:     "",
			LimitType:  "requests_per_hour",
			Current:    50,
			Limit:      100,
			Percentage: 50.0,
		},
	}
	
	assert.True(t, result.Allowed)
	assert.Equal(t, "tenant-123", result.TenantID)
	assert.Equal(t, "user-456", result.UserID)
	assert.Equal(t, "models", result.Resource)
	assert.Equal(t, "list", result.Action)
	assert.Equal(t, now, result.CheckedAt)
	assert.NotNil(t, result.LimitResult)
	assert.True(t, result.LimitResult.Allowed)
}

func TestLimitResult_Structure(t *testing.T) {
	limit := &LimitResult{
		Allowed:    false,
		Reason:     "hourly request limit exceeded",
		LimitType:  "requests_per_hour",
		Current:    150,
		Limit:      100,
		Percentage: 150.0,
	}
	
	assert.False(t, limit.Allowed)
	assert.Equal(t, "hourly request limit exceeded", limit.Reason)
	assert.Equal(t, "requests_per_hour", limit.LimitType)
	assert.Equal(t, int64(150), limit.Current)
	assert.Equal(t, int64(100), limit.Limit)
	assert.Equal(t, 150.0, limit.Percentage)
}

func TestTenantFilter_Structure(t *testing.T) {
	status := TenantStatusActive
	tenantType := TenantTypeTeam
	parentID := "parent-123"
	now := time.Now()
	
	filter := &TenantFilter{
		Status:        &status,
		Type:          &tenantType,
		ParentID:      &parentID,
		CreatedAfter:  &now,
		CreatedBefore: &now,
	}
	
	assert.Equal(t, TenantStatusActive, *filter.Status)
	assert.Equal(t, TenantTypeTeam, *filter.Type)
	assert.Equal(t, "parent-123", *filter.ParentID)
	assert.Equal(t, now, *filter.CreatedAfter)
	assert.Equal(t, now, *filter.CreatedBefore)
}

// Helper functions

func createTestTenantManager(t *testing.T) *TenantManager {
	config := &TenantConfig{
		Enabled:            true,
		DefaultTenantType:  TenantTypePersonal,
		TrackUsage:         true,
		UsageResetInterval: time.Hour,
		CleanupInterval:    24 * time.Hour,
		StorageType:        "memory",
		EnforceLimits:      true,
		SoftLimitThreshold: 0.8,
	}
	
	logger := zaptest.NewLogger(t).Sugar()
	manager, err := NewTenantManager(config, nil, nil, logger)
	require.NoError(t, err)
	
	return manager
}

func managerTimePtr(t time.Time) *time.Time {
	return &t
}
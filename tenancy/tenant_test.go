package tenancy

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yanolja/ogem/security"
)

func TestTenant_NewTenant(t *testing.T) {
	now := time.Now()
	tenant := &Tenant{
		ID:          "tenant-123",
		Name:        "test-tenant",
		DisplayName: "Test Tenant",
		Description: "A test tenant for unit testing",
		Type:        TenantTypeTeam,
		Status:      TenantStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	assert.Equal(t, "tenant-123", tenant.ID)
	assert.Equal(t, "test-tenant", tenant.Name)
	assert.Equal(t, "Test Tenant", tenant.DisplayName)
	assert.Equal(t, TenantTypeTeam, tenant.Type)
	assert.Equal(t, TenantStatusActive, tenant.Status)
	assert.True(t, tenant.IsActive())
}

func TestTenant_IsActive(t *testing.T) {
	tests := []struct {
		name     string
		status   TenantStatus
		expected bool
	}{
		{
			name:     "active tenant",
			status:   TenantStatusActive,
			expected: true,
		},
		{
			name:     "suspended tenant",
			status:   TenantStatusSuspended,
			expected: false,
		},
		{
			name:     "deleted tenant",
			status:   TenantStatusDeleted,
			expected: false,
		},
		{
			name:     "pending tenant",
			status:   TenantStatusPending,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant := &Tenant{Status: tt.status}
			assert.Equal(t, tt.expected, tenant.IsActive())
		})
	}
}

func TestTenant_IsExpired(t *testing.T) {
	now := time.Now()
	pastTime := now.Add(-24 * time.Hour)
	futureTime := now.Add(24 * time.Hour)

	tests := []struct {
		name         string
		subscription *Subscription
		expected     bool
	}{
		{
			name:         "no subscription",
			subscription: nil,
			expected:     false,
		},
		{
			name: "subscription without end date",
			subscription: &Subscription{
				Status: "active",
			},
			expected: false,
		},
		{
			name: "subscription with future end date",
			subscription: &Subscription{
				Status:  "active",
				EndDate: &futureTime,
			},
			expected: false,
		},
		{
			name: "subscription with past end date",
			subscription: &Subscription{
				Status:  "expired",
				EndDate: &pastTime,
			},
			expected: true,
		},
		{
			name: "trial with future end date",
			subscription: &Subscription{
				Status:        "trial",
				TrialEndDate:  &futureTime,
			},
			expected: false,
		},
		{
			name: "trial with past end date",
			subscription: &Subscription{
				Status:        "trial",
				TrialEndDate:  &pastTime,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant := &Tenant{Subscription: tt.subscription}
			assert.Equal(t, tt.expected, tenant.IsExpired())
		})
	}
}

func TestTenant_IsTrialActive(t *testing.T) {
	now := time.Now()
	pastTime := now.Add(-24 * time.Hour)
	futureTime := now.Add(24 * time.Hour)
	farPastTime := now.Add(-48 * time.Hour)

	tests := []struct {
		name         string
		subscription *Subscription
		expected     bool
	}{
		{
			name:         "no subscription",
			subscription: nil,
			expected:     false,
		},
		{
			name: "subscription without trial dates",
			subscription: &Subscription{
				Status: "active",
			},
			expected: false,
		},
		{
			name: "active trial period",
			subscription: &Subscription{
				Status:         "trial",
				TrialStartDate: &pastTime,
				TrialEndDate:   &futureTime,
			},
			expected: true,
		},
		{
			name: "trial not yet started",
			subscription: &Subscription{
				Status:         "trial",
				TrialStartDate: &futureTime,
				TrialEndDate:   &futureTime,
			},
			expected: false,
		},
		{
			name: "trial already ended",
			subscription: &Subscription{
				Status:         "trial",
				TrialStartDate: &farPastTime,
				TrialEndDate:   &pastTime,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant := &Tenant{Subscription: tt.subscription}
			assert.Equal(t, tt.expected, tenant.IsTrialActive())
		})
	}
}

func TestTenant_GetEffectiveLimits(t *testing.T) {
	tests := []struct {
		name       string
		tenantType TenantType
		limits     *TenantLimits
	}{
		{
			name:       "tenant with no limits gets defaults",
			tenantType: TenantTypeTeam,
			limits:     nil,
		},
		{
			name:       "tenant with custom limits",
			tenantType: TenantTypeTeam,
			limits: &TenantLimits{
				RequestsPerHour: 500,
				TokensPerHour:   50000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant := &Tenant{
				Type:   tt.tenantType,
				Limits: tt.limits,
			}

			effectiveLimits := tenant.GetEffectiveLimits()
			assert.NotNil(t, effectiveLimits)

			if tt.limits == nil {
				defaultLimits := DefaultTenantLimits(tt.tenantType)
				assert.Equal(t, defaultLimits, effectiveLimits)
			} else {
				assert.Equal(t, tt.limits, effectiveLimits)
			}
		})
	}
}

func TestTenant_HasFeature(t *testing.T) {
	tests := []struct {
		name     string
		settings *TenantSettings
		feature  string
		expected bool
	}{
		{
			name:     "no settings",
			settings: nil,
			feature:  "advanced_analytics",
			expected: false,
		},
		{
			name: "no features map",
			settings: &TenantSettings{
				Features: nil,
			},
			feature:  "advanced_analytics",
			expected: false,
		},
		{
			name: "feature enabled",
			settings: &TenantSettings{
				Features: map[string]bool{
					"advanced_analytics": true,
					"custom_models":      false,
				},
			},
			feature:  "advanced_analytics",
			expected: true,
		},
		{
			name: "feature disabled",
			settings: &TenantSettings{
				Features: map[string]bool{
					"advanced_analytics": true,
					"custom_models":      false,
				},
			},
			feature:  "custom_models",
			expected: false,
		},
		{
			name: "feature not configured",
			settings: &TenantSettings{
				Features: map[string]bool{
					"advanced_analytics": true,
				},
			},
			feature:  "unknown_feature",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant := &Tenant{Settings: tt.settings}
			assert.Equal(t, tt.expected, tenant.HasFeature(tt.feature))
		})
	}
}

func TestTenant_IsModelAllowed(t *testing.T) {
	tests := []struct {
		name     string
		settings *TenantSettings
		model    string
		expected bool
	}{
		{
			name:     "no settings - allow all",
			settings: nil,
			model:    "gpt-4o",
			expected: true,
		},
		{
			name: "empty allowed models - allow all",
			settings: &TenantSettings{
				AllowedModels: []string{},
			},
			model:    "gpt-4o",
			expected: true,
		},
		{
			name: "model in allowed list",
			settings: &TenantSettings{
				AllowedModels: []string{"gpt-4o", "claude-3.5-sonnet-20241022"},
			},
			model:    "gpt-4o",
			expected: true,
		},
		{
			name: "model not in allowed list",
			settings: &TenantSettings{
				AllowedModels: []string{"gpt-3.5-turbo", "claude-3"},
			},
			model:    "gpt-4o",
			expected: false,
		},
		{
			name: "wildcard allows all",
			settings: &TenantSettings{
				AllowedModels: []string{"*"},
			},
			model:    "any-model",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant := &Tenant{Settings: tt.settings}
			assert.Equal(t, tt.expected, tenant.IsModelAllowed(tt.model))
		})
	}
}

func TestTenant_IsProviderAllowed(t *testing.T) {
	tests := []struct {
		name     string
		settings *TenantSettings
		provider string
		expected bool
	}{
		{
			name:     "no settings - allow all",
			settings: nil,
			provider: "openai",
			expected: true,
		},
		{
			name: "empty allowed providers - allow all",
			settings: &TenantSettings{
				AllowedProviders: []string{},
			},
			provider: "openai",
			expected: true,
		},
		{
			name: "provider in allowed list",
			settings: &TenantSettings{
				AllowedProviders: []string{"openai", "anthropic", "google"},
			},
			provider: "openai",
			expected: true,
		},
		{
			name: "provider not in allowed list",
			settings: &TenantSettings{
				AllowedProviders: []string{"anthropic", "google"},
			},
			provider: "openai",
			expected: false,
		},
		{
			name: "wildcard allows all",
			settings: &TenantSettings{
				AllowedProviders: []string{"*"},
			},
			provider: "any-provider",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant := &Tenant{Settings: tt.settings}
			assert.Equal(t, tt.expected, tenant.IsProviderAllowed(tt.provider))
		})
	}
}

func TestDefaultTenantLimits(t *testing.T) {
	tests := []struct {
		tenantType       TenantType
		expectedRequests int64
		expectedTokens   int64
		expectedUsers    int
	}{
		{
			tenantType:       TenantTypeEnterprise,
			expectedRequests: 10000,
			expectedTokens:   1000000,
			expectedUsers:    1000,
		},
		{
			tenantType:       TenantTypeTeam,
			expectedRequests: 1000,
			expectedTokens:   100000,
			expectedUsers:    50,
		},
		{
			tenantType:       TenantTypeTrial,
			expectedRequests: 100,
			expectedTokens:   10000,
			expectedUsers:    5,
		},
		{
			tenantType:       TenantTypePersonal,
			expectedRequests: 200,
			expectedTokens:   20000,
			expectedUsers:    1,
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.tenantType), func(t *testing.T) {
			limits := DefaultTenantLimits(tt.tenantType)
			assert.NotNil(t, limits)
			assert.Equal(t, tt.expectedRequests, limits.RequestsPerHour)
			assert.Equal(t, tt.expectedTokens, limits.TokensPerHour)
			assert.Equal(t, tt.expectedUsers, limits.MaxUsers)
		})
	}
}

func TestTenantContext_GetTenantFromContext(t *testing.T) {
	tenant := &Tenant{
		ID:   "tenant-123",
		Name: "test-tenant",
	}

	tenantCtx := &TenantContext{
		Tenant: tenant,
		UserID: "user-456",
		TeamID: "team-789",
	}

	tests := []struct {
		name     string
		ctx      context.Context
		expected *TenantContext
		found    bool
	}{
		{
			name:     "context without tenant",
			ctx:      context.Background(),
			expected: nil,
			found:    false,
		},
		{
			name:     "context with tenant",
			ctx:      WithTenantContext(context.Background(), tenantCtx),
			expected: tenantCtx,
			found:    true,
		},
		{
			name:     "context with invalid tenant value",
			ctx:      context.WithValue(context.Background(), "tenant", "invalid"),
			expected: nil,
			found:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, found := GetTenantFromContext(tt.ctx)
			assert.Equal(t, tt.found, found)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTenantSettings_Configuration(t *testing.T) {
	settings := &TenantSettings{
		AllowedModels:      []string{"gpt-4o"},
		DefaultModel:       "gpt-3.5-turbo",
		AllowedProviders:   []string{"openai", "anthropic"},
		PreferredProviders: []string{"openai"},
		Features: map[string]bool{
			"advanced_analytics": true,
			"custom_models":      false,
		},
		RoutingStrategy: "round_robin",
		RoutingWeights: map[string]float64{
			"openai":    0.7,
			"anthropic": 0.3,
		},
		EnableMetrics:      true,
		EnableAuditLogging: true,
		LogLevel:           "INFO",
		DataRegion:         "us-west-2",
		ComplianceProfiles: []string{"SOC2", "HIPAA"},
		CustomConfig: map[string]interface{}{
			"custom_timeout":     30,
			"enable_debug_logs": false,
		},
	}

	assert.Equal(t, []string{"gpt-4o"}, settings.AllowedModels)
	assert.Equal(t, "gpt-3.5-turbo", settings.DefaultModel)
	assert.Equal(t, []string{"openai", "anthropic"}, settings.AllowedProviders)
	assert.True(t, settings.Features["advanced_analytics"])
	assert.False(t, settings.Features["custom_models"])
	assert.Equal(t, "round_robin", settings.RoutingStrategy)
	assert.Equal(t, 0.7, settings.RoutingWeights["openai"])
	assert.True(t, settings.EnableMetrics)
	assert.Equal(t, "us-west-2", settings.DataRegion)
	assert.Contains(t, settings.ComplianceProfiles, "SOC2")
	assert.Equal(t, 30, settings.CustomConfig["custom_timeout"])
}

func TestTenantLimits_Structure(t *testing.T) {
	limits := &TenantLimits{
		RequestsPerHour:       1000,
		RequestsPerDay:        10000,
		RequestsPerMonth:      200000,
		TokensPerHour:         100000,
		TokensPerDay:          1000000,
		TokensPerMonth:        20000000,
		CostPerHour:           100.0,
		CostPerDay:            1000.0,
		CostPerMonth:          20000.0,
		MaxConcurrentRequests: 20,
		MaxUsers:              50,
		MaxTeams:              5,
		MaxProjects:           50,
		MaxStorageGB:          100,
		MaxFiles:              10000,
		ModelLimits: map[string]*ModelLimits{
			"gpt-4o": {
				RequestsPerHour:     100,
				TokensPerHour:       10000,
				CostPerHour:         50.0,
				MaxTokensPerRequest: 4000,
				Enabled:             true,
			},
		},
	}

	assert.Equal(t, int64(1000), limits.RequestsPerHour)
	assert.Equal(t, int64(10000), limits.RequestsPerDay)
	assert.Equal(t, float64(100.0), limits.CostPerHour)
	assert.Equal(t, 20, limits.MaxConcurrentRequests)
	assert.NotNil(t, limits.ModelLimits["gpt-4o"])
	assert.True(t, limits.ModelLimits["gpt-4o"].Enabled)
}

func TestTenantSecurity_Configuration(t *testing.T) {
	security := &TenantSecurity{
		RequireSSO:         true,
		AllowedAuthMethods: []string{"oauth2", "saml"},
		SessionTimeout:     8 * time.Hour,
		AllowedIPs:         []string{"192.168.1.0/24", "10.0.0.0/8"},
		BlockedIPs:         []string{"192.168.1.100"},
		AllowedCountries:   []string{"US", "CA", "GB"},
		PIIMaskingEnabled:  true,
		PIIMaskingStrategy: "aggressive",
		EncryptionRequired: true,
		ContentFiltering: &security.ContentFilteringConfig{
			Enabled:            true,
			BlockInappropriate: true,
			BlockHarmful:       true,
			BlockSpam:          false,
			CustomRules: []security.ContentFilterRule{
				{
					Name:    "company_secrets",
					Pattern: "confidential|secret|private",
					Action:  "block",
				},
			},
		},
		DataRetentionDays:  365,
		AuditRetentionDays: 2555, // 7 years
		RequireMFA:         true,
		PasswordPolicy: &PasswordPolicy{
			MinLength:        12,
			RequireUppercase: true,
			RequireLowercase: true,
			RequireNumbers:   true,
			RequireSymbols:   true,
			MaxAge:           90 * 24 * time.Hour,
			PreventReuse:     5,
		},
	}

	assert.True(t, security.RequireSSO)
	assert.Contains(t, security.AllowedAuthMethods, "oauth2")
	assert.Equal(t, 8*time.Hour, security.SessionTimeout)
	assert.Contains(t, security.AllowedIPs, "192.168.1.0/24")
	assert.True(t, security.PIIMaskingEnabled)
	assert.True(t, security.ContentFiltering.Enabled)
	assert.Equal(t, 365, security.DataRetentionDays)
	assert.True(t, security.RequireMFA)
	assert.Equal(t, 12, security.PasswordPolicy.MinLength)
}

func TestOrganization_Structure(t *testing.T) {
	org := &Organization{
		LegalName: "Test Company Inc.",
		BusinessAddress: &Address{
			Street1:    "123 Main St",
			Street2:    "Suite 100",
			City:       "San Francisco",
			State:      "CA",
			PostalCode: "94105",
			Country:    "US",
		},
		BillingAddress: &Address{
			Street1:    "456 Billing Ave",
			City:       "San Francisco",
			State:      "CA",
			PostalCode: "94105",
			Country:    "US",
		},
		PrimaryContact: &Contact{
			Name:  "John Doe",
			Email: "john.doe@testcompany.com",
			Phone: "+1-555-123-4567",
			Title: "CTO",
		},
		BillingContact: &Contact{
			Name:  "Jane Smith",
			Email: "billing@testcompany.com",
			Phone: "+1-555-123-4568",
			Title: "Finance Manager",
		},
		Industry:    "Technology",
		CompanySize: "51-200",
		TaxID:       "12-3456789",
		Website:     "https://testcompany.com",
	}

	assert.Equal(t, "Test Company Inc.", org.LegalName)
	assert.Equal(t, "123 Main St", org.BusinessAddress.Street1)
	assert.Equal(t, "San Francisco", org.BusinessAddress.City)
	assert.Equal(t, "John Doe", org.PrimaryContact.Name)
	assert.Equal(t, "john.doe@testcompany.com", org.PrimaryContact.Email)
	assert.Equal(t, "Technology", org.Industry)
	assert.Equal(t, "12-3456789", org.TaxID)
}

func TestSubscription_Structure(t *testing.T) {
	now := time.Now()
	trialStart := now.Add(-7 * 24 * time.Hour)
	trialEnd := now.Add(7 * 24 * time.Hour)
	renewalDate := now.Add(30 * 24 * time.Hour)

	subscription := &Subscription{
		PlanID:       "plan-enterprise",
		PlanName:     "Enterprise Plan",
		PlanType:     "subscription",
		BillingCycle: "monthly",
		Currency:     "USD",
		Amount:       299.99,
		Status:       "active",
		StartDate:    now,
		RenewalDate:  renewalDate,
		TrialStartDate: &trialStart,
		TrialEndDate:   &trialEnd,
		CurrentUsage: &UsageMetrics{
			RequestsThisHour:  50,
			RequestsThisDay:   500,
			RequestsThisMonth: 5000,
			TokensThisHour:    5000,
			TokensThisDay:     50000,
			TokensThisMonth:   500000,
			CostThisHour:      5.50,
			CostThisDay:       55.00,
			CostThisMonth:     550.00,
			StorageUsedGB:     25,
			FilesCount:        1250,
			ActiveUsers:       15,
			TeamsCount:        3,
			ProjectsCount:     12,
			LastUpdated:       now,
		},
		PaymentMethod: &PaymentMethod{
			Type:        "credit_card",
			LastFour:    "4567",
			ExpiryMonth: 12,
			ExpiryYear:  2025,
			Brand:       "Visa",
		},
	}

	assert.Equal(t, "plan-enterprise", subscription.PlanID)
	assert.Equal(t, "Enterprise Plan", subscription.PlanName)
	assert.Equal(t, "monthly", subscription.BillingCycle)
	assert.Equal(t, 299.99, subscription.Amount)
	assert.Equal(t, "active", subscription.Status)
	assert.NotNil(t, subscription.CurrentUsage)
	assert.Equal(t, int64(50), subscription.CurrentUsage.RequestsThisHour)
	assert.Equal(t, "credit_card", subscription.PaymentMethod.Type)
	assert.Equal(t, "4567", subscription.PaymentMethod.LastFour)
}

func TestUsageMetrics_Structure(t *testing.T) {
	now := time.Now()
	usage := &UsageMetrics{
		RequestsThisHour:  100,
		RequestsThisDay:   1000,
		RequestsThisMonth: 10000,
		TokensThisHour:    10000,
		TokensThisDay:     100000,
		TokensThisMonth:   1000000,
		CostThisHour:      10.00,
		CostThisDay:       100.00,
		CostThisMonth:     1000.00,
		StorageUsedGB:     50,
		FilesCount:        2500,
		ActiveUsers:       25,
		TeamsCount:        5,
		ProjectsCount:     20,
		LastUpdated:       now,
	}

	assert.Equal(t, int64(100), usage.RequestsThisHour)
	assert.Equal(t, int64(1000), usage.RequestsThisDay)
	assert.Equal(t, int64(10000), usage.RequestsThisMonth)
	assert.Equal(t, 10.00, usage.CostThisHour)
	assert.Equal(t, int64(50), usage.StorageUsedGB)
	assert.Equal(t, 25, usage.ActiveUsers)
}

func TestTenant_JSONSerialization(t *testing.T) {
	now := time.Now()
	tenant := &Tenant{
		ID:          "tenant-123",
		Name:        "test-tenant",
		DisplayName: "Test Tenant",
		Type:        TenantTypeTeam,
		Status:      TenantStatusActive,
		Settings: &TenantSettings{
			AllowedModels:    []string{"gpt-3.5-turbo"},
			DefaultModel:     "gpt-3.5-turbo",
			EnableMetrics:    true,
			LogLevel:         "INFO",
		},
		Limits: &TenantLimits{
			RequestsPerHour: 1000,
			TokensPerHour:   100000,
			MaxUsers:        50,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(tenant)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "tenant-123")
	assert.Contains(t, string(jsonData), "test-tenant")

	// Test JSON unmarshaling
	var unmarshaled Tenant
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, tenant.ID, unmarshaled.ID)
	assert.Equal(t, tenant.Name, unmarshaled.Name)
	assert.Equal(t, tenant.Type, unmarshaled.Type)
	assert.Equal(t, tenant.Status, unmarshaled.Status)
}

func TestModelLimits_Configuration(t *testing.T) {
	modelLimits := &ModelLimits{
		RequestsPerHour:     100,
		TokensPerHour:       10000,
		CostPerHour:         50.0,
		MaxTokensPerRequest: 4000,
		Enabled:             true,
	}

	assert.Equal(t, int64(100), modelLimits.RequestsPerHour)
	assert.Equal(t, int64(10000), modelLimits.TokensPerHour)
	assert.Equal(t, 50.0, modelLimits.CostPerHour)
	assert.Equal(t, 4000, modelLimits.MaxTokensPerRequest)
	assert.True(t, modelLimits.Enabled)
}

func TestPasswordPolicy_Configuration(t *testing.T) {
	policy := &PasswordPolicy{
		MinLength:        12,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireNumbers:   true,
		RequireSymbols:   true,
		MaxAge:           90 * 24 * time.Hour,
		PreventReuse:     5,
	}

	assert.Equal(t, 12, policy.MinLength)
	assert.True(t, policy.RequireUppercase)
	assert.True(t, policy.RequireLowercase)
	assert.True(t, policy.RequireNumbers)
	assert.True(t, policy.RequireSymbols)
	assert.Equal(t, 90*24*time.Hour, policy.MaxAge)
	assert.Equal(t, 5, policy.PreventReuse)
}

func TestTenantType_Constants(t *testing.T) {
	assert.Equal(t, TenantType("enterprise"), TenantTypeEnterprise)
	assert.Equal(t, TenantType("team"), TenantTypeTeam)
	assert.Equal(t, TenantType("personal"), TenantTypePersonal)
	assert.Equal(t, TenantType("trial"), TenantTypeTrial)
}

func TestTenantStatus_Constants(t *testing.T) {
	assert.Equal(t, TenantStatus("active"), TenantStatusActive)
	assert.Equal(t, TenantStatus("suspended"), TenantStatusSuspended)
	assert.Equal(t, TenantStatus("deleted"), TenantStatusDeleted)
	assert.Equal(t, TenantStatus("pending"), TenantStatusPending)
}

func TestTenant_HierarchicalRelationships(t *testing.T) {
	parent := &Tenant{
		ID:       "parent-tenant",
		Name:     "parent",
		Type:     TenantTypeEnterprise,
		Children: []string{"child1", "child2", "child3"},
	}

	child := &Tenant{
		ID:       "child1",
		Name:     "child1",
		Type:     TenantTypeTeam,
		ParentID: "parent-tenant",
	}

	assert.Equal(t, "parent-tenant", parent.ID)
	assert.Equal(t, 3, len(parent.Children))
	assert.Contains(t, parent.Children, "child1")
	assert.Equal(t, "parent-tenant", child.ParentID)
	assert.Empty(t, child.Children)
}

func TestTenant_MetadataHandling(t *testing.T) {
	tenant := &Tenant{
		ID:   "tenant-123",
		Name: "test-tenant",
		Metadata: map[string]string{
			"environment":     "production",
			"region":          "us-west-2",
			"contact_email":   "admin@example.com",
			"cost_center":     "engineering",
			"compliance_tier": "high",
		},
	}

	assert.Equal(t, "production", tenant.Metadata["environment"])
	assert.Equal(t, "us-west-2", tenant.Metadata["region"])
	assert.Equal(t, "admin@example.com", tenant.Metadata["contact_email"])
	assert.Equal(t, "engineering", tenant.Metadata["cost_center"])
	assert.Equal(t, "high", tenant.Metadata["compliance_tier"])
}

func TestTenant_EdgeCases(t *testing.T) {
	tenant := &Tenant{}

	// Test methods on empty tenant
	assert.False(t, tenant.IsActive())
	assert.False(t, tenant.IsExpired())
	assert.False(t, tenant.IsTrialActive())
	assert.False(t, tenant.HasFeature("any_feature"))
	assert.True(t, tenant.IsModelAllowed("any_model"))     // Default allow all
	assert.True(t, tenant.IsProviderAllowed("any_provider")) // Default allow all

	limits := tenant.GetEffectiveLimits()
	assert.NotNil(t, limits) // Should return default limits
}
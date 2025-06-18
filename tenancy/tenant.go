package tenancy

import (
	"context"
	"time"

	"github.com/yanolja/ogem/security"
)

// TenantStatus represents the current status of a tenant
type TenantStatus string

const (
	TenantStatusActive    TenantStatus = "active"
	TenantStatusSuspended TenantStatus = "suspended"
	TenantStatusDeleted   TenantStatus = "deleted"
	TenantStatusPending   TenantStatus = "pending"
)

// TenantType represents different types of tenancy
type TenantType string

const (
	TenantTypeEnterprise TenantType = "enterprise"
	TenantTypeTeam       TenantType = "team"
	TenantTypePersonal   TenantType = "personal"
	TenantTypeTrial      TenantType = "trial"
)

// Tenant represents a multi-tenant organization
type Tenant struct {
	// Basic tenant information
	ID          string            `json:"id" yaml:"id"`
	Name        string            `json:"name" yaml:"name"`
	DisplayName string            `json:"display_name" yaml:"display_name"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Type        TenantType        `json:"type" yaml:"type"`
	Status      TenantStatus      `json:"status" yaml:"status"`
	
	// Tenant configuration
	Settings    *TenantSettings   `json:"settings" yaml:"settings"`
	Limits      *TenantLimits     `json:"limits" yaml:"limits"`
	Security    *TenantSecurity   `json:"security" yaml:"security"`
	
	// Organizational information
	Organization *Organization    `json:"organization,omitempty" yaml:"organization,omitempty"`
	
	// Billing and subscription
	Subscription *Subscription    `json:"subscription,omitempty" yaml:"subscription,omitempty"`
	
	// Metadata and timestamps
	Metadata    map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at" yaml:"updated_at"`
	DeletedAt   *time.Time        `json:"deleted_at,omitempty" yaml:"deleted_at,omitempty"`
	
	// Parent-child relationships for hierarchical tenancy
	ParentID    string            `json:"parent_id,omitempty" yaml:"parent_id,omitempty"`
	Children    []string          `json:"children,omitempty" yaml:"children,omitempty"`
}

// TenantSettings configures tenant-specific behavior
type TenantSettings struct {
	// API configuration
	AllowedModels       []string          `json:"allowed_models" yaml:"allowed_models"`
	DefaultModel        string            `json:"default_model" yaml:"default_model"`
	AllowedProviders    []string          `json:"allowed_providers" yaml:"allowed_providers"`
	PreferredProviders  []string          `json:"preferred_providers" yaml:"preferred_providers"`
	
	// Feature flags
	Features            map[string]bool   `json:"features" yaml:"features"`
	
	// Custom routing configuration
	RoutingStrategy     string            `json:"routing_strategy" yaml:"routing_strategy"`
	RoutingWeights      map[string]float64 `json:"routing_weights,omitempty" yaml:"routing_weights,omitempty"`
	
	// Monitoring and logging
	EnableMetrics       bool              `json:"enable_metrics" yaml:"enable_metrics"`
	EnableAuditLogging  bool              `json:"enable_audit_logging" yaml:"enable_audit_logging"`
	LogLevel           string            `json:"log_level" yaml:"log_level"`
	
	// Data residency and compliance
	DataRegion         string            `json:"data_region" yaml:"data_region"`
	ComplianceProfiles []string          `json:"compliance_profiles,omitempty" yaml:"compliance_profiles,omitempty"`
	
	// Custom configuration
	CustomConfig       map[string]interface{} `json:"custom_config,omitempty" yaml:"custom_config,omitempty"`
}

// TenantLimits defines resource limits for a tenant
type TenantLimits struct {
	// Request limits
	RequestsPerHour     int64   `json:"requests_per_hour" yaml:"requests_per_hour"`
	RequestsPerDay      int64   `json:"requests_per_day" yaml:"requests_per_day"`
	RequestsPerMonth    int64   `json:"requests_per_month" yaml:"requests_per_month"`
	
	// Token limits
	TokensPerHour       int64   `json:"tokens_per_hour" yaml:"tokens_per_hour"`
	TokensPerDay        int64   `json:"tokens_per_day" yaml:"tokens_per_day"`
	TokensPerMonth      int64   `json:"tokens_per_month" yaml:"tokens_per_month"`
	
	// Cost limits
	CostPerHour         float64 `json:"cost_per_hour" yaml:"cost_per_hour"`
	CostPerDay          float64 `json:"cost_per_day" yaml:"cost_per_day"`
	CostPerMonth        float64 `json:"cost_per_month" yaml:"cost_per_month"`
	
	// Concurrent limits
	MaxConcurrentRequests int   `json:"max_concurrent_requests" yaml:"max_concurrent_requests"`
	MaxUsers             int    `json:"max_users" yaml:"max_users"`
	MaxTeams             int    `json:"max_teams" yaml:"max_teams"`
	MaxProjects          int    `json:"max_projects" yaml:"max_projects"`
	
	// Storage limits
	MaxStorageGB         int64  `json:"max_storage_gb" yaml:"max_storage_gb"`
	MaxFiles             int64  `json:"max_files" yaml:"max_files"`
	
	// Model-specific limits
	ModelLimits         map[string]*ModelLimits `json:"model_limits,omitempty" yaml:"model_limits,omitempty"`
}

// ModelLimits defines limits for specific models
type ModelLimits struct {
	RequestsPerHour     int64   `json:"requests_per_hour" yaml:"requests_per_hour"`
	TokensPerHour       int64   `json:"tokens_per_hour" yaml:"tokens_per_hour"`
	CostPerHour         float64 `json:"cost_per_hour" yaml:"cost_per_hour"`
	MaxTokensPerRequest int     `json:"max_tokens_per_request" yaml:"max_tokens_per_request"`
	Enabled             bool    `json:"enabled" yaml:"enabled"`
}

// TenantSecurity configures tenant-specific security settings
type TenantSecurity struct {
	// Authentication requirements
	RequireSSO          bool              `json:"require_sso" yaml:"require_sso"`
	AllowedAuthMethods  []string          `json:"allowed_auth_methods" yaml:"allowed_auth_methods"`
	SessionTimeout      time.Duration     `json:"session_timeout" yaml:"session_timeout"`
	
	// Network security
	AllowedIPs          []string          `json:"allowed_ips,omitempty" yaml:"allowed_ips,omitempty"`
	BlockedIPs          []string          `json:"blocked_ips,omitempty" yaml:"blocked_ips,omitempty"`
	AllowedCountries    []string          `json:"allowed_countries,omitempty" yaml:"allowed_countries,omitempty"`
	
	// Data protection
	PIIMaskingEnabled   bool              `json:"pii_masking_enabled" yaml:"pii_masking_enabled"`
	PIIMaskingStrategy  string            `json:"pii_masking_strategy" yaml:"pii_masking_strategy"`
	EncryptionRequired  bool              `json:"encryption_required" yaml:"encryption_required"`
	
	// Content filtering
	ContentFiltering    *security.ContentFilteringConfig `json:"content_filtering,omitempty" yaml:"content_filtering,omitempty"`
	
	// Compliance requirements
	DataRetentionDays   int               `json:"data_retention_days" yaml:"data_retention_days"`
	AuditRetentionDays  int               `json:"audit_retention_days" yaml:"audit_retention_days"`
	
	// Advanced security features
	RequireMFA          bool              `json:"require_mfa" yaml:"require_mfa"`
	PasswordPolicy      *PasswordPolicy   `json:"password_policy,omitempty" yaml:"password_policy,omitempty"`
}

// PasswordPolicy defines password requirements
type PasswordPolicy struct {
	MinLength        int  `json:"min_length" yaml:"min_length"`
	RequireUppercase bool `json:"require_uppercase" yaml:"require_uppercase"`
	RequireLowercase bool `json:"require_lowercase" yaml:"require_lowercase"`
	RequireNumbers   bool `json:"require_numbers" yaml:"require_numbers"`
	RequireSymbols   bool `json:"require_symbols" yaml:"require_symbols"`
	MaxAge           time.Duration `json:"max_age" yaml:"max_age"`
	PreventReuse     int  `json:"prevent_reuse" yaml:"prevent_reuse"`
}

// Organization represents the organizational structure of a tenant
type Organization struct {
	// Basic information
	LegalName       string            `json:"legal_name" yaml:"legal_name"`
	BusinessAddress *Address          `json:"business_address,omitempty" yaml:"business_address,omitempty"`
	BillingAddress  *Address          `json:"billing_address,omitempty" yaml:"billing_address,omitempty"`
	
	// Contact information
	PrimaryContact  *Contact          `json:"primary_contact" yaml:"primary_contact"`
	BillingContact  *Contact          `json:"billing_contact,omitempty" yaml:"billing_contact,omitempty"`
	TechnicalContact *Contact         `json:"technical_contact,omitempty" yaml:"technical_contact,omitempty"`
	
	// Business details
	Industry        string            `json:"industry,omitempty" yaml:"industry,omitempty"`
	CompanySize     string            `json:"company_size,omitempty" yaml:"company_size,omitempty"`
	TaxID           string            `json:"tax_id,omitempty" yaml:"tax_id,omitempty"`
	Website         string            `json:"website,omitempty" yaml:"website,omitempty"`
}

// Address represents a physical address
type Address struct {
	Street1    string `json:"street1" yaml:"street1"`
	Street2    string `json:"street2,omitempty" yaml:"street2,omitempty"`
	City       string `json:"city" yaml:"city"`
	State      string `json:"state,omitempty" yaml:"state,omitempty"`
	PostalCode string `json:"postal_code" yaml:"postal_code"`
	Country    string `json:"country" yaml:"country"`
}

// Contact represents contact information
type Contact struct {
	Name  string `json:"name" yaml:"name"`
	Email string `json:"email" yaml:"email"`
	Phone string `json:"phone,omitempty" yaml:"phone,omitempty"`
	Title string `json:"title,omitempty" yaml:"title,omitempty"`
}

// Subscription represents billing and subscription information
type Subscription struct {
	// Plan details
	PlanID          string            `json:"plan_id" yaml:"plan_id"`
	PlanName        string            `json:"plan_name" yaml:"plan_name"`
	PlanType        string            `json:"plan_type" yaml:"plan_type"`
	
	// Billing
	BillingCycle    string            `json:"billing_cycle" yaml:"billing_cycle"` // "monthly", "yearly"
	Currency        string            `json:"currency" yaml:"currency"`
	Amount          float64           `json:"amount" yaml:"amount"`
	
	// Subscription status
	Status          string            `json:"status" yaml:"status"`
	StartDate       time.Time         `json:"start_date" yaml:"start_date"`
	EndDate         *time.Time        `json:"end_date,omitempty" yaml:"end_date,omitempty"`
	RenewalDate     time.Time         `json:"renewal_date" yaml:"renewal_date"`
	
	// Trial information
	TrialStartDate  *time.Time        `json:"trial_start_date,omitempty" yaml:"trial_start_date,omitempty"`
	TrialEndDate    *time.Time        `json:"trial_end_date,omitempty" yaml:"trial_end_date,omitempty"`
	
	// Usage tracking
	CurrentUsage    *UsageMetrics     `json:"current_usage,omitempty" yaml:"current_usage,omitempty"`
	
	// Payment information
	PaymentMethod   *PaymentMethod    `json:"payment_method,omitempty" yaml:"payment_method,omitempty"`
}

// UsageMetrics tracks current usage against limits
type UsageMetrics struct {
	RequestsThisHour   int64   `json:"requests_this_hour"`
	RequestsThisDay    int64   `json:"requests_this_day"`
	RequestsThisMonth  int64   `json:"requests_this_month"`
	TokensThisHour     int64   `json:"tokens_this_hour"`
	TokensThisDay      int64   `json:"tokens_this_day"`
	TokensThisMonth    int64   `json:"tokens_this_month"`
	CostThisHour       float64 `json:"cost_this_hour"`
	CostThisDay        float64 `json:"cost_this_day"`
	CostThisMonth      float64 `json:"cost_this_month"`
	StorageUsedGB      int64   `json:"storage_used_gb"`
	FilesCount         int64   `json:"files_count"`
	ActiveUsers        int     `json:"active_users"`
	TeamsCount         int     `json:"teams_count"`
	ProjectsCount      int     `json:"projects_count"`
	LastUpdated        time.Time `json:"last_updated"`
}

// PaymentMethod represents payment information
type PaymentMethod struct {
	Type        string `json:"type" yaml:"type"` // "credit_card", "bank_transfer", "invoice"
	LastFour    string `json:"last_four,omitempty" yaml:"last_four,omitempty"`
	ExpiryMonth int    `json:"expiry_month,omitempty" yaml:"expiry_month,omitempty"`
	ExpiryYear  int    `json:"expiry_year,omitempty" yaml:"expiry_year,omitempty"`
	Brand       string `json:"brand,omitempty" yaml:"brand,omitempty"`
}

// TenantContext provides tenant information in request context
type TenantContext struct {
	Tenant   *Tenant `json:"tenant"`
	UserID   string  `json:"user_id"`
	TeamID   string  `json:"team_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

// GetTenantFromContext extracts tenant information from context
func GetTenantFromContext(ctx context.Context) (*TenantContext, bool) {
	if tenantCtx := ctx.Value("tenant"); tenantCtx != nil {
		if tc, ok := tenantCtx.(*TenantContext); ok {
			return tc, true
		}
	}
	return nil, false
}

// WithTenantContext adds tenant information to context
func WithTenantContext(ctx context.Context, tenantCtx *TenantContext) context.Context {
	return context.WithValue(ctx, "tenant", tenantCtx)
}

// IsActive returns true if the tenant is currently active
func (t *Tenant) IsActive() bool {
	return t.Status == TenantStatusActive
}

// IsExpired returns true if the tenant's subscription has expired
func (t *Tenant) IsExpired() bool {
	if t.Subscription == nil {
		return false
	}
	
	if t.Subscription.EndDate != nil && time.Now().After(*t.Subscription.EndDate) {
		return true
	}
	
	if t.Subscription.TrialEndDate != nil && time.Now().After(*t.Subscription.TrialEndDate) {
		return true
	}
	
	return false
}

// IsTrialActive returns true if the tenant is in an active trial period
func (t *Tenant) IsTrialActive() bool {
	if t.Subscription == nil || t.Subscription.TrialStartDate == nil || t.Subscription.TrialEndDate == nil {
		return false
	}
	
	now := time.Now()
	return now.After(*t.Subscription.TrialStartDate) && now.Before(*t.Subscription.TrialEndDate)
}

// GetEffectiveLimits returns the effective limits considering subscription and overrides
func (t *Tenant) GetEffectiveLimits() *TenantLimits {
	if t.Limits == nil {
		return DefaultTenantLimits(t.Type)
	}
	return t.Limits
}

// HasFeature checks if a feature is enabled for the tenant
func (t *Tenant) HasFeature(feature string) bool {
	if t.Settings == nil || t.Settings.Features == nil {
		return false
	}
	enabled, exists := t.Settings.Features[feature]
	return exists && enabled
}

// IsModelAllowed checks if a model is allowed for the tenant
func (t *Tenant) IsModelAllowed(model string) bool {
	if t.Settings == nil || len(t.Settings.AllowedModels) == 0 {
		return true // Allow all if not restricted
	}
	
	for _, allowedModel := range t.Settings.AllowedModels {
		if allowedModel == model || allowedModel == "*" {
			return true
		}
	}
	return false
}

// IsProviderAllowed checks if a provider is allowed for the tenant
func (t *Tenant) IsProviderAllowed(provider string) bool {
	if t.Settings == nil || len(t.Settings.AllowedProviders) == 0 {
		return true // Allow all if not restricted
	}
	
	for _, allowedProvider := range t.Settings.AllowedProviders {
		if allowedProvider == provider || allowedProvider == "*" {
			return true
		}
	}
	return false
}

// DefaultTenantLimits returns default limits based on tenant type
func DefaultTenantLimits(tenantType TenantType) *TenantLimits {
	switch tenantType {
	case TenantTypeEnterprise:
		return &TenantLimits{
			RequestsPerHour:       10000,
			RequestsPerDay:        100000,
			RequestsPerMonth:      2000000,
			TokensPerHour:         1000000,
			TokensPerDay:          10000000,
			TokensPerMonth:        200000000,
			CostPerHour:           1000.0,
			CostPerDay:            10000.0,
			CostPerMonth:          200000.0,
			MaxConcurrentRequests: 100,
			MaxUsers:              1000,
			MaxTeams:              50,
			MaxProjects:           500,
			MaxStorageGB:          1000,
			MaxFiles:              100000,
		}
	case TenantTypeTeam:
		return &TenantLimits{
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
		}
	case TenantTypeTrial:
		return &TenantLimits{
			RequestsPerHour:       100,
			RequestsPerDay:        500,
			RequestsPerMonth:      5000,
			TokensPerHour:         10000,
			TokensPerDay:          50000,
			TokensPerMonth:        500000,
			CostPerHour:           10.0,
			CostPerDay:            50.0,
			CostPerMonth:          500.0,
			MaxConcurrentRequests: 5,
			MaxUsers:              5,
			MaxTeams:              1,
			MaxProjects:           5,
			MaxStorageGB:          10,
			MaxFiles:              1000,
		}
	default: // TenantTypePersonal
		return &TenantLimits{
			RequestsPerHour:       200,
			RequestsPerDay:        2000,
			RequestsPerMonth:      20000,
			TokensPerHour:         20000,
			TokensPerDay:          200000,
			TokensPerMonth:        2000000,
			CostPerHour:           20.0,
			CostPerDay:            200.0,
			CostPerMonth:          2000.0,
			MaxConcurrentRequests: 10,
			MaxUsers:              1,
			MaxTeams:              0,
			MaxProjects:           10,
			MaxStorageGB:          20,
			MaxFiles:              2000,
		}
	}
}
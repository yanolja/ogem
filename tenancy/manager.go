package tenancy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/yanolja/ogem/monitoring"
	"github.com/yanolja/ogem/security"
)

// TenantManager provides multi-tenancy management capabilities
type TenantManager struct {
	config          *TenantConfig
	tenants         map[string]*Tenant
	usageTracking   map[string]*UsageMetrics
	securityManager *security.SecurityManager
	monitor         *monitoring.MonitoringManager
	logger          *zap.SugaredLogger
	mutex           sync.RWMutex
	
	// Background services
	usageResetTicker *time.Ticker
	cleanupTicker    *time.Ticker
	stopChan         chan struct{}
}

// TenantConfig configures the tenant manager
type TenantConfig struct {
	// Enable multi-tenancy
	Enabled bool `yaml:"enabled"`
	
	// Default tenant configuration
	DefaultTenantType TenantType `yaml:"default_tenant_type"`
	
	// Usage tracking configuration
	TrackUsage          bool          `yaml:"track_usage"`
	UsageResetInterval  time.Duration `yaml:"usage_reset_interval"`
	UsageRetentionDays  int           `yaml:"usage_retention_days"`
	
	// Auto-provisioning
	AllowAutoProvisioning bool   `yaml:"allow_auto_provisioning"`
	AutoProvisioningType   TenantType `yaml:"auto_provisioning_type"`
	
	// Hierarchical tenancy
	EnableHierarchy bool `yaml:"enable_hierarchy"`
	MaxDepth        int  `yaml:"max_depth"`
	
	// Tenant isolation
	StrictIsolation     bool `yaml:"strict_isolation"`
	SharedResources     bool `yaml:"shared_resources"`
	
	// Storage configuration
	StorageType         string `yaml:"storage_type"` // "memory", "redis", "database"
	DatabaseURL         string `yaml:"database_url,omitempty"`
	RedisURL            string `yaml:"redis_url,omitempty"`
	
	// Background service intervals
	CleanupInterval     time.Duration `yaml:"cleanup_interval"`
	
	// Limits enforcement
	EnforceLimits       bool `yaml:"enforce_limits"`
	SoftLimitThreshold  float64 `yaml:"soft_limit_threshold"` // 0.8 = 80%
	
	// Billing integration
	BillingEnabled      bool   `yaml:"billing_enabled"`
	BillingProvider     string `yaml:"billing_provider,omitempty"`
	BillingWebhookURL   string `yaml:"billing_webhook_url,omitempty"`
}

// NewTenantManager creates a new tenant manager
func NewTenantManager(config *TenantConfig, securityManager *security.SecurityManager, monitor *monitoring.MonitoringManager, logger *zap.SugaredLogger) (*TenantManager, error) {
	if config == nil {
		config = DefaultTenantConfig()
	}
	
	manager := &TenantManager{
		config:          config,
		tenants:         make(map[string]*Tenant),
		usageTracking:   make(map[string]*UsageMetrics),
		securityManager: securityManager,
		monitor:         monitor,
		logger:          logger,
		stopChan:        make(chan struct{}),
	}
	
	// Initialize storage backend
	if err := manager.initializeStorage(); err != nil {
		return nil, fmt.Errorf("failed to initialize tenant storage: %v", err)
	}
	
	// Start background services
	if config.Enabled {
		manager.startBackgroundServices()
	}
	
	return manager, nil
}

// DefaultTenantConfig returns default tenant configuration
func DefaultTenantConfig() *TenantConfig {
	return &TenantConfig{
		Enabled:               true,
		DefaultTenantType:     TenantTypePersonal,
		TrackUsage:           true,
		UsageResetInterval:   time.Hour,
		UsageRetentionDays:   90,
		AllowAutoProvisioning: true,
		AutoProvisioningType:  TenantTypePersonal,
		EnableHierarchy:      true,
		MaxDepth:             3,
		StrictIsolation:      true,
		SharedResources:      false,
		StorageType:          "memory",
		CleanupInterval:      24 * time.Hour,
		EnforceLimits:        true,
		SoftLimitThreshold:   0.8,
		BillingEnabled:       false,
	}
}

// CreateTenant creates a new tenant
func (tm *TenantManager) CreateTenant(ctx context.Context, tenant *Tenant) error {
	if !tm.config.Enabled {
		return fmt.Errorf("multi-tenancy is disabled")
	}
	
	tm.mutex.Lock()
	defer tm.mutex.Unlock()
	
	// Validate tenant
	if err := tm.validateTenant(tenant); err != nil {
		return fmt.Errorf("tenant validation failed: %v", err)
	}
	
	// Check if tenant already exists
	if _, exists := tm.tenants[tenant.ID]; exists {
		return fmt.Errorf("tenant %s already exists", tenant.ID)
	}
	
	// Set creation metadata
	now := time.Now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	
	// Set defaults if not provided
	if tenant.Settings == nil {
		tenant.Settings = tm.getDefaultTenantSettings(tenant.Type)
	}
	if tenant.Limits == nil {
		tenant.Limits = DefaultTenantLimits(tenant.Type)
	}
	if tenant.Security == nil {
		tenant.Security = tm.getDefaultTenantSecurity()
	}
	
	// Store tenant
	tm.tenants[tenant.ID] = tenant
	
	// Initialize usage tracking
	if tm.config.TrackUsage {
		tm.usageTracking[tenant.ID] = &UsageMetrics{
			LastUpdated: now,
		}
	}
	
	// Persist to storage
	if err := tm.persistTenant(tenant); err != nil {
		delete(tm.tenants, tenant.ID)
		delete(tm.usageTracking, tenant.ID)
		return fmt.Errorf("failed to persist tenant: %v", err)
	}
	
	// Log tenant creation
	tm.logger.Infow("Tenant created",
		"tenant_id", tenant.ID,
		"tenant_name", tenant.Name,
		"tenant_type", tenant.Type,
		"status", tenant.Status)
	
	// Record metrics
	if tm.monitor != nil {
		tm.recordTenantMetric("tenant_created", tenant)
	}
	
	return nil
}

// GetTenant retrieves a tenant by ID
func (tm *TenantManager) GetTenant(ctx context.Context, tenantID string) (*Tenant, error) {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()
	
	tenant, exists := tm.tenants[tenantID]
	if !exists {
		return nil, fmt.Errorf("tenant %s not found", tenantID)
	}
	
	// Return a copy to prevent modification
	tenantCopy := *tenant
	return &tenantCopy, nil
}

// UpdateTenant updates an existing tenant
func (tm *TenantManager) UpdateTenant(ctx context.Context, tenant *Tenant) error {
	if !tm.config.Enabled {
		return fmt.Errorf("multi-tenancy is disabled")
	}
	
	tm.mutex.Lock()
	defer tm.mutex.Unlock()
	
	// Check if tenant exists
	existing, exists := tm.tenants[tenant.ID]
	if !exists {
		return fmt.Errorf("tenant %s not found", tenant.ID)
	}
	
	// Validate updated tenant
	if err := tm.validateTenant(tenant); err != nil {
		return fmt.Errorf("tenant validation failed: %v", err)
	}
	
	// Preserve creation time
	tenant.CreatedAt = existing.CreatedAt
	tenant.UpdatedAt = time.Now()
	
	// Update tenant
	tm.tenants[tenant.ID] = tenant
	
	// Persist to storage
	if err := tm.persistTenant(tenant); err != nil {
		return fmt.Errorf("failed to persist tenant: %v", err)
	}
	
	tm.logger.Infow("Tenant updated",
		"tenant_id", tenant.ID,
		"tenant_name", tenant.Name)
	
	if tm.monitor != nil {
		tm.recordTenantMetric("tenant_updated", tenant)
	}
	
	return nil
}

// DeleteTenant soft deletes a tenant
func (tm *TenantManager) DeleteTenant(ctx context.Context, tenantID string) error {
	if !tm.config.Enabled {
		return fmt.Errorf("multi-tenancy is disabled")
	}
	
	tm.mutex.Lock()
	defer tm.mutex.Unlock()
	
	tenant, exists := tm.tenants[tenantID]
	if !exists {
		return fmt.Errorf("tenant %s not found", tenantID)
	}
	
	// Soft delete
	now := time.Now()
	tenant.Status = TenantStatusDeleted
	tenant.DeletedAt = &now
	tenant.UpdatedAt = now
	
	// Persist to storage
	if err := tm.persistTenant(tenant); err != nil {
		return fmt.Errorf("failed to persist tenant deletion: %v", err)
	}
	
	tm.logger.Infow("Tenant deleted",
		"tenant_id", tenantID,
		"tenant_name", tenant.Name)
	
	if tm.monitor != nil {
		tm.recordTenantMetric("tenant_deleted", tenant)
	}
	
	return nil
}

// ListTenants returns all tenants (optionally filtered)
func (tm *TenantManager) ListTenants(ctx context.Context, filter *TenantFilter) ([]*Tenant, error) {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()
	
	var tenants []*Tenant
	
	for _, tenant := range tm.tenants {
		if filter == nil || tm.matchesFilter(tenant, filter) {
			tenantCopy := *tenant
			tenants = append(tenants, &tenantCopy)
		}
	}
	
	return tenants, nil
}

// TenantFilter defines filtering criteria for listing tenants
type TenantFilter struct {
	Status      *TenantStatus `json:"status,omitempty"`
	Type        *TenantType   `json:"type,omitempty"`
	ParentID    *string       `json:"parent_id,omitempty"`
	CreatedAfter *time.Time   `json:"created_after,omitempty"`
	CreatedBefore *time.Time  `json:"created_before,omitempty"`
}

// CheckAccess validates if a request is authorized for a tenant
func (tm *TenantManager) CheckAccess(ctx context.Context, tenantID, userID string, resource, action string) (*AccessResult, error) {
	tenant, err := tm.GetTenant(ctx, tenantID)
	if err != nil {
		return &AccessResult{Allowed: false, Reason: "tenant not found"}, err
	}
	
	result := &AccessResult{
		Allowed:   true,
		TenantID:  tenantID,
		UserID:    userID,
		Resource:  resource,
		Action:    action,
		CheckedAt: time.Now(),
	}
	
	// Check tenant status
	if !tenant.IsActive() {
		result.Allowed = false
		result.Reason = fmt.Sprintf("tenant status is %s", tenant.Status)
		return result, nil
	}
	
	// Check subscription expiry
	if tenant.IsExpired() {
		result.Allowed = false
		result.Reason = "tenant subscription expired"
		return result, nil
	}
	
	// Check usage limits
	if tm.config.EnforceLimits {
		limitResult, err := tm.checkUsageLimits(tenant, resource)
		if err != nil {
			tm.logger.Warnw("Failed to check usage limits", "tenant_id", tenantID, "error", err)
		}
		if limitResult != nil && !limitResult.Allowed {
			result.Allowed = false
			result.Reason = limitResult.Reason
			result.LimitResult = limitResult
			return result, nil
		}
	}
	
	// Additional security checks can be added here
	
	return result, nil
}

// AccessResult represents the result of an access check
type AccessResult struct {
	Allowed     bool         `json:"allowed"`
	TenantID    string       `json:"tenant_id"`
	UserID      string       `json:"user_id"`
	Resource    string       `json:"resource"`
	Action      string       `json:"action"`
	Reason      string       `json:"reason,omitempty"`
	CheckedAt   time.Time    `json:"checked_at"`
	LimitResult *LimitResult `json:"limit_result,omitempty"`
}

// LimitResult represents the result of a limit check
type LimitResult struct {
	Allowed    bool    `json:"allowed"`
	Reason     string  `json:"reason"`
	LimitType  string  `json:"limit_type"`
	Current    int64   `json:"current"`
	Limit      int64   `json:"limit"`
	Percentage float64 `json:"percentage"`
}

// RecordUsage records usage for a tenant
func (tm *TenantManager) RecordUsage(ctx context.Context, tenantID string, usage *UsageRecord) error {
	if !tm.config.TrackUsage {
		return nil
	}
	
	tm.mutex.Lock()
	defer tm.mutex.Unlock()
	
	metrics, exists := tm.usageTracking[tenantID]
	if !exists {
		metrics = &UsageMetrics{LastUpdated: time.Now()}
		tm.usageTracking[tenantID] = metrics
	}
	
	// Update metrics based on usage type
	switch usage.Type {
	case "request":
		metrics.RequestsThisHour += usage.Count
		metrics.RequestsThisDay += usage.Count
		metrics.RequestsThisMonth += usage.Count
	case "token":
		metrics.TokensThisHour += usage.Count
		metrics.TokensThisDay += usage.Count
		metrics.TokensThisMonth += usage.Count
	case "cost":
		metrics.CostThisHour += usage.Amount
		metrics.CostThisDay += usage.Amount
		metrics.CostThisMonth += usage.Amount
	}
	
	metrics.LastUpdated = time.Now()
	
	// Record metrics for monitoring
	if tm.monitor != nil {
		tm.recordUsageMetric(tenantID, usage)
	}
	
	return nil
}

// UsageRecord represents a usage entry
type UsageRecord struct {
	Type      string    `json:"type"` // "request", "token", "cost"
	Count     int64     `json:"count"`
	Amount    float64   `json:"amount"`
	Model     string    `json:"model,omitempty"`
	Provider  string    `json:"provider,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// GetUsageMetrics returns current usage metrics for a tenant
func (tm *TenantManager) GetUsageMetrics(ctx context.Context, tenantID string) (*UsageMetrics, error) {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()
	
	metrics, exists := tm.usageTracking[tenantID]
	if !exists {
		return &UsageMetrics{LastUpdated: time.Now()}, nil
	}
	
	// Return a copy
	metricsCopy := *metrics
	return &metricsCopy, nil
}

// Helper methods

func (tm *TenantManager) validateTenant(tenant *Tenant) error {
	if tenant.ID == "" {
		return fmt.Errorf("tenant ID is required")
	}
	if tenant.Name == "" {
		return fmt.Errorf("tenant name is required")
	}
	if tenant.Type == "" {
		tenant.Type = tm.config.DefaultTenantType
	}
	if tenant.Status == "" {
		tenant.Status = TenantStatusActive
	}
	
	// Validate hierarchical relationships
	if tm.config.EnableHierarchy && tenant.ParentID != "" {
		if _, exists := tm.tenants[tenant.ParentID]; !exists {
			return fmt.Errorf("parent tenant %s not found", tenant.ParentID)
		}
	}
	
	return nil
}

func (tm *TenantManager) getDefaultTenantSettings(tenantType TenantType) *TenantSettings {
	settings := &TenantSettings{
		AllowedModels:      []string{"*"},
		AllowedProviders:   []string{"*"},
		Features:           make(map[string]bool),
		EnableMetrics:      true,
		EnableAuditLogging: true,
		LogLevel:          "info",
		DataRegion:        "us-east-1",
	}
	
	// Set features based on tenant type
	switch tenantType {
	case TenantTypeEnterprise:
		settings.Features["advanced_analytics"] = true
		settings.Features["custom_models"] = true
		settings.Features["priority_support"] = true
		settings.Features["sso"] = true
		settings.Features["audit_logs"] = true
	case TenantTypeTeam:
		settings.Features["team_collaboration"] = true
		settings.Features["shared_workspaces"] = true
		settings.Features["basic_analytics"] = true
	case TenantTypeTrial:
		settings.Features["basic_features"] = true
	default: // Personal
		settings.Features["personal_workspace"] = true
	}
	
	return settings
}

func (tm *TenantManager) getDefaultTenantSecurity() *TenantSecurity {
	return &TenantSecurity{
		RequireSSO:         false,
		AllowedAuthMethods: []string{"password", "oauth2"},
		SessionTimeout:     24 * time.Hour,
		PIIMaskingEnabled:  true,
		PIIMaskingStrategy: "replace",
		EncryptionRequired: false,
		DataRetentionDays:  90,
		AuditRetentionDays: 365,
		RequireMFA:         false,
		PasswordPolicy: &PasswordPolicy{
			MinLength:        8,
			RequireUppercase: true,
			RequireLowercase: true,
			RequireNumbers:   true,
			RequireSymbols:   false,
			MaxAge:           90 * 24 * time.Hour,
			PreventReuse:     5,
		},
	}
}

func (tm *TenantManager) matchesFilter(tenant *Tenant, filter *TenantFilter) bool {
	if filter.Status != nil && tenant.Status != *filter.Status {
		return false
	}
	if filter.Type != nil && tenant.Type != *filter.Type {
		return false
	}
	if filter.ParentID != nil && tenant.ParentID != *filter.ParentID {
		return false
	}
	if filter.CreatedAfter != nil && tenant.CreatedAt.Before(*filter.CreatedAfter) {
		return false
	}
	if filter.CreatedBefore != nil && tenant.CreatedAt.After(*filter.CreatedBefore) {
		return false
	}
	return true
}

func (tm *TenantManager) checkUsageLimits(tenant *Tenant, resource string) (*LimitResult, error) {
	limits := tenant.GetEffectiveLimits()
	usage, exists := tm.usageTracking[tenant.ID]
	if !exists {
		return nil, nil // No usage yet, allow
	}
	
	// Check hourly request limit
	if limits.RequestsPerHour > 0 {
		percentage := float64(usage.RequestsThisHour) / float64(limits.RequestsPerHour)
		if percentage >= 1.0 {
			return &LimitResult{
				Allowed:    false,
				Reason:     "hourly request limit exceeded",
				LimitType:  "requests_per_hour",
				Current:    usage.RequestsThisHour,
				Limit:      limits.RequestsPerHour,
				Percentage: percentage * 100,
			}, nil
		}
		if percentage >= tm.config.SoftLimitThreshold {
			tm.logger.Warnw("Approaching hourly request limit",
				"tenant_id", tenant.ID,
				"current", usage.RequestsThisHour,
				"limit", limits.RequestsPerHour,
				"percentage", percentage*100)
		}
	}
	
	// Check other limits (tokens, cost, etc.) similarly...
	// This is a simplified implementation
	
	return nil, nil
}

func (tm *TenantManager) initializeStorage() error {
	// This is a simplified implementation
	// In production, you would implement proper database/Redis storage
	tm.logger.Infow("Tenant storage initialized", "type", tm.config.StorageType)
	return nil
}

func (tm *TenantManager) persistTenant(tenant *Tenant) error {
	// This is a simplified implementation
	// In production, you would persist to database/Redis
	return nil
}

func (tm *TenantManager) startBackgroundServices() {
	// Usage reset service
	if tm.config.TrackUsage {
		tm.usageResetTicker = time.NewTicker(tm.config.UsageResetInterval)
		go tm.runUsageResetService()
	}
	
	// Cleanup service
	tm.cleanupTicker = time.NewTicker(tm.config.CleanupInterval)
	go tm.runCleanupService()
}

func (tm *TenantManager) runUsageResetService() {
	for {
		select {
		case <-tm.usageResetTicker.C:
			tm.resetUsageCounters()
		case <-tm.stopChan:
			return
		}
	}
}

func (tm *TenantManager) runCleanupService() {
	for {
		select {
		case <-tm.cleanupTicker.C:
			tm.cleanupExpiredData()
		case <-tm.stopChan:
			return
		}
	}
}

func (tm *TenantManager) resetUsageCounters() {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()
	
	now := time.Now()
	for _, usage := range tm.usageTracking {
		// Reset hourly counters every hour
		if now.Sub(usage.LastUpdated) >= time.Hour {
			usage.RequestsThisHour = 0
			usage.TokensThisHour = 0
			usage.CostThisHour = 0
		}
		
		// Reset daily counters at midnight
		if now.Hour() == 0 && usage.LastUpdated.Day() != now.Day() {
			usage.RequestsThisDay = 0
			usage.TokensThisDay = 0
			usage.CostThisDay = 0
		}
		
		// Reset monthly counters at month start
		if now.Day() == 1 && usage.LastUpdated.Month() != now.Month() {
			usage.RequestsThisMonth = 0
			usage.TokensThisMonth = 0
			usage.CostThisMonth = 0
		}
		
		usage.LastUpdated = now
	}
}

func (tm *TenantManager) cleanupExpiredData() {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()
	
	now := time.Now()
	retentionPeriod := time.Duration(tm.config.UsageRetentionDays) * 24 * time.Hour
	
	// Clean up old usage data
	for tenantID, usage := range tm.usageTracking {
		if now.Sub(usage.LastUpdated) > retentionPeriod {
			delete(tm.usageTracking, tenantID)
		}
	}
	
	// Clean up deleted tenants after retention period
	for tenantID, tenant := range tm.tenants {
		if tenant.Status == TenantStatusDeleted && tenant.DeletedAt != nil {
			if now.Sub(*tenant.DeletedAt) > retentionPeriod {
				delete(tm.tenants, tenantID)
				delete(tm.usageTracking, tenantID)
			}
		}
	}
}

func (tm *TenantManager) recordTenantMetric(action string, tenant *Tenant) {
	metric := &monitoring.Metric{
		Name:  "tenant_operations_total",
		Type:  monitoring.MetricTypeCounter,
		Value: 1,
		Labels: map[string]string{
			"action":      action,
			"tenant_type": string(tenant.Type),
			"tenant_id":   tenant.ID,
		},
		Timestamp: time.Now(),
	}
	
	if err := tm.monitor.RecordMetric(metric); err != nil {
		tm.logger.Warnw("Failed to record tenant metric", "error", err)
	}
}

func (tm *TenantManager) recordUsageMetric(tenantID string, usage *UsageRecord) {
	metric := &monitoring.Metric{
		Name:  "tenant_usage_total",
		Type:  monitoring.MetricTypeCounter,
		Value: float64(usage.Count),
		Labels: map[string]string{
			"tenant_id": tenantID,
			"type":      usage.Type,
			"model":     usage.Model,
			"provider":  usage.Provider,
		},
		Timestamp: usage.Timestamp,
	}
	
	if err := tm.monitor.RecordMetric(metric); err != nil {
		tm.logger.Warnw("Failed to record usage metric", "error", err)
	}
}

// Stop gracefully stops the tenant manager
func (tm *TenantManager) Stop() {
	close(tm.stopChan)
	
	if tm.usageResetTicker != nil {
		tm.usageResetTicker.Stop()
	}
	if tm.cleanupTicker != nil {
		tm.cleanupTicker.Stop()
	}
	
	tm.logger.Info("Tenant manager stopped")
}

// GetTenantStats returns overall tenant statistics
func (tm *TenantManager) GetTenantStats() map[string]interface{} {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()
	
	stats := make(map[string]interface{})
	
	// Count tenants by status and type
	statusCounts := make(map[TenantStatus]int)
	typeCounts := make(map[TenantType]int)
	
	for _, tenant := range tm.tenants {
		statusCounts[tenant.Status]++
		typeCounts[tenant.Type]++
	}
	
	stats["total_tenants"] = len(tm.tenants)
	stats["tenants_by_status"] = statusCounts
	stats["tenants_by_type"] = typeCounts
	stats["usage_tracking_enabled"] = tm.config.TrackUsage
	stats["tracked_usage_entries"] = len(tm.usageTracking)
	
	return stats
}
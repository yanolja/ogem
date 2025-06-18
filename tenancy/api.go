package tenancy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// TenantAPI provides REST API endpoints for tenant management
type TenantAPI struct {
	tenantManager *TenantManager
	logger        *zap.SugaredLogger
}

// NewTenantAPI creates a new tenant API instance
func NewTenantAPI(tenantManager *TenantManager, logger *zap.SugaredLogger) *TenantAPI {
	return &TenantAPI{
		tenantManager: tenantManager,
		logger:        logger,
	}
}

// RegisterRoutes registers all tenant API routes
func (api *TenantAPI) RegisterRoutes(router *mux.Router) {
	// Tenant management
	router.HandleFunc("/tenants", api.CreateTenant).Methods("POST")
	router.HandleFunc("/tenants", api.ListTenants).Methods("GET")
	router.HandleFunc("/tenants/{tenant_id}", api.GetTenant).Methods("GET")
	router.HandleFunc("/tenants/{tenant_id}", api.UpdateTenant).Methods("PUT")
	router.HandleFunc("/tenants/{tenant_id}", api.DeleteTenant).Methods("DELETE")
	
	// Tenant status management
	router.HandleFunc("/tenants/{tenant_id}/activate", api.ActivateTenant).Methods("POST")
	router.HandleFunc("/tenants/{tenant_id}/suspend", api.SuspendTenant).Methods("POST")
	
	// Usage and metrics
	router.HandleFunc("/tenants/{tenant_id}/usage", api.GetTenantUsage).Methods("GET")
	router.HandleFunc("/tenants/{tenant_id}/limits", api.GetTenantLimits).Methods("GET")
	router.HandleFunc("/tenants/{tenant_id}/limits", api.UpdateTenantLimits).Methods("PUT")
	
	// Tenant settings
	router.HandleFunc("/tenants/{tenant_id}/settings", api.GetTenantSettings).Methods("GET")
	router.HandleFunc("/tenants/{tenant_id}/settings", api.UpdateTenantSettings).Methods("PUT")
	
	// Security settings
	router.HandleFunc("/tenants/{tenant_id}/security", api.GetTenantSecurity).Methods("GET")
	router.HandleFunc("/tenants/{tenant_id}/security", api.UpdateTenantSecurity).Methods("PUT")
	
	// Subscription management
	router.HandleFunc("/tenants/{tenant_id}/subscription", api.GetTenantSubscription).Methods("GET")
	router.HandleFunc("/tenants/{tenant_id}/subscription", api.UpdateTenantSubscription).Methods("PUT")
	
	// Access control
	router.HandleFunc("/tenants/{tenant_id}/access-check", api.CheckTenantAccess).Methods("POST")
	
	// Statistics and analytics
	router.HandleFunc("/tenants/stats", api.GetTenantStats).Methods("GET")
	router.HandleFunc("/tenants/{tenant_id}/analytics", api.GetTenantAnalytics).Methods("GET")
}

// CreateTenant handles POST /tenants
func (api *TenantAPI) CreateTenant(w http.ResponseWriter, r *http.Request) {
	var req CreateTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON payload")
		return
	}
	
	// Validate request
	if err := api.validateCreateTenantRequest(&req); err != nil {
		api.writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	
	// Create tenant object
	tenant := &Tenant{
		ID:          req.ID,
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Type:        req.Type,
		Status:      TenantStatusActive,
		Settings:    req.Settings,
		Limits:      req.Limits,
		Security:    req.Security,
		Organization: req.Organization,
		Subscription: req.Subscription,
		Metadata:    req.Metadata,
		ParentID:    req.ParentID,
	}
	
	// Create tenant
	if err := api.tenantManager.CreateTenant(r.Context(), tenant); err != nil {
		api.logger.Errorw("Failed to create tenant", "error", err, "tenant_id", req.ID)
		api.writeError(w, http.StatusInternalServerError, "creation_failed", "Failed to create tenant")
		return
	}
	
	api.writeJSON(w, http.StatusCreated, tenant)
}

// GetTenant handles GET /tenants/{tenant_id}
func (api *TenantAPI) GetTenant(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	
	tenant, err := api.tenantManager.GetTenant(r.Context(), tenantID)
	if err != nil {
		api.writeError(w, http.StatusNotFound, "tenant_not_found", "Tenant not found")
		return
	}
	
	api.writeJSON(w, http.StatusOK, tenant)
}

// UpdateTenant handles PUT /tenants/{tenant_id}
func (api *TenantAPI) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	
	var req UpdateTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON payload")
		return
	}
	
	// Get existing tenant
	tenant, err := api.tenantManager.GetTenant(r.Context(), tenantID)
	if err != nil {
		api.writeError(w, http.StatusNotFound, "tenant_not_found", "Tenant not found")
		return
	}
	
	// Apply updates
	api.applyTenantUpdates(tenant, &req)
	
	// Update tenant
	if err := api.tenantManager.UpdateTenant(r.Context(), tenant); err != nil {
		api.logger.Errorw("Failed to update tenant", "error", err, "tenant_id", tenantID)
		api.writeError(w, http.StatusInternalServerError, "update_failed", "Failed to update tenant")
		return
	}
	
	api.writeJSON(w, http.StatusOK, tenant)
}

// DeleteTenant handles DELETE /tenants/{tenant_id}
func (api *TenantAPI) DeleteTenant(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	
	if err := api.tenantManager.DeleteTenant(r.Context(), tenantID); err != nil {
		api.logger.Errorw("Failed to delete tenant", "error", err, "tenant_id", tenantID)
		api.writeError(w, http.StatusInternalServerError, "deletion_failed", "Failed to delete tenant")
		return
	}
	
	w.WriteHeader(http.StatusNoContent)
}

// ListTenants handles GET /tenants
func (api *TenantAPI) ListTenants(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters for filtering
	filter := &TenantFilter{}
	
	if status := r.URL.Query().Get("status"); status != "" {
		tenantStatus := TenantStatus(status)
		filter.Status = &tenantStatus
	}
	
	if tenantType := r.URL.Query().Get("type"); tenantType != "" {
		tType := TenantType(tenantType)
		filter.Type = &tType
	}
	
	if parentID := r.URL.Query().Get("parent_id"); parentID != "" {
		filter.ParentID = &parentID
	}
	
	// Parse pagination parameters
	limit := 50 // default
	offset := 0
	
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}
	
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	
	// Get tenants
	tenants, err := api.tenantManager.ListTenants(r.Context(), filter)
	if err != nil {
		api.logger.Errorw("Failed to list tenants", "error", err)
		api.writeError(w, http.StatusInternalServerError, "list_failed", "Failed to list tenants")
		return
	}
	
	// Apply pagination
	total := len(tenants)
	start := offset
	end := offset + limit
	
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	
	paginatedTenants := tenants[start:end]
	
	response := ListTenantsResponse{
		Tenants: paginatedTenants,
		Pagination: PaginationInfo{
			Total:  total,
			Limit:  limit,
			Offset: offset,
			Count:  len(paginatedTenants),
		},
	}
	
	api.writeJSON(w, http.StatusOK, response)
}

// ActivateTenant handles POST /tenants/{tenant_id}/activate
func (api *TenantAPI) ActivateTenant(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	
	tenant, err := api.tenantManager.GetTenant(r.Context(), tenantID)
	if err != nil {
		api.writeError(w, http.StatusNotFound, "tenant_not_found", "Tenant not found")
		return
	}
	
	tenant.Status = TenantStatusActive
	tenant.UpdatedAt = time.Now()
	
	if err := api.tenantManager.UpdateTenant(r.Context(), tenant); err != nil {
		api.logger.Errorw("Failed to activate tenant", "error", err, "tenant_id", tenantID)
		api.writeError(w, http.StatusInternalServerError, "activation_failed", "Failed to activate tenant")
		return
	}
	
	api.writeJSON(w, http.StatusOK, tenant)
}

// SuspendTenant handles POST /tenants/{tenant_id}/suspend
func (api *TenantAPI) SuspendTenant(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	
	tenant, err := api.tenantManager.GetTenant(r.Context(), tenantID)
	if err != nil {
		api.writeError(w, http.StatusNotFound, "tenant_not_found", "Tenant not found")
		return
	}
	
	tenant.Status = TenantStatusSuspended
	tenant.UpdatedAt = time.Now()
	
	if err := api.tenantManager.UpdateTenant(r.Context(), tenant); err != nil {
		api.logger.Errorw("Failed to suspend tenant", "error", err, "tenant_id", tenantID)
		api.writeError(w, http.StatusInternalServerError, "suspension_failed", "Failed to suspend tenant")
		return
	}
	
	api.writeJSON(w, http.StatusOK, tenant)
}

// GetTenantUsage handles GET /tenants/{tenant_id}/usage
func (api *TenantAPI) GetTenantUsage(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	
	usage, err := api.tenantManager.GetUsageMetrics(r.Context(), tenantID)
	if err != nil {
		api.logger.Errorw("Failed to get tenant usage", "error", err, "tenant_id", tenantID)
		api.writeError(w, http.StatusInternalServerError, "usage_failed", "Failed to get usage metrics")
		return
	}
	
	api.writeJSON(w, http.StatusOK, usage)
}

// GetTenantLimits handles GET /tenants/{tenant_id}/limits
func (api *TenantAPI) GetTenantLimits(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	
	tenant, err := api.tenantManager.GetTenant(r.Context(), tenantID)
	if err != nil {
		api.writeError(w, http.StatusNotFound, "tenant_not_found", "Tenant not found")
		return
	}
	
	api.writeJSON(w, http.StatusOK, tenant.GetEffectiveLimits())
}

// UpdateTenantLimits handles PUT /tenants/{tenant_id}/limits
func (api *TenantAPI) UpdateTenantLimits(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	
	var limits TenantLimits
	if err := json.NewDecoder(r.Body).Decode(&limits); err != nil {
		api.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON payload")
		return
	}
	
	tenant, err := api.tenantManager.GetTenant(r.Context(), tenantID)
	if err != nil {
		api.writeError(w, http.StatusNotFound, "tenant_not_found", "Tenant not found")
		return
	}
	
	tenant.Limits = &limits
	tenant.UpdatedAt = time.Now()
	
	if err := api.tenantManager.UpdateTenant(r.Context(), tenant); err != nil {
		api.logger.Errorw("Failed to update tenant limits", "error", err, "tenant_id", tenantID)
		api.writeError(w, http.StatusInternalServerError, "update_failed", "Failed to update limits")
		return
	}
	
	api.writeJSON(w, http.StatusOK, tenant.Limits)
}

// GetTenantSettings handles GET /tenants/{tenant_id}/settings
func (api *TenantAPI) GetTenantSettings(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	
	tenant, err := api.tenantManager.GetTenant(r.Context(), tenantID)
	if err != nil {
		api.writeError(w, http.StatusNotFound, "tenant_not_found", "Tenant not found")
		return
	}
	
	api.writeJSON(w, http.StatusOK, tenant.Settings)
}

// UpdateTenantSettings handles PUT /tenants/{tenant_id}/settings
func (api *TenantAPI) UpdateTenantSettings(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	
	var settings TenantSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		api.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON payload")
		return
	}
	
	tenant, err := api.tenantManager.GetTenant(r.Context(), tenantID)
	if err != nil {
		api.writeError(w, http.StatusNotFound, "tenant_not_found", "Tenant not found")
		return
	}
	
	tenant.Settings = &settings
	tenant.UpdatedAt = time.Now()
	
	if err := api.tenantManager.UpdateTenant(r.Context(), tenant); err != nil {
		api.logger.Errorw("Failed to update tenant settings", "error", err, "tenant_id", tenantID)
		api.writeError(w, http.StatusInternalServerError, "update_failed", "Failed to update settings")
		return
	}
	
	api.writeJSON(w, http.StatusOK, tenant.Settings)
}

// GetTenantSecurity handles GET /tenants/{tenant_id}/security
func (api *TenantAPI) GetTenantSecurity(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	
	tenant, err := api.tenantManager.GetTenant(r.Context(), tenantID)
	if err != nil {
		api.writeError(w, http.StatusNotFound, "tenant_not_found", "Tenant not found")
		return
	}
	
	api.writeJSON(w, http.StatusOK, tenant.Security)
}

// UpdateTenantSecurity handles PUT /tenants/{tenant_id}/security
func (api *TenantAPI) UpdateTenantSecurity(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	
	var security TenantSecurity
	if err := json.NewDecoder(r.Body).Decode(&security); err != nil {
		api.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON payload")
		return
	}
	
	tenant, err := api.tenantManager.GetTenant(r.Context(), tenantID)
	if err != nil {
		api.writeError(w, http.StatusNotFound, "tenant_not_found", "Tenant not found")
		return
	}
	
	tenant.Security = &security
	tenant.UpdatedAt = time.Now()
	
	if err := api.tenantManager.UpdateTenant(r.Context(), tenant); err != nil {
		api.logger.Errorw("Failed to update tenant security", "error", err, "tenant_id", tenantID)
		api.writeError(w, http.StatusInternalServerError, "update_failed", "Failed to update security settings")
		return
	}
	
	api.writeJSON(w, http.StatusOK, tenant.Security)
}

// GetTenantSubscription handles GET /tenants/{tenant_id}/subscription
func (api *TenantAPI) GetTenantSubscription(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	
	tenant, err := api.tenantManager.GetTenant(r.Context(), tenantID)
	if err != nil {
		api.writeError(w, http.StatusNotFound, "tenant_not_found", "Tenant not found")
		return
	}
	
	api.writeJSON(w, http.StatusOK, tenant.Subscription)
}

// UpdateTenantSubscription handles PUT /tenants/{tenant_id}/subscription
func (api *TenantAPI) UpdateTenantSubscription(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	
	var subscription Subscription
	if err := json.NewDecoder(r.Body).Decode(&subscription); err != nil {
		api.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON payload")
		return
	}
	
	tenant, err := api.tenantManager.GetTenant(r.Context(), tenantID)
	if err != nil {
		api.writeError(w, http.StatusNotFound, "tenant_not_found", "Tenant not found")
		return
	}
	
	tenant.Subscription = &subscription
	tenant.UpdatedAt = time.Now()
	
	if err := api.tenantManager.UpdateTenant(r.Context(), tenant); err != nil {
		api.logger.Errorw("Failed to update tenant subscription", "error", err, "tenant_id", tenantID)
		api.writeError(w, http.StatusInternalServerError, "update_failed", "Failed to update subscription")
		return
	}
	
	api.writeJSON(w, http.StatusOK, tenant.Subscription)
}

// CheckTenantAccess handles POST /tenants/{tenant_id}/access-check
func (api *TenantAPI) CheckTenantAccess(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	
	var req AccessCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON payload")
		return
	}
	
	result, err := api.tenantManager.CheckAccess(r.Context(), tenantID, req.UserID, req.Resource, req.Action)
	if err != nil {
		api.logger.Errorw("Failed to check tenant access", "error", err, "tenant_id", tenantID)
		api.writeError(w, http.StatusInternalServerError, "access_check_failed", "Failed to check access")
		return
	}
	
	api.writeJSON(w, http.StatusOK, result)
}

// GetTenantStats handles GET /tenants/stats
func (api *TenantAPI) GetTenantStats(w http.ResponseWriter, r *http.Request) {
	stats := api.tenantManager.GetTenantStats()
	api.writeJSON(w, http.StatusOK, stats)
}

// GetTenantAnalytics handles GET /tenants/{tenant_id}/analytics
func (api *TenantAPI) GetTenantAnalytics(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]
	
	// Get tenant to verify it exists
	tenant, err := api.tenantManager.GetTenant(r.Context(), tenantID)
	if err != nil {
		api.writeError(w, http.StatusNotFound, "tenant_not_found", "Tenant not found")
		return
	}
	
	// Get usage metrics
	usage, err := api.tenantManager.GetUsageMetrics(r.Context(), tenantID)
	if err != nil {
		api.logger.Errorw("Failed to get tenant usage for analytics", "error", err, "tenant_id", tenantID)
		usage = &UsageMetrics{} // Return empty metrics on error
	}
	
	// Build analytics response
	analytics := TenantAnalytics{
		TenantID:      tenantID,
		TenantName:    tenant.Name,
		TenantType:    tenant.Type,
		Status:        tenant.Status,
		CreatedAt:     tenant.CreatedAt,
		Usage:         usage,
		Limits:        tenant.GetEffectiveLimits(),
		Utilization:   api.calculateUtilization(usage, tenant.GetEffectiveLimits()),
		LastUpdated:   time.Now(),
	}
	
	api.writeJSON(w, http.StatusOK, analytics)
}

// Request/Response types

type CreateTenantRequest struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	DisplayName  string            `json:"display_name,omitempty"`
	Description  string            `json:"description,omitempty"`
	Type         TenantType        `json:"type"`
	Settings     *TenantSettings   `json:"settings,omitempty"`
	Limits       *TenantLimits     `json:"limits,omitempty"`
	Security     *TenantSecurity   `json:"security,omitempty"`
	Organization *Organization     `json:"organization,omitempty"`
	Subscription *Subscription     `json:"subscription,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	ParentID     string            `json:"parent_id,omitempty"`
}

type UpdateTenantRequest struct {
	Name         *string            `json:"name,omitempty"`
	DisplayName  *string            `json:"display_name,omitempty"`
	Description  *string            `json:"description,omitempty"`
	Type         *TenantType        `json:"type,omitempty"`
	Status       *TenantStatus      `json:"status,omitempty"`
	Settings     *TenantSettings    `json:"settings,omitempty"`
	Limits       *TenantLimits      `json:"limits,omitempty"`
	Security     *TenantSecurity    `json:"security,omitempty"`
	Organization *Organization      `json:"organization,omitempty"`
	Subscription *Subscription      `json:"subscription,omitempty"`
	Metadata     map[string]string  `json:"metadata,omitempty"`
}

type ListTenantsResponse struct {
	Tenants    []*Tenant      `json:"tenants"`
	Pagination PaginationInfo `json:"pagination"`
}

type PaginationInfo struct {
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Count  int `json:"count"`
}

type AccessCheckRequest struct {
	UserID   string `json:"user_id"`
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

type TenantAnalytics struct {
	TenantID      string                 `json:"tenant_id"`
	TenantName    string                 `json:"tenant_name"`
	TenantType    TenantType             `json:"tenant_type"`
	Status        TenantStatus           `json:"status"`
	CreatedAt     time.Time              `json:"created_at"`
	Usage         *UsageMetrics          `json:"usage"`
	Limits        *TenantLimits          `json:"limits"`
	Utilization   map[string]float64     `json:"utilization"`
	LastUpdated   time.Time              `json:"last_updated"`
}

// Helper methods

func (api *TenantAPI) validateCreateTenantRequest(req *CreateTenantRequest) error {
	if req.ID == "" {
		return fmt.Errorf("tenant ID is required")
	}
	if req.Name == "" {
		return fmt.Errorf("tenant name is required")
	}
	if req.Type == "" {
		req.Type = TenantTypePersonal
	}
	return nil
}

func (api *TenantAPI) applyTenantUpdates(tenant *Tenant, req *UpdateTenantRequest) {
	if req.Name != nil {
		tenant.Name = *req.Name
	}
	if req.DisplayName != nil {
		tenant.DisplayName = *req.DisplayName
	}
	if req.Description != nil {
		tenant.Description = *req.Description
	}
	if req.Type != nil {
		tenant.Type = *req.Type
	}
	if req.Status != nil {
		tenant.Status = *req.Status
	}
	if req.Settings != nil {
		tenant.Settings = req.Settings
	}
	if req.Limits != nil {
		tenant.Limits = req.Limits
	}
	if req.Security != nil {
		tenant.Security = req.Security
	}
	if req.Organization != nil {
		tenant.Organization = req.Organization
	}
	if req.Subscription != nil {
		tenant.Subscription = req.Subscription
	}
	if req.Metadata != nil {
		if tenant.Metadata == nil {
			tenant.Metadata = make(map[string]string)
		}
		for k, v := range req.Metadata {
			tenant.Metadata[k] = v
		}
	}
}

func (api *TenantAPI) calculateUtilization(usage *UsageMetrics, limits *TenantLimits) map[string]float64 {
	utilization := make(map[string]float64)
	
	if limits.RequestsPerHour > 0 {
		utilization["requests_per_hour"] = float64(usage.RequestsThisHour) / float64(limits.RequestsPerHour) * 100
	}
	if limits.RequestsPerDay > 0 {
		utilization["requests_per_day"] = float64(usage.RequestsThisDay) / float64(limits.RequestsPerDay) * 100
	}
	if limits.RequestsPerMonth > 0 {
		utilization["requests_per_month"] = float64(usage.RequestsThisMonth) / float64(limits.RequestsPerMonth) * 100
	}
	if limits.TokensPerHour > 0 {
		utilization["tokens_per_hour"] = float64(usage.TokensThisHour) / float64(limits.TokensPerHour) * 100
	}
	if limits.TokensPerDay > 0 {
		utilization["tokens_per_day"] = float64(usage.TokensThisDay) / float64(limits.TokensPerDay) * 100
	}
	if limits.TokensPerMonth > 0 {
		utilization["tokens_per_month"] = float64(usage.TokensThisMonth) / float64(limits.TokensPerMonth) * 100
	}
	if limits.CostPerHour > 0 {
		utilization["cost_per_hour"] = usage.CostThisHour / limits.CostPerHour * 100
	}
	if limits.CostPerDay > 0 {
		utilization["cost_per_day"] = usage.CostThisDay / limits.CostPerDay * 100
	}
	if limits.CostPerMonth > 0 {
		utilization["cost_per_month"] = usage.CostThisMonth / limits.CostPerMonth * 100
	}
	if limits.MaxStorageGB > 0 {
		utilization["storage"] = float64(usage.StorageUsedGB) / float64(limits.MaxStorageGB) * 100
	}
	if limits.MaxFiles > 0 {
		utilization["files"] = float64(usage.FilesCount) / float64(limits.MaxFiles) * 100
	}
	
	return utilization
}

func (api *TenantAPI) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	
	if err := json.NewEncoder(w).Encode(data); err != nil {
		api.logger.Errorw("Failed to encode JSON response", "error", err)
	}
}

func (api *TenantAPI) writeError(w http.ResponseWriter, status int, errorType, message string) {
	errorResponse := map[string]interface{}{
		"error": map[string]interface{}{
			"type":    errorType,
			"message": message,
			"code":    status,
		},
	}
	
	api.writeJSON(w, status, errorResponse)
}
package tenancy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/yanolja/ogem/auth"
)

// TenantMiddleware provides HTTP middleware for multi-tenant request handling
type TenantMiddleware struct {
	tenantManager *TenantManager
	authManager   *auth.AuthManager
	logger        *zap.SugaredLogger
	config        *MiddlewareConfig
}

// MiddlewareConfig configures the tenant middleware
type MiddlewareConfig struct {
	// Tenant identification method
	IdentificationMethod TenantIdentificationMethod `yaml:"identification_method"`
	
	// Header names for tenant identification
	TenantHeaderName     string `yaml:"tenant_header_name"`
	TenantDomainHeader   string `yaml:"tenant_domain_header"`
	
	// URL path configurations
	TenantPathPrefix     string `yaml:"tenant_path_prefix"`     // e.g., "/t/{tenant_id}"
	TenantSubdomain      bool   `yaml:"tenant_subdomain"`       // e.g., "tenant.example.com"
	
	// Default tenant for requests without tenant context
	DefaultTenantID      string `yaml:"default_tenant_id"`
	
	// Auto-provisioning settings
	AutoProvisionTenants bool   `yaml:"auto_provision_tenants"`
	
	// Error handling
	RequireTenant        bool   `yaml:"require_tenant"`
	
	// Cache settings for tenant lookup
	EnableCaching        bool          `yaml:"enable_caching"`
	CacheTTL            int           `yaml:"cache_ttl_seconds"`
}

// TenantIdentificationMethod defines how tenants are identified in requests
type TenantIdentificationMethod string

const (
	IdentificationMethodHeader    TenantIdentificationMethod = "header"
	IdentificationMethodPath      TenantIdentificationMethod = "path"
	IdentificationMethodSubdomain TenantIdentificationMethod = "subdomain"
	IdentificationMethodJWT       TenantIdentificationMethod = "jwt"
	IdentificationMethodCombined  TenantIdentificationMethod = "combined"
)

// NewTenantMiddleware creates a new tenant middleware
func NewTenantMiddleware(tenantManager *TenantManager, authManager *auth.AuthManager, config *MiddlewareConfig, logger *zap.SugaredLogger) *TenantMiddleware {
	if config == nil {
		config = DefaultMiddlewareConfig()
	}
	
	return &TenantMiddleware{
		tenantManager: tenantManager,
		authManager:   authManager,
		logger:        logger,
		config:        config,
	}
}

// DefaultMiddlewareConfig returns default middleware configuration
func DefaultMiddlewareConfig() *MiddlewareConfig {
	return &MiddlewareConfig{
		IdentificationMethod: IdentificationMethodHeader,
		TenantHeaderName:     "X-Tenant-ID",
		TenantDomainHeader:   "X-Tenant-Domain",
		TenantPathPrefix:     "/t",
		TenantSubdomain:      false,
		DefaultTenantID:      "",
		AutoProvisionTenants: false,
		RequireTenant:        true,
		EnableCaching:        true,
		CacheTTL:            300, // 5 minutes
	}
}

// Middleware returns the HTTP middleware function
func (tm *TenantMiddleware) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract tenant information from request
			tenantCtx, err := tm.extractTenantContext(r)
			if err != nil {
				tm.handleTenantError(w, r, err)
				return
			}
			
			// Validate tenant access
			if tenantCtx != nil && tenantCtx.Tenant != nil {
				if err := tm.validateTenantAccess(r.Context(), tenantCtx); err != nil {
					tm.handleTenantError(w, r, err)
					return
				}
			}
			
			// Add tenant context to request
			if tenantCtx != nil {
				ctx := WithTenantContext(r.Context(), tenantCtx)
				r = r.WithContext(ctx)
			}
			
			// Set response headers for tenant identification
			if tenantCtx != nil && tenantCtx.Tenant != nil {
				w.Header().Set("X-Tenant-ID", tenantCtx.Tenant.ID)
				w.Header().Set("X-Tenant-Name", tenantCtx.Tenant.Name)
			}
			
			next.ServeHTTP(w, r)
		})
	}
}

// extractTenantContext extracts tenant information from the HTTP request
func (tm *TenantMiddleware) extractTenantContext(r *http.Request) (*TenantContext, error) {
	var tenantID string
	var userID string
	var teamID string
	var projectID string
	
	// Extract user information from authentication context
	if tm.authManager != nil {
		if authCtx := auth.GetAuthContextFromRequest(r); authCtx != nil {
			userID = authCtx.UserID
			// Extract additional IDs from JWT claims if available
			if claims := authCtx.Claims; claims != nil {
				if tid, ok := claims["team_id"].(string); ok {
					teamID = tid
				}
				if pid, ok := claims["project_id"].(string); ok {
					projectID = pid
				}
				if tenant, ok := claims["tenant_id"].(string); ok {
					tenantID = tenant
				}
			}
		}
	}
	
	// Extract tenant ID based on identification method
	switch tm.config.IdentificationMethod {
	case IdentificationMethodHeader:
		tenantID = tm.extractFromHeader(r)
	case IdentificationMethodPath:
		tenantID = tm.extractFromPath(r)
	case IdentificationMethodSubdomain:
		tenantID = tm.extractFromSubdomain(r)
	case IdentificationMethodJWT:
		// Already extracted above from JWT claims
	case IdentificationMethodCombined:
		// Try multiple methods in order of preference
		if id := tm.extractFromHeader(r); id != "" {
			tenantID = id
		} else if id := tm.extractFromPath(r); id != "" {
			tenantID = id
		} else if id := tm.extractFromSubdomain(r); id != "" {
			tenantID = id
		}
	}
	
	// Use default tenant if none found and configured
	if tenantID == "" && tm.config.DefaultTenantID != "" {
		tenantID = tm.config.DefaultTenantID
	}
	
	// Return early if no tenant identified and not required
	if tenantID == "" {
		if tm.config.RequireTenant {
			return nil, fmt.Errorf("tenant identification required but not found")
		}
		return nil, nil
	}
	
	// Retrieve tenant information
	tenant, err := tm.tenantManager.GetTenant(r.Context(), tenantID)
	if err != nil {
		// Auto-provision tenant if enabled
		if tm.config.AutoProvisionTenants {
			tenant, err = tm.autoProvisionTenant(r.Context(), tenantID, userID)
			if err != nil {
				return nil, fmt.Errorf("failed to auto-provision tenant %s: %v", tenantID, err)
			}
		} else {
			return nil, fmt.Errorf("tenant %s not found: %v", tenantID, err)
		}
	}
	
	return &TenantContext{
		Tenant:    tenant,
		UserID:    userID,
		TeamID:    teamID,
		ProjectID: projectID,
	}, nil
}

// extractFromHeader extracts tenant ID from HTTP headers
func (tm *TenantMiddleware) extractFromHeader(r *http.Request) string {
	// Try the configured tenant header first
	if tenantID := r.Header.Get(tm.config.TenantHeaderName); tenantID != "" {
		return tenantID
	}
	
	// Try common alternative headers
	alternatives := []string{
		"X-Tenant-ID",
		"X-Organization-ID", 
		"X-Org-ID",
		"Tenant-ID",
		"Organization-ID",
	}
	
	for _, header := range alternatives {
		if tenantID := r.Header.Get(header); tenantID != "" {
			return tenantID
		}
	}
	
	// Try domain-based identification
	if domain := r.Header.Get(tm.config.TenantDomainHeader); domain != "" {
		return tm.tenantIDFromDomain(domain)
	}
	
	return ""
}

// extractFromPath extracts tenant ID from URL path
func (tm *TenantMiddleware) extractFromPath(r *http.Request) string {
	path := r.URL.Path
	
	// Remove leading slash and split path components
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	
	// Look for tenant prefix pattern: /t/{tenant_id}/...
	if len(parts) >= 2 && parts[0] == strings.TrimPrefix(tm.config.TenantPathPrefix, "/") {
		return parts[1]
	}
	
	// Look for organization prefix pattern: /org/{tenant_id}/...
	if len(parts) >= 2 && (parts[0] == "org" || parts[0] == "organization") {
		return parts[1]
	}
	
	return ""
}

// extractFromSubdomain extracts tenant ID from subdomain
func (tm *TenantMiddleware) extractFromSubdomain(r *http.Request) string {
	if !tm.config.TenantSubdomain {
		return ""
	}
	
	host := r.Host
	
	// Remove port if present
	if colonIndex := strings.Index(host, ":"); colonIndex != -1 {
		host = host[:colonIndex]
	}
	
	// Split by dots to get subdomain parts
	parts := strings.Split(host, ".")
	
	// For subdomain like "tenant.example.com", tenant ID is the first part
	if len(parts) >= 3 {
		subdomain := parts[0]
		// Filter out common subdomains that aren't tenant IDs
		if subdomain != "www" && subdomain != "api" && subdomain != "app" {
			return subdomain
		}
	}
	
	return ""
}

// tenantIDFromDomain converts a domain name to tenant ID
func (tm *TenantMiddleware) tenantIDFromDomain(domain string) string {
	// This is a simplified implementation
	// In practice, you might maintain a mapping table
	return strings.Replace(domain, ".", "-", -1)
}

// validateTenantAccess validates if the request is authorized for the tenant
func (tm *TenantMiddleware) validateTenantAccess(ctx context.Context, tenantCtx *TenantContext) error {
	tenant := tenantCtx.Tenant
	
	// Check if tenant is active
	if !tenant.IsActive() {
		return fmt.Errorf("tenant %s is not active (status: %s)", tenant.ID, tenant.Status)
	}
	
	// Check if tenant subscription is valid
	if tenant.IsExpired() {
		return fmt.Errorf("tenant %s subscription has expired", tenant.ID)
	}
	
	// Additional validation can be added here
	// - IP restrictions
	// - Geographic restrictions
	// - User membership validation
	
	return nil
}

// autoProvisionTenant creates a new tenant automatically
func (tm *TenantMiddleware) autoProvisionTenant(ctx context.Context, tenantID, userID string) (*Tenant, error) {
	tenant := &Tenant{
		ID:          tenantID,
		Name:        tenantID, // Use ID as name for auto-provisioned tenants
		DisplayName: fmt.Sprintf("Auto-provisioned tenant %s", tenantID),
		Type:        tm.tenantManager.config.AutoProvisioningType,
		Status:      TenantStatusActive,
		Metadata: map[string]string{
			"auto_provisioned": "true",
			"created_by":       userID,
		},
	}
	
	if err := tm.tenantManager.CreateTenant(ctx, tenant); err != nil {
		return nil, err
	}
	
	tm.logger.Infow("Auto-provisioned tenant",
		"tenant_id", tenantID,
		"user_id", userID)
	
	return tenant, nil
}

// handleTenantError handles tenant-related errors in HTTP responses
func (tm *TenantMiddleware) handleTenantError(w http.ResponseWriter, r *http.Request, err error) {
	tm.logger.Warnw("Tenant middleware error",
		"error", err,
		"path", r.URL.Path,
		"method", r.Method,
		"remote_addr", r.RemoteAddr)
	
	// Determine appropriate HTTP status code
	var statusCode int
	var errorType string
	
	errMsg := err.Error()
	switch {
	case strings.Contains(errMsg, "not found"):
		statusCode = http.StatusNotFound
		errorType = "tenant_not_found"
	case strings.Contains(errMsg, "not active"):
		statusCode = http.StatusForbidden
		errorType = "tenant_inactive"
	case strings.Contains(errMsg, "expired"):
		statusCode = http.StatusPaymentRequired
		errorType = "tenant_expired"
	case strings.Contains(errMsg, "required"):
		statusCode = http.StatusBadRequest
		errorType = "tenant_required"
	default:
		statusCode = http.StatusInternalServerError
		errorType = "tenant_error"
	}
	
	// Prepare error response
	errorResponse := map[string]interface{}{
		"error": map[string]interface{}{
			"type":    errorType,
			"message": errMsg,
			"code":    statusCode,
		},
	}
	
	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	// Write error response
	if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
		tm.logger.Errorw("Failed to encode error response", "error", err)
	}
}

// TenantEnforcementMiddleware provides additional tenant isolation enforcement
type TenantEnforcementMiddleware struct {
	tenantManager *TenantManager
	logger        *zap.SugaredLogger
}

// NewTenantEnforcementMiddleware creates a new tenant enforcement middleware
func NewTenantEnforcementMiddleware(tenantManager *TenantManager, logger *zap.SugaredLogger) *TenantEnforcementMiddleware {
	return &TenantEnforcementMiddleware{
		tenantManager: tenantManager,
		logger:        logger,
	}
}

// Middleware returns the HTTP middleware function for tenant enforcement
func (tem *TenantEnforcementMiddleware) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get tenant context from request
			tenantCtx, ok := GetTenantFromContext(r.Context())
			if !ok || tenantCtx.Tenant == nil {
				// No tenant context, let the request proceed
				// This might be handled by the main tenant middleware
				next.ServeHTTP(w, r)
				return
			}
			
			// Enforce tenant-specific restrictions
			if err := tem.enforceRestrictions(r.Context(), tenantCtx, r); err != nil {
				tem.handleEnforcementError(w, r, err)
				return
			}
			
			// Add additional security headers
			tem.addSecurityHeaders(w, tenantCtx.Tenant)
			
			next.ServeHTTP(w, r)
		})
	}
}

// enforceRestrictions enforces tenant-specific restrictions
func (tem *TenantEnforcementMiddleware) enforceRestrictions(ctx context.Context, tenantCtx *TenantContext, r *http.Request) error {
	tenant := tenantCtx.Tenant
	
	// Check IP restrictions
	if tenant.Security != nil {
		if err := tem.checkIPRestrictions(r, tenant.Security); err != nil {
			return err
		}
	}
	
	// Check session timeout
	if tenant.Security != nil && tenant.Security.SessionTimeout > 0 {
		if err := tem.checkSessionTimeout(r, tenant.Security.SessionTimeout); err != nil {
			return err
		}
	}
	
	// Check usage limits
	if err := tem.checkUsageLimits(ctx, tenant, r); err != nil {
		return err
	}
	
	return nil
}

// checkIPRestrictions validates IP-based access restrictions
func (tem *TenantEnforcementMiddleware) checkIPRestrictions(r *http.Request, security *TenantSecurity) error {
	clientIP := tem.getClientIP(r)
	
	// Check blocked IPs
	for _, blockedIP := range security.BlockedIPs {
		if tem.matchesIPPattern(clientIP, blockedIP) {
			return fmt.Errorf("access denied: IP %s is blocked", clientIP)
		}
	}
	
	// Check allowed IPs (if configured)
	if len(security.AllowedIPs) > 0 {
		allowed := false
		for _, allowedIP := range security.AllowedIPs {
			if tem.matchesIPPattern(clientIP, allowedIP) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("access denied: IP %s is not in allowed list", clientIP)
		}
	}
	
	return nil
}

// checkSessionTimeout validates session timeout
func (tem *TenantEnforcementMiddleware) checkSessionTimeout(r *http.Request, timeout time.Duration) error {
	// This would typically check the session timestamp from JWT or session store
	// For now, this is a placeholder implementation
	return nil
}

// checkUsageLimits validates tenant usage limits
func (tem *TenantEnforcementMiddleware) checkUsageLimits(ctx context.Context, tenant *Tenant, r *http.Request) error {
	// Check if this request would exceed tenant limits
	accessResult, err := tem.tenantManager.CheckAccess(ctx, tenant.ID, "", r.URL.Path, r.Method)
	if err != nil {
		return fmt.Errorf("failed to check access: %v", err)
	}
	
	if !accessResult.Allowed {
		return fmt.Errorf("access denied: %s", accessResult.Reason)
	}
	
	return nil
}

// getClientIP extracts the client IP address from the request
func (tem *TenantEnforcementMiddleware) getClientIP(r *http.Request) string {
	// Check for forwarded IP
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ips := strings.Split(forwarded, ",")
		return strings.TrimSpace(ips[0])
	}
	
	// Check for real IP
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}
	
	// Fall back to remote address
	return strings.Split(r.RemoteAddr, ":")[0]
}

// matchesIPPattern checks if IP matches a pattern (simplified implementation)
func (tem *TenantEnforcementMiddleware) matchesIPPattern(ip, pattern string) bool {
	// This is a simplified implementation
	// In production, you would implement proper CIDR matching
	return ip == pattern || pattern == "*"
}

// addSecurityHeaders adds tenant-specific security headers
func (tem *TenantEnforcementMiddleware) addSecurityHeaders(w http.ResponseWriter, tenant *Tenant) {
	// Add standard security headers
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	
	// Add tenant-specific headers
	if tenant.Security != nil && tenant.Security.EncryptionRequired {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	}
	
	// Add data residency headers if configured
	if tenant.Settings != nil && tenant.Settings.DataRegion != "" {
		w.Header().Set("X-Data-Region", tenant.Settings.DataRegion)
	}
}

// handleEnforcementError handles enforcement errors
func (tem *TenantEnforcementMiddleware) handleEnforcementError(w http.ResponseWriter, r *http.Request, err error) {
	tem.logger.Warnw("Tenant enforcement error",
		"error", err,
		"path", r.URL.Path,
		"method", r.Method,
		"remote_addr", r.RemoteAddr)
	
	// Determine appropriate HTTP status code
	var statusCode int
	errMsg := err.Error()
	
	switch {
	case strings.Contains(errMsg, "IP") && strings.Contains(errMsg, "blocked"):
		statusCode = http.StatusForbidden
	case strings.Contains(errMsg, "not in allowed"):
		statusCode = http.StatusForbidden
	case strings.Contains(errMsg, "limit exceeded"):
		statusCode = http.StatusTooManyRequests
	case strings.Contains(errMsg, "session"):
		statusCode = http.StatusUnauthorized
	default:
		statusCode = http.StatusForbidden
	}
	
	// Prepare error response
	errorResponse := map[string]interface{}{
		"error": map[string]interface{}{
			"type":    "tenant_enforcement_error",
			"message": errMsg,
			"code":    statusCode,
		},
	}
	
	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	
	// Write error response
	if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
		tem.logger.Errorw("Failed to encode error response", "error", err)
	}
}
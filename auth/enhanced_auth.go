package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// AuthMethod represents different authentication methods
type AuthMethod string

const (
	AuthMethodMaster   AuthMethod = "master"
	AuthMethodVirtual  AuthMethod = "virtual"
	AuthMethodJWT      AuthMethod = "jwt"
	AuthMethodOAuth2   AuthMethod = "oauth2"
)

// EnhancedAuthContext extends the basic AuthContext with additional fields
type EnhancedAuthContext struct {
	*AuthContext
	Method       AuthMethod
	Username     string
	Email        string
	Roles        []string
	Permissions  []string
	TeamID       string
	Organization string
	VirtualKey   *VirtualKey
	JWTClaims    *JWTClaims
}

// EnhancedAuthConfig represents configuration for enhanced authentication
type EnhancedAuthConfig struct {
	// JWT configuration
	JWT *JWTConfig `yaml:"jwt,omitempty"`
	
	// OAuth2 providers configuration
	OAuth2 map[string]*OAuth2Config `yaml:"oauth2,omitempty"`
	
	// Enable virtual keys
	EnableVirtualKeys bool `yaml:"enable_virtual_keys"`
	
	// Master API key
	MasterAPIKey string `yaml:"master_api_key"`
	
	// Default roles and permissions for new users
	DefaultRoles       []string `yaml:"default_roles"`
	DefaultPermissions []string `yaml:"default_permissions"`
}

// UnifiedAuthManager combines multiple authentication methods
type UnifiedAuthManager struct {
	virtualKeyManager VirtualKeyManager
	jwtManager        *JWTManager
	oauth2Manager     *OAuth2Manager
	masterKey         string
	logger            *zap.SugaredLogger
	config            *EnhancedAuthConfig
}

// EnhancedVirtualKeyManager extends the basic VirtualKeyManager interface
type EnhancedVirtualKeyManager interface {
	ValidateKey(ctx context.Context, key string) (*VirtualKey, error)
	CreateKey(ctx context.Context, request KeyRequest) (*VirtualKey, error)
	GetKey(ctx context.Context, keyID string) (*VirtualKey, error)
	ListKeys(ctx context.Context) ([]*VirtualKey, error)
	DeleteKey(ctx context.Context, keyID string) error
	UpdateUsage(ctx context.Context, keyID string, tokens int64, requests int64, cost float64) error
}

// NewUnifiedAuthManager creates a new unified authentication manager
func NewUnifiedAuthManager(
	config *EnhancedAuthConfig,
	virtualKeyManager VirtualKeyManager,
	logger *zap.SugaredLogger,
) (*UnifiedAuthManager, error) {
	manager := &UnifiedAuthManager{
		virtualKeyManager: virtualKeyManager,
		masterKey:         config.MasterAPIKey,
		logger:            logger,
		config:            config,
	}
	
	// Initialize JWT manager if configured
	if config.JWT != nil {
		manager.jwtManager = NewJWTManager(config.JWT, logger)
	}
	
	// Initialize OAuth2 manager if configured
	if config.OAuth2 != nil && len(config.OAuth2) > 0 {
		oauth2Manager, err := NewOAuth2Manager(config.OAuth2, manager.jwtManager, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize OAuth2 manager: %v", err)
		}
		manager.oauth2Manager = oauth2Manager
	}
	
	return manager, nil
}

// AuthenticateRequest authenticates a request using multiple methods
func (u *UnifiedAuthManager) AuthenticateRequest(r *http.Request) (*AuthContext, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, fmt.Errorf("authorization header required")
	}
	
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid authorization header format")
	}
	
	scheme := strings.ToLower(parts[0])
	token := parts[1]
	
	switch scheme {
	case "bearer":
		return u.authenticateBearer(token, r)
	case "basic":
		return nil, fmt.Errorf("basic authentication not supported")
	default:
		return nil, fmt.Errorf("unsupported authentication scheme: %s", scheme)
	}
}

// authenticateBearer handles Bearer token authentication
func (u *UnifiedAuthManager) authenticateBearer(token string, r *http.Request) (*AuthContext, error) {
	// Check if it's the master key
	if token == u.masterKey && u.masterKey != "" {
		return &AuthContext{
			UserID:   "master",
			TenantID: "",
			KeyID:    "master",
			Claims: map[string]interface{}{
				"method":      string(AuthMethodMaster),
				"username":    "master",
				"roles":       []string{"admin"},
				"permissions": []string{"*"},
			},
		}, nil
	}
	
	// Try JWT authentication
	if u.jwtManager != nil {
		if claims, err := u.jwtManager.ValidateToken(token); err == nil {
			return &AuthContext{
				UserID:   claims.UserID,
				TenantID: claims.TeamID,
				KeyID:    "",
				Claims: map[string]interface{}{
					"method":       string(AuthMethodJWT),
					"username":     claims.Username,
					"email":        claims.Email,
					"roles":        claims.Roles,
					"permissions":  claims.Permissions,
					"team_id":      claims.TeamID,
					"organization": claims.Organization,
					"jwt_claims":   claims,
				},
			}, nil
		}
	}
	
	// Try virtual key authentication
	if u.virtualKeyManager != nil && u.config.EnableVirtualKeys {
		// ValidateKey expects modelName, but we don't have it here, so pass empty string
		if vkm, ok := u.virtualKeyManager.(Manager); ok {
			if vkey, err := vkm.ValidateKey(r.Context(), token, ""); err == nil {
				return &AuthContext{
					UserID:   "virtual:" + vkey.ID,
					TenantID: "",
					KeyID:    vkey.ID,
					Claims: map[string]interface{}{
						"method":      string(AuthMethodVirtual),
						"username":    vkey.Name,
						"roles":       u.config.DefaultRoles,
						"permissions": u.config.DefaultPermissions,
						"virtual_key": vkey,
					},
				}, nil
			}
		}
	}
	
	return nil, fmt.Errorf("invalid token")
}

// ValidatePermission checks if the auth context has the required permission
func (u *UnifiedAuthManager) ValidatePermission(authCtx *AuthContext, permission string) bool {
	if authCtx == nil || authCtx.Claims == nil {
		return false
	}
	
	// Admin has all permissions
	if roles, ok := authCtx.Claims["roles"].([]string); ok {
		for _, role := range roles {
			if role == "admin" {
				return true
			}
		}
	}
	
	// Check wildcard permissions
	if permissions, ok := authCtx.Claims["permissions"].([]string); ok {
		for _, perm := range permissions {
			if perm == "*" {
				return true
			}
			if perm == permission {
				return true
			}
			
			// Check prefix match (e.g., "chat:*" matches "chat:create")
			if strings.HasSuffix(perm, ":*") {
				prefix := strings.TrimSuffix(perm, "*")
				if strings.HasPrefix(permission, prefix) {
					return true
				}
			}
		}
	}
	
	return false
}

// ValidateRole checks if the auth context has the required role
func (u *UnifiedAuthManager) ValidateRole(authCtx *AuthContext, role string) bool {
	if authCtx == nil || authCtx.Claims == nil {
		return false
	}
	
	if roles, ok := authCtx.Claims["roles"].([]string); ok {
		for _, r := range roles {
			if r == role {
				return true
			}
		}
	}
	return false
}

// GenerateJWTToken generates a JWT token for a user
func (u *UnifiedAuthManager) GenerateJWTToken(userID, email, username string, roles, permissions []string, teamID, organization string) (string, string, error) {
	if u.jwtManager == nil {
		return "", "", fmt.Errorf("JWT authentication not enabled")
	}
	
	return u.jwtManager.GenerateToken(userID, email, username, roles, permissions, teamID, organization)
}

// RefreshJWTToken refreshes a JWT token
func (u *UnifiedAuthManager) RefreshJWTToken(refreshToken string, roles, permissions []string, teamID, organization string) (string, error) {
	if u.jwtManager == nil {
		return "", fmt.Errorf("JWT authentication not enabled")
	}
	
	return u.jwtManager.RefreshToken(refreshToken, roles, permissions, teamID, organization)
}

// GetOAuth2AuthURL gets OAuth2 authorization URL
func (u *UnifiedAuthManager) GetOAuth2AuthURL(provider string) (string, error) {
	if u.oauth2Manager == nil {
		return "", fmt.Errorf("OAuth2 authentication not enabled")
	}
	
	state := u.oauth2Manager.GenerateStateToken()
	return u.oauth2Manager.GetAuthURL(provider, state)
}

// HandleOAuth2Callback handles OAuth2 callback
func (u *UnifiedAuthManager) HandleOAuth2Callback(provider, code string) (*OAuth2UserInfo, string, string, error) {
	if u.oauth2Manager == nil {
		return nil, "", "", fmt.Errorf("OAuth2 authentication not enabled")
	}
	
	// Exchange code for token
	token, err := u.oauth2Manager.ExchangeCode(provider, code)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to exchange code: %v", err)
	}
	
	// Get user info
	userInfo, err := u.oauth2Manager.GetUserInfo(provider, token)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get user info: %v", err)
	}
	
	// Generate JWT tokens
	userID := fmt.Sprintf("%s:%s", provider, userInfo.ID)
	accessToken, refreshToken, err := u.GenerateJWTToken(
		userID,
		userInfo.Email,
		userInfo.Username,
		u.config.DefaultRoles,
		u.config.DefaultPermissions,
		"", // teamID
		"", // organization
	)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to generate JWT tokens: %v", err)
	}
	
	return userInfo, accessToken, refreshToken, nil
}

// Middleware creates HTTP middleware for enhanced authentication
func (u *UnifiedAuthManager) Middleware() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			authCtx, err := u.AuthenticateRequest(r)
			if err != nil {
				u.logger.Debugw("Authentication failed", "error", err, "path", r.URL.Path)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Add auth context to request context
			ctx := context.WithValue(r.Context(), "auth_context", authCtx)
			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

// RequirePermission creates middleware that requires a specific permission
func (u *UnifiedAuthManager) RequirePermission(permission string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			authCtx, err := GetAuthContextFromContext(r.Context())
			if err != nil {
				u.logger.Debugw("No auth context found", "error", err)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if !u.ValidatePermission(authCtx, permission) {
				u.logger.Debugw("Permission denied", "user", authCtx.UserID, "permission", permission)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		}
	}
}

// RequireRole creates middleware that requires a specific role
func (u *UnifiedAuthManager) RequireRole(role string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			authCtx, err := GetAuthContextFromContext(r.Context())
			if err != nil {
				u.logger.Debugw("No auth context found", "error", err)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if !u.ValidateRole(authCtx, role) {
				u.logger.Debugw("Role denied", "user", authCtx.UserID, "role", role)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		}
	}
}

// RegisterAuthRoutes registers authentication routes
func (u *UnifiedAuthManager) RegisterAuthRoutes(mux *http.ServeMux) {
	// JWT routes
	if u.jwtManager != nil {
		mux.HandleFunc("/auth/jwt/login", u.jwtManager.HandleLogin)
		mux.HandleFunc("/auth/jwt/refresh", u.jwtManager.HandleRefresh)
	}
	
	// OAuth2 routes
	if u.oauth2Manager != nil {
		u.oauth2Manager.RegisterOAuth2Routes(mux)
	}
}

// Extract auth context from request context
func GetAuthContextFromContext(ctx context.Context) (*AuthContext, error) {
	authCtx, ok := ctx.Value("auth_context").(*AuthContext)
	if !ok {
		return nil, fmt.Errorf("no auth context found")
	}
	return authCtx, nil
}

// TrackVirtualKeyUsage tracks usage for virtual keys
func (u *UnifiedAuthManager) TrackVirtualKeyUsage(ctx context.Context, tokens int64, requests int64, cost float64) error {
	authCtx, err := GetAuthContextFromContext(ctx)
	if err != nil || authCtx.Claims == nil {
		// No auth context
		return nil
	}
	
	method, ok := authCtx.Claims["method"].(string)
	if !ok || method != string(AuthMethodVirtual) {
		// Not a virtual key
		return nil
	}
	
	virtualKey, ok := authCtx.Claims["virtual_key"].(*VirtualKey)
	if !ok || virtualKey == nil {
		// No virtual key in context
		return nil
	}

	if u.virtualKeyManager == nil {
		return fmt.Errorf("virtual key manager not available")
	}

	// UpdateUsage expects keyValue (not keyID) and doesn't take requests parameter
	if vkm, ok := u.virtualKeyManager.(Manager); ok {
		return vkm.UpdateUsage(ctx, virtualKey.Key, tokens, cost)
	}
	return fmt.Errorf("virtual key manager does not implement Manager interface")
}

// DefaultEnhancedAuthConfig returns a default enhanced auth configuration
func DefaultEnhancedAuthConfig() *EnhancedAuthConfig {
	return &EnhancedAuthConfig{
		EnableVirtualKeys:  true,
		DefaultRoles:       []string{"user"},
		DefaultPermissions: []string{"chat:create", "embedding:create", "image:create"},
		JWT: &JWTConfig{
			Algorithm:         "HS256",
			TokenExpiration:   time.Hour * 1,     // 1 hour
			RefreshExpiration: time.Hour * 24 * 7, // 7 days
			Issuer:            "ogem-proxy",
			Audience:          "ogem-api",
		},
	}
}
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/microsoft"
)

// OAuth2Config represents OAuth2 provider configuration
type OAuth2Config struct {
	// Provider type (google, microsoft, github, generic)
	Provider string `yaml:"provider"`
	
	// OAuth2 client credentials
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	
	// OAuth2 endpoints (for generic providers)
	AuthURL     string `yaml:"auth_url,omitempty"`
	TokenURL    string `yaml:"token_url,omitempty"`
	UserInfoURL string `yaml:"user_info_url,omitempty"`
	
	// OAuth2 scopes
	Scopes []string `yaml:"scopes"`
	
	// Redirect URL
	RedirectURL string `yaml:"redirect_url"`
	
	// Additional parameters
	Params map[string]string `yaml:"params,omitempty"`
}

// OAuth2Manager handles OAuth2 authentication flows
type OAuth2Manager struct {
	configs map[string]*oauth2.Config
	logger  *zap.SugaredLogger
	jwtManager *JWTManager
}

// OAuth2UserInfo represents user information from OAuth2 provider
type OAuth2UserInfo struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Username string `json:"login,omitempty"`
	Picture  string `json:"picture,omitempty"`
	Provider string `json:"-"`
}

// NewOAuth2Manager creates a new OAuth2 manager
func NewOAuth2Manager(configs map[string]*OAuth2Config, jwtManager *JWTManager, logger *zap.SugaredLogger) (*OAuth2Manager, error) {
	oauth2Configs := make(map[string]*oauth2.Config)
	
	for name, config := range configs {
		oauth2Config, err := createOAuth2Config(config)
		if err != nil {
			return nil, fmt.Errorf("failed to create OAuth2 config for %s: %v", name, err)
		}
		oauth2Configs[name] = oauth2Config
	}
	
	return &OAuth2Manager{
		configs: oauth2Configs,
		logger:  logger,
		jwtManager: jwtManager,
	}, nil
}

// createOAuth2Config creates oauth2.Config from OAuth2Config
func createOAuth2Config(config *OAuth2Config) (*oauth2.Config, error) {
	oauth2Config := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURL,
		Scopes:       config.Scopes,
	}
	
	switch strings.ToLower(config.Provider) {
	case "google":
		oauth2Config.Endpoint = google.Endpoint
		if len(oauth2Config.Scopes) == 0 {
			oauth2Config.Scopes = []string{"openid", "email", "profile"}
		}
	case "microsoft":
		oauth2Config.Endpoint = microsoft.AzureADEndpoint("")
		if len(oauth2Config.Scopes) == 0 {
			oauth2Config.Scopes = []string{"openid", "email", "profile"}
		}
	case "github":
		oauth2Config.Endpoint = oauth2.Endpoint{
			AuthURL:  "https://github.com/login/oauth/authorize",
			TokenURL: "https://github.com/login/oauth/access_token",
		}
		if len(oauth2Config.Scopes) == 0 {
			oauth2Config.Scopes = []string{"user:email"}
		}
	case "generic":
		if config.AuthURL == "" || config.TokenURL == "" {
			return nil, fmt.Errorf("auth_url and token_url are required for generic provider")
		}
		oauth2Config.Endpoint = oauth2.Endpoint{
			AuthURL:  config.AuthURL,
			TokenURL: config.TokenURL,
		}
	default:
		return nil, fmt.Errorf("unsupported OAuth2 provider: %s", config.Provider)
	}
	
	return oauth2Config, nil
}

// GenerateStateToken generates a secure state token for OAuth2 flow
func (o *OAuth2Manager) GenerateStateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// GetAuthURL generates OAuth2 authorization URL
func (o *OAuth2Manager) GetAuthURL(provider, state string) (string, error) {
	config, exists := o.configs[provider]
	if !exists {
		return "", fmt.Errorf("OAuth2 provider %s not configured", provider)
	}
	
	return config.AuthCodeURL(state, oauth2.AccessTypeOffline), nil
}

// ExchangeCode exchanges authorization code for access token
func (o *OAuth2Manager) ExchangeCode(provider, code string) (*oauth2.Token, error) {
	config, exists := o.configs[provider]
	if !exists {
		return nil, fmt.Errorf("OAuth2 provider %s not configured", provider)
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	token, err := config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %v", err)
	}
	
	return token, nil
}

// GetUserInfo fetches user information from OAuth2 provider
func (o *OAuth2Manager) GetUserInfo(provider string, token *oauth2.Token) (*OAuth2UserInfo, error) {
	config, exists := o.configs[provider]
	if !exists {
		return nil, fmt.Errorf("OAuth2 provider %s not configured", provider)
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	client := config.Client(ctx, token)
	
	var userInfoURL string
	switch strings.ToLower(provider) {
	case "google":
		userInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"
	case "microsoft":
		userInfoURL = "https://graph.microsoft.com/v1.0/me"
	case "github":
		userInfoURL = "https://api.github.com/user"
	default:
		// For generic providers, this should be configured
		return nil, fmt.Errorf("user info URL not configured for provider %s", provider)
	}
	
	resp, err := client.Get(userInfoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user info: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch user info: HTTP %d", resp.StatusCode)
	}
	
	var userInfo OAuth2UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %v", err)
	}
	
	userInfo.Provider = provider
	
	// Handle provider-specific field mappings
	switch strings.ToLower(provider) {
	case "github":
		if userInfo.Username == "" {
			userInfo.Username = userInfo.Name
		}
	case "microsoft":
		if userInfo.Username == "" {
			userInfo.Username = userInfo.Email
		}
	}
	
	return &userInfo, nil
}

// HandleOAuth2Login handles OAuth2 login initiation
func (o *OAuth2Manager) HandleOAuth2Login(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		http.Error(w, "Provider parameter required", http.StatusBadRequest)
		return
	}
	
	state := o.GenerateStateToken()
	authURL, err := o.GetAuthURL(provider, state)
	if err != nil {
		o.logger.Errorw("Failed to generate auth URL", "provider", provider, "error", err)
		http.Error(w, "Invalid provider", http.StatusBadRequest)
		return
	}
	
	// Store state in session/cookie for verification
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600, // 10 minutes
	})
	
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// HandleOAuth2Callback handles OAuth2 callback
func (o *OAuth2Manager) HandleOAuth2Callback(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		http.Error(w, "Provider parameter required", http.StatusBadRequest)
		return
	}
	
	// Verify state parameter
	state := r.URL.Query().Get("state")
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != state {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}
	
	// Clear state cookie
	http.SetCookie(w, &http.Cookie{
		Name:   "oauth_state",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	
	// Check for error in callback
	if errorParam := r.URL.Query().Get("error"); errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		o.logger.Errorw("OAuth2 callback error", "error", errorParam, "description", errorDesc)
		http.Error(w, "OAuth2 authentication failed: "+errorParam, http.StatusBadRequest)
		return
	}
	
	// Exchange code for token
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Authorization code required", http.StatusBadRequest)
		return
	}
	
	token, err := o.ExchangeCode(provider, code)
	if err != nil {
		o.logger.Errorw("Failed to exchange code", "provider", provider, "error", err)
		http.Error(w, "Failed to exchange authorization code", http.StatusInternalServerError)
		return
	}
	
	// Get user information
	userInfo, err := o.GetUserInfo(provider, token)
	if err != nil {
		o.logger.Errorw("Failed to get user info", "provider", provider, "error", err)
		http.Error(w, "Failed to get user information", http.StatusInternalServerError)
		return
	}
	
	// Generate JWT tokens
	userID := fmt.Sprintf("%s:%s", provider, userInfo.ID)
	roles := []string{"user"} // Default role
	permissions := []string{"chat:create", "embedding:create"} // Default permissions
	
	// TODO: Integrate with user management system to get proper roles and permissions
	
	accessToken, refreshToken, err := o.jwtManager.GenerateToken(
		userID,
		userInfo.Email,
		userInfo.Username,
		roles,
		permissions,
		"", // teamID
		"", // organization
	)
	if err != nil {
		o.logger.Errorw("Failed to generate JWT tokens", "error", err)
		http.Error(w, "Failed to generate tokens", http.StatusInternalServerError)
		return
	}
	
	// Return tokens as JSON
	response := TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(o.jwtManager.config.TokenExpiration.Seconds()),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// RegisterOAuth2Routes registers OAuth2 routes
func (o *OAuth2Manager) RegisterOAuth2Routes(mux *http.ServeMux) {
	mux.HandleFunc("/auth/oauth2/login", o.HandleOAuth2Login)
	mux.HandleFunc("/auth/oauth2/callback", o.HandleOAuth2Callback)
}

// ValidateOAuth2Token validates an OAuth2 access token with the provider
func (o *OAuth2Manager) ValidateOAuth2Token(provider string, tokenString string) (*OAuth2UserInfo, error) {
	// Create a token object
	token := &oauth2.Token{
		AccessToken: tokenString,
		TokenType:   "Bearer",
	}
	
	// Get user info to validate token
	userInfo, err := o.GetUserInfo(provider, token)
	if err != nil {
		return nil, fmt.Errorf("invalid OAuth2 token: %v", err)
	}
	
	return userInfo, nil
}
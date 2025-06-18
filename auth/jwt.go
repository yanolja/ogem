package auth

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// JWT Configuration
type JWTConfig struct {
	// JWT signing key (RS256 or HS256)
	SigningKey interface{}
	
	// JWT verification key (for RS256, this is the public key)
	VerificationKey interface{}
	
	// JWT algorithm (RS256, HS256, etc.)
	Algorithm string
	
	// Token expiration duration
	TokenExpiration time.Duration
	
	// Refresh token expiration
	RefreshExpiration time.Duration
	
	// JWT issuer
	Issuer string
	
	// JWT audience
	Audience string
}

// JWTManager handles JWT token operations
type JWTManager struct {
	config *JWTConfig
	logger *zap.SugaredLogger
}

// JWTClaims represents the structure of JWT claims
type JWTClaims struct {
	UserID       string   `json:"user_id"`
	Email        string   `json:"email"`
	Username     string   `json:"username"`
	Roles        []string `json:"roles"`
	Permissions  []string `json:"permissions"`
	TeamID       string   `json:"team_id,omitempty"`
	Organization string   `json:"organization,omitempty"`
	jwt.RegisteredClaims
}

// NewJWTManager creates a new JWT manager
func NewJWTManager(config *JWTConfig, logger *zap.SugaredLogger) *JWTManager {
	return &JWTManager{
		config: config,
		logger: logger,
	}
}

// GenerateToken creates a new JWT token for the user
func (j *JWTManager) GenerateToken(userID, email, username string, roles, permissions []string, teamID, organization string) (string, string, error) {
	now := time.Now()
	
	// Access token claims
	accessClaims := &JWTClaims{
		UserID:       userID,
		Email:        email,
		Username:     username,
		Roles:        roles,
		Permissions:  permissions,
		TeamID:       teamID,
		Organization: organization,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    j.config.Issuer,
			Subject:   userID,
			Audience:  []string{j.config.Audience},
			ExpiresAt: jwt.NewNumericDate(now.Add(j.config.TokenExpiration)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	
	// Refresh token claims (minimal)
	refreshClaims := &JWTClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    j.config.Issuer,
			Subject:   userID,
			Audience:  []string{j.config.Audience},
			ExpiresAt: jwt.NewNumericDate(now.Add(j.config.RefreshExpiration)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	
	// Create tokens
	var accessMethod, refreshMethod jwt.SigningMethod
	switch j.config.Algorithm {
	case "RS256":
		accessMethod = jwt.SigningMethodRS256
		refreshMethod = jwt.SigningMethodRS256
	case "HS256":
		accessMethod = jwt.SigningMethodHS256
		refreshMethod = jwt.SigningMethodHS256
	default:
		return "", "", fmt.Errorf("unsupported JWT algorithm: %s", j.config.Algorithm)
	}
	
	accessToken := jwt.NewWithClaims(accessMethod, accessClaims)
	refreshToken := jwt.NewWithClaims(refreshMethod, refreshClaims)
	
	accessTokenString, err := accessToken.SignedString(j.config.SigningKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to sign access token: %v", err)
	}
	
	refreshTokenString, err := refreshToken.SignedString(j.config.SigningKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to sign refresh token: %v", err)
	}
	
	return accessTokenString, refreshTokenString, nil
}

// ValidateToken validates and parses a JWT token
func (j *JWTManager) ValidateToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate algorithm
		switch j.config.Algorithm {
		case "RS256":
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return j.config.VerificationKey, nil
		case "HS256":
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return j.config.SigningKey, nil
		default:
			return nil, fmt.Errorf("unsupported JWT algorithm: %s", j.config.Algorithm)
		}
	})
	
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %v", err)
	}
	
	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	
	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}
	
	// Validate issuer and audience
	if claims.Issuer != j.config.Issuer {
		return nil, fmt.Errorf("invalid issuer")
	}
	
	validAudience := false
	for _, aud := range claims.Audience {
		if aud == j.config.Audience {
			validAudience = true
			break
		}
	}
	if !validAudience {
		return nil, fmt.Errorf("invalid audience")
	}
	
	return claims, nil
}

// RefreshToken generates a new access token from a valid refresh token
func (j *JWTManager) RefreshToken(refreshTokenString string, roles, permissions []string, teamID, organization string) (string, error) {
	claims, err := j.ValidateToken(refreshTokenString)
	if err != nil {
		return "", fmt.Errorf("invalid refresh token: %v", err)
	}
	
	// Generate new access token
	accessToken, _, err := j.GenerateToken(claims.UserID, claims.Email, claims.Username, roles, permissions, teamID, organization)
	if err != nil {
		return "", fmt.Errorf("failed to generate new access token: %v", err)
	}
	
	return accessToken, nil
}

// ExtractTokenFromRequest extracts JWT token from HTTP request
func (j *JWTManager) ExtractTokenFromRequest(r *http.Request) (string, error) {
	// Check Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			return parts[1], nil
		}
	}
	
	// Check cookie
	cookie, err := r.Cookie("access_token")
	if err == nil && cookie.Value != "" {
		return cookie.Value, nil
	}
	
	// Check query parameter
	token := r.URL.Query().Get("access_token")
	if token != "" {
		return token, nil
	}
	
	return "", fmt.Errorf("no token found in request")
}

// JWTMiddleware creates HTTP middleware for JWT authentication
func (j *JWTManager) JWTMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, err := j.ExtractTokenFromRequest(r)
		if err != nil {
			http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
			return
		}
		
		claims, err := j.ValidateToken(token)
		if err != nil {
			http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
			return
		}
		
		// Add claims to request context
		ctx := context.WithValue(r.Context(), "jwt_claims", claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// GetClaimsFromContext extracts JWT claims from request context
func GetClaimsFromContext(ctx context.Context) (*JWTClaims, error) {
	claims, ok := ctx.Value("jwt_claims").(*JWTClaims)
	if !ok {
		return nil, fmt.Errorf("no JWT claims found in context")
	}
	return claims, nil
}

// TokenResponse represents the response structure for token endpoints
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

// LoginRequest represents login request structure
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email,omitempty"`
}

// RefreshRequest represents refresh token request
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// HandleLogin handles user login and token generation
func (j *JWTManager) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var loginReq LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	// TODO: Integrate with actual user authentication
	// For now, this is a placeholder that accepts any credentials
	userID := "user_" + loginReq.Username
	email := loginReq.Email
	if email == "" {
		email = loginReq.Username + "@example.com"
	}
	
	// Default permissions for demo
	roles := []string{"user"}
	permissions := []string{"chat:create", "embedding:create"}
	
	accessToken, refreshToken, err := j.GenerateToken(userID, email, loginReq.Username, roles, permissions, "", "")
	if err != nil {
		j.logger.Errorw("Failed to generate tokens", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	
	response := TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(j.config.TokenExpiration.Seconds()),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleRefresh handles token refresh
func (j *JWTManager) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var refreshReq RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&refreshReq); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	// TODO: Fetch current user roles and permissions from database
	roles := []string{"user"}
	permissions := []string{"chat:create", "embedding:create"}
	
	accessToken, err := j.RefreshToken(refreshReq.RefreshToken, roles, permissions, "", "")
	if err != nil {
		j.logger.Errorw("Failed to refresh token", "error", err)
		http.Error(w, "Invalid refresh token", http.StatusUnauthorized)
		return
	}
	
	response := TokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int64(j.config.TokenExpiration.Seconds()),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// LoadRSAKeysFromFiles loads RSA keys from PEM files
func LoadRSAKeysFromFiles(privateKeyPath, publicKeyPath string) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	// TODO: Implement RSA key loading from files
	return nil, nil, fmt.Errorf("RSA key loading not implemented")
}

// DefaultJWTConfig returns a default JWT configuration
func DefaultJWTConfig() *JWTConfig {
	return &JWTConfig{
		Algorithm:         "HS256",
		TokenExpiration:   time.Hour * 1,     // 1 hour
		RefreshExpiration: time.Hour * 24 * 7, // 7 days
		Issuer:            "ogem-proxy",
		Audience:          "ogem-api",
	}
}
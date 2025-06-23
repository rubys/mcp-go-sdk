package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AuthInfo represents authentication information for a token
type AuthInfo struct {
	Token     string    `json:"token"`
	ClientID  string    `json:"client_id"`
	Scopes    []string  `json:"scopes"`
	ExpiresAt *int64    `json:"expires_at,omitempty"`
}

// TokenVerifier interface for verifying access tokens
type TokenVerifier interface {
	VerifyAccessToken(ctx context.Context, token string) (*AuthInfo, error)
}

// AuthError represents authentication/authorization errors
type AuthError struct {
	Code        string `json:"error"`
	Description string `json:"error_description"`
	HTTPStatus  int    `json:"-"`
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Description)
}

// Predefined auth errors
var (
	ErrInvalidToken      = &AuthError{"invalid_token", "", 401}
	ErrInsufficientScope = &AuthError{"insufficient_scope", "Insufficient scope", 403}
	ErrServerError       = &AuthError{"server_error", "Internal Server Error", 500}
)

// BearerAuthConfig configures bearer token authentication middleware
type BearerAuthConfig struct {
	Verifier            TokenVerifier
	RequiredScopes      []string
	ResourceMetadataURL string
}

// MockTokenVerifier for testing
type MockTokenVerifier struct {
	AuthInfoMap map[string]*AuthInfo
	ErrorMap    map[string]*AuthError
}

func NewMockTokenVerifier() *MockTokenVerifier {
	return &MockTokenVerifier{
		AuthInfoMap: make(map[string]*AuthInfo),
		ErrorMap:    make(map[string]*AuthError),
	}
}

func (m *MockTokenVerifier) VerifyAccessToken(ctx context.Context, token string) (*AuthInfo, error) {
	if err, exists := m.ErrorMap[token]; exists {
		return nil, err
	}
	if info, exists := m.AuthInfoMap[token]; exists {
		return info, nil
	}
	return nil, &AuthError{"invalid_token", "Token not found", 401}
}

func (m *MockTokenVerifier) SetValidToken(token string, info *AuthInfo) {
	m.AuthInfoMap[token] = info
}

func (m *MockTokenVerifier) SetTokenError(token string, err *AuthError) {
	m.ErrorMap[token] = err
}

// RequireBearerAuth creates middleware that validates bearer tokens
func RequireBearerAuth(config BearerAuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract Authorization header
			authHeader := r.Header.Get("Authorization")
			
			if authHeader == "" {
				writeAuthError(w, &AuthError{"invalid_token", "Missing Authorization header", 401}, config.ResourceMetadataURL)
				return
			}

			// Check Bearer format
			if !strings.HasPrefix(authHeader, "Bearer ") || len(authHeader) < 8 {
				writeAuthError(w, &AuthError{"invalid_token", "Invalid Authorization header format, expected 'Bearer TOKEN'", 401}, config.ResourceMetadataURL)
				return
			}

			token := authHeader[7:] // Remove "Bearer " prefix

			// Verify token
			authInfo, err := config.Verifier.VerifyAccessToken(r.Context(), token)
			if err != nil {
				if authErr, ok := err.(*AuthError); ok {
					writeAuthError(w, authErr, config.ResourceMetadataURL)
				} else {
					writeAuthError(w, &AuthError{"server_error", "Internal Server Error", 500}, config.ResourceMetadataURL)
				}
				return
			}

			// Check token expiration
			if authInfo.ExpiresAt != nil && *authInfo.ExpiresAt < time.Now().Unix() {
				writeAuthError(w, &AuthError{"invalid_token", "Token has expired", 401}, config.ResourceMetadataURL)
				return
			}

			// Check required scopes
			if len(config.RequiredScopes) > 0 {
				if !hasRequiredScopes(authInfo.Scopes, config.RequiredScopes) {
					writeAuthError(w, &AuthError{"insufficient_scope", "Insufficient scope", 403}, config.ResourceMetadataURL)
					return
				}
			}

			// Store auth info in context
			ctx := context.WithValue(r.Context(), "auth", authInfo)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// hasRequiredScopes checks if user has all required scopes
func hasRequiredScopes(userScopes, requiredScopes []string) bool {
	scopeMap := make(map[string]bool)
	for _, scope := range userScopes {
		scopeMap[scope] = true
	}
	
	for _, required := range requiredScopes {
		if !scopeMap[required] {
			return false
		}
	}
	return true
}

// writeAuthError writes authentication error response with proper headers
func writeAuthError(w http.ResponseWriter, authErr *AuthError, resourceMetadataURL string) {
	w.Header().Set("Content-Type", "application/json")
	
	// Set WWW-Authenticate header for 401/403 errors
	if authErr.HTTPStatus == 401 || authErr.HTTPStatus == 403 {
		wwwAuth := fmt.Sprintf("Bearer error=\"%s\", error_description=\"%s\"", authErr.Code, authErr.Description)
		if resourceMetadataURL != "" {
			wwwAuth += fmt.Sprintf(", resource_metadata=\"%s\"", resourceMetadataURL)
		}
		w.Header().Set("WWW-Authenticate", wwwAuth)
	}
	
	w.WriteHeader(authErr.HTTPStatus)
	response := fmt.Sprintf(`{"error":"%s","error_description":"%s"}`, authErr.Code, authErr.Description)
	w.Write([]byte(response))
}

// Test helper to extract auth info from request context
func GetAuthInfo(r *http.Request) *AuthInfo {
	if auth := r.Context().Value("auth"); auth != nil {
		if authInfo, ok := auth.(*AuthInfo); ok {
			return authInfo
		}
	}
	return nil
}

func TestRequireBearerAuth_ValidToken(t *testing.T) {
	mockVerifier := NewMockTokenVerifier()
	validAuthInfo := &AuthInfo{
		Token:    "valid-token",
		ClientID: "client-123",
		Scopes:   []string{"read", "write"},
	}
	mockVerifier.SetValidToken("valid-token", validAuthInfo)

	config := BearerAuthConfig{
		Verifier: mockVerifier,
	}

	middleware := RequireBearerAuth(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authInfo := GetAuthInfo(r)
		require.NotNil(t, authInfo)
		assert.Equal(t, "valid-token", authInfo.Token)
		assert.Equal(t, "client-123", authInfo.ClientID)
		assert.Equal(t, []string{"read", "write"}, authInfo.Scopes)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "success", w.Body.String())
}

func TestRequireBearerAuth_ExpiredToken(t *testing.T) {
	mockVerifier := NewMockTokenVerifier()
	expiredTime := time.Now().Unix() - 100 // 100 seconds ago
	expiredAuthInfo := &AuthInfo{
		Token:     "expired-token",
		ClientID:  "client-123",
		Scopes:    []string{"read", "write"},
		ExpiresAt: &expiredTime,
	}
	mockVerifier.SetValidToken("expired-token", expiredAuthInfo)

	config := BearerAuthConfig{
		Verifier: mockVerifier,
	}

	middleware := RequireBearerAuth(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for expired token")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer expired-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Bearer error=\"invalid_token\"")
	assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Token has expired")
	assert.Contains(t, w.Body.String(), "invalid_token")
	assert.Contains(t, w.Body.String(), "Token has expired")
}

func TestRequireBearerAuth_NonExpiredToken(t *testing.T) {
	mockVerifier := NewMockTokenVerifier()
	futureTime := time.Now().Unix() + 3600 // 1 hour from now
	validAuthInfo := &AuthInfo{
		Token:     "valid-token",
		ClientID:  "client-123",
		Scopes:    []string{"read", "write"},
		ExpiresAt: &futureTime,
	}
	mockVerifier.SetValidToken("valid-token", validAuthInfo)

	config := BearerAuthConfig{
		Verifier: mockVerifier,
	}

	middleware := RequireBearerAuth(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authInfo := GetAuthInfo(r)
		require.NotNil(t, authInfo)
		assert.Equal(t, "valid-token", authInfo.Token)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireBearerAuth_RequiredScopes_Insufficient(t *testing.T) {
	mockVerifier := NewMockTokenVerifier()
	authInfo := &AuthInfo{
		Token:    "valid-token",
		ClientID: "client-123",
		Scopes:   []string{"read"}, // Only has read, not write
	}
	mockVerifier.SetValidToken("valid-token", authInfo)

	config := BearerAuthConfig{
		Verifier:       mockVerifier,
		RequiredScopes: []string{"read", "write"},
	}

	middleware := RequireBearerAuth(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for insufficient scopes")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Bearer error=\"insufficient_scope\"")
	assert.Contains(t, w.Body.String(), "insufficient_scope")
}

func TestRequireBearerAuth_RequiredScopes_Sufficient(t *testing.T) {
	mockVerifier := NewMockTokenVerifier()
	authInfo := &AuthInfo{
		Token:    "valid-token",
		ClientID: "client-123",
		Scopes:   []string{"read", "write", "admin"},
	}
	mockVerifier.SetValidToken("valid-token", authInfo)

	config := BearerAuthConfig{
		Verifier:       mockVerifier,
		RequiredScopes: []string{"read", "write"},
	}

	middleware := RequireBearerAuth(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authInfo := GetAuthInfo(r)
		require.NotNil(t, authInfo)
		assert.Equal(t, []string{"read", "write", "admin"}, authInfo.Scopes)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireBearerAuth_MissingAuthorizationHeader(t *testing.T) {
	mockVerifier := NewMockTokenVerifier()
	config := BearerAuthConfig{
		Verifier: mockVerifier,
	}

	middleware := RequireBearerAuth(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called without authorization header")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Bearer error=\"invalid_token\"")
	assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Missing Authorization header")
	assert.Contains(t, w.Body.String(), "invalid_token")
	assert.Contains(t, w.Body.String(), "Missing Authorization header")
}

func TestRequireBearerAuth_InvalidAuthorizationHeaderFormat(t *testing.T) {
	mockVerifier := NewMockTokenVerifier()
	config := BearerAuthConfig{
		Verifier: mockVerifier,
	}

	middleware := RequireBearerAuth(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called with invalid authorization header")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "InvalidFormat")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Bearer error=\"invalid_token\"")
	assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Invalid Authorization header format, expected 'Bearer TOKEN'")
	assert.Contains(t, w.Body.String(), "invalid_token")
}

func TestRequireBearerAuth_TokenVerificationFails(t *testing.T) {
	mockVerifier := NewMockTokenVerifier()
	mockVerifier.SetTokenError("invalid-token", &AuthError{"invalid_token", "Token expired", 401})

	config := BearerAuthConfig{
		Verifier: mockVerifier,
	}

	middleware := RequireBearerAuth(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for invalid token")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Bearer error=\"invalid_token\"")
	assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Token expired")
	assert.Contains(t, w.Body.String(), "Token expired")
}

func TestRequireBearerAuth_InsufficientScopeError(t *testing.T) {
	mockVerifier := NewMockTokenVerifier()
	mockVerifier.SetTokenError("valid-token", &AuthError{"insufficient_scope", "Required scopes: read, write", 403})

	config := BearerAuthConfig{
		Verifier: mockVerifier,
	}

	middleware := RequireBearerAuth(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for insufficient scope")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Bearer error=\"insufficient_scope\"")
	assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Required scopes: read, write")
	assert.Contains(t, w.Body.String(), "insufficient_scope")
}

func TestRequireBearerAuth_ServerError(t *testing.T) {
	mockVerifier := NewMockTokenVerifier()
	mockVerifier.SetTokenError("valid-token", &AuthError{"server_error", "Internal server issue", 500})

	config := BearerAuthConfig{
		Verifier: mockVerifier,
	}

	middleware := RequireBearerAuth(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for server error")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Empty(t, w.Header().Get("WWW-Authenticate")) // No WWW-Authenticate for 500 errors
	assert.Contains(t, w.Body.String(), "server_error")
	assert.Contains(t, w.Body.String(), "Internal server issue")
}

func TestRequireBearerAuth_UnexpectedError(t *testing.T) {
	// Create a verifier that returns a non-AuthError
	mockVerifier := &UnexpectedErrorVerifier{}

	config := BearerAuthConfig{
		Verifier: mockVerifier,
	}

	middleware := RequireBearerAuth(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for unexpected error")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer any-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "server_error")
	assert.Contains(t, w.Body.String(), "Internal Server Error")
}

// UnexpectedErrorVerifier for testing unexpected errors
type UnexpectedErrorVerifier struct{}

func (u *UnexpectedErrorVerifier) VerifyAccessToken(ctx context.Context, token string) (*AuthInfo, error) {
	return nil, fmt.Errorf("unexpected database error")
}

func TestRequireBearerAuth_WithResourceMetadataURL(t *testing.T) {
	mockVerifier := NewMockTokenVerifier()
	config := BearerAuthConfig{
		Verifier:            mockVerifier,
		ResourceMetadataURL: "https://api.example.com/.well-known/oauth-protected-resource",
	}

	middleware := RequireBearerAuth(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called without authorization")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	wwwAuth := w.Header().Get("WWW-Authenticate")
	assert.Contains(t, wwwAuth, "Bearer error=\"invalid_token\"")
	assert.Contains(t, wwwAuth, "Missing Authorization header")
	assert.Contains(t, wwwAuth, "resource_metadata=\"https://api.example.com/.well-known/oauth-protected-resource\"")
}

func TestRequireBearerAuth_ResourceMetadataWithTokenError(t *testing.T) {
	mockVerifier := NewMockTokenVerifier()
	mockVerifier.SetTokenError("invalid-token", &AuthError{"invalid_token", "Token expired", 401})

	config := BearerAuthConfig{
		Verifier:            mockVerifier,
		ResourceMetadataURL: "https://api.example.com/.well-known/oauth-protected-resource",
	}

	middleware := RequireBearerAuth(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for invalid token")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	wwwAuth := w.Header().Get("WWW-Authenticate")
	assert.Contains(t, wwwAuth, "Bearer error=\"invalid_token\"")
	assert.Contains(t, wwwAuth, "Token expired")
	assert.Contains(t, wwwAuth, "resource_metadata=\"https://api.example.com/.well-known/oauth-protected-resource\"")
}

func TestRequireBearerAuth_ResourceMetadataWithInsufficientScope(t *testing.T) {
	mockVerifier := NewMockTokenVerifier()
	mockVerifier.SetTokenError("valid-token", &AuthError{"insufficient_scope", "Required scopes: admin", 403})

	config := BearerAuthConfig{
		Verifier:            mockVerifier,
		ResourceMetadataURL: "https://api.example.com/.well-known/oauth-protected-resource",
	}

	middleware := RequireBearerAuth(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for insufficient scope")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	wwwAuth := w.Header().Get("WWW-Authenticate")
	assert.Contains(t, wwwAuth, "Bearer error=\"insufficient_scope\"")
	assert.Contains(t, wwwAuth, "Required scopes: admin")
	assert.Contains(t, wwwAuth, "resource_metadata=\"https://api.example.com/.well-known/oauth-protected-resource\"")
}

func TestRequireBearerAuth_ResourceMetadataWithExpiredToken(t *testing.T) {
	mockVerifier := NewMockTokenVerifier()
	expiredTime := time.Now().Unix() - 100
	expiredAuthInfo := &AuthInfo{
		Token:     "expired-token",
		ClientID:  "client-123",
		Scopes:    []string{"read", "write"},
		ExpiresAt: &expiredTime,
	}
	mockVerifier.SetValidToken("expired-token", expiredAuthInfo)

	config := BearerAuthConfig{
		Verifier:            mockVerifier,
		ResourceMetadataURL: "https://api.example.com/.well-known/oauth-protected-resource",
	}

	middleware := RequireBearerAuth(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for expired token")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer expired-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	wwwAuth := w.Header().Get("WWW-Authenticate")
	assert.Contains(t, wwwAuth, "Bearer error=\"invalid_token\"")
	assert.Contains(t, wwwAuth, "Token has expired")
	assert.Contains(t, wwwAuth, "resource_metadata=\"https://api.example.com/.well-known/oauth-protected-resource\"")
}

func TestRequireBearerAuth_ResourceMetadataWithScopeCheck(t *testing.T) {
	mockVerifier := NewMockTokenVerifier()
	authInfo := &AuthInfo{
		Token:    "valid-token",
		ClientID: "client-123",
		Scopes:   []string{"read"},
	}
	mockVerifier.SetValidToken("valid-token", authInfo)

	config := BearerAuthConfig{
		Verifier:            mockVerifier,
		RequiredScopes:      []string{"read", "write"},
		ResourceMetadataURL: "https://api.example.com/.well-known/oauth-protected-resource",
	}

	middleware := RequireBearerAuth(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for insufficient scope")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	wwwAuth := w.Header().Get("WWW-Authenticate")
	assert.Contains(t, wwwAuth, "Bearer error=\"insufficient_scope\"")
	assert.Contains(t, wwwAuth, "Insufficient scope")
	assert.Contains(t, wwwAuth, "resource_metadata=\"https://api.example.com/.well-known/oauth-protected-resource\"")
}

func TestRequireBearerAuth_ResourceMetadataNotAffectedByServerErrors(t *testing.T) {
	mockVerifier := NewMockTokenVerifier()
	mockVerifier.SetTokenError("valid-token", &AuthError{"server_error", "Internal server issue", 500})

	config := BearerAuthConfig{
		Verifier:            mockVerifier,
		ResourceMetadataURL: "https://api.example.com/.well-known/oauth-protected-resource",
	}

	middleware := RequireBearerAuth(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for server error")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Empty(t, w.Header().Get("WWW-Authenticate")) // No WWW-Authenticate header for server errors
	assert.Contains(t, w.Body.String(), "server_error")
}

// Test concurrent access to verify thread safety
func TestRequireBearerAuth_ConcurrentAccess(t *testing.T) {
	mockVerifier := NewMockTokenVerifier()
	validAuthInfo := &AuthInfo{
		Token:    "valid-token",
		ClientID: "client-123",
		Scopes:   []string{"read", "write"},
	}
	mockVerifier.SetValidToken("valid-token", validAuthInfo)

	config := BearerAuthConfig{
		Verifier: mockVerifier,
	}

	middleware := RequireBearerAuth(config)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authInfo := GetAuthInfo(r)
		require.NotNil(t, authInfo)
		w.WriteHeader(http.StatusOK)
	}))

	// Test concurrent requests
	const numRequests = 100
	results := make(chan int, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", "Bearer valid-token")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)
			results <- w.Code
		}()
	}

	// Collect all results
	for i := 0; i < numRequests; i++ {
		code := <-results
		assert.Equal(t, http.StatusOK, code)
	}
}
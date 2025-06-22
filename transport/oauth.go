package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	ErrOAuthAuthorizationRequired = errors.New("OAuth authorization required")
	ErrInvalidState               = errors.New("invalid state parameter")
	ErrTokenNotFound              = errors.New("token not found")
)

// Token represents an OAuth 2.0 token
type Token struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresIn    int       `json:"expires_in,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	Scope        string    `json:"scope,omitempty"`
}

// IsExpired returns true if the token is expired
func (t *Token) IsExpired() bool {
	if t.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(t.ExpiresAt)
}

// TokenStore defines the interface for storing and retrieving OAuth tokens
type TokenStore interface {
	SaveToken(token *Token) error
	GetToken() (*Token, error)
}

// MemoryTokenStore is an in-memory implementation of TokenStore
type MemoryTokenStore struct {
	mu    sync.RWMutex
	token *Token
}

// NewMemoryTokenStore creates a new in-memory token store
func NewMemoryTokenStore() *MemoryTokenStore {
	return &MemoryTokenStore{}
}

// SaveToken saves a token to memory
func (s *MemoryTokenStore) SaveToken(token *Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = token
	return nil
}

// GetToken retrieves a token from memory
func (s *MemoryTokenStore) GetToken() (*Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.token == nil {
		return nil, ErrTokenNotFound
	}
	return s.token, nil
}

// OAuthConfig contains OAuth configuration
type OAuthConfig struct {
	ClientID              string
	ClientSecret          string
	RedirectURI           string
	Scopes                []string
	TokenStore            TokenStore
	AuthServerMetadataURL string
	PKCEEnabled           bool
	HTTPClient            *http.Client
}

// AuthServerMetadata contains OAuth server metadata from the well-known endpoint
type AuthServerMetadata struct {
	Issuer                                 string   `json:"issuer"`
	AuthorizationEndpoint                  string   `json:"authorization_endpoint"`
	TokenEndpoint                          string   `json:"token_endpoint"`
	UserinfoEndpoint                       string   `json:"userinfo_endpoint,omitempty"`
	JwksURI                                string   `json:"jwks_uri,omitempty"`
	RegistrationEndpoint                   string   `json:"registration_endpoint,omitempty"`
	ScopesSupported                        []string `json:"scopes_supported,omitempty"`
	ResponseTypesSupported                 []string `json:"response_types_supported,omitempty"`
	GrantTypesSupported                    []string `json:"grant_types_supported,omitempty"`
	TokenEndpointAuthMethodsSupported      []string `json:"token_endpoint_auth_methods_supported,omitempty"`
	CodeChallengeMethodsSupported          []string `json:"code_challenge_methods_supported,omitempty"`
	RevocationEndpoint                     string   `json:"revocation_endpoint,omitempty"`
	IntrospectionEndpoint                  string   `json:"introspection_endpoint,omitempty"`
	ServiceDocumentation                   string   `json:"service_documentation,omitempty"`
	AuthorizationResponseIssParameterSupported bool `json:"authorization_response_iss_parameter_supported,omitempty"`
}

// OAuthHandler handles OAuth authentication flow
type OAuthHandler struct {
	config         OAuthConfig
	httpClient     *http.Client
	mu             sync.RWMutex
	serverMetadata *AuthServerMetadata
	expectedState  string
	codeVerifier   string
}

// NewOAuthHandler creates a new OAuth handler
func NewOAuthHandler(config OAuthConfig) *OAuthHandler {
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}
	return &OAuthHandler{
		config:     config,
		httpClient: httpClient,
	}
}

// GetAuthorizationURL generates the authorization URL
func (h *OAuthHandler) GetAuthorizationURL(state string) (string, error) {
	metadata, err := h.GetServerMetadata(context.Background())
	if err != nil {
		return "", fmt.Errorf("failed to get server metadata: %w", err)
	}

	authURL, err := url.Parse(metadata.AuthorizationEndpoint)
	if err != nil {
		return "", fmt.Errorf("invalid authorization endpoint: %w", err)
	}

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", h.config.ClientID)
	params.Set("redirect_uri", h.config.RedirectURI)
	params.Set("state", state)
	
	if len(h.config.Scopes) > 0 {
		params.Set("scope", strings.Join(h.config.Scopes, " "))
	}

	// Add PKCE parameters if enabled
	if h.config.PKCEEnabled {
		h.mu.Lock()
		verifier, err := GenerateCodeVerifier()
		if err != nil {
			h.mu.Unlock()
			return "", fmt.Errorf("failed to generate code verifier: %w", err)
		}
		h.codeVerifier = verifier
		h.expectedState = state
		h.mu.Unlock()

		challenge := GenerateCodeChallenge(verifier)
		params.Set("code_challenge", challenge)
		params.Set("code_challenge_method", "S256")
	}

	authURL.RawQuery = params.Encode()
	return authURL.String(), nil
}

// GetServerMetadata fetches OAuth server metadata
func (h *OAuthHandler) GetServerMetadata(ctx context.Context) (*AuthServerMetadata, error) {
	h.mu.RLock()
	if h.serverMetadata != nil {
		h.mu.RUnlock()
		return h.serverMetadata, nil
	}
	h.mu.RUnlock()

	// Construct well-known endpoint URL
	wellKnownURL := h.config.AuthServerMetadataURL
	if wellKnownURL == "" {
		// Try to construct from the base URL
		baseURL := strings.TrimSuffix(h.config.AuthServerMetadataURL, "/")
		wellKnownURL = baseURL + "/.well-known/oauth-authorization-server"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", wellKnownURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send protected resource request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var metadata AuthServerMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("failed to decode metadata: %w", err)
	}

	h.mu.Lock()
	h.serverMetadata = &metadata
	h.mu.Unlock()

	return &metadata, nil
}

// ProcessAuthorizationResponse processes the authorization response and exchanges the code for a token
func (h *OAuthHandler) ProcessAuthorizationResponse(ctx context.Context, code, state, codeVerifier string) error {
	// Validate state
	h.mu.RLock()
	expectedState := h.expectedState
	h.mu.RUnlock()

	if expectedState == "" {
		return errors.New("no expected state set")
	}

	if state != expectedState {
		return ErrInvalidState
	}

	// Exchange code for token
	token, err := h.exchangeCodeForToken(ctx, code, codeVerifier)
	if err != nil {
		return fmt.Errorf("failed to exchange code for token: %w", err)
	}

	// Save token
	if err := h.config.TokenStore.SaveToken(token); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}

	return nil
}

// exchangeCodeForToken exchanges an authorization code for an access token
func (h *OAuthHandler) exchangeCodeForToken(ctx context.Context, code, codeVerifier string) (*Token, error) {
	metadata := h.serverMetadata
	if metadata == nil {
		return nil, errors.New("server metadata not loaded")
	}

	params := url.Values{}
	params.Set("grant_type", "authorization_code")
	params.Set("code", code)
	params.Set("redirect_uri", h.config.RedirectURI)
	params.Set("client_id", h.config.ClientID)

	if h.config.ClientSecret != "" {
		params.Set("client_secret", h.config.ClientSecret)
	}

	if h.config.PKCEEnabled && codeVerifier != "" {
		params.Set("code_verifier", codeVerifier)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", metadata.TokenEndpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var oauthErr OAuthError
		if err := json.NewDecoder(resp.Body).Decode(&oauthErr); err != nil {
			return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
		return nil, &oauthErr
	}

	var token Token
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	// Calculate expiration time
	if token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	return &token, nil
}

// GetClientID returns the client ID
func (h *OAuthHandler) GetClientID() string {
	return h.config.ClientID
}

// GetAuthorizationHeader returns the authorization header value
func (h *OAuthHandler) GetAuthorizationHeader(ctx context.Context) (string, error) {
	token, err := h.config.TokenStore.GetToken()
	if err != nil {
		return "", &OAuthAuthorizationRequiredError{Handler: h}
	}

	if token.AccessToken == "" {
		return "", &OAuthAuthorizationRequiredError{Handler: h}
	}

	if token.IsExpired() {
		if token.RefreshToken != "" {
			// Try to refresh the token
			newToken, err := h.refreshToken(ctx, token.RefreshToken)
			if err != nil {
				return "", &OAuthAuthorizationRequiredError{Handler: h}
			}
			token = newToken
		} else {
			return "", &OAuthAuthorizationRequiredError{Handler: h}
		}
	}

	return fmt.Sprintf("%s %s", token.TokenType, token.AccessToken), nil
}

// refreshToken refreshes an access token using a refresh token
func (h *OAuthHandler) refreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	metadata, err := h.GetServerMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get server metadata: %w", err)
	}

	params := url.Values{}
	params.Set("grant_type", "refresh_token")
	params.Set("refresh_token", refreshToken)
	params.Set("client_id", h.config.ClientID)

	if h.config.ClientSecret != "" {
		params.Set("client_secret", h.config.ClientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", metadata.TokenEndpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var oauthErr OAuthError
		if err := json.NewDecoder(resp.Body).Decode(&oauthErr); err != nil {
			return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
		return nil, &oauthErr
	}

	var token Token
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	// Calculate expiration time
	if token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	// Save the new token
	if err := h.config.TokenStore.SaveToken(&token); err != nil {
		return nil, fmt.Errorf("failed to save token: %w", err)
	}

	return &token, nil
}

// OAuthError represents an OAuth error response
type OAuthError struct {
	ErrorCode        string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
	ErrorURI         string `json:"error_uri,omitempty"`
}

// Error implements the error interface
func (e *OAuthError) Error() string {
	if e.ErrorDescription != "" {
		return fmt.Sprintf("OAuth error: %s - %s", e.ErrorCode, e.ErrorDescription)
	}
	return fmt.Sprintf("OAuth error: %s", e.ErrorCode)
}

// OAuthAuthorizationRequiredError is returned when OAuth authorization is required
type OAuthAuthorizationRequiredError struct {
	Handler *OAuthHandler
}

// Error implements the error interface
func (e *OAuthAuthorizationRequiredError) Error() string {
	return ErrOAuthAuthorizationRequired.Error()
}

// Is implements errors.Is support
func (e *OAuthAuthorizationRequiredError) Is(target error) bool {
	return target == ErrOAuthAuthorizationRequired
}

// ValidateRedirectURI validates that a redirect URI is safe to use
func ValidateRedirectURI(redirectURI string) error {
	if redirectURI == "" {
		return errors.New("redirect URI cannot be empty")
	}

	u, err := url.Parse(redirectURI)
	if err != nil {
		return fmt.Errorf("invalid redirect URI: %w", err)
	}

	// Check scheme
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("redirect URI must use http or https scheme")
	}

	// For http, only allow localhost
	if u.Scheme == "http" {
		hostname := u.Hostname()
		if hostname != "localhost" && hostname != "127.0.0.1" && hostname != "::1" {
			return fmt.Errorf("http redirect URIs are only allowed for localhost")
		}
	}

	return nil
}
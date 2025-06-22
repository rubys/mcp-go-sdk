package client

import (
	"errors"
	"fmt"

	"github.com/rubys/mcp-go-sdk/transport"
)

// Token represents an OAuth 2.0 token
type Token = transport.Token

// TokenStore defines the interface for storing and retrieving OAuth tokens
type TokenStore = transport.TokenStore

// MemoryTokenStore is an in-memory implementation of TokenStore
type MemoryTokenStore = transport.MemoryTokenStore

// NewMemoryTokenStore creates a new in-memory token store
func NewMemoryTokenStore() *MemoryTokenStore {
	return transport.NewMemoryTokenStore()
}

// OAuthConfig contains OAuth configuration for the client
type OAuthConfig struct {
	ClientID              string
	ClientSecret          string
	RedirectURI           string
	Scopes                []string
	TokenStore            TokenStore
	AuthServerMetadataURL string
	PKCEEnabled           bool
}

// NewOAuthStreamableHttpClient creates a new MCP client with OAuth-enabled Streamable HTTP transport
func NewOAuthStreamableHttpClient(url string, oauthConfig OAuthConfig) (*Client, error) {
	// Validate OAuth config
	if err := transport.ValidateRedirectURI(oauthConfig.RedirectURI); err != nil {
		return nil, fmt.Errorf("invalid redirect URI: %w", err)
	}

	// Create transport OAuth config
	transportConfig := transport.OAuthConfig{
		ClientID:              oauthConfig.ClientID,
		ClientSecret:          oauthConfig.ClientSecret,
		RedirectURI:           oauthConfig.RedirectURI,
		Scopes:                oauthConfig.Scopes,
		TokenStore:            oauthConfig.TokenStore,
		AuthServerMetadataURL: oauthConfig.AuthServerMetadataURL,
		PKCEEnabled:           oauthConfig.PKCEEnabled,
	}

	// Create OAuth handler
	oauthHandler := transport.NewOAuthHandler(transportConfig)

	// Create transport with OAuth
	trans := transport.NewStreamableHTTPTransport(url)
	
	// Enable OAuth on the transport
	trans.EnableOAuth(oauthHandler)

	// Create client
	return New(trans), nil
}

// IsOAuthAuthorizationRequiredError checks if an error indicates OAuth authorization is required
func IsOAuthAuthorizationRequiredError(err error) bool {
	var oauthErr *transport.OAuthAuthorizationRequiredError
	return errors.As(err, &oauthErr)
}

// GetOAuthHandler extracts the OAuth handler from an OAuthAuthorizationRequiredError
func GetOAuthHandler(err error) *transport.OAuthHandler {
	var oauthErr *transport.OAuthAuthorizationRequiredError
	if errors.As(err, &oauthErr) {
		return oauthErr.Handler
	}
	return nil
}
package client

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/rubys/mcp-go-sdk/transport"
)

func TestNewOAuthStreamableHttpClient(t *testing.T) {
	// Create a token store with a valid token
	tokenStore := transport.NewMemoryTokenStore()
	validToken := &transport.Token{
		AccessToken:  "test-token",
		TokenType:    "Bearer",
		RefreshToken: "refresh-token",
		ExpiresIn:    3600,
		ExpiresAt:    time.Now().Add(1 * time.Hour), // Valid for 1 hour
	}
	if err := tokenStore.SaveToken(validToken); err != nil {
		t.Fatalf("Failed to save token: %v", err)
	}

	// Create OAuth config
	oauthConfig := transport.OAuthConfig{
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8085/callback",
		Scopes:      []string{"mcp.read", "mcp.write"},
		TokenStore:  tokenStore,
		PKCEEnabled: true,
	}

	// Test OAuth client creation without starting it
	ctx := context.Background()
	
	// Create streamable HTTP transport with OAuth
	transportConfig := transport.StreamableHTTPConfig{
		ClientEndpoint:        "http://example.com/rpc",
		RequestTimeout:        10 * time.Second,
		MaxConcurrentRequests: 10,
		MessageBuffer:         50,
	}
	
	httpTransport, err := transport.NewStreamableHTTPTransportWithConfig(ctx, transportConfig)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer httpTransport.Close()

	// Create OAuth handler and enable it on transport
	oauthHandler := transport.NewOAuthHandler(transport.OAuthConfig{
		ClientID:     oauthConfig.ClientID,
		RedirectURI:  oauthConfig.RedirectURI,
		Scopes:       oauthConfig.Scopes,
		PKCEEnabled:  oauthConfig.PKCEEnabled,
		TokenStore:   oauthConfig.TokenStore,
	})
	
	httpTransport.EnableOAuth(oauthHandler)

	// Verify OAuth is enabled
	if !httpTransport.IsOAuthEnabled() {
		t.Errorf("Expected IsOAuthEnabled() to return true")
	}

	// Verify the OAuth handler is set
	if httpTransport.GetOAuthHandler() == nil {
		t.Errorf("Expected GetOAuthHandler() to return a handler")
	}

	// Test that OAuth handler has correct configuration
	handler := httpTransport.GetOAuthHandler()
	if handler.GetClientID() != "test-client" {
		t.Errorf("Expected client ID 'test-client', got %s", handler.GetClientID())
	}
}

func TestIsOAuthAuthorizationRequiredError(t *testing.T) {
	// Create a test error
	err := &transport.OAuthAuthorizationRequiredError{
		Handler: transport.NewOAuthHandler(transport.OAuthConfig{}),
	}

	// Verify IsOAuthAuthorizationRequiredError returns true
	if !IsOAuthAuthorizationRequiredError(err) {
		t.Errorf("Expected IsOAuthAuthorizationRequiredError to return true")
	}

	// Verify GetOAuthHandler returns the handler
	handler := GetOAuthHandler(err)
	if handler == nil {
		t.Errorf("Expected GetOAuthHandler to return a handler")
	}

	// Test with a different error
	err2 := fmt.Errorf("some other error")

	// Verify IsOAuthAuthorizationRequiredError returns false
	if IsOAuthAuthorizationRequiredError(err2) {
		t.Errorf("Expected IsOAuthAuthorizationRequiredError to return false")
	}

	// Verify GetOAuthHandler returns nil
	handler = GetOAuthHandler(err2)
	if handler != nil {
		t.Errorf("Expected GetOAuthHandler to return nil")
	}
}
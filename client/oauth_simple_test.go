package client

import (
	"testing"

	"github.com/rubys/mcp-go-sdk/transport"
)

func TestOAuthClientCreation(t *testing.T) {
	// Create a token store
	tokenStore := NewMemoryTokenStore()

	// Create OAuth config
	oauthConfig := OAuthConfig{
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8085/callback",
		Scopes:      []string{"mcp.read", "mcp.write"},
		TokenStore:  tokenStore,
		PKCEEnabled: true,
	}

	// Create client with OAuth
	client, err := NewOAuthStreamableHttpClient("http://localhost:8080", oauthConfig)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Verify that the client was created successfully
	trans := client.GetTransport()
	streamableHTTP, ok := trans.(*transport.StreamableHTTP)
	if !ok {
		t.Fatalf("Expected transport to be *transport.StreamableHTTP, got %T", trans)
	}

	// Verify OAuth is enabled
	if !streamableHTTP.IsOAuthEnabled() {
		t.Errorf("Expected IsOAuthEnabled() to return true")
	}

	// Verify the OAuth handler is set
	if streamableHTTP.GetOAuthHandler() == nil {
		t.Errorf("Expected GetOAuthHandler() to return a handler")
	}
}
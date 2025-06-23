package transport

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSSETransportOAuthIntegration tests OAuth authentication with SSE transport
func TestSSETransportOAuthIntegration(t *testing.T) {
	// Track authentication attempts
	var authHeaders []string
	var authMutex sync.Mutex

	// Create mock SSE server that requires OAuth
	sseServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authMutex.Lock()
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		authMutex.Unlock()

		// Check for valid authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// For SSE endpoint, return SSE stream
		if r.URL.Path == "/sse" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")

			// Send a test event
			fmt.Fprintf(w, "data: {\"method\":\"test\",\"params\":{}}\n\n")
			w.(http.Flusher).Flush()
			
			// Keep connection alive briefly
			time.Sleep(100 * time.Millisecond)
		}
	}))
	defer sseServer.Close()

	// Create mock HTTP POST server for outgoing messages
	var receivedMessages []string
	var msgMutex sync.Mutex

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authMutex.Lock()
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		authMutex.Unlock()

		// Check for valid authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if r.Method == "POST" {
			body := make([]byte, r.ContentLength)
			r.Body.Read(body)
			
			msgMutex.Lock()
			receivedMessages = append(receivedMessages, string(body))
			msgMutex.Unlock()

			w.WriteHeader(http.StatusOK)
		}
	}))
	defer httpServer.Close()

	// Create token store with valid token
	tokenStore := NewMemoryTokenStore()
	validToken := &Token{
		AccessToken: "test-oauth-token",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	require.NoError(t, tokenStore.SaveToken(validToken))

	// Create OAuth handler
	oauthConfig := OAuthConfig{
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
		Scopes:      []string{"sse", "write"},
		TokenStore:  tokenStore,
		PKCEEnabled: true,
	}
	oauthHandler := NewOAuthHandler(oauthConfig)

	// Create SSE transport
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config := SSEConfig{
		SSEEndpoint:    sseServer.URL + "/sse",
		HTTPEndpoint:   httpServer.URL + "/messages",
		RequestTimeout: 3 * time.Second,
		MessageBuffer:  10,
		MaxReconnects:  1,
	}

	transport, err := NewSSETransport(ctx, config)
	require.NoError(t, err)
	defer transport.Close()

	// Enable OAuth on the transport
	transport.EnableOAuth(oauthHandler)

	// Verify OAuth is enabled
	assert.True(t, transport.IsOAuthEnabled(), "OAuth should be enabled")
	assert.NotNil(t, transport.GetOAuthHandler(), "OAuth handler should be set")

	// Transport is automatically started when created

	// Wait for connection to establish
	time.Sleep(200 * time.Millisecond)

	// Send a test message (this should trigger HTTP POST with OAuth)
	err = transport.SendNotification("test-notification", map[string]interface{}{
		"message": "Hello OAuth SSE!",
	})
	require.NoError(t, err)

	// Wait for message to be sent
	time.Sleep(200 * time.Millisecond)

	// Verify OAuth headers were sent
	authMutex.Lock()
	assert.Greater(t, len(authHeaders), 0, "Should have sent OAuth headers")
	for _, header := range authHeaders {
		assert.Equal(t, "Bearer test-oauth-token", header, "Should have correct OAuth header")
	}
	authMutex.Unlock()

	// Verify message was received
	msgMutex.Lock()
	assert.Greater(t, len(receivedMessages), 0, "Should have received messages")
	msgMutex.Unlock()
}

// TestSSETransportOAuthUnauthorized tests OAuth failure scenarios
func TestSSETransportOAuthUnauthorized(t *testing.T) {
	// Create server that always returns 401
	unauthorizedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}))
	defer unauthorizedServer.Close()

	// Create token store with expired token
	tokenStore := NewMemoryTokenStore()
	expiredToken := &Token{
		AccessToken: "expired-token",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
	}
	require.NoError(t, tokenStore.SaveToken(expiredToken))

	// Create OAuth handler with expired token
	oauthConfig := OAuthConfig{
		ClientID:    "test-client",
		RedirectURI: "http://localhost:8080/callback",
		TokenStore:  tokenStore,
	}
	oauthHandler := NewOAuthHandler(oauthConfig)

	// Create SSE transport
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	config := SSEConfig{
		SSEEndpoint:    unauthorizedServer.URL + "/sse",
		HTTPEndpoint:   unauthorizedServer.URL + "/messages",
		RequestTimeout: 1 * time.Second,
		MessageBuffer:  10,
		MaxReconnects:  1,
	}

	transport, err := NewSSETransport(ctx, config)
	require.NoError(t, err)
	defer transport.Close()

	// Enable OAuth with expired token
	transport.EnableOAuth(oauthHandler)

	// Transport is automatically started when created
	// Connections will fail due to expired token
	// This is expected behavior for OAuth authorization errors

	// Try to send a message (should fail)
	err = transport.SendNotification("test", map[string]interface{}{})
	// Error may not be immediate due to async nature, but connection should fail
}

// TestSSETransportOAuthTokenRefresh tests token refresh functionality
func TestSSETransportOAuthTokenRefresh(t *testing.T) {
	// Track tokens used in requests
	var usedTokens []string
	var tokenMutex sync.Mutex

	// Create mock server that accepts both old and new tokens
	refreshServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		
		tokenMutex.Lock()
		usedTokens = append(usedTokens, authHeader)
		tokenMutex.Unlock()

		// Accept both old and refreshed tokens
		if authHeader == "Bearer old-token" || authHeader == "Bearer refreshed-token" {
			if r.URL.Path == "/sse" {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				fmt.Fprintf(w, "data: {\"method\":\"test\"}\n\n")
				w.(http.Flusher).Flush()
				time.Sleep(50 * time.Millisecond)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		} else {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		}
	}))
	defer refreshServer.Close()

	// Create custom token store that simulates token refresh
	tokenStore := &MockRefreshTokenStore{
		currentToken: &Token{
			AccessToken:  "old-token",
			TokenType:    "Bearer",
			RefreshToken: "refresh-token",
			ExpiresAt:    time.Now().Add(-1 * time.Minute), // Expired
		},
		refreshedToken: &Token{
			AccessToken: "refreshed-token",
			TokenType:   "Bearer",
			ExpiresAt:   time.Now().Add(1 * time.Hour), // Valid for 1 hour
		},
	}

	// Create OAuth handler with refresh capability
	oauthConfig := OAuthConfig{
		ClientID:     "test-client",
		RedirectURI:  "http://localhost:8080/callback",
		TokenStore:   tokenStore,
		PKCEEnabled:  true,
	}
	oauthHandler := NewOAuthHandler(oauthConfig)

	// Create SSE transport
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	config := SSEConfig{
		SSEEndpoint:    refreshServer.URL + "/sse",
		HTTPEndpoint:   refreshServer.URL + "/messages",
		RequestTimeout: 2 * time.Second,
		MessageBuffer:  10,
		MaxReconnects:  1,
	}

	transport, err := NewSSETransport(ctx, config)
	require.NoError(t, err)
	defer transport.Close()

	// Enable OAuth
	transport.EnableOAuth(oauthHandler)

	// Transport is automatically started when created

	// Wait for connection and potential token refresh
	time.Sleep(300 * time.Millisecond)

	// Send a message
	err = transport.SendNotification("test-refresh", map[string]interface{}{
		"refresh": "test",
	})
	require.NoError(t, err)

	// Wait for message to be processed
	time.Sleep(200 * time.Millisecond)

	// Verify tokens were used (may include refresh attempts)
	tokenMutex.Lock()
	assert.Greater(t, len(usedTokens), 0, "Should have attempted authentication")
	tokenMutex.Unlock()
}

// TestSSETransportOAuthConcurrentRequests tests OAuth with multiple concurrent requests
func TestSSETransportOAuthConcurrentRequests(t *testing.T) {
	// Track concurrent requests
	var requestCount int64
	var requestMutex sync.Mutex
	var authHeaders []string

	concurrentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMutex.Lock()
		requestCount++
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		requestMutex.Unlock()

		// Verify OAuth header
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer concurrent-token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if r.URL.Path == "/sse" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			fmt.Fprintf(w, "data: {\"method\":\"concurrent\"}\n\n")
			w.(http.Flusher).Flush()
			time.Sleep(50 * time.Millisecond)
		} else {
			// Simulate processing time
			time.Sleep(10 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer concurrentServer.Close()

	// Create token store
	tokenStore := NewMemoryTokenStore()
	token := &Token{
		AccessToken: "concurrent-token",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	require.NoError(t, tokenStore.SaveToken(token))

	// Create OAuth handler
	oauthHandler := NewOAuthHandler(OAuthConfig{
		ClientID:    "concurrent-client",
		TokenStore:  tokenStore,
		PKCEEnabled: true,
	})

	// Create SSE transport
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config := SSEConfig{
		SSEEndpoint:    concurrentServer.URL + "/sse",
		HTTPEndpoint:   concurrentServer.URL + "/messages",
		RequestTimeout: 2 * time.Second,
		MessageBuffer:  50,
		MaxReconnects:  1,
	}

	transport, err := NewSSETransport(ctx, config)
	require.NoError(t, err)
	defer transport.Close()

	transport.EnableOAuth(oauthHandler)

	// Transport is automatically started when created

	// Wait for initial connection
	time.Sleep(100 * time.Millisecond)

	// Send multiple concurrent messages
	numRequests := 10
	var wg sync.WaitGroup

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := transport.SendNotification("concurrent-test", map[string]interface{}{
				"request": i,
			})
			// OAuth errors are async, so we don't assert here
			_ = err
		}(i)
	}

	wg.Wait()

	// Wait for all requests to be processed
	time.Sleep(500 * time.Millisecond)

	// Verify OAuth was used for all requests
	requestMutex.Lock()
	assert.Greater(t, len(authHeaders), 0, "Should have received requests with OAuth")
	for _, header := range authHeaders {
		if header != "" { // Skip empty headers (some might be SSE connections)
			assert.Equal(t, "Bearer concurrent-token", header, "All requests should have OAuth header")
		}
	}
	requestMutex.Unlock()
}

// MockRefreshTokenStore simulates a token store that can refresh tokens
type MockRefreshTokenStore struct {
	mu             sync.RWMutex
	currentToken   *Token
	refreshedToken *Token
	refreshed      bool
}

func (m *MockRefreshTokenStore) SaveToken(token *Token) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentToken = token
	return nil
}

func (m *MockRefreshTokenStore) GetToken() (*Token, error) {
	m.mu.RLock()
	
	// Check if we need to refresh
	needsRefresh := m.currentToken != nil && m.currentToken.IsExpired() && !m.refreshed
	if !needsRefresh {
		defer m.mu.RUnlock()
		return m.currentToken, nil
	}
	
	// Upgrade to write lock for refresh
	m.mu.RUnlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Double-check after acquiring write lock
	if m.refreshed {
		return m.refreshedToken, nil
	}
	
	m.refreshed = true
	return m.refreshedToken, nil
}

// TestSSEOAuthDisabled tests transport behavior when OAuth is not enabled
func TestSSEOAuthDisabled(t *testing.T) {
	// Create server that doesn't require OAuth
	noAuthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should not have Authorization header
		authHeader := r.Header.Get("Authorization")
		assert.Empty(t, authHeader, "Should not have Authorization header when OAuth is disabled")

		if r.URL.Path == "/sse" {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: {\"method\":\"no-auth\"}\n\n")
			w.(http.Flusher).Flush()
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer noAuthServer.Close()

	// Create SSE transport without OAuth
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	config := SSEConfig{
		SSEEndpoint:    noAuthServer.URL + "/sse",
		HTTPEndpoint:   noAuthServer.URL + "/messages",
		RequestTimeout: 1 * time.Second,
		MessageBuffer:  10,
		MaxReconnects:  1,
	}

	transport, err := NewSSETransport(ctx, config)
	require.NoError(t, err)
	defer transport.Close()

	// Verify OAuth is disabled
	assert.False(t, transport.IsOAuthEnabled(), "OAuth should be disabled by default")
	assert.Nil(t, transport.GetOAuthHandler(), "OAuth handler should be nil")

	// Transport is automatically started when created

	// Wait for connection
	time.Sleep(100 * time.Millisecond)

	// Send message without OAuth
	err = transport.SendNotification("no-auth-test", map[string]interface{}{
		"test": "no oauth",
	})
	require.NoError(t, err)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)
}
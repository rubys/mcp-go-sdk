package client

import (
	"context"
	"fmt"
	"testing"

	"github.com/rubys/mcp-go-sdk/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Basic Advanced Client Initialization Tests
// These tests demonstrate advanced patterns for client initialization
// compatible with the existing Go MCP SDK architecture

func TestBasicAdvancedInit_ProtocolVersionValidation(t *testing.T) {
	ctx := context.Background()

	// Create mock transport for testing initialization
	mockTransport := &MockInitTransport{
		responses: make(map[string]*shared.Response),
	}

	// Test different protocol versions
	tests := []struct {
		name             string
		serverVersion    string
		expectSuccess    bool
		expectedErrorMsg string
	}{
		{
			name:          "Latest protocol version",
			serverVersion: shared.LATEST_PROTOCOL_VERSION,
			expectSuccess: true,
		},
		{
			name:          "Supported older version",
			serverVersion: "2024-10-07",
			expectSuccess: true,
		},
		{
			name:             "Invalid version format",
			serverVersion:    "invalid-version",
			expectSuccess:    true, // Basic client doesn't validate protocol version
			expectedErrorMsg: "",
		},
		{
			name:             "Empty version",
			serverVersion:    "",
			expectSuccess:    true, // Basic client doesn't validate protocol version
			expectedErrorMsg: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Configure mock response
			mockTransport.responses["initialize"] = &shared.Response{
				JSONRPC: "2.0",
				ID:      1,
				Result: map[string]interface{}{
					"protocolVersion": test.serverVersion,
					"capabilities":    map[string]interface{}{},
					"serverInfo": map[string]interface{}{
						"name":    "test-server",
						"version": "1.0.0",
					},
				},
			}

			client := New(mockTransport)
			client.config = ClientConfig{
				ClientInfo: shared.ClientInfo{
					Name:    "test-client",
					Version: "1.0.0",
				},
				Capabilities: shared.ClientCapabilities{},
			}

			err := client.Initialize(ctx)

			if test.expectSuccess {
				assert.NoError(t, err, "Initialization should succeed for version %s", test.serverVersion)
				if client.serverInfo != nil {
					assert.Equal(t, "test-server", client.serverInfo.Name)
				}
			} else {
				assert.Error(t, err, "Initialization should fail for version %s", test.serverVersion)
				if test.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), test.expectedErrorMsg)
				}
			}
		})
	}
}

func TestBasicAdvancedInit_CapabilitiesHandling(t *testing.T) {
	ctx := context.Background()

	mockTransport := &MockInitTransport{
		responses: make(map[string]*shared.Response),
	}

	// Configure server with specific capabilities
	mockTransport.responses["initialize"] = &shared.Response{
		JSONRPC: "2.0",
		ID:      1,
		Result: map[string]interface{}{
			"protocolVersion": shared.LATEST_PROTOCOL_VERSION,
			"capabilities": map[string]interface{}{
				"resources": map[string]interface{}{
					"subscribe":   true,
					"listChanged": true,
				},
				"tools": map[string]interface{}{
					"listChanged": false,
				},
				// No prompts capability
			},
			"serverInfo": map[string]interface{}{
				"name":    "capabilities-server",
				"version": "1.0.0",
			},
		},
	}

	// Create client with custom capabilities
	client := New(mockTransport)
	client.config = ClientConfig{
		ClientInfo: shared.ClientInfo{
			Name:    "advanced-client",
			Version: "1.0.0",
		},
		Capabilities: shared.ClientCapabilities{
			Sampling: &shared.SamplingCapability{},
			Roots: &shared.RootsCapability{
				ListChanged: true,
			},
		},
	}

	err := client.Initialize(ctx)
	require.NoError(t, err, "Initialization should succeed")

	// Verify server info was populated correctly
	assert.Equal(t, "capabilities-server", client.serverInfo.Name)
	assert.Equal(t, "1.0.0", client.serverInfo.Version)

	// Test that client configuration was used in the initialize request
	initRequest := mockTransport.lastRequest
	require.NotNil(t, initRequest, "Initialize request should have been sent")
	assert.Equal(t, "initialize", initRequest.Method)

	// Verify the request included client capabilities
	params, ok := initRequest.Params.(map[string]interface{})
	require.True(t, ok, "Request params should be a map")

	capabilities, ok := params["capabilities"].(shared.ClientCapabilities)
	require.True(t, ok, "Capabilities should be present")
	assert.NotNil(t, capabilities.Sampling, "Sampling capability should be included")
	assert.NotNil(t, capabilities.Roots, "Roots capability should be included")
	assert.True(t, capabilities.Roots.ListChanged, "Roots list changed should be true")
}

func TestBasicAdvancedInit_ServerInstructions(t *testing.T) {
	ctx := context.Background()

	mockTransport := &MockInitTransport{
		responses: make(map[string]*shared.Response),
	}

	// Configure server response with instructions
	instructions := "Please use this server for demonstration purposes only"
	mockTransport.responses["initialize"] = &shared.Response{
		JSONRPC: "2.0",
		ID:      1,
		Result: map[string]interface{}{
			"protocolVersion": shared.LATEST_PROTOCOL_VERSION,
			"capabilities":    map[string]interface{}{},
			"serverInfo": map[string]interface{}{
				"name":    "instructional-server",
				"version": "1.0.0",
			},
			"instructions": instructions,
		},
	}

	client := New(mockTransport)
	client.config = ClientConfig{
		ClientInfo: shared.ClientInfo{
			Name:    "instructions-client",
			Version: "1.0.0",
		},
		Capabilities: shared.ClientCapabilities{},
	}

	err := client.Initialize(ctx)
	require.NoError(t, err, "Initialization should succeed")

	// Verify instructions are not directly stored in basic client
	// (This demonstrates that advanced features like instructions would
	// require an enhanced client implementation)
	assert.Equal(t, "instructional-server", client.serverInfo.Name)
}

func TestBasicAdvancedInit_ErrorHandling(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		response     *shared.Response
		expectError  bool
		errorContains string
	}{
		{
			name: "Server error response",
			response: &shared.Response{
				JSONRPC: "2.0",
				ID:      1,
				Error: &shared.RPCError{
					Code:    -1,
					Message: "Server initialization failed",
				},
			},
			expectError:   true,
			errorContains: "initialization failed",
		},
		{
			name: "Invalid response format",
			response: &shared.Response{
				JSONRPC: "2.0",
				ID:      1,
				Result:  "invalid-format",
			},
			expectError:   false, // Basic client doesn't validate response format strictly
			errorContains: "",
		},
		{
			name: "Missing server info",
			response: &shared.Response{
				JSONRPC: "2.0",
				ID:      1,
				Result: map[string]interface{}{
					"protocolVersion": shared.LATEST_PROTOCOL_VERSION,
					"capabilities":    map[string]interface{}{},
					// Missing serverInfo
				},
			},
			expectError:   false, // Basic client doesn't strictly validate this
			errorContains: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockTransport := &MockInitTransport{
				responses: make(map[string]*shared.Response),
			}
			mockTransport.responses["initialize"] = test.response

			client := New(mockTransport)
			client.config = ClientConfig{
				ClientInfo: shared.ClientInfo{
					Name:    "error-test-client",
					Version: "1.0.0",
				},
				Capabilities: shared.ClientCapabilities{},
			}

			err := client.Initialize(ctx)

			if test.expectError {
				assert.Error(t, err, "Should return error for test case: %s", test.name)
				if test.errorContains != "" {
					assert.Contains(t, err.Error(), test.errorContains)
				}
			} else {
				assert.NoError(t, err, "Should not return error for test case: %s", test.name)
			}
		})
	}
}

func TestBasicAdvancedInit_ConcurrentInitialization(t *testing.T) {
	ctx := context.Background()

	// Test that multiple clients can initialize concurrently
	const numClients = 5
	clientErrors := make(chan error, numClients)

	for i := 0; i < numClients; i++ {
		go func(clientID int) {
			mockTransport := &MockInitTransport{
				responses: make(map[string]*shared.Response),
			}

			mockTransport.responses["initialize"] = &shared.Response{
				JSONRPC: "2.0",
				ID:      1,
				Result: map[string]interface{}{
					"protocolVersion": shared.LATEST_PROTOCOL_VERSION,
					"capabilities":    map[string]interface{}{},
					"serverInfo": map[string]interface{}{
						"name":    fmt.Sprintf("concurrent-server-%d", clientID),
						"version": "1.0.0",
					},
				},
			}

			client := New(mockTransport)
			client.config = ClientConfig{
				ClientInfo: shared.ClientInfo{
					Name:    fmt.Sprintf("concurrent-client-%d", clientID),
					Version: "1.0.0",
				},
				Capabilities: shared.ClientCapabilities{},
			}

			clientErrors <- client.Initialize(ctx)
		}(i)
	}

	// Collect all results
	for i := 0; i < numClients; i++ {
		err := <-clientErrors
		assert.NoError(t, err, "Client %d should initialize successfully", i)
	}
}

// Benchmark client initialization
func BenchmarkBasicAdvancedInit_Initialization(b *testing.B) {
	ctx := context.Background()

	mockTransport := &MockInitTransport{
		responses: make(map[string]*shared.Response),
	}

	mockTransport.responses["initialize"] = &shared.Response{
		JSONRPC: "2.0",
		ID:      1,
		Result: map[string]interface{}{
			"protocolVersion": shared.LATEST_PROTOCOL_VERSION,
			"capabilities": map[string]interface{}{
				"resources": map[string]interface{}{},
				"tools":     map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "benchmark-server",
				"version": "1.0.0",
			},
		},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		client := New(mockTransport)
		client.config = ClientConfig{
			ClientInfo: shared.ClientInfo{
				Name:    "benchmark-client",
				Version: "1.0.0",
			},
			Capabilities: shared.ClientCapabilities{
				Sampling: &shared.SamplingCapability{},
			},
		}

		_ = client.Initialize(ctx)
	}
}

// MockInitTransport implements a transport for testing initialization
type MockInitTransport struct {
	responses   map[string]*shared.Response
	lastRequest *shared.Request
}

func (m *MockInitTransport) SendRequest(method string, params interface{}) (<-chan *shared.Response, error) {
	// Store the request for verification
	m.lastRequest = &shared.Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}

	responseChan := make(chan *shared.Response, 1)

	// Send the configured response
	if response, exists := m.responses[method]; exists {
		responseChan <- response
	} else {
		// Default response for unknown methods
		responseChan <- &shared.Response{
			JSONRPC: "2.0",
			ID:      1,
			Error: &shared.RPCError{
				Code:    -32601,
				Message: "Method not found",
			},
		}
	}

	close(responseChan)
	return responseChan, nil
}

func (m *MockInitTransport) SendNotification(method string, params interface{}) error {
	// Just accept notifications without doing anything
	return nil
}

func (m *MockInitTransport) Channels() (<-chan *shared.Request, <-chan *shared.Notification) {
	// Return empty channels
	return make(chan *shared.Request), make(chan *shared.Notification)
}

func (m *MockInitTransport) Close() error {
	return nil
}

func (m *MockInitTransport) SendResponse(id interface{}, result interface{}, err *shared.RPCError) error {
	// Mock implementation for response sending
	return nil
}

func (m *MockInitTransport) Stats() interface{} {
	return map[string]int{"requests": 1}
}
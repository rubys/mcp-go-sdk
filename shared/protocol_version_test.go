package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockTransport implements a simple transport for testing protocol version negotiation
type MockTransport struct {
	requests      chan *Request
	responses     chan *Response
	notifications chan *Notification
	closed        bool
}

func NewMockTransport() *MockTransport {
	return &MockTransport{
		requests:      make(chan *Request, 10),
		responses:     make(chan *Response, 10),
		notifications: make(chan *Notification, 10),
	}
}

func (mt *MockTransport) SendRequest(method string, params interface{}) (<-chan *Response, error) {
	if mt.closed {
		return nil, fmt.Errorf("transport closed")
	}

	request := &Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}

	responseChan := make(chan *Response, 1)
	mt.requests <- request

	// Send mock response after short delay
	go func() {
		time.Sleep(10 * time.Millisecond)
		select {
		case response := <-mt.responses:
			responseChan <- response
		case <-time.After(100 * time.Millisecond):
			// Timeout
			close(responseChan)
		}
	}()

	return responseChan, nil
}

func (mt *MockTransport) SendResponse(id interface{}, result interface{}, err *RPCError) error {
	response := &Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   err,
	}
	
	select {
	case mt.responses <- response:
		return nil
	default:
		return fmt.Errorf("response channel full")
	}
}

func (mt *MockTransport) SendNotification(method string, params interface{}) error {
	notification := &Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	
	select {
	case mt.notifications <- notification:
		return nil
	default:
		return fmt.Errorf("notification channel full")
	}
}

func (mt *MockTransport) Channels() (<-chan *Request, <-chan *Notification) {
	return mt.requests, mt.notifications
}

func (mt *MockTransport) Close() error {
	mt.closed = true
	close(mt.requests)
	close(mt.responses)
	close(mt.notifications)
	return nil
}

// MockServer provides basic initialize handling for version negotiation tests
type MockServer struct {
	transport         *MockTransport
	protocolVersion   string
	supportedVersions []string
	strict            bool // If true, reject unsupported versions
}

func NewMockServer(transport *MockTransport, version string, supportedVersions []string, strict bool) *MockServer {
	return &MockServer{
		transport:         transport,
		protocolVersion:   version,
		supportedVersions: supportedVersions,
		strict:            strict,
	}
}

func (ms *MockServer) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case req := <-ms.transport.requests:
				if req == nil {
					return
				}
				ms.handleRequest(req)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (ms *MockServer) handleRequest(req *Request) {
	switch req.Method {
	case "initialize":
		ms.handleInitialize(req)
	default:
		ms.transport.SendResponse(req.ID, nil, &RPCError{
			Code:    -32601,
			Message: "Method not found",
		})
	}
}

func (ms *MockServer) handleInitialize(req *Request) {
	var initReq InitializeRequest
	
	// Parse the request parameters
	if req.Params != nil {
		if paramsMap, ok := req.Params.(map[string]interface{}); ok {
			if protocolVersion, ok := paramsMap["protocolVersion"].(string); ok {
				initReq.ProtocolVersion = protocolVersion
			}
			if clientInfo, ok := paramsMap["clientInfo"].(map[string]interface{}); ok {
				initReq.ClientInfo.Name, _ = clientInfo["name"].(string)
				initReq.ClientInfo.Version, _ = clientInfo["version"].(string)
			}
		}
	}

	// Version negotiation logic
	if ms.strict {
		// In strict mode, only accept exact matches or supported versions
		supported := false
		if initReq.ProtocolVersion == ms.protocolVersion {
			supported = true
		} else {
			for _, supportedVersion := range ms.supportedVersions {
				if initReq.ProtocolVersion == supportedVersion {
					supported = true
					break
				}
			}
		}
		
		if !supported {
			ms.transport.SendResponse(req.ID, nil, &RPCError{
				Code:    -32602,
				Message: fmt.Sprintf("Unsupported protocol version: %s. Supported versions: %v", initReq.ProtocolVersion, append(ms.supportedVersions, ms.protocolVersion)),
			})
			return
		}
	}

	// Determine response version - prefer client version if supported, otherwise use server version
	responseVersion := ms.protocolVersion
	if initReq.ProtocolVersion != "" {
		for _, supportedVersion := range append(ms.supportedVersions, ms.protocolVersion) {
			if initReq.ProtocolVersion == supportedVersion {
				responseVersion = initReq.ProtocolVersion
				break
			}
		}
	}

	response := InitializeResponse{
		ProtocolVersion: responseVersion,
		ServerInfo: Implementation{
			Name:    "mock-server",
			Version: "1.0.0",
		},
		Capabilities: ServerCapabilities{},
	}

	ms.transport.SendResponse(req.ID, response, nil)
}

// Protocol Version Negotiation Tests

func TestProtocolVersionNegotiation_MatchingVersions(t *testing.T) {
	transport := NewMockTransport()
	defer transport.Close()

	server := NewMockServer(transport, "2024-11-05", []string{"2024-10-07"}, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server.Start(ctx)

	// Client requests with matching version
	clientVersion := "2024-11-05"
	resp, err := transport.SendRequest("initialize", map[string]interface{}{
		"protocolVersion": clientVersion,
		"clientInfo": map[string]interface{}{
			"name":    "test-client",
			"version": "1.0.0",
		},
		"capabilities": ClientCapabilities{},
	})
	require.NoError(t, err)

	// Wait for response
	select {
	case response := <-resp:
		require.NotNil(t, response)
		assert.Nil(t, response.Error, "Should not have error for matching version")
		
		// Parse response
		var initResp InitializeResponse
		respData, err := json.Marshal(response.Result)
		require.NoError(t, err)
		err = json.Unmarshal(respData, &initResp)
		require.NoError(t, err)
		
		assert.Equal(t, clientVersion, initResp.ProtocolVersion, "Server should respond with client's requested version")
		assert.Equal(t, "mock-server", initResp.ServerInfo.Name)
		
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Timeout waiting for response")
	}
}

func TestProtocolVersionNegotiation_VersionMismatch(t *testing.T) {
	transport := NewMockTransport()
	defer transport.Close()

	// Strict server that rejects unsupported versions
	server := NewMockServer(transport, "2024-11-05", []string{"2024-10-07"}, true)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server.Start(ctx)

	// Client requests with unsupported version
	clientVersion := "2024-12-01" // Future version not supported
	resp, err := transport.SendRequest("initialize", map[string]interface{}{
		"protocolVersion": clientVersion,
		"clientInfo": map[string]interface{}{
			"name":    "test-client",
			"version": "1.0.0",
		},
		"capabilities": ClientCapabilities{},
	})
	require.NoError(t, err)

	// Wait for response
	select {
	case response := <-resp:
		require.NotNil(t, response)
		assert.NotNil(t, response.Error, "Should have error for unsupported version")
		assert.Equal(t, -32602, response.Error.Code)
		assert.Contains(t, response.Error.Message, "Unsupported protocol version")
		assert.Contains(t, response.Error.Message, clientVersion)
		
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Timeout waiting for response")
	}
}

func TestProtocolVersionNegotiation_FallbackToSupported(t *testing.T) {
	transport := NewMockTransport()
	defer transport.Close()

	// Non-strict server that falls back to supported versions
	server := NewMockServer(transport, "2024-11-05", []string{"2024-10-07", "2024-09-15"}, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server.Start(ctx)

	// Client requests with older supported version
	clientVersion := "2024-10-07"
	resp, err := transport.SendRequest("initialize", map[string]interface{}{
		"protocolVersion": clientVersion,
		"clientInfo": map[string]interface{}{
			"name":    "test-client",
			"version": "1.0.0",
		},
		"capabilities": ClientCapabilities{},
	})
	require.NoError(t, err)

	// Wait for response
	select {
	case response := <-resp:
		require.NotNil(t, response)
		assert.Nil(t, response.Error, "Should not have error for supported version")
		
		// Parse response
		var initResp InitializeResponse
		respData, err := json.Marshal(response.Result)
		require.NoError(t, err)
		err = json.Unmarshal(respData, &initResp)
		require.NoError(t, err)
		
		assert.Equal(t, clientVersion, initResp.ProtocolVersion, "Server should use client's supported version")
		
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Timeout waiting for response")
	}
}

func TestProtocolVersionNegotiation_MultipleSupportedVersions(t *testing.T) {
	transport := NewMockTransport()
	defer transport.Close()

	// Server with multiple supported versions
	supportedVersions := []string{"2024-09-15", "2024-10-07", "2024-11-05"}
	server := NewMockServer(transport, "2024-11-05", supportedVersions[:2], false) // Latest not in supportedVersions list
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server.Start(ctx)

	// Test each supported version
	for _, version := range supportedVersions {
		t.Run(fmt.Sprintf("version_%s", version), func(t *testing.T) {
			resp, err := transport.SendRequest("initialize", map[string]interface{}{
				"protocolVersion": version,
				"clientInfo": map[string]interface{}{
					"name":    "test-client",
					"version": "1.0.0",
				},
				"capabilities": ClientCapabilities{},
			})
			require.NoError(t, err)

			// Wait for response
			select {
			case response := <-resp:
				require.NotNil(t, response)
				assert.Nil(t, response.Error, "Should not have error for version %s", version)
				
				// Parse response
				var initResp InitializeResponse
				respData, err := json.Marshal(response.Result)
				require.NoError(t, err)
				err = json.Unmarshal(respData, &initResp)
				require.NoError(t, err)
				
				assert.Equal(t, version, initResp.ProtocolVersion, "Server should respond with requested version %s", version)
				
			case <-time.After(200 * time.Millisecond):
				t.Fatalf("Timeout waiting for response for version %s", version)
			}
		})
	}
}

func TestProtocolVersionNegotiation_VersionUpgradeDowngrade(t *testing.T) {
	tests := []struct {
		name              string
		serverVersion     string
		supportedVersions []string
		clientVersion     string
		expectedVersion   string
		strict            bool
	}{
		{
			name:              "upgrade_to_newer_supported",
			serverVersion:     "2024-11-05",
			supportedVersions: []string{"2024-10-07"},
			clientVersion:     "2024-10-07",
			expectedVersion:   "2024-10-07",
			strict:            false,
		},
		{
			name:              "downgrade_to_older_supported",
			serverVersion:     "2024-09-15",
			supportedVersions: []string{"2024-10-07", "2024-11-05"},
			clientVersion:     "2024-11-05",
			expectedVersion:   "2024-11-05",
			strict:            false,
		},
		{
			name:              "fallback_to_server_version",
			serverVersion:     "2024-11-05",
			supportedVersions: []string{"2024-10-07"},
			clientVersion:     "2024-12-01", // Unsupported future version
			expectedVersion:   "2024-11-05", // Should fallback to server version
			strict:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := NewMockTransport()
			defer transport.Close()

			server := NewMockServer(transport, tt.serverVersion, tt.supportedVersions, tt.strict)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			server.Start(ctx)

			resp, err := transport.SendRequest("initialize", map[string]interface{}{
				"protocolVersion": tt.clientVersion,
				"clientInfo": map[string]interface{}{
					"name":    "test-client",
					"version": "1.0.0",
				},
				"capabilities": ClientCapabilities{},
			})
			require.NoError(t, err)

			// Wait for response
			select {
			case response := <-resp:
				require.NotNil(t, response)
				
				if tt.strict && tt.clientVersion != tt.serverVersion && !contains(tt.supportedVersions, tt.clientVersion) {
					assert.NotNil(t, response.Error, "Should have error in strict mode for unsupported version")
				} else {
					assert.Nil(t, response.Error, "Should not have error")
					
					// Parse response
					var initResp InitializeResponse
					respData, err := json.Marshal(response.Result)
					require.NoError(t, err)
					err = json.Unmarshal(respData, &initResp)
					require.NoError(t, err)
					
					assert.Equal(t, tt.expectedVersion, initResp.ProtocolVersion, "Should negotiate to expected version")
				}
				
			case <-time.After(200 * time.Millisecond):
				t.Fatal("Timeout waiting for response")
			}
		})
	}
}

func TestProtocolVersionNegotiation_InvalidVersionRejection(t *testing.T) {
	transport := NewMockTransport()
	defer transport.Close()

	server := NewMockServer(transport, "2024-11-05", []string{"2024-10-07"}, true) // Strict mode
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server.Start(ctx)

	invalidVersions := []string{
		"invalid-version",
		"",
		"1.0.0", // Wrong format
		"2024-13-01", // Invalid date
		"2024-02-30", // Invalid date
	}

	for _, invalidVersion := range invalidVersions {
		t.Run(fmt.Sprintf("invalid_%s", invalidVersion), func(t *testing.T) {
			resp, err := transport.SendRequest("initialize", map[string]interface{}{
				"protocolVersion": invalidVersion,
				"clientInfo": map[string]interface{}{
					"name":    "test-client",
					"version": "1.0.0",
				},
				"capabilities": ClientCapabilities{},
			})
			require.NoError(t, err)

			// Wait for response
			select {
			case response := <-resp:
				require.NotNil(t, response)
				assert.NotNil(t, response.Error, "Should have error for invalid version: %s", invalidVersion)
				assert.Equal(t, -32602, response.Error.Code)
				
			case <-time.After(200 * time.Millisecond):
				t.Fatalf("Timeout waiting for response for invalid version: %s", invalidVersion)
			}
		})
	}
}

// Helper function to check if slice contains string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Test the actual protocol version constant
func TestProtocolVersionConstant(t *testing.T) {
	assert.Equal(t, "2024-11-05", ProtocolVersion, "Protocol version should match expected MCP version")
	assert.NotEmpty(t, ProtocolVersion, "Protocol version should not be empty")
	
	// Verify it follows the expected date format (YYYY-MM-DD)
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}$`, ProtocolVersion, "Protocol version should follow YYYY-MM-DD format")
}

// Benchmark version negotiation performance
func BenchmarkProtocolVersionNegotiation(b *testing.B) {
	transport := NewMockTransport()
	defer transport.Close()

	server := NewMockServer(transport, "2024-11-05", []string{"2024-10-07"}, false)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	server.Start(ctx)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := transport.SendRequest("initialize", map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"clientInfo": map[string]interface{}{
					"name":    "bench-client",
					"version": "1.0.0",
				},
				"capabilities": ClientCapabilities{},
			})
			if err != nil {
				b.Fatal(err)
			}
			
			// Wait for response
			select {
			case response := <-resp:
				if response == nil || response.Error != nil {
					b.Fatal("Invalid response")
				}
			case <-time.After(100 * time.Millisecond):
				b.Fatal("Timeout")
			}
		}
	})
}
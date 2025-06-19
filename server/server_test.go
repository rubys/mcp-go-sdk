package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockTransport for testing
type MockTransport struct {
	requests      chan *shared.Request
	notifications chan *shared.Notification
	responses     []MockResponse
	mu            sync.Mutex
}

type MockResponse struct {
	ID     interface{}
	Result interface{}
	Error  *shared.RPCError
}

func NewMockTransport() *MockTransport {
	return &MockTransport{
		requests:      make(chan *shared.Request, 10),
		notifications: make(chan *shared.Notification, 10),
		responses:     make([]MockResponse, 0),
	}
}

func (m *MockTransport) Channels() (<-chan *shared.Request, <-chan *shared.Notification) {
	return m.requests, m.notifications
}

func (m *MockTransport) SendResponse(id interface{}, result interface{}, err *shared.RPCError) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, MockResponse{
		ID:     id,
		Result: result,
		Error:  err,
	})
	return nil
}

func (m *MockTransport) SendNotification(method string, params interface{}) error {
	return nil
}

func (m *MockTransport) Close() error {
	return nil
}

func (m *MockTransport) GetResponses() []MockResponse {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]MockResponse, len(m.responses))
	copy(result, m.responses)
	return result
}

func TestServer_BasicFunctionality(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	transport := NewMockTransport()

	config := ServerConfig{
		Name:                  "Test Server",
		Version:               "1.0.0",
		MaxConcurrentRequests: 10,
		RequestTimeout:        1 * time.Second,
		Capabilities: shared.ServerCapabilities{
			Resources: &shared.ResourcesCapability{
				Subscribe: true,
			},
		},
	}

	server := NewServer(ctx, transport, config)

	// Register a test resource
	server.RegisterResource(
		"test://resource",
		"Test Resource",
		"A test resource",
		func(ctx context.Context, uri string) ([]shared.Content, error) {
			return []shared.Content{
				shared.TextContent{
					Type: shared.ContentTypeText,
					Text: "Test content",
				},
			}, nil
		},
	)

	// Start the server
	err := server.Start()
	require.NoError(t, err)

	// Send an initialize request
	initReq := &shared.Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": shared.ProtocolVersion,
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}

	transport.requests <- initReq

	// Wait for response
	time.Sleep(100 * time.Millisecond)

	// Check responses
	responses := transport.GetResponses()
	assert.Len(t, responses, 1)
	assert.Equal(t, 1, responses[0].ID)
	assert.Nil(t, responses[0].Error)

	// Test resource list
	listReq := &shared.Request{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "resources/list",
	}

	transport.requests <- listReq
	time.Sleep(100 * time.Millisecond)

	responses = transport.GetResponses()
	assert.Len(t, responses, 2)

	// Clean up
	server.Close()
}

func TestServer_ConcurrentRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	transport := NewMockTransport()

	config := ServerConfig{
		Name:                  "Test Server",
		Version:               "1.0.0",
		MaxConcurrentRequests: 50,
		RequestTimeout:        2 * time.Second,
	}

	server := NewServer(ctx, transport, config)
	err := server.Start()
	require.NoError(t, err)

	// Send multiple concurrent requests
	const numRequests = 20
	var wg sync.WaitGroup

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := &shared.Request{
				JSONRPC: "2.0",
				ID:      i,
				Method:  "resources/list",
			}
			transport.requests <- req
		}(i)
	}

	wg.Wait()

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	responses := transport.GetResponses()
	assert.Len(t, responses, numRequests)

	server.Close()
}

func TestServer_ResourceHandlers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	transport := NewMockTransport()
	config := ServerConfig{
		Name:    "Test Server",
		Version: "1.0.0",
	}

	server := NewServer(ctx, transport, config)

	// Test resource registration
	server.RegisterResource(
		"test://example",
		"Example Resource",
		"An example resource for testing",
		func(ctx context.Context, uri string) ([]shared.Content, error) {
			return []shared.Content{
				shared.TextContent{
					Type: shared.ContentTypeText,
					Text: "Example content for " + uri,
				},
			}, nil
		},
	)

	err := server.Start()
	require.NoError(t, err)

	// Test resources/list
	listReq := &shared.Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "resources/list",
	}

	transport.requests <- listReq
	time.Sleep(100 * time.Millisecond)

	responses := transport.GetResponses()
	require.Len(t, responses, 1)

	// Verify resource is in the list
	result, ok := responses[0].Result.(map[string]interface{})
	require.True(t, ok)

	resources, ok := result["resources"].([]shared.Resource)
	require.True(t, ok)
	require.Len(t, resources, 1)

	assert.Equal(t, "test://example", resources[0].URI)
	assert.Equal(t, "Example Resource", resources[0].Name)

	server.Close()
}

func BenchmarkServer_RequestThroughput(b *testing.B) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	transport := NewMockTransport()
	config := ServerConfig{
		Name:                  "Benchmark Server",
		Version:               "1.0.0",
		MaxConcurrentRequests: 100,
	}

	server := NewServer(ctx, transport, config)
	server.Start()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			req := &shared.Request{
				JSONRPC: "2.0",
				ID:      i,
				Method:  "resources/list",
			}
			transport.requests <- req
			i++
		}
	})

	server.Close()
}

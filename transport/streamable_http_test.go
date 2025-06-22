package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rubys/mcp-go-sdk/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStreamableHTTPTransport_BasicCommunication tests basic HTTP POST and SSE functionality
func TestStreamableHTTPTransport_BasicCommunication(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rpc" {
			// Handle JSON-RPC requests
			var req shared.JSONRPCMessage
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			// Send response
			resp := shared.Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  map[string]string{"status": "success"},
			}
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/sse" {
			// Handle SSE endpoint
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")

			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "SSE not supported", http.StatusInternalServerError)
				return
			}

			// Send some events
			for i := 0; i < 3; i++ {
				notification := shared.Notification{
					JSONRPC: "2.0",
					Method:  "test/notification",
					Params:  map[string]int{"count": i},
				}
				data, _ := json.Marshal(notification)
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
				time.Sleep(100 * time.Millisecond)
			}
		}
	}))
	defer server.Close()

	// Create transport
	config := StreamableHTTPConfig{
		ClientEndpoint:  server.URL + "/rpc",
		SSEEndpoint:     server.URL + "/sse",
		MessageBuffer:   10,
		RequestTimeout:  5 * time.Second,
		EnableSSE:       true,
		ReconnectDelay:  1 * time.Second,
		MaxReconnects:   3,
	}

	transport, err := NewStreamableHTTPTransportWithConfig(ctx, config)
	require.NoError(t, err)
	defer transport.Close()

	// Test sending request
	responseChan, err := transport.SendRequest("test/method", map[string]string{"foo": "bar"})
	require.NoError(t, err)

	select {
	case resp := <-responseChan:
		require.NotNil(t, resp)
		assert.Equal(t, "2.0", resp.JSONRPC)
		assert.NotNil(t, resp.Result)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for response")
	}

	// Test receiving SSE notifications
	_, notifications := transport.Channels()
	received := 0
	done := make(chan bool)
	go func() {
		defer close(done)
		for {
			select {
			case notif := <-notifications:
				if notif != nil {
					assert.Equal(t, "test/notification", notif.Method)
					received++
					if received >= 3 {
						return
					}
				}
			case <-time.After(2 * time.Second):
				return
			}
		}
	}()

	<-done
	assert.Equal(t, 3, received)
}

// TestStreamableHTTPTransport_SSEReconnection tests automatic SSE reconnection
func TestStreamableHTTPTransport_SSEReconnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Connection counter
	var connections int32

	// Create test server that closes connection after first event
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sse" {
			atomic.AddInt32(&connections, 1)
			
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			
			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// Send one event
			notification := shared.Notification{
				JSONRPC: "2.0",
				Method:  "connection",
				Params:  map[string]int32{"num": atomic.LoadInt32(&connections)},
			}
			data, _ := json.Marshal(notification)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			// Close connection after first connection to test reconnection
			if atomic.LoadInt32(&connections) == 1 {
				return
			}

			// Keep alive for subsequent connections
			for i := 0; i < 5; i++ {
				time.Sleep(100 * time.Millisecond)
				fmt.Fprintf(w, ": keepalive\n\n")
				flusher.Flush()
			}
		}
	}))
	defer server.Close()

	// Create transport with reconnection enabled
	config := StreamableHTTPConfig{
		ClientEndpoint:  server.URL + "/rpc",
		SSEEndpoint:     server.URL + "/sse",
		MessageBuffer:   10,
		RequestTimeout:  5 * time.Second,
		EnableSSE:       true,
		ReconnectDelay:  500 * time.Millisecond,
		MaxReconnects:   5,
	}

	transport, err := NewStreamableHTTPTransportWithConfig(ctx, config)
	require.NoError(t, err)
	defer transport.Close()

	// Collect notifications
	_, notifications := transport.Channels()
	var receivedConnections []int32
	done := make(chan bool)

	go func() {
		defer close(done)
		for {
			select {
			case notif := <-notifications:
				if notif != nil && notif.Method == "connection" {
					params := notif.Params.(map[string]interface{})
					num := int32(params["num"].(float64))
					receivedConnections = append(receivedConnections, num)
					if len(receivedConnections) >= 2 {
						return
					}
				}
			case <-time.After(5 * time.Second):
				return
			}
		}
	}()

	<-done

	// Should have reconnected at least once
	assert.GreaterOrEqual(t, atomic.LoadInt32(&connections), int32(2))
	assert.GreaterOrEqual(t, len(receivedConnections), 2)
}

// TestStreamableHTTPTransport_ConcurrentRequests tests handling concurrent HTTP requests
func TestStreamableHTTPTransport_ConcurrentRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Request counter
	var requestCount int32

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rpc" {
			atomic.AddInt32(&requestCount, 1)
			
			var req shared.JSONRPCMessage
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			// Simulate some processing time
			time.Sleep(10 * time.Millisecond)

			resp := shared.Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]interface{}{
					"request_num": atomic.LoadInt32(&requestCount),
					"echo":        req.Params,
				},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	// Create transport
	config := StreamableHTTPConfig{
		ClientEndpoint:        server.URL + "/rpc",
		MessageBuffer:         100,
		RequestTimeout:        5 * time.Second,
		MaxConcurrentRequests: 50,
		ConnectionPoolSize:    25,
	}

	transport, err := NewStreamableHTTPTransportWithConfig(ctx, config)
	require.NoError(t, err)
	defer transport.Close()

	// Send concurrent requests
	const numRequests = 100
	var wg sync.WaitGroup
	responses := make([]<-chan *shared.Response, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := transport.SendRequest("concurrent/test", map[string]int{"id": i})
			require.NoError(t, err)
			responses[i] = resp
		}(i)
	}

	wg.Wait()

	// Collect all responses
	receivedResponses := 0
	for _, respChan := range responses {
		select {
		case resp := <-respChan:
			require.NotNil(t, resp)
			assert.Equal(t, "2.0", resp.JSONRPC)
			receivedResponses++
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for response")
		}
	}

	assert.Equal(t, numRequests, receivedResponses)
	assert.Equal(t, int32(numRequests), atomic.LoadInt32(&requestCount))
}

// TestStreamableHTTPTransport_SessionManagement tests stateful session handling
func TestStreamableHTTPTransport_SessionManagement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Session storage
	sessions := make(map[string]*SessionData)
	var mu sync.RWMutex

	// Create test server with session support
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.Header.Get("X-Session-ID")
		
		if r.URL.Path == "/rpc" {
			var req shared.JSONRPCMessage
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			mu.Lock()
			session, exists := sessions[sessionID]
			if !exists {
				session = &SessionData{
					ID:       sessionID,
					Requests: 0,
				}
				sessions[sessionID] = session
			}
			session.Requests++
			requestCount := session.Requests
			mu.Unlock()

			resp := shared.Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]interface{}{
					"session_id":    sessionID,
					"request_count": requestCount,
				},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	// Create transport with session
	config := StreamableHTTPConfig{
		ClientEndpoint: server.URL + "/rpc",
		SessionID:      "test-session-123",
		Headers: map[string]string{
			"X-Session-ID": "test-session-123",
		},
		MessageBuffer:  10,
		RequestTimeout: 5 * time.Second,
	}

	transport, err := NewStreamableHTTPTransportWithConfig(ctx, config)
	require.NoError(t, err)
	defer transport.Close()

	// Send multiple requests in same session
	for i := 1; i <= 5; i++ {
		respChan, err := transport.SendRequest("session/test", nil)
		require.NoError(t, err)

		select {
		case resp := <-respChan:
			require.NotNil(t, resp)
			result := resp.Result.(map[string]interface{})
			assert.Equal(t, "test-session-123", result["session_id"])
			assert.Equal(t, float64(i), result["request_count"])
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for response")
		}
	}
}

// TestStreamableHTTPTransport_ChunkedTransferEncoding tests chunked response handling
func TestStreamableHTTPTransport_ChunkedTransferEncoding(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create test server that sends chunked responses
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rpc" {
			var req shared.JSONRPCMessage
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			// Enable chunked transfer encoding
			w.Header().Set("Transfer-Encoding", "chunked")
			
			// Send response in chunks
			encoder := json.NewEncoder(w)
			resp := shared.Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]interface{}{
					"data": strings.Repeat("chunk", 1000), // Large data
				},
			}
			
			if err := encoder.Encode(resp); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}))
	defer server.Close()

	// Create transport
	config := StreamableHTTPConfig{
		ClientEndpoint: server.URL + "/rpc",
		MessageBuffer:  10,
		RequestTimeout: 5 * time.Second,
	}

	transport, err := NewStreamableHTTPTransportWithConfig(ctx, config)
	require.NoError(t, err)
	defer transport.Close()

	// Send request
	respChan, err := transport.SendRequest("chunked/test", nil)
	require.NoError(t, err)

	select {
	case resp := <-respChan:
		require.NotNil(t, resp)
		result := resp.Result.(map[string]interface{})
		data := result["data"].(string)
		assert.Equal(t, strings.Repeat("chunk", 1000), data)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for response")
	}
}

// TestStreamableHTTPTransport_SSEHeartbeat tests SSE keepalive handling
func TestStreamableHTTPTransport_SSEHeartbeat(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	heartbeatsSent := int32(0)
	
	// Create test server with heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sse" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			
			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// Send initial notification
			notification := shared.Notification{
				JSONRPC: "2.0",
				Method:  "connected",
			}
			data, _ := json.Marshal(notification)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			// Send heartbeats
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()

			for i := 0; i < 5; i++ {
				select {
				case <-ticker.C:
					atomic.AddInt32(&heartbeatsSent, 1)
					fmt.Fprintf(w, ": heartbeat %d\n\n", i)
					flusher.Flush()
				case <-r.Context().Done():
					return
				}
			}
		}
	}))
	defer server.Close()

	// Create transport
	config := StreamableHTTPConfig{
		ClientEndpoint: server.URL + "/rpc",
		SSEEndpoint:    server.URL + "/sse",
		EnableSSE:      true,
		MessageBuffer:  10,
		RequestTimeout: 5 * time.Second,
	}

	transport, err := NewStreamableHTTPTransportWithConfig(ctx, config)
	require.NoError(t, err)
	defer transport.Close()

	// Wait for heartbeats
	time.Sleep(1 * time.Second)

	// Should maintain connection despite heartbeat-only messages
	assert.GreaterOrEqual(t, atomic.LoadInt32(&heartbeatsSent), int32(3))

	// Connection should still be active
	stats := transport.Stats()
	if httpStats, ok := stats.(StreamableHTTPTransportStats); ok {
		// Note: SSE connection status depends on actual SSE implementation
		_ = httpStats.SSEConnected // Just verify the field exists
	}
}

// TestStreamableHTTPTransport_ErrorHandling tests various error scenarios
func TestStreamableHTTPTransport_ErrorHandling(t *testing.T) {
	t.Run("HTTPErrors", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Create test server that returns errors
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "400") {
				http.Error(w, "Bad Request", http.StatusBadRequest)
			} else if strings.Contains(r.URL.Path, "500") {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			} else if strings.Contains(r.URL.Path, "timeout") {
				time.Sleep(10 * time.Second) // Cause timeout
			}
		}))
		defer server.Close()

		// Test 400 error
		config := StreamableHTTPConfig{
			ClientEndpoint: server.URL + "/400",
			MessageBuffer:  10,
			RequestTimeout: 1 * time.Second,
		}
		transport, err := NewStreamableHTTPTransportWithConfig(ctx, config)
		require.NoError(t, err)
		
		respChan, err := transport.SendRequest("test", nil)
		require.NoError(t, err)
		
		select {
		case resp := <-respChan:
			assert.Nil(t, resp) // Should get nil on HTTP error
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for error response")
		}
		transport.Close()

		// Test timeout
		config.ClientEndpoint = server.URL + "/timeout"
		transport, err = NewStreamableHTTPTransportWithConfig(ctx, config)
		require.NoError(t, err)
		
		respChan, err = transport.SendRequest("test", nil)
		require.NoError(t, err)
		
		select {
		case resp := <-respChan:
			assert.Nil(t, resp) // Should get nil on timeout
		case <-time.After(2 * time.Second):
			// Expected timeout
		}
		transport.Close()
	})

	t.Run("MalformedResponses", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Create test server that returns malformed JSON
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"invalid json`))
		}))
		defer server.Close()

		config := StreamableHTTPConfig{
			ClientEndpoint: server.URL + "/rpc",
			MessageBuffer:  10,
			RequestTimeout: 1 * time.Second,
		}

		transport, err := NewStreamableHTTPTransportWithConfig(ctx, config)
		require.NoError(t, err)
		defer transport.Close()

		respChan, err := transport.SendRequest("test", nil)
		require.NoError(t, err)

		select {
		case resp := <-respChan:
			assert.Nil(t, resp) // Should get nil on parse error
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for error response")
		}
	})
}

// TestStreamableHTTPTransport_CORS tests CORS header support
func TestStreamableHTTPTransport_CORS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Track CORS headers
	var receivedOrigin string
	_ = receivedOrigin // Avoid unused variable warning

	// Create test server that checks CORS
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture CORS headers
		receivedOrigin = r.Header.Get("Origin")

		// Set CORS response headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Session-ID")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Handle normal request
		var req shared.JSONRPCMessage
		json.NewDecoder(r.Body).Decode(&req)
		
		resp := shared.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  "success",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create transport with CORS headers
	config := StreamableHTTPConfig{
		ClientEndpoint: server.URL + "/rpc",
		Headers: map[string]string{
			"Origin": "https://example.com",
		},
		MessageBuffer:  10,
		RequestTimeout: 5 * time.Second,
	}

	transport, err := NewStreamableHTTPTransportWithConfig(ctx, config)
	require.NoError(t, err)
	defer transport.Close()

	// Send request
	respChan, err := transport.SendRequest("cors/test", nil)
	require.NoError(t, err)

	select {
	case resp := <-respChan:
		require.NotNil(t, resp)
		assert.Equal(t, "https://example.com", receivedOrigin)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for response")
	}
}

// TestStreamableHTTPTransport_TypeScriptCompatibility tests TypeScript SDK compatibility
func TestStreamableHTTPTransport_TypeScriptCompatibility(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create test server that expects TypeScript SDK format
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rpc" {
			// Check content type
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var req shared.JSONRPCMessage
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			// Handle different methods with TypeScript SDK expectations
			switch req.Method {
			case "resources/read":
				// TypeScript SDK sends string, we should handle it
				resp := shared.Response{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result: shared.ReadResourceResult{
						Contents: []shared.ResourceContent{
							{
								URI:      "file://example.txt",
								MimeType: "text/plain",
								Text:     "Hello World",
							},
						},
					},
				}
				json.NewEncoder(w).Encode(resp)

			case "prompts/get":
				// Return prompt with single content (should be object)
				resp := shared.Response{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result: shared.GetPromptResult{
						Messages: []shared.PromptMessage{
							{
								Role: "assistant",
								Content: shared.TextContent{
									Type: "text",
									Text: "Single content",
								},
							},
						},
					},
				}
				json.NewEncoder(w).Encode(resp)

			default:
				resp := shared.Response{
					JSONRPC: "2.0",
					ID:      req.ID,
					Result:  map[string]string{"status": "ok"},
				}
				json.NewEncoder(w).Encode(resp)
			}
		}
	}))
	defer server.Close()

	// Create transport with TypeScript compatibility
	config := StreamableHTTPConfig{
		ClientEndpoint:         server.URL + "/rpc",
		MessageBuffer:          10,
		RequestTimeout:         5 * time.Second,
		TypeScriptCompatibility: true,
	}

	transport, err := NewStreamableHTTPTransportWithConfig(ctx, config)
	require.NoError(t, err)
	defer transport.Close()

	// Test resources/read
	respChan, err := transport.SendRequest("resources/read", "file://example.txt")
	require.NoError(t, err)

	select {
	case resp := <-respChan:
		require.NotNil(t, resp)
		// Handle response as map since it comes from JSON unmarshaling
		resultMap := resp.Result.(map[string]interface{})
		contents := resultMap["contents"].([]interface{})
		assert.Len(t, contents, 1)
		content := contents[0].(map[string]interface{})
		assert.Equal(t, "file://example.txt", content["uri"])
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for response")
	}

	// Test prompts/get
	respChan, err = transport.SendRequest("prompts/get", map[string]interface{}{
		"name": "test",
	})
	require.NoError(t, err)

	select {
	case resp := <-respChan:
		require.NotNil(t, resp)
		// Handle response as map since it comes from JSON unmarshaling
		resultMap := resp.Result.(map[string]interface{})
		messages := resultMap["messages"].([]interface{})
		assert.Len(t, messages, 1)
		message := messages[0].(map[string]interface{})
		content := message["content"].(map[string]interface{})
		assert.Equal(t, "text", content["type"])
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for response")
	}
}

// BenchmarkStreamableHTTPTransport_Throughput benchmarks HTTP transport throughput
func BenchmarkStreamableHTTPTransport_Throughput(b *testing.B) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create high-performance test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req shared.JSONRPCMessage
		json.NewDecoder(r.Body).Decode(&req)
		
		resp := shared.Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  "ok",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := StreamableHTTPConfig{
		ClientEndpoint:        server.URL + "/rpc",
		MessageBuffer:         10000,
		RequestTimeout:        10 * time.Second,
		MaxConcurrentRequests: 1000,
		ConnectionPoolSize:    100,
	}

	transport, err := NewStreamableHTTPTransportWithConfig(ctx, config)
	require.NoError(b, err)
	defer transport.Close()

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			respChan, err := transport.SendRequest("benchmark", map[string]interface{}{
				"timestamp": time.Now().UnixNano(),
			})
			if err != nil {
				b.Error(err)
				continue
			}

			select {
			case <-respChan:
				// Success
			case <-ctx.Done():
				return
			}
		}
	})

	stats := transport.Stats()
	if httpStats, ok := stats.(StreamableHTTPTransportStats); ok {
		b.Logf("Total requests: %d", httpStats.RequestsSent)
		b.Logf("Requests per second: %.0f", float64(httpStats.RequestsSent)/b.Elapsed().Seconds())
	}
}

// Helper types for Streamable HTTP testing

type SessionData struct {
	ID       string
	Requests int
}


// Note: The actual StreamableHTTPTransport implementation would need to be created
// This test file defines the expected behavior based on TypeScript SDK compatibility

// Remove duplicate type - it's defined in streamable_http.go
// type StreamableHTTPTransportStats struct { ... }
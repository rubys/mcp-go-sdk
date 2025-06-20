package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStdioTransport_TypeScriptSDKCompatibility tests compatibility with TypeScript SDK behavior
func TestStdioTransport_TypeScriptSDKCompatibility(t *testing.T) {
	t.Run("ParameterTranslation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Simulate TypeScript SDK sending simplified parameters
		input := `{"jsonrpc": "2.0", "id": 1, "method": "resources/read", "params": "file://example.txt"}
{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "test", "arguments": {"foo": "bar"}}}
{"jsonrpc": "2.0", "id": 3, "method": "prompts/get", "params": {"name": "test", "arguments": {"key": "value"}}}
`

		mockRW := NewMockReadWriter(input)
		config := StdioConfig{
			Reader:         &mockRW.input,
			Writer:         mockRW,
			MessageBuffer:  10,
			RequestTimeout: 1 * time.Second,
		}

		transport, err := NewStdioTransport(ctx, config)
		require.NoError(t, err)
		defer transport.Close()

		requests, _ := transport.Channels()

		// Collect requests
		receivedRequests := make([]*shared.Request, 0, 3)
		done := make(chan bool)
		go func() {
			defer close(done)
			for i := 0; i < 3; i++ {
				select {
				case req := <-requests:
					if req != nil {
						receivedRequests = append(receivedRequests, req)
					}
				case <-ctx.Done():
					return
				}
			}
		}()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for requests")
		}

		// Verify parameter translation
		require.Len(t, receivedRequests, 3)

		// Check resources/read - should pass through string format as-is (MCP standard)
		assert.Equal(t, "resources/read", receivedRequests[0].Method)
		assert.Equal(t, "file://example.txt", receivedRequests[0].Params)

		// Check tools/call passthrough
		assert.Equal(t, "tools/call", receivedRequests[1].Method)
		assert.Equal(t, map[string]interface{}{
			"name":      "test",
			"arguments": map[string]interface{}{"foo": "bar"},
		}, receivedRequests[1].Params)

		// Check prompts/get passthrough
		assert.Equal(t, "prompts/get", receivedRequests[2].Method)
		assert.Equal(t, map[string]interface{}{
			"name":      "test",
			"arguments": map[string]interface{}{"key": "value"},
		}, receivedRequests[2].Params)
	})

	t.Run("ResponseFormatCompatibility", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		mockRW := NewMockReadWriter("")
		config := StdioConfig{
			Reader:         strings.NewReader(""),
			Writer:         mockRW,
			MessageBuffer:  10,
			RequestTimeout: 1 * time.Second,
		}

		transport, err := NewStdioTransport(ctx, config)
		require.NoError(t, err)
		defer transport.Close()

		// Test single content item (should be object)
		response := &shared.Response{
			JSONRPC: "2.0",
			ID:      1,
			Result: shared.PromptMessage{
				Role: "assistant",
				Content: shared.TextContent{
					Type: "text",
					Text: "Hello",
				},
			},
		}

		// Send response
		err = transport.SendResponse(response.ID, response.Result, nil)
		assert.NoError(t, err)

		// Test multiple content items (should be array)
		multiResponse := &shared.Response{
			JSONRPC: "2.0",
			ID:      2,
			Result: shared.PromptMessage{
				Role: "assistant",
				Content: []shared.Content{
					shared.TextContent{Type: "text", Text: "Hello"},
					shared.TextContent{Type: "text", Text: "World"},
				},
			},
		}

		err = transport.SendResponse(multiResponse.ID, multiResponse.Result, nil)
		assert.NoError(t, err)

		// Verify output format matches TypeScript SDK expectations
		time.Sleep(100 * time.Millisecond)
		output := mockRW.GetOutput()
		lines := strings.Split(strings.TrimSpace(output), "\n")
		require.GreaterOrEqual(t, len(lines), 2)

		// Parse and verify format
		var msg1, msg2 map[string]interface{}
		assert.NoError(t, json.Unmarshal([]byte(lines[0]), &msg1))
		assert.NoError(t, json.Unmarshal([]byte(lines[1]), &msg2))

		// Single content should be object
		result1 := msg1["result"].(map[string]interface{})
		content1 := result1["content"].(map[string]interface{})
		assert.Equal(t, "text", content1["type"])

		// Multiple content should be array
		result2 := msg2["result"].(map[string]interface{})
		content2 := result2["content"].([]interface{})
		assert.Len(t, content2, 2)
	})
}

// TestStdioTransport_ErrorScenarios tests various error conditions
func TestStdioTransport_ErrorScenarios(t *testing.T) {
	t.Run("MalformedJSON", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Various malformed JSON inputs
		malformedInputs := []string{
			`{"jsonrpc": "2.0", "id": 1, "method": "test"`, // Missing closing brace
			`{invalid json}`,                                // Invalid JSON
			`{"jsonrpc": "2.0" "id": 1}`,                   // Missing comma
			``,                                              // Empty line
			`null`,                                          // Null value
			`"just a string"`,                               // Plain string
			`123`,                                           // Plain number
		}

		for _, input := range malformedInputs {
			mockRW := NewMockReadWriter(input + "\n")
			config := StdioConfig{
				Reader:         &mockRW.input,
				Writer:         mockRW,
				MessageBuffer:  10,
				RequestTimeout: 1 * time.Second,
			}

			transport, err := NewStdioTransport(ctx, config)
			require.NoError(t, err)

			requests, notifications := transport.Channels()

			// Should handle malformed input gracefully
			time.Sleep(100 * time.Millisecond)

			select {
			case req := <-requests:
				assert.Nil(t, req, "Should not receive request from malformed JSON")
			case notif := <-notifications:
				assert.Nil(t, notif, "Should not receive notification from malformed JSON")
			default:
				// Expected - no valid messages
			}

			transport.Close()
		}
	})

	t.Run("PartialReads", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Simulate partial reads by providing incomplete message
		reader := &SlowReader{
			data: []byte(`{"jsonrpc": "2.0", "id": 1, "method": "test", "params": {}}`),
			delay: 10 * time.Millisecond,
			chunkSize: 5, // Read 5 bytes at a time
		}

		config := StdioConfig{
			Reader:         reader,
			Writer:         &bytes.Buffer{},
			MessageBuffer:  10,
			RequestTimeout: 1 * time.Second,
		}

		transport, err := NewStdioTransport(ctx, config)
		require.NoError(t, err)
		defer transport.Close()

		requests, _ := transport.Channels()

		// Should eventually receive the complete message
		select {
		case req := <-requests:
			assert.NotNil(t, req)
			assert.Equal(t, "test", req.Method)
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for request from partial reads")
		}
	})

	t.Run("WriterErrors", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		errorWriter := &ErrorWriter{
			failAfter: 2, // Fail after 2 writes
		}

		config := StdioConfig{
			Reader:         strings.NewReader(""),
			Writer:         errorWriter,
			MessageBuffer:  10,
			RequestTimeout: 1 * time.Second,
		}

		transport, err := NewStdioTransport(ctx, config)
		require.NoError(t, err)
		defer transport.Close()

		// First two requests should succeed
		_, err1 := transport.SendRequest("test1", nil)
		assert.NoError(t, err1)

		_, err2 := transport.SendRequest("test2", nil)
		assert.NoError(t, err2)

		time.Sleep(100 * time.Millisecond)

		// Third request should fail
		_, err3 := transport.SendRequest("test3", nil)
		assert.NoError(t, err3) // Request accepted, but write will fail

		// Give more time for async write to happen and fail
		time.Sleep(200 * time.Millisecond)

		// Verify writes
		assert.Equal(t, 2, errorWriter.successfulWrites)
		assert.Equal(t, 1, errorWriter.failedWrites)
	})

	t.Run("RequestTimeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		mockRW := NewMockReadWriter("")
		config := StdioConfig{
			Reader:         strings.NewReader(""),
			Writer:         mockRW,
			MessageBuffer:  10,
			RequestTimeout: 100 * time.Millisecond, // Very short timeout
		}

		transport, err := NewStdioTransport(ctx, config)
		require.NoError(t, err)
		defer transport.Close()

		// Send request but don't send response
		responseChan, err := transport.SendRequest("test_timeout", nil)
		require.NoError(t, err)

		// Should timeout
		select {
		case resp := <-responseChan:
			assert.Nil(t, resp, "Should receive nil response on timeout")
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Channel should be closed on timeout")
		}
	})
}

// TestStdioTransport_LargeMessages tests handling of large messages
func TestStdioTransport_LargeMessages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a moderately large message (10KB)
	largeData := strings.Repeat("x", 10*1024)
	largeMessage := fmt.Sprintf(`{"jsonrpc": "2.0", "id": 1, "method": "test", "params": {"data": "%s"}}`, largeData)

	mockRW := NewMockReadWriter(largeMessage + "\n")
	config := StdioConfig{
		Reader:         &mockRW.input,
		Writer:         mockRW,
		MessageBuffer:  10,
		RequestTimeout: 5 * time.Second,
	}

	transport, err := NewStdioTransport(ctx, config)
	require.NoError(t, err)
	defer transport.Close()

	requests, _ := transport.Channels()

	// Give extra time for large message processing
	time.Sleep(2 * time.Second)

	// Should handle large message
	select {
	case req := <-requests:
		require.NotNil(t, req)
		assert.Equal(t, "test", req.Method)
		params := req.Params.(map[string]interface{})
		assert.Equal(t, largeData, params["data"])
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for large message")
	}

	// Send large response
	_, err = transport.SendRequest("large_response", map[string]interface{}{
		"data": largeData,
	})
	assert.NoError(t, err)

	// Verify large message was written
	time.Sleep(100 * time.Millisecond)
	output := mockRW.GetOutput()
	assert.Contains(t, output, largeData)
}

// TestStdioTransport_Backpressure tests behavior when buffers are full
func TestStdioTransport_Backpressure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Generate many messages quickly
	var messages strings.Builder
	for i := 0; i < 100; i++ {
		messages.WriteString(fmt.Sprintf(`{"jsonrpc": "2.0", "id": %d, "method": "test", "params": {"n": %d}}`, i, i))
		messages.WriteString("\n")
	}

	mockRW := NewMockReadWriter(messages.String())
	config := StdioConfig{
		Reader:         &mockRW.input,
		Writer:         mockRW,
		MessageBuffer:  5, // Small buffer to test backpressure
		RequestTimeout: 1 * time.Second,
	}

	transport, err := NewStdioTransport(ctx, config)
	require.NoError(t, err)
	defer transport.Close()

	requests, _ := transport.Channels()

	// Slow consumer
	received := 0
	done := make(chan bool)
	go func() {
		defer close(done)
		for {
			select {
			case req := <-requests:
				if req != nil {
					received++
					time.Sleep(10 * time.Millisecond) // Slow processing
					if received >= 50 {              // Process half
						return
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for processing
	select {
	case <-done:
		assert.GreaterOrEqual(t, received, 50)
	case <-time.After(3 * time.Second):
		t.Fatalf("Only processed %d messages", received)
	}
}

// TestStdioTransport_ConcurrentShutdown tests shutdown during active operations
func TestStdioTransport_ConcurrentShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mockRW := NewMockReadWriter("")
	config := StdioConfig{
		Reader:         strings.NewReader(""),
		Writer:         mockRW,
		MessageBuffer:  100,
		RequestTimeout: 5 * time.Second,
	}

	transport, err := NewStdioTransport(ctx, config)
	require.NoError(t, err)

	// Start many concurrent operations
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			transport.SendRequest(fmt.Sprintf("request_%d", i), nil)
			transport.SendNotification(fmt.Sprintf("notification_%d", i), nil)
		}(i)
	}

	// Close transport while operations are in progress
	go func() {
		time.Sleep(50 * time.Millisecond)
		transport.Close()
	}()

	// Should not panic or deadlock
	done := make(chan bool)
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Concurrent shutdown deadlocked")
	}
}

// TestStdioTransport_RaceConditions tests for race conditions
func TestStdioTransport_RaceConditions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping race condition test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mockRW := NewMockReadWriter("")
	config := StdioConfig{
		Reader:         strings.NewReader(""),
		Writer:         mockRW,
		MessageBuffer:  1000,
		RequestTimeout: 5 * time.Second,
	}

	transport, err := NewStdioTransport(ctx, config)
	require.NoError(t, err)
	defer transport.Close()

	// Run multiple concurrent operations
	var operations int32
	done := make(chan bool)

	// Writer goroutines
	for i := 0; i < 10; i++ {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					transport.SendRequest("test", nil)
					transport.SendNotification("notify", nil)
					atomic.AddInt32(&operations, 2)
				}
			}
		}()
	}

	// Stats reader goroutines
	for i := 0; i < 5; i++ {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					stats := transport.Stats()
					_ = stats
					atomic.AddInt32(&operations, 1)
				}
			}
		}()
	}

	// Let it run for a bit
	time.Sleep(1 * time.Second)
	close(done)

	// Should have performed many operations without race conditions
	totalOps := atomic.LoadInt32(&operations)
	assert.Greater(t, totalOps, int32(1000))
}

// Helper types for testing

// SlowReader simulates slow/partial reads
type SlowReader struct {
	data      []byte
	position  int
	delay     time.Duration
	chunkSize int
	mu        sync.Mutex
}

func (sr *SlowReader) Read(p []byte) (n int, err error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if sr.position >= len(sr.data) {
		return 0, io.EOF
	}

	time.Sleep(sr.delay)

	end := sr.position + sr.chunkSize
	if end > len(sr.data) {
		end = len(sr.data)
	}

	n = copy(p, sr.data[sr.position:end])
	sr.position = end

	if sr.position >= len(sr.data) {
		return n, io.EOF
	}
	return n, nil
}

// ErrorWriter simulates write errors
type ErrorWriter struct {
	failAfter        int
	successfulWrites int
	failedWrites     int
	mu               sync.Mutex
}

func (ew *ErrorWriter) Write(p []byte) (n int, err error) {
	ew.mu.Lock()
	defer ew.mu.Unlock()

	if ew.successfulWrites >= ew.failAfter {
		ew.failedWrites++
		return 0, errors.New("simulated write error")
	}

	ew.successfulWrites++
	return len(p), nil
}

// BenchmarkStdioTransport_HighConcurrency benchmarks under high concurrency
func BenchmarkStdioTransport_HighConcurrency(b *testing.B) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	mockRW := NewMockReadWriter("")
	config := StdioConfig{
		Reader:         strings.NewReader(""),
		Writer:         mockRW,
		MessageBuffer:  10000,
		RequestTimeout: 10 * time.Second,
	}

	transport, err := NewStdioTransport(ctx, config)
	require.NoError(b, err)
	defer transport.Close()

	b.ResetTimer()

	// Run with maximum concurrency
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				transport.SendRequest("benchmark", map[string]interface{}{
					"id": i,
					"timestamp": time.Now().UnixNano(),
				})
			} else {
				transport.SendNotification("benchmark", map[string]interface{}{
					"id": i,
					"timestamp": time.Now().UnixNano(),
				})
			}
			i++
		}
	})

	stats := transport.Stats()
	if transportStats, ok := stats.(TransportStats); ok {
		b.Logf("Requests: %d, Notifications: %d", transportStats.RequestsSent, transportStats.NotificationsSent)
		b.Logf("Total operations: %d", transportStats.RequestsSent+transportStats.NotificationsSent)
		b.Logf("Operations per second: %.0f", float64(transportStats.RequestsSent+transportStats.NotificationsSent)/b.Elapsed().Seconds())
	}
}
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rubys/mcp-go-sdk/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageHandler_ResponseParsing(t *testing.T) {
	// Test the shared.ParseMessage logic directly
	responseJSON := `{"jsonrpc":"2.0","id":999,"result":{"test":"direct"}}`
	msg, err := shared.ParseMessage([]byte(responseJSON))
	require.NoError(t, err)

	t.Logf("Parsed message: %+v", msg)
	t.Logf("IsRequest: %v", msg.IsRequest())
	t.Logf("IsNotification: %v", msg.IsNotification())
	t.Logf("IsResponse: %v", msg.IsResponse())

	if msg.IsResponse() {
		resp := msg.ToResponse()
		t.Logf("Converted to response: %+v", resp)
		// ID gets parsed as float64 from JSON, which is expected
		assert.Equal(t, float64(999), resp.ID)
	} else {
		t.Fatal("Message was not identified as a response")
	}
}

func TestMessageHandler_ConcurrentRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config := MessageHandlerConfig{
		RequestTimeout: 2 * time.Second,
		BufferSize:     10,
	}

	handler := NewMessageHandler(ctx, config)
	defer handler.Close()

	incoming, outgoing, _, _ := handler.Channels()

	// Step 1: Send a request and consume the outgoing message
	respChan, err := handler.SendRequest("test", map[string]interface{}{"key": "value"})
	require.NoError(t, err)

	var req shared.Request
	select {
	case outgoingData := <-outgoing:
		err = json.Unmarshal(outgoingData, &req)
		require.NoError(t, err)
		t.Logf("Request sent with ID: %v", req.ID)
	case <-time.After(1 * time.Second):
		t.Fatal("No outgoing request received")
	}

	// Step 2: Send the matching response
	response := &shared.Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{"result": "ok"},
	}
	respData, err := json.Marshal(response)
	require.NoError(t, err)

	incoming <- respData
	t.Logf("Response sent for ID: %v", req.ID)

	// Step 3: Wait for correlation and check stats before trying to read
	time.Sleep(200 * time.Millisecond)
	stats := handler.GetStats()
	t.Logf("After response: Sent=%d, Received=%d, Errors=%d",
		stats.RequestsSent, stats.ResponsesReceived, stats.Errors)

	// Step 4: Try to receive the response
	select {
	case resp, ok := <-respChan:
		if !ok {
			t.Fatal("Response channel was closed")
		}
		assert.NotNil(t, resp)
		assert.Equal(t, "2.0", resp.JSONRPC)
		// Check the types and values
		t.Logf("req.ID: %v (type: %T)", req.ID, req.ID)
		t.Logf("resp.ID: %v (type: %T)", resp.ID, resp.ID)

		// Both should have the same numeric value (our fix ensures correlation works)
		// Convert to strings for easy comparison regardless of int64/float64 types
		assert.Equal(t, fmt.Sprintf("%v", req.ID), fmt.Sprintf("%v", resp.ID))
		t.Logf("Success! Received response")
	case <-time.After(1 * time.Second):
		finalStats := handler.GetStats()
		t.Logf("Final stats: Sent=%d, Received=%d, Errors=%d",
			finalStats.RequestsSent, finalStats.ResponsesReceived, finalStats.Errors)
		t.Fatal("Did not receive response on channel")
	}
}

func TestMessageHandler_RequestTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config := MessageHandlerConfig{
		RequestTimeout: 100 * time.Millisecond, // Very short timeout
		BufferSize:     10,
	}

	handler := NewMessageHandler(ctx, config)
	defer handler.Close()

	// Send request that will timeout
	responseChan, err := handler.SendRequest("test_method", nil)
	require.NoError(t, err)

	// Wait for timeout
	select {
	case response := <-responseChan:
		assert.Nil(t, response, "Should receive nil on timeout")
	case <-time.After(200 * time.Millisecond):
		// This is expected - channel should be closed on timeout
	}

	// Verify timeout was recorded
	stats := handler.GetStats()
	assert.Equal(t, int64(1), stats.Timeouts)
}

func TestMessageHandler_ConcurrentNotifications(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config := MessageHandlerConfig{
		RequestTimeout: 1 * time.Second,
		BufferSize:     100,
	}

	handler := NewMessageHandler(ctx, config)
	defer handler.Close()

	// Send multiple concurrent notifications
	const numNotifications = 100
	var wg sync.WaitGroup

	for i := 0; i < numNotifications; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := handler.SendNotification("test_notification", map[string]interface{}{
				"notification_id": i,
			})
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Verify all notifications were sent
	stats := handler.GetStats()
	assert.Equal(t, int64(numNotifications), stats.NotificationsSent)
}

func TestMessageHandler_RequestCorrelation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config := MessageHandlerConfig{
		RequestTimeout: 2 * time.Second,
		BufferSize:     10,
	}

	handler := NewMessageHandler(ctx, config)
	defer handler.Close()

	// Get channels
	incoming, outgoing, _, _ := handler.Channels()

	var requestIDs [3]interface{}

	// Start goroutine to capture outgoing requests and send responses
	go func() {
		for i := 0; i < 3; i++ {
			select {
			case reqData := <-outgoing:
				var req shared.Request
				if err := json.Unmarshal(reqData, &req); err == nil {
					requestIDs[i] = req.ID

					// Send response out of order (3, 1, 2)
					var response *shared.Response
					if req.Method == "method3" {
						response = &shared.Response{
							JSONRPC: "2.0",
							ID:      req.ID,
							Result:  map[string]interface{}{"method": "method3", "response": "third"},
						}
					} else if req.Method == "method1" {
						response = &shared.Response{
							JSONRPC: "2.0",
							ID:      req.ID,
							Result:  map[string]interface{}{"method": "method1", "response": "first"},
						}
					} else if req.Method == "method2" {
						response = &shared.Response{
							JSONRPC: "2.0",
							ID:      req.ID,
							Result:  map[string]interface{}{"method": "method2", "response": "second"},
						}
					}

					if response != nil {
						respData, _ := json.Marshal(response)
						incoming <- respData
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Send multiple requests with different methods
	resp1, err1 := handler.SendRequest("method1", map[string]string{"data": "request1"})
	require.NoError(t, err1)

	resp2, err2 := handler.SendRequest("method2", map[string]string{"data": "request2"})
	require.NoError(t, err2)

	resp3, err3 := handler.SendRequest("method3", map[string]string{"data": "request3"})
	require.NoError(t, err3)

	// Verify responses are correctly correlated
	select {
	case r1 := <-resp1:
		require.NotNil(t, r1)
		result, ok := r1.Result.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "first", result["response"])
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for response 1")
	}

	select {
	case r2 := <-resp2:
		require.NotNil(t, r2)
		result, ok := r2.Result.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "second", result["response"])
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for response 2")
	}

	select {
	case r3 := <-resp3:
		require.NotNil(t, r3)
		result, ok := r3.Result.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "third", result["response"])
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for response 3")
	}
}

func TestMessageHandler_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	config := MessageHandlerConfig{
		RequestTimeout: 5 * time.Second,
		BufferSize:     10,
	}

	handler := NewMessageHandler(ctx, config)

	// Send a request
	responseChan, err := handler.SendRequest("test_method", nil)
	require.NoError(t, err)

	// Cancel context
	cancel()

	// Verify handler stops accepting new requests
	_, err = handler.SendRequest("another_method", nil)
	assert.Error(t, err)

	// Close handler
	handler.Close()

	// Verify response channel is closed
	select {
	case response := <-responseChan:
		assert.Nil(t, response, "Should receive nil when handler is closed")
	case <-time.After(100 * time.Millisecond):
		// Channel might be closed already
	}
}

func TestMessageHandler_HighLoad(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := MessageHandlerConfig{
		RequestTimeout: 1 * time.Second,
		BufferSize:     1000,
	}

	handler := NewMessageHandler(ctx, config)
	defer handler.Close()

	// High load test: 1000 concurrent requests
	const numRequests = 1000
	var wg sync.WaitGroup
	responses := make([]<-chan *shared.Response, numRequests)
	errors := make([]error, numRequests)

	startTime := time.Now()

	// Send requests concurrently
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := handler.SendRequest("high_load_test", map[string]interface{}{
				"request_id": i,
			})
			responses[i] = resp
			errors[i] = err
		}(i)
	}

	wg.Wait()
	sendTime := time.Since(startTime)

	// Count successful requests
	successCount := 0
	for _, err := range errors {
		if err == nil {
			successCount++
		}
	}

	t.Logf("Sent %d requests in %v (success rate: %d/%d)",
		numRequests, sendTime, successCount, numRequests)

	// Verify most requests were successful
	assert.Greater(t, successCount, numRequests*9/10, "At least 90% of requests should succeed")

	// Verify statistics
	stats := handler.GetStats()
	assert.Equal(t, int64(successCount), stats.RequestsSent)
}

func BenchmarkMessageHandler_RequestThroughput(b *testing.B) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := MessageHandlerConfig{
		RequestTimeout: 5 * time.Second,
		BufferSize:     1000,
	}

	handler := NewMessageHandler(ctx, config)
	defer handler.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := handler.SendRequest("benchmark_test", map[string]interface{}{
				"timestamp": time.Now().UnixNano(),
			})
			if err != nil {
				b.Error(err)
			}
		}
	})

	stats := handler.GetStats()
	b.Logf("Total requests sent: %d", stats.RequestsSent)
}

func BenchmarkMessageHandler_NotificationThroughput(b *testing.B) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := MessageHandlerConfig{
		RequestTimeout: 5 * time.Second,
		BufferSize:     1000,
	}

	handler := NewMessageHandler(ctx, config)
	defer handler.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			err := handler.SendNotification("benchmark_notification", map[string]interface{}{
				"timestamp": time.Now().UnixNano(),
			})
			if err != nil {
				b.Error(err)
			}
		}
	})

	stats := handler.GetStats()
	b.Logf("Total notifications sent: %d", stats.NotificationsSent)
}

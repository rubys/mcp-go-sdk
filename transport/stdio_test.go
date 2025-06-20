package transport

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockReadWriter implements io.Reader and io.Writer for testing
type MockReadWriter struct {
	input  strings.Reader
	output strings.Builder
	mu     sync.Mutex
}

func NewMockReadWriter(input string) *MockReadWriter {
	return &MockReadWriter{
		input: *strings.NewReader(input),
	}
}

func (m *MockReadWriter) Read(p []byte) (n int, err error) {
	return m.input.Read(p)
}

func (m *MockReadWriter) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.output.Write(p)
}

func (m *MockReadWriter) GetOutput() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.output.String()
}

func TestStdioTransport_BasicCommunication(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Prepare test input (JSON-RPC messages)
	input := `{"jsonrpc": "2.0", "id": 1, "method": "test", "params": {"data": "test1"}}
{"jsonrpc": "2.0", "id": 2, "method": "test", "params": {"data": "test2"}}
{"jsonrpc": "2.0", "method": "notification", "params": {"data": "notify1"}}
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

	requests, notifications := transport.Channels()

	// Give time for input to be read and processed
	time.Sleep(500 * time.Millisecond)

	// Collect received messages
	var receivedRequests []*shared.Request
	var receivedNotifications []*shared.Notification

	done := make(chan bool)
	go func() {
		defer close(done)
		requestCount := 0
		notificationCount := 0

		for {
			select {
			case req := <-requests:
				if req != nil {
					receivedRequests = append(receivedRequests, req)
					requestCount++
				}
			case notif := <-notifications:
				if notif != nil {
					receivedNotifications = append(receivedNotifications, notif)
					notificationCount++
				}
			case <-ctx.Done():
				return
			}
			
			// Exit when we have all expected messages
			if requestCount == 2 && notificationCount == 1 {
				return
			}
		}
	}()

	// Wait for messages to be processed
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for messages")
	}

	// Verify received messages
	assert.Len(t, receivedRequests, 2)
	assert.Len(t, receivedNotifications, 1)

	// Verify request details
	assert.Equal(t, "test", receivedRequests[0].Method)
	assert.Equal(t, "test", receivedRequests[1].Method)

	// Verify notification details
	assert.Equal(t, "notification", receivedNotifications[0].Method)
}

func TestStdioTransport_ConcurrentRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mockRW := NewMockReadWriter("")

	config := StdioConfig{
		Reader:         strings.NewReader(""),
		Writer:         mockRW,
		MessageBuffer:  100,
		RequestTimeout: 2 * time.Second,
	}

	transport, err := NewStdioTransport(ctx, config)
	require.NoError(t, err)
	defer transport.Close()

	// Send multiple concurrent requests
	const numRequests = 50
	var wg sync.WaitGroup
	responses := make([]<-chan *shared.Response, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := transport.SendRequest("concurrent_test", map[string]interface{}{
				"request_id": i,
			})
			require.NoError(t, err)
			responses[i] = resp
		}(i)
	}

	wg.Wait()

	// Give time for all requests to be written to output
	time.Sleep(500 * time.Millisecond)

	// Verify all requests were sent
	stats := transport.Stats()
	if transportStats, ok := stats.(TransportStats); ok {
		assert.Equal(t, int64(numRequests), transportStats.RequestsSent)
	}

	// Check that output contains JSON-RPC messages
	output := mockRW.GetOutput()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Len(t, lines, numRequests)

	// Verify each line is valid JSON-RPC
	for _, line := range lines {
		if line != "" {
			msg, err := shared.ParseMessage([]byte(line))
			assert.NoError(t, err)
			assert.NotNil(t, msg)
			assert.Equal(t, "2.0", msg.JSONRPC)
		}
	}
}

func TestStdioTransport_ConcurrentNotifications(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mockRW := NewMockReadWriter("")

	config := StdioConfig{
		Reader:         strings.NewReader(""),
		Writer:         mockRW,
		MessageBuffer:  100,
		RequestTimeout: 1 * time.Second,
	}

	transport, err := NewStdioTransport(ctx, config)
	require.NoError(t, err)
	defer transport.Close()

	// Send multiple concurrent notifications
	const numNotifications = 100
	var wg sync.WaitGroup

	for i := 0; i < numNotifications; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := transport.SendNotification("concurrent_notification", map[string]interface{}{
				"notification_id": i,
			})
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Give time for all notifications to be written to output
	time.Sleep(1 * time.Second) // Longer delay for 100 notifications

	// Verify all notifications were sent
	stats := transport.Stats()
	if transportStats, ok := stats.(TransportStats); ok {
		assert.Equal(t, int64(numNotifications), transportStats.NotificationsSent)
	}

	// Check output
	output := mockRW.GetOutput()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Len(t, lines, numNotifications)
}

func TestStdioTransport_ResponseHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Input contains response to a request
	input := `{"jsonrpc": "2.0", "id": 1, "result": {"status": "success"}}
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

	// Send a request first (this will generate ID 1)
	responseChan, err := transport.SendRequest("test_method", nil)
	require.NoError(t, err)

	// Wait for response
	select {
	case response := <-responseChan:
		require.NotNil(t, response)
		assert.Equal(t, "2.0", response.JSONRPC)
		assert.Equal(t, int64(1), response.ID)
		assert.NotNil(t, response.Result)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for response")
	}
}

func TestStdioTransport_Stats(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
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

	// Initial stats
	stats := transport.Stats()
	if transportStats, ok := stats.(TransportStats); ok {
		assert.Equal(t, int64(0), transportStats.RequestsSent)
		assert.Equal(t, int64(0), transportStats.NotificationsSent)
	}

	// Send some requests and notifications
	transport.SendRequest("test1", nil)
	transport.SendRequest("test2", nil)
	transport.SendNotification("notif1", nil)
	transport.SendNotification("notif2", nil)
	transport.SendNotification("notif3", nil)

	// Wait a bit for async processing
	time.Sleep(100 * time.Millisecond)

	// Check updated stats
	stats = transport.Stats()
	if transportStats, ok := stats.(TransportStats); ok {
		assert.Equal(t, int64(2), transportStats.RequestsSent)
		assert.Equal(t, int64(3), transportStats.NotificationsSent)
	}
}

func TestStdioTransport_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	mockRW := NewMockReadWriter("")

	config := StdioConfig{
		Reader:         strings.NewReader(""),
		Writer:         mockRW,
		MessageBuffer:  10,
		RequestTimeout: 5 * time.Second,
	}

	transport, err := NewStdioTransport(ctx, config)
	require.NoError(t, err)

	// Send a request
	responseChan, err := transport.SendRequest("test_method", nil)
	require.NoError(t, err)

	// Cancel context
	cancel()

	// Wait a bit for cancellation to propagate
	time.Sleep(100 * time.Millisecond)

	// Verify new requests fail
	_, err = transport.SendRequest("another_method", nil)
	assert.Error(t, err)

	// Close transport
	err = transport.Close()
	assert.NoError(t, err)

	// Verify response channel is handled gracefully
	select {
	case response := <-responseChan:
		// Response might be nil due to cancellation
		_ = response
	case <-time.After(100 * time.Millisecond):
		// Channel might be closed
	}
}

func BenchmarkStdioTransport_RequestThroughput(b *testing.B) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mockRW := NewMockReadWriter("")

	config := StdioConfig{
		Reader:         strings.NewReader(""),
		Writer:         mockRW,
		MessageBuffer:  1000,
		RequestTimeout: 5 * time.Second,
	}

	transport, err := NewStdioTransport(ctx, config)
	require.NoError(b, err)
	defer transport.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := transport.SendRequest("benchmark_test", map[string]interface{}{
				"timestamp": time.Now().UnixNano(),
			})
			if err != nil {
				b.Error(err)
			}
		}
	})

	stats := transport.Stats()
	if transportStats, ok := stats.(TransportStats); ok {
		b.Logf("Total requests sent: %d", transportStats.RequestsSent)
	}
}

func BenchmarkStdioTransport_NotificationThroughput(b *testing.B) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mockRW := NewMockReadWriter("")

	config := StdioConfig{
		Reader:         strings.NewReader(""),
		Writer:         mockRW,
		MessageBuffer:  1000,
		RequestTimeout: 5 * time.Second,
	}

	transport, err := NewStdioTransport(ctx, config)
	require.NoError(b, err)
	defer transport.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			err := transport.SendNotification("benchmark_notification", map[string]interface{}{
				"timestamp": time.Now().UnixNano(),
			})
			if err != nil {
				b.Error(err)
			}
		}
	})

	stats := transport.Stats()
	if transportStats, ok := stats.(TransportStats); ok {
		b.Logf("Total notifications sent: %d", transportStats.NotificationsSent)
	}
}

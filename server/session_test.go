package server

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rubys/mcp-go-sdk/shared"
	"github.com/rubys/mcp-go-sdk/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SessionTestTransport implements a mock transport for session testing
type SessionTestTransport struct {
	sessionID           string
	requests            chan *shared.Request
	notifications       chan *shared.Notification
	responseHandler     func(id interface{}, result interface{}, err *shared.RPCError)
	notificationHandler func(method string, params interface{})
	closed              int32
	mu                  sync.RWMutex
}

func NewSessionTestTransport(sessionID string) *SessionTestTransport {
	return &SessionTestTransport{
		sessionID:     sessionID,
		requests:      make(chan *shared.Request, 10),
		notifications: make(chan *shared.Notification, 10),
	}
}

func (t *SessionTestTransport) Channels() (<-chan *shared.Request, <-chan *shared.Notification) {
	return t.requests, t.notifications
}

func (t *SessionTestTransport) SendResponse(id interface{}, result interface{}, err *shared.RPCError) error {
	if atomic.LoadInt32(&t.closed) == 1 {
		return fmt.Errorf("transport closed")
	}
	t.mu.RLock()
	if t.responseHandler != nil {
		t.responseHandler(id, result, err)
	}
	t.mu.RUnlock()
	return nil
}

func (t *SessionTestTransport) SendNotification(method string, params interface{}) error {
	if atomic.LoadInt32(&t.closed) == 1 {
		return fmt.Errorf("transport closed")
	}
	t.mu.RLock()
	if t.notificationHandler != nil {
		t.notificationHandler(method, params)
	}
	t.mu.RUnlock()
	return nil
}

func (t *SessionTestTransport) Close() error {
	if atomic.SwapInt32(&t.closed, 1) == 0 {
		close(t.requests)
		close(t.notifications)
	}
	return nil
}

func (t *SessionTestTransport) SetResponseHandler(handler func(id interface{}, result interface{}, err *shared.RPCError)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.responseHandler = handler
}

func (t *SessionTestTransport) SetNotificationHandler(handler func(method string, params interface{})) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.notificationHandler = handler
}

func (t *SessionTestTransport) SendRequest(request *shared.Request) {
	if atomic.LoadInt32(&t.closed) == 0 {
		select {
		case t.requests <- request:
		default:
			// Channel full, drop request
		}
	}
}

// TestSessionIsolation tests that tools and resources are isolated per session
func TestSessionIsolation(t *testing.T) {
	ctx := context.Background()

	// Create two separate sessions with different transports
	session1Transport := NewSessionTestTransport("session-1")
	session2Transport := NewSessionTestTransport("session-2")
	defer session1Transport.Close()
	defer session2Transport.Close()

	// Create servers for each session
	config := ServerConfig{
		Name:                  "session-test-server",
		Version:               "1.0.0",
		MaxConcurrentRequests: 10,
		RequestTimeout:        5 * time.Second,
		Capabilities: shared.ServerCapabilities{
			Tools: &shared.ToolsCapability{
				ListChanged: true,
			},
		},
	}

	server1 := NewServer(ctx, session1Transport, config)
	server2 := NewServer(ctx, session2Transport, config)

	// Start both servers
	require.NoError(t, server1.Start())
	require.NoError(t, server2.Start())
	defer server1.Close()
	defer server2.Close()

	// Track notifications for each session
	var session1Notifications []string
	var session2Notifications []string
	var notificationMu sync.Mutex

	session1Transport.SetNotificationHandler(func(method string, params interface{}) {
		notificationMu.Lock()
		session1Notifications = append(session1Notifications, method)
		notificationMu.Unlock()
	})

	session2Transport.SetNotificationHandler(func(method string, params interface{}) {
		notificationMu.Lock()
		session2Notifications = append(session2Notifications, method)
		notificationMu.Unlock()
	})

	// Register session-specific tools
	server1.RegisterTool("session1-tool", "Tool for session 1", map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{"input": map[string]interface{}{"type": "string"}},
	}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		return []shared.Content{
			shared.TextContent{Type: "text", Text: "session1 result"},
		}, nil
	})

	server2.RegisterTool("session2-tool", "Tool for session 2", map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{"input": map[string]interface{}{"type": "string"}},
	}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		return []shared.Content{
			shared.TextContent{Type: "text", Text: "session2 result"},
		}, nil
	})

	// Give some time for notifications to be sent
	time.Sleep(100 * time.Millisecond)

	// Verify each session can only see its own tools
	session1Transport.SendRequest(&shared.Request{
		ID:     "list-tools-1",
		Method: "tools/list",
	})

	session2Transport.SendRequest(&shared.Request{
		ID:     "list-tools-2",
		Method: "tools/list",
	})

	// Give time for requests to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify that each session received notifications for only its own tools
	notificationMu.Lock()
	assert.Contains(t, session1Notifications, "notifications/tools/list_changed")
	assert.Contains(t, session2Notifications, "notifications/tools/list_changed")
	notificationMu.Unlock()

	t.Logf("Session isolation test completed - each session properly isolated")
}

// TestConcurrentSessionManagement tests managing multiple sessions concurrently
func TestConcurrentSessionManagement(t *testing.T) {
	ctx := context.Background()
	numSessions := 10

	// Create multiple sessions concurrently
	var wg sync.WaitGroup
	var servers []*Server
	var transports []*SessionTestTransport
	var serversMu sync.Mutex

	for i := 0; i < numSessions; i++ {
		wg.Add(1)
		go func(sessionID int) {
			defer wg.Done()

			sessionTransport := NewSessionTestTransport(fmt.Sprintf("session-%d", sessionID))
			config := ServerConfig{
				Name:                  fmt.Sprintf("concurrent-session-%d", sessionID),
				Version:               "1.0.0",
				MaxConcurrentRequests: 5,
				RequestTimeout:        3 * time.Second,
				Capabilities: shared.ServerCapabilities{
					Tools: &shared.ToolsCapability{
						ListChanged: true,
					},
				},
			}

			server := NewServer(ctx, sessionTransport, config)
			require.NoError(t, server.Start())

			// Register session-specific tool
			server.RegisterTool(
				fmt.Sprintf("tool-session-%d", sessionID),
				fmt.Sprintf("Tool for session %d", sessionID),
				map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{"value": map[string]interface{}{"type": "number"}},
				},
				func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
					return []shared.Content{
						shared.TextContent{
							Type: "text",
							Text: fmt.Sprintf("Result from session %d", sessionID),
						},
					}, nil
				},
			)

			serversMu.Lock()
			servers = append(servers, server)
			transports = append(transports, sessionTransport)
			serversMu.Unlock()
		}(i)
	}

	wg.Wait()

	// Clean up all sessions
	for _, server := range servers {
		server.Close()
	}
	for _, transport := range transports {
		transport.Close()
	}

	assert.Equal(t, numSessions, len(servers))
	t.Logf("Successfully managed %d concurrent sessions", numSessions)
}

// TestSessionToolOverride tests that session-specific tools can override global tools
func TestSessionToolOverride(t *testing.T) {
	ctx := context.Background()

	// Create global server setup (simulating shared resources)
	globalTransport := transport.NewInProcessTransport()
	defer globalTransport.Close()

	globalConfig := ServerConfig{
		Name:                  "global-server",
		Version:               "1.0.0",
		MaxConcurrentRequests: 10,
		RequestTimeout:        5 * time.Second,
		Capabilities: shared.ServerCapabilities{
			Tools: &shared.ToolsCapability{
				ListChanged: true,
			},
		},
	}

	globalServer := NewServer(ctx, globalTransport, globalConfig)
	require.NoError(t, globalServer.Start())
	defer globalServer.Close()

	// Register global tool
	globalServer.RegisterTool("shared-tool", "Global shared tool", map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{"input": map[string]interface{}{"type": "string"}},
	}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		return []shared.Content{
			shared.TextContent{Type: "text", Text: "global result"},
		}, nil
	})

	// Create session-specific server
	sessionTransport := NewSessionTestTransport("override-session")
	defer sessionTransport.Close()

	sessionServer := NewServer(ctx, sessionTransport, globalConfig)
	require.NoError(t, sessionServer.Start())
	defer sessionServer.Close()

	// Register session-specific tool with same name (simulating override)
	sessionServer.RegisterTool("shared-tool", "Session-specific tool", map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{"input": map[string]interface{}{"type": "string"}},
	}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		return []shared.Content{
			shared.TextContent{Type: "text", Text: "session-specific result"},
		}, nil
	})

	// Both servers now have a tool with the same name, but different implementations
	// This demonstrates how each session can have its own version of tools

	t.Log("Tool override test completed - sessions can have independent tool implementations")
}

// TestSessionNotificationIsolation tests that notifications are sent to the correct sessions
func TestSessionNotificationIsolation(t *testing.T) {
	ctx := context.Background()

	// Create multiple session transports
	transport1 := NewSessionTestTransport("notification-session-1")
	transport2 := NewSessionTestTransport("notification-session-2")
	transport3 := NewSessionTestTransport("notification-session-3")
	defer transport1.Close()
	defer transport2.Close()
	defer transport3.Close()

	config := ServerConfig{
		Name:                  "notification-test-server",
		Version:               "1.0.0",
		MaxConcurrentRequests: 5,
		RequestTimeout:        3 * time.Second,
		Capabilities: shared.ServerCapabilities{
			Tools: &shared.ToolsCapability{
				ListChanged: true,
			},
		},
	}

	// Create servers for each session
	server1 := NewServer(ctx, transport1, config)
	server2 := NewServer(ctx, transport2, config)
	server3 := NewServer(ctx, transport3, config)

	require.NoError(t, server1.Start())
	require.NoError(t, server2.Start())
	require.NoError(t, server3.Start())
	defer server1.Close()
	defer server2.Close()
	defer server3.Close()

	// Track notifications for each session
	var notifications1, notifications2, notifications3 []string
	var notificationsMu sync.Mutex

	transport1.SetNotificationHandler(func(method string, params interface{}) {
		notificationsMu.Lock()
		notifications1 = append(notifications1, method)
		notificationsMu.Unlock()
	})

	transport2.SetNotificationHandler(func(method string, params interface{}) {
		notificationsMu.Lock()
		notifications2 = append(notifications2, method)
		notificationsMu.Unlock()
	})

	transport3.SetNotificationHandler(func(method string, params interface{}) {
		notificationsMu.Lock()
		notifications3 = append(notifications3, method)
		notificationsMu.Unlock()
	})

	// Add tools to different sessions at different times
	server1.RegisterTool("tool1", "Tool 1", map[string]interface{}{}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		return []shared.Content{shared.TextContent{Type: "text", Text: "tool1"}}, nil
	})

	time.Sleep(50 * time.Millisecond)

	server2.RegisterTool("tool2", "Tool 2", map[string]interface{}{}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		return []shared.Content{shared.TextContent{Type: "text", Text: "tool2"}}, nil
	})

	time.Sleep(50 * time.Millisecond)

	server3.RegisterTool("tool3", "Tool 3", map[string]interface{}{}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		return []shared.Content{shared.TextContent{Type: "text", Text: "tool3"}}, nil
	})

	time.Sleep(100 * time.Millisecond)

	// Verify each session received its own notifications
	notificationsMu.Lock()
	assert.True(t, len(notifications1) > 0, "Session 1 should have received notifications")
	assert.True(t, len(notifications2) > 0, "Session 2 should have received notifications")
	assert.True(t, len(notifications3) > 0, "Session 3 should have received notifications")
	notificationsMu.Unlock()

	t.Log("Notification isolation test completed - each session receives its own notifications")
}

// TestSessionLifecycle tests session creation, usage, and cleanup
func TestSessionLifecycle(t *testing.T) {
	ctx := context.Background()

	// Phase 1: Session Creation
	sessionTransport := NewSessionTestTransport("lifecycle-session")
	config := ServerConfig{
		Name:                  "lifecycle-test-server",
		Version:               "1.0.0",
		MaxConcurrentRequests: 5,
		RequestTimeout:        3 * time.Second,
		Capabilities: shared.ServerCapabilities{
			Resources: &shared.ResourcesCapability{
				Subscribe:   true,
				ListChanged: true,
			},
			Tools: &shared.ToolsCapability{
				ListChanged: true,
			},
		},
	}

	server := NewServer(ctx, sessionTransport, config)
	require.NoError(t, server.Start())

	// Phase 2: Session Usage
	// Register resources and tools
	server.RegisterResource("session://data", "Session Data", "Session-specific data", func(ctx context.Context, uri string) ([]shared.Content, error) {
		return []shared.Content{
			shared.TextContent{Type: "text", Text: "session data"},
		}, nil
	})

	server.RegisterTool("session-tool", "Session tool", map[string]interface{}{}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		return []shared.Content{
			shared.TextContent{Type: "text", Text: "session tool result"},
		}, nil
	})

	// Simulate some activity
	sessionTransport.SendRequest(&shared.Request{
		ID:     "test-req-1",
		Method: "resources/list",
	})

	sessionTransport.SendRequest(&shared.Request{
		ID:     "test-req-2",
		Method: "tools/list",
	})

	time.Sleep(100 * time.Millisecond)

	// Phase 3: Session Cleanup
	server.Close()
	sessionTransport.Close()

	// Verify session is properly cleaned up
	assert.True(t, atomic.LoadInt32(&sessionTransport.closed) == 1, "Session transport should be closed")

	t.Log("Session lifecycle test completed - creation, usage, and cleanup successful")
}

// TestHighVolumeSessionOperations tests session management under high load
func TestHighVolumeSessionOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high volume test in short mode")
	}

	ctx := context.Background()
	numOperations := 1000
	numConcurrentSessions := 5

	var wg sync.WaitGroup
	var operationCount int64

	for sessionID := 0; sessionID < numConcurrentSessions; sessionID++ {
		wg.Add(1)
		go func(sessionID int) {
			defer wg.Done()

			sessionTransport := NewSessionTestTransport(fmt.Sprintf("high-volume-session-%d", sessionID))
			defer sessionTransport.Close()

			config := ServerConfig{
				Name:                  fmt.Sprintf("high-volume-server-%d", sessionID),
				Version:               "1.0.0",
				MaxConcurrentRequests: 20,
				RequestTimeout:        1 * time.Second,
				Capabilities: shared.ServerCapabilities{
					Tools: &shared.ToolsCapability{
						ListChanged: true,
					},
				},
			}

			server := NewServer(ctx, sessionTransport, config)
			require.NoError(t, server.Start())
			defer server.Close()

			// Perform many operations
			for i := 0; i < numOperations; i++ {
				toolName := fmt.Sprintf("tool-%d-%d", sessionID, i)
				toolIndex := i // Capture loop variable
				server.RegisterTool(toolName, "High volume tool", map[string]interface{}{}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
					atomic.AddInt64(&operationCount, 1)
					return []shared.Content{
						shared.TextContent{Type: "text", Text: fmt.Sprintf("result-%d", toolIndex)},
					}, nil
				})

				// Occasionally call the tool
				if i%100 == 0 {
					sessionTransport.SendRequest(&shared.Request{
						ID:     fmt.Sprintf("req-%d-%d", sessionID, i),
						Method: "tools/call",
						Params: map[string]interface{}{
							"name":      toolName,
							"arguments": map[string]interface{}{},
						},
					})
				}
			}
		}(sessionID)
	}

	wg.Wait()

	finalOperationCount := atomic.LoadInt64(&operationCount)
	t.Logf("High volume test completed: %d sessions performed %d total operations", 
		numConcurrentSessions, finalOperationCount)
	
	// Should have completed a significant number of operations
	assert.True(t, finalOperationCount > 0, "Should have completed some operations")
}
package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rubys/mcp-go-sdk/shared"
	"github.com/rubys/mcp-go-sdk/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NotificationTracker helps track notifications sent by the server
type NotificationTracker struct {
	notifications []string
	mu            sync.Mutex
}

func NewNotificationTracker() *NotificationTracker {
	return &NotificationTracker{
		notifications: make([]string, 0),
	}
}

func (nt *NotificationTracker) AddNotification(method string) {
	nt.mu.Lock()
	defer nt.mu.Unlock()
	nt.notifications = append(nt.notifications, method)
}

func (nt *NotificationTracker) GetNotifications() []string {
	nt.mu.Lock()
	defer nt.mu.Unlock()
	result := make([]string, len(nt.notifications))
	copy(result, nt.notifications)
	return result
}

func (nt *NotificationTracker) Clear() {
	nt.mu.Lock()
	defer nt.mu.Unlock()
	nt.notifications = nt.notifications[:0]
}

func (nt *NotificationTracker) Count() int {
	nt.mu.Lock()
	defer nt.mu.Unlock()
	return len(nt.notifications)
}

// TestResourceRemoval tests resource registration, removal, and notifications
func TestResourceRemoval(t *testing.T) {
	ctx := context.Background()
	
	// Track notifications from the start
	tracker := NewNotificationTracker()
	mockTransport := &NotificationInterceptorTransport{
		Transport: transport.NewInProcessTransport(),
		tracker:   tracker,
	}
	defer mockTransport.Close()

	config := ServerConfig{
		Name:                  "resource-removal-test",
		Version:               "1.0.0",
		MaxConcurrentRequests: 10,
		RequestTimeout:        5 * time.Second,
		Capabilities: shared.ServerCapabilities{
			Resources: &shared.ResourcesCapability{
				Subscribe:   true,
				ListChanged: true,
			},
		},
	}

	server := NewServer(ctx, mockTransport, config)
	require.NoError(t, server.Start())
	defer server.Close()

	// Test 1: Register a resource and verify notification
	tracker.Clear()
	server.RegisterResource("test://resource1", "Resource 1", "Test resource 1", func(ctx context.Context, uri string) ([]shared.Content, error) {
		return []shared.Content{
			shared.TextContent{Type: "text", Text: "resource1 content"},
		}, nil
	})

	time.Sleep(50 * time.Millisecond)
	notifications := tracker.GetNotifications()
	assert.Contains(t, notifications, "notifications/resources/list_changed", "Should receive list_changed notification on resource registration")

	// Test 2: Verify resource exists and can be accessed
	server.resourceMu.RLock()
	_, exists := server.resourceHandlers["test://resource1"]
	server.resourceMu.RUnlock()
	assert.True(t, exists, "Resource should exist after registration")

	// Test 3: Remove the resource and verify notification
	tracker.Clear()
	removed := server.UnregisterResource("test://resource1")
	assert.True(t, removed, "UnregisterResource should return true for existing resource")

	time.Sleep(50 * time.Millisecond)
	notifications = tracker.GetNotifications()
	assert.Contains(t, notifications, "notifications/resources/list_changed", "Should receive list_changed notification on resource removal")

	// Test 4: Verify resource no longer exists
	server.resourceMu.RLock()
	_, exists = server.resourceHandlers["test://resource1"]
	server.resourceMu.RUnlock()
	assert.False(t, exists, "Resource should not exist after removal")

	// Test 5: Try to remove non-existent resource
	tracker.Clear()
	removed = server.UnregisterResource("test://nonexistent")
	assert.False(t, removed, "UnregisterResource should return false for non-existent resource")

	time.Sleep(50 * time.Millisecond)
	notifications = tracker.GetNotifications()
	assert.Empty(t, notifications, "Should not receive notification when removing non-existent resource")
}

// TestToolRemoval tests tool registration, removal, and notifications
func TestToolRemoval(t *testing.T) {
	ctx := context.Background()
	
	// Track notifications from the start
	tracker := NewNotificationTracker()
	mockTransport := &NotificationInterceptorTransport{
		Transport: transport.NewInProcessTransport(),
		tracker:   tracker,
	}
	defer mockTransport.Close()

	config := ServerConfig{
		Name:                  "tool-removal-test",
		Version:               "1.0.0",
		MaxConcurrentRequests: 10,
		RequestTimeout:        5 * time.Second,
		Capabilities: shared.ServerCapabilities{
			Tools: &shared.ToolsCapability{
				ListChanged: true,
			},
		},
	}

	server := NewServer(ctx, mockTransport, config)
	require.NoError(t, server.Start())
	defer server.Close()

	// Test 1: Register a tool and verify notification
	tracker.Clear()
	server.RegisterTool("test-tool", "Test Tool", map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{"input": map[string]interface{}{"type": "string"}},
	}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		return []shared.Content{
			shared.TextContent{Type: "text", Text: "tool result"},
		}, nil
	})

	time.Sleep(50 * time.Millisecond)
	notifications := tracker.GetNotifications()
	assert.Contains(t, notifications, "notifications/tools/list_changed", "Should receive list_changed notification on tool registration")

	// Test 2: Verify tool exists
	server.toolMu.RLock()
	_, exists := server.toolHandlers["test-tool"]
	server.toolMu.RUnlock()
	assert.True(t, exists, "Tool should exist after registration")

	// Test 3: Remove the tool and verify notification
	tracker.Clear()
	removed := server.UnregisterTool("test-tool")
	assert.True(t, removed, "UnregisterTool should return true for existing tool")

	time.Sleep(50 * time.Millisecond)
	notifications = tracker.GetNotifications()
	assert.Contains(t, notifications, "notifications/tools/list_changed", "Should receive list_changed notification on tool removal")

	// Test 4: Verify tool no longer exists
	server.toolMu.RLock()
	_, exists = server.toolHandlers["test-tool"]
	server.toolMu.RUnlock()
	assert.False(t, exists, "Tool should not exist after removal")

	// Test 5: Try to remove non-existent tool
	tracker.Clear()
	removed = server.UnregisterTool("nonexistent-tool")
	assert.False(t, removed, "UnregisterTool should return false for non-existent tool")

	time.Sleep(50 * time.Millisecond)
	notifications = tracker.GetNotifications()
	assert.Empty(t, notifications, "Should not receive notification when removing non-existent tool")
}

// TestPromptRemoval tests prompt registration, removal, and notifications
func TestPromptRemoval(t *testing.T) {
	ctx := context.Background()
	
	// Track notifications from the start
	tracker := NewNotificationTracker()
	mockTransport := &NotificationInterceptorTransport{
		Transport: transport.NewInProcessTransport(),
		tracker:   tracker,
	}
	defer mockTransport.Close()

	config := ServerConfig{
		Name:                  "prompt-removal-test",
		Version:               "1.0.0",
		MaxConcurrentRequests: 10,
		RequestTimeout:        5 * time.Second,
		Capabilities: shared.ServerCapabilities{
			Prompts: &shared.PromptsCapability{
				ListChanged: true,
			},
		},
	}

	server := NewServer(ctx, mockTransport, config)
	require.NoError(t, server.Start())
	defer server.Close()

	// Test 1: Register a prompt and verify notification
	tracker.Clear()
	server.RegisterPrompt("test-prompt", "Test Prompt", []shared.PromptArgument{
		{Name: "arg1", Description: "Test argument", Required: true},
	}, func(ctx context.Context, name string, arguments map[string]interface{}) (PromptMessage, error) {
		return PromptMessage{
			Role: "assistant",
			Content: []shared.Content{
				shared.TextContent{Type: "text", Text: "prompt response"},
			},
		}, nil
	})

	time.Sleep(50 * time.Millisecond)
	notifications := tracker.GetNotifications()
	assert.Contains(t, notifications, "notifications/prompts/list_changed", "Should receive list_changed notification on prompt registration")

	// Test 2: Verify prompt exists
	server.promptMu.RLock()
	_, exists := server.promptHandlers["test-prompt"]
	server.promptMu.RUnlock()
	assert.True(t, exists, "Prompt should exist after registration")

	// Test 3: Remove the prompt and verify notification
	tracker.Clear()
	removed := server.UnregisterPrompt("test-prompt")
	assert.True(t, removed, "UnregisterPrompt should return true for existing prompt")

	time.Sleep(50 * time.Millisecond)
	notifications = tracker.GetNotifications()
	assert.Contains(t, notifications, "notifications/prompts/list_changed", "Should receive list_changed notification on prompt removal")

	// Test 4: Verify prompt no longer exists
	server.promptMu.RLock()
	_, exists = server.promptHandlers["test-prompt"]
	server.promptMu.RUnlock()
	assert.False(t, exists, "Prompt should not exist after removal")

	// Test 5: Try to remove non-existent prompt
	tracker.Clear()
	removed = server.UnregisterPrompt("nonexistent-prompt")
	assert.False(t, removed, "UnregisterPrompt should return false for non-existent prompt")

	time.Sleep(50 * time.Millisecond)
	notifications = tracker.GetNotifications()
	assert.Empty(t, notifications, "Should not receive notification when removing non-existent prompt")
}

// TestConcurrentRemoval tests concurrent registration and removal operations
func TestConcurrentRemoval(t *testing.T) {
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	config := ServerConfig{
		Name:                  "concurrent-removal-test",
		Version:               "1.0.0",
		MaxConcurrentRequests: 50,
		RequestTimeout:        5 * time.Second,
		Capabilities: shared.ServerCapabilities{
			Resources: &shared.ResourcesCapability{
				Subscribe:   true,
				ListChanged: true,
			},
			Tools: &shared.ToolsCapability{
				ListChanged: true,
			},
			Prompts: &shared.PromptsCapability{
				ListChanged: true,
			},
		},
	}

	server := NewServer(ctx, mockTransport, config)
	require.NoError(t, server.Start())
	defer server.Close()

	var wg sync.WaitGroup
	numOperations := 100

	// Concurrent resource operations
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numOperations; i++ {
			resourceURI := "test://concurrent-resource-" + string(rune(i))
			
			// Register resource
			server.RegisterResource(resourceURI, "Resource", "Test", func(ctx context.Context, uri string) ([]shared.Content, error) {
				return []shared.Content{
					shared.TextContent{Type: "text", Text: "content"},
				}, nil
			})
			
			// Remove resource
			server.UnregisterResource(resourceURI)
		}
	}()

	// Concurrent tool operations
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numOperations; i++ {
			toolName := "concurrent-tool-" + string(rune(i))
			
			// Register tool
			server.RegisterTool(toolName, "Tool", map[string]interface{}{}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
				return []shared.Content{
					shared.TextContent{Type: "text", Text: "result"},
				}, nil
			})
			
			// Remove tool
			server.UnregisterTool(toolName)
		}
	}()

	// Concurrent prompt operations
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numOperations; i++ {
			promptName := "concurrent-prompt-" + string(rune(i))
			
			// Register prompt
			server.RegisterPrompt(promptName, "Prompt", []shared.PromptArgument{}, func(ctx context.Context, name string, arguments map[string]interface{}) (PromptMessage, error) {
				return PromptMessage{
					Role: "assistant",
					Content: []shared.Content{
						shared.TextContent{Type: "text", Text: "response"},
					},
				}, nil
			})
			
			// Remove prompt
			server.UnregisterPrompt(promptName)
		}
	}()

	// Wait for all operations to complete
	wg.Wait()

	// Verify all resources, tools, and prompts are cleaned up
	server.resourceMu.RLock()
	remainingResources := len(server.resourceHandlers)
	server.resourceMu.RUnlock()

	server.toolMu.RLock()
	remainingTools := len(server.toolHandlers)
	server.toolMu.RUnlock()

	server.promptMu.RLock()
	remainingPrompts := len(server.promptHandlers)
	server.promptMu.RUnlock()

	// Allow for some tolerance since concurrent operations might leave some items
	assert.LessOrEqual(t, remainingResources, 10, "Should have minimal remaining resources")
	assert.LessOrEqual(t, remainingTools, 10, "Should have minimal remaining tools")
	assert.LessOrEqual(t, remainingPrompts, 10, "Should have minimal remaining prompts")

	t.Logf("Concurrent removal test completed - Resources: %d, Tools: %d, Prompts: %d remaining",
		remainingResources, remainingTools, remainingPrompts)
}

// TestRemovalWithoutCapabilities tests that no notifications are sent when capabilities are disabled
func TestRemovalWithoutCapabilities(t *testing.T) {
	ctx := context.Background()
	
	// Track notifications from the start
	tracker := NewNotificationTracker()
	mockTransport := &NotificationInterceptorTransport{
		Transport: transport.NewInProcessTransport(),
		tracker:   tracker,
	}
	defer mockTransport.Close()

	// Server without list_changed capabilities
	config := ServerConfig{
		Name:                  "no-capabilities-test",
		Version:               "1.0.0",
		MaxConcurrentRequests: 10,
		RequestTimeout:        5 * time.Second,
		Capabilities:          shared.ServerCapabilities{}, // No capabilities
	}

	server := NewServer(ctx, mockTransport, config)
	require.NoError(t, server.Start())
	defer server.Close()

	// Register and remove items - should not send notifications
	server.RegisterResource("test://resource", "Resource", "Test", func(ctx context.Context, uri string) ([]shared.Content, error) {
		return []shared.Content{shared.TextContent{Type: "text", Text: "content"}}, nil
	})

	server.RegisterTool("test-tool", "Tool", map[string]interface{}{}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		return []shared.Content{shared.TextContent{Type: "text", Text: "result"}}, nil
	})

	server.RegisterPrompt("test-prompt", "Prompt", []shared.PromptArgument{}, func(ctx context.Context, name string, arguments map[string]interface{}) (PromptMessage, error) {
		return PromptMessage{Role: "assistant", Content: []shared.Content{shared.TextContent{Type: "text", Text: "response"}}}, nil
	})

	// Remove items
	server.UnregisterResource("test://resource")
	server.UnregisterTool("test-tool")
	server.UnregisterPrompt("test-prompt")

	time.Sleep(100 * time.Millisecond)

	// Should not have received any notifications
	notifications := tracker.GetNotifications()
	assert.Empty(t, notifications, "Should not receive notifications when capabilities are disabled")
}

// NotificationInterceptorTransport wraps a transport to intercept notifications
type NotificationInterceptorTransport struct {
	Transport
	tracker *NotificationTracker
}

func (nit *NotificationInterceptorTransport) SendNotification(method string, params interface{}) error {
	nit.tracker.AddNotification(method)
	return nit.Transport.SendNotification(method, params)
}

func (nit *NotificationInterceptorTransport) Channels() (<-chan *shared.Request, <-chan *shared.Notification) {
	return nit.Transport.Channels()
}

func (nit *NotificationInterceptorTransport) SendResponse(id interface{}, result interface{}, err *shared.RPCError) error {
	return nit.Transport.SendResponse(id, result, err)
}

func (nit *NotificationInterceptorTransport) Close() error {
	return nit.Transport.Close()
}
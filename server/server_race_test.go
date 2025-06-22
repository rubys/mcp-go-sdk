package server

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rubys/mcp-go-sdk/shared"
	"github.com/rubys/mcp-go-sdk/transport"
)

// TestRaceConditions performs extensive concurrent operations to detect race conditions
func TestRaceConditions(t *testing.T) {
	// Create a mock transport for testing
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	// Create server with mock transport
	ctx := context.Background()
	config := ServerConfig{
		Name:                  "race-test-server",
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
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()

	// Sync group to coordinate all operations
	var wg sync.WaitGroup
	
	// Test duration for concurrent operations
	testDuration := 500 * time.Millisecond

	// Counter for tracking operations (for potential future use)
	_ = int64(0) // operationsCount placeholder

	// 1. Concurrent tool registration and access
	runConcurrentOperation(&wg, testDuration, "add-tools", func() {
		name := fmt.Sprintf("tool-%d", time.Now().UnixNano())
		server.RegisterTool(name, "Test tool", map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"input": map[string]interface{}{"type": "string"},
			},
		}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
			return []shared.Content{
				shared.TextContent{Type: "text", Text: "tool result"},
			}, nil
		})
	})

	// 2. Concurrent resource registration and access  
	runConcurrentOperation(&wg, testDuration, "add-resources", func() {
		uri := fmt.Sprintf("test://resource-%d", time.Now().UnixNano())
		server.RegisterResource(uri, "Test Resource", "Test description", func(ctx context.Context, uri string) ([]shared.Content, error) {
			return []shared.Content{
				shared.TextContent{Type: "text", Text: "resource content"},
			}, nil
		})
	})

	// 3. Concurrent prompt registration and access
	runConcurrentOperation(&wg, testDuration, "add-prompts", func() {
		name := fmt.Sprintf("prompt-%d", time.Now().UnixNano())
		server.RegisterPrompt(name, "Test prompt", []shared.PromptArgument{
			{Name: "arg1", Description: "Test argument", Required: false},
		}, func(ctx context.Context, name string, arguments map[string]interface{}) (PromptMessage, error) {
			return PromptMessage{
				Role: "assistant",
				Content: []shared.Content{
					shared.TextContent{Type: "text", Text: "prompt response"},
				},
			}, nil
		})
	})

	// 4. Concurrent handler registration
	runConcurrentOperation(&wg, testDuration, "add-handlers", func() {
		method := fmt.Sprintf("test/method-%d", time.Now().UnixNano())
		server.RegisterHandler(method, func(ctx context.Context, params interface{}) (interface{}, error) {
			return map[string]string{"result": "success"}, nil
		})
	})

	// 5. Concurrent read access to tools
	server.RegisterTool("persistent-tool", "Persistent test tool", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"message": map[string]interface{}{"type": "string"},
		},
	}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		return []shared.Content{
			shared.TextContent{Type: "text", Text: "persistent tool result"},
		}, nil
	})

	runConcurrentOperation(&wg, testDuration, "read-tools", func() {
		// Simulate reading tool metadata
		server.toolMu.RLock()
		_ = len(server.toolMeta)
		server.toolMu.RUnlock()
	})

	// 6. Concurrent read access to resources
	server.RegisterResource("test://persistent-resource", "Persistent Resource", "Test", func(ctx context.Context, uri string) ([]shared.Content, error) {
		return []shared.Content{
			shared.TextContent{Type: "text", Text: "persistent resource content"},
		}, nil
	})

	runConcurrentOperation(&wg, testDuration, "read-resources", func() {
		// Simulate reading resource metadata
		server.resourceMu.RLock()
		_ = len(server.resourceMeta)
		server.resourceMu.RUnlock()
	})

	// 7. Concurrent read access to prompts
	server.RegisterPrompt("persistent-prompt", "Persistent prompt", []shared.PromptArgument{}, func(ctx context.Context, name string, arguments map[string]interface{}) (PromptMessage, error) {
		return PromptMessage{
			Role: "user",
			Content: []shared.Content{
				shared.TextContent{Type: "text", Text: "persistent prompt"},
			},
		}, nil
	})

	runConcurrentOperation(&wg, testDuration, "read-prompts", func() {
		// Simulate reading prompt metadata
		server.promptMu.RLock()
		_ = len(server.promptMeta)
		server.promptMu.RUnlock()
	})

	// 8. Concurrent access to request handlers
	runConcurrentOperation(&wg, testDuration, "read-handlers", func() {
		// Simulate reading handler metadata
		server.handlerMu.RLock()
		_ = len(server.requestHandlers)
		server.handlerMu.RUnlock()
	})

	// Wait for all operations to complete
	wg.Wait()
	
	t.Logf("Race condition test completed successfully")
}

// TestConcurrentResourceAccess tests concurrent resource registration and handler execution
func TestConcurrentResourceAccess(t *testing.T) {
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	ctx := context.Background()
	config := ServerConfig{
		Name:                  "resource-race-test",
		Version:               "1.0.0",
		MaxConcurrentRequests: 20,
		RequestTimeout:        2 * time.Second,
		Capabilities: shared.ServerCapabilities{
			Resources: &shared.ResourcesCapability{
				Subscribe:   true,
				ListChanged: true,
			},
		},
	}

	server := NewServer(ctx, mockTransport, config)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()

	var wg sync.WaitGroup
	testDuration := 300 * time.Millisecond

	// Concurrent resource addition
	runConcurrentOperation(&wg, testDuration, "add-resources", func() {
		uri := fmt.Sprintf("test://concurrent-%d", time.Now().UnixNano())
		server.RegisterResource(uri, "Concurrent Resource", "Test", func(ctx context.Context, uri string) ([]shared.Content, error) {
			// Simulate some work
			time.Sleep(1 * time.Millisecond)
			return []shared.Content{
				shared.TextContent{Type: "text", Text: fmt.Sprintf("Content for %s", uri)},
			}, nil
		})
	})

	// Concurrent resource reading
	runConcurrentOperation(&wg, testDuration, "read-resources", func() {
		server.resourceMu.RLock()
		for uri, handler := range server.resourceHandlers {
			server.resourceMu.RUnlock()
			
			// Execute handler (this should not cause deadlock)
			_, err := handler(ctx, uri)
			if err != nil {
				t.Errorf("Resource handler failed: %v", err)
			}
			
			server.resourceMu.RLock()
		}
		server.resourceMu.RUnlock()
	})

	wg.Wait()
	t.Log("Concurrent resource access test completed successfully")
}

// TestConcurrentToolExecution tests concurrent tool registration and execution
func TestConcurrentToolExecution(t *testing.T) {
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	ctx := context.Background()
	config := ServerConfig{
		Name:                  "tool-race-test",
		Version:               "1.0.0",
		MaxConcurrentRequests: 30,
		RequestTimeout:        2 * time.Second,
		Capabilities: shared.ServerCapabilities{
			Tools: &shared.ToolsCapability{
				ListChanged: true,
			},
		},
	}

	server := NewServer(ctx, mockTransport, config)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()

	var wg sync.WaitGroup
	testDuration := 300 * time.Millisecond

	// Add a persistent tool for execution testing
	server.RegisterTool("execution-tool", "Execution test tool", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"value": map[string]interface{}{"type": "number"},
		},
	}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		// Simulate work
		time.Sleep(1 * time.Millisecond)
		return []shared.Content{
			shared.TextContent{Type: "text", Text: "execution result"},
		}, nil
	})

	// Concurrent tool addition
	runConcurrentOperation(&wg, testDuration, "add-tools", func() {
		name := fmt.Sprintf("dynamic-tool-%d", time.Now().UnixNano())
		server.RegisterTool(name, "Dynamic tool", map[string]interface{}{
			"type": "object",
		}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
			return []shared.Content{
				shared.TextContent{Type: "text", Text: "dynamic result"},
			}, nil
		})
	})

	// Concurrent tool execution
	runConcurrentOperation(&wg, testDuration, "execute-tools", func() {
		server.toolMu.RLock()
		if handler, exists := server.toolHandlers["execution-tool"]; exists {
			server.toolMu.RUnlock()
			
			// Execute tool (should not cause deadlock)
			_, err := handler(ctx, "execution-tool", map[string]interface{}{"value": 42})
			if err != nil {
				t.Errorf("Tool execution failed: %v", err)
			}
		} else {
			server.toolMu.RUnlock()
		}
	})

	// Concurrent tool metadata reading
	runConcurrentOperation(&wg, testDuration, "read-tool-meta", func() {
		server.toolMu.RLock()
		_ = len(server.toolMeta)
		server.toolMu.RUnlock()
	})

	wg.Wait()
	t.Log("Concurrent tool execution test completed successfully")
}

// TestPromptHandlerDeadlock tests that prompt handlers adding more prompts don't cause deadlock
func TestPromptHandlerDeadlock(t *testing.T) {
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	ctx := context.Background()
	config := ServerConfig{
		Name:                  "prompt-deadlock-test",
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
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()

	// Register a prompt that tries to register another prompt in its handler
	server.RegisterPrompt("recursive-prompt", "Recursive prompt test", []shared.PromptArgument{}, func(ctx context.Context, name string, arguments map[string]interface{}) (PromptMessage, error) {
		// This would cause deadlock with improper mutex usage
		go func() {
			newName := fmt.Sprintf("generated-prompt-%d", time.Now().UnixNano())
			server.RegisterPrompt(newName, "Generated prompt", []shared.PromptArgument{}, func(ctx context.Context, name string, arguments map[string]interface{}) (PromptMessage, error) {
				return PromptMessage{
					Role: "assistant",
					Content: []shared.Content{
						shared.TextContent{Type: "text", Text: "generated response"},
					},
				}, nil
			})
		}()

		return PromptMessage{
			Role: "user",
			Content: []shared.Content{
				shared.TextContent{Type: "text", Text: "recursive response"},
			},
		}, nil
	})

	// Test that prompt execution doesn't deadlock
	done := make(chan struct{})
	go func() {
		defer close(done)
		
		server.promptMu.RLock()
		if handler, exists := server.promptHandlers["recursive-prompt"]; exists {
			server.promptMu.RUnlock()
			
			_, err := handler(ctx, "recursive-prompt", map[string]interface{}{})
			if err != nil {
				t.Errorf("Prompt handler failed: %v", err)
			}
		} else {
			server.promptMu.RUnlock()
		}
	}()

	// Ensure operation completes without deadlock
	select {
	case <-done:
		t.Log("Prompt handler deadlock test passed")
	case <-time.After(2 * time.Second):
		t.Fatal("Deadlock detected: prompt handler did not complete")
	}
}

// TestHighConcurrencyWorkerPool tests the server under high concurrency load
func TestHighConcurrencyWorkerPool(t *testing.T) {
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	ctx := context.Background()
	config := ServerConfig{
		Name:                  "high-concurrency-test",
		Version:               "1.0.0",
		MaxConcurrentRequests: 100, // High concurrency
		RequestTimeout:        1 * time.Second,
		Capabilities: shared.ServerCapabilities{
			Tools: &shared.ToolsCapability{
				ListChanged: true,
			},
		},
	}

	server := NewServer(ctx, mockTransport, config)
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()

	// Register a tool that simulates work
	server.RegisterTool("work-tool", "Work simulation tool", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"duration_ms": map[string]interface{}{"type": "number"},
		},
	}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		// Simulate variable work duration
		duration := 1 * time.Millisecond
		if durationArg, ok := arguments["duration_ms"].(float64); ok {
			duration = time.Duration(durationArg) * time.Millisecond
		}
		time.Sleep(duration)
		
		return []shared.Content{
			shared.TextContent{Type: "text", Text: "work completed"},
		}, nil
	})

	var wg sync.WaitGroup
	numWorkers := 200 // More workers than the server's concurrent limit

	// Launch workers that all try to execute tools simultaneously
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			server.toolMu.RLock()
			if handler, exists := server.toolHandlers["work-tool"]; exists {
				server.toolMu.RUnlock()
				
				_, err := handler(ctx, "work-tool", map[string]interface{}{
					"duration_ms": float64(workerID % 10), // Variable work duration
				})
				if err != nil {
					t.Errorf("Worker %d tool execution failed: %v", workerID, err)
				}
			} else {
				server.toolMu.RUnlock()
			}
		}(i)
	}

	// Wait for all workers to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("High concurrency test completed successfully")
	case <-time.After(10 * time.Second):
		t.Fatal("High concurrency test timed out")
	}
}

// Helper function to run concurrent operations for a specified duration
func runConcurrentOperation(wg *sync.WaitGroup, duration time.Duration, name string, operation func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		deadline := time.After(duration)
		for {
			select {
			case <-deadline:
				return
			default:
				operation()
				// Small sleep to prevent CPU spinning
				time.Sleep(time.Microsecond)
			}
		}
	}()
}
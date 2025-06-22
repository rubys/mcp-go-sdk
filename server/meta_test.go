package server

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rubys/mcp-go-sdk/shared"
	"github.com/rubys/mcp-go-sdk/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMetaFieldMarshalling tests that meta fields are properly marshalled and unmarshalled
func TestMetaFieldMarshalling(t *testing.T) {
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	config := ServerConfig{
		Name:                  "meta-test-server",
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

	// Track progress tokens received by the handler
	var receivedProgressTokens []interface{}
	var receivedContexts []context.Context

	// Register a tool that accesses progress tokens
	server.RegisterTool("progress-tool", "Tool that uses progress tokens", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"message": map[string]interface{}{
				"type":        "string",
				"description": "Message to process",
			},
		},
	}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		// Store the context to verify progress token was passed
		receivedContexts = append(receivedContexts, ctx)
		
		// Extract progress token from context
		if progressToken := ctx.Value("progressToken"); progressToken != nil {
			receivedProgressTokens = append(receivedProgressTokens, progressToken)
		}

		message := "default message"
		if msg, ok := arguments["message"].(string); ok {
			message = msg
		}

		return []shared.Content{
			shared.TextContent{Type: "text", Text: "Processed: " + message},
		}, nil
	})

	// Test cases for different meta field configurations
	testCases := []struct {
		name              string
		params            interface{}
		expectedToken     interface{}
		expectTokenInCtx  bool
	}{
		{
			name: "string progress token",
			params: map[string]interface{}{
				"name":      "progress-tool",
				"arguments": map[string]interface{}{"message": "test1"},
				"_meta": map[string]interface{}{
					"progressToken": "string-token-123",
				},
			},
			expectedToken:    "string-token-123",
			expectTokenInCtx: true,
		},
		{
			name: "numeric progress token",
			params: map[string]interface{}{
				"name":      "progress-tool",
				"arguments": map[string]interface{}{"message": "test2"},
				"_meta": map[string]interface{}{
					"progressToken": 42,
				},
			},
			expectedToken:    float64(42), // JSON unmarshals numbers as float64
			expectTokenInCtx: true,
		},
		{
			name: "object progress token",
			params: map[string]interface{}{
				"name":      "progress-tool",
				"arguments": map[string]interface{}{"message": "test3"},
				"_meta": map[string]interface{}{
					"progressToken": map[string]interface{}{
						"id":        "task-456",
						"sessionId": "session-789",
					},
				},
			},
			expectedToken: map[string]interface{}{
				"id":        "task-456",
				"sessionId": "session-789",
			},
			expectTokenInCtx: true,
		},
		{
			name: "meta without progress token",
			params: map[string]interface{}{
				"name":      "progress-tool",
				"arguments": map[string]interface{}{"message": "test4"},
				"_meta": map[string]interface{}{
					"customField": "custom-value",
				},
			},
			expectedToken:    nil,
			expectTokenInCtx: false,
		},
		{
			name: "no meta field",
			params: map[string]interface{}{
				"name":      "progress-tool",
				"arguments": map[string]interface{}{"message": "test5"},
			},
			expectedToken:    nil,
			expectTokenInCtx: false,
		},
		{
			name: "empty meta field",
			params: map[string]interface{}{
				"name":      "progress-tool",
				"arguments": map[string]interface{}{"message": "test6"},
				"_meta":     map[string]interface{}{},
			},
			expectedToken:    nil,
			expectTokenInCtx: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clear previous results
			receivedProgressTokens = nil
			receivedContexts = nil

			// Call the tool handler directly through the server's handleToolsCall
			result, err := server.handleToolsCall(ctx, tc.params)
			require.Nil(t, err, "Tool call should succeed")
			require.NotNil(t, result, "Result should not be nil")

			// Verify handler was called
			require.Len(t, receivedContexts, 1, "Handler should have been called once")

			if tc.expectTokenInCtx {
				// Verify progress token was passed to context
				require.Len(t, receivedProgressTokens, 1, "Should have received progress token")
				assert.Equal(t, tc.expectedToken, receivedProgressTokens[0], "Progress token should match expected value")

				// Verify token is in context
				actualToken := receivedContexts[0].Value("progressToken")
				assert.Equal(t, tc.expectedToken, actualToken, "Context should contain the progress token")
			} else {
				// Verify no progress token was passed
				assert.Len(t, receivedProgressTokens, 0, "Should not have received progress token")

				// Verify no token in context
				actualToken := receivedContexts[0].Value("progressToken")
				assert.Nil(t, actualToken, "Context should not contain progress token")
			}

			// Verify the tool response structure
			resultMap, ok := result.(map[string]interface{})
			require.True(t, ok, "Result should be a map")
			
			content, ok := resultMap["content"]
			require.True(t, ok, "Result should have content field")
			
			contentSlice, ok := content.([]shared.Content)
			require.True(t, ok, "Content should be a slice")
			require.Len(t, contentSlice, 1, "Should have one content item")
			
			textContent, ok := contentSlice[0].(shared.TextContent)
			require.True(t, ok, "Content should be text content")
			assert.Contains(t, textContent.Text, "Processed:", "Content should contain processed message")
		})
	}
}

// TestProgressTokenTypes tests various progress token data types
func TestProgressTokenTypes(t *testing.T) {
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	config := ServerConfig{
		Name:                  "token-types-test",
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

	var lastProgressToken interface{}

	// Register a tool that captures progress tokens
	server.RegisterTool("token-capture", "Captures progress tokens", map[string]interface{}{
		"type": "object",
	}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		lastProgressToken = ctx.Value("progressToken")
		return []shared.Content{
			shared.TextContent{Type: "text", Text: "Token captured"},
		}, nil
	})

	// Test different progress token types
	tokenTests := []struct {
		name          string
		token         interface{}
		expectedType  string
		expectedValue interface{}
	}{
		{
			name:          "string token",
			token:         "progress-abc-123",
			expectedType:  "string",
			expectedValue: "progress-abc-123",
		},
		{
			name:          "integer token",
			token:         123,
			expectedType:  "float64", // JSON unmarshals integers as float64
			expectedValue: float64(123),
		},
		{
			name:          "float token",
			token:         123.456,
			expectedType:  "float64",
			expectedValue: 123.456,
		},
		{
			name:          "boolean token",
			token:         true,
			expectedType:  "bool",
			expectedValue: true,
		},
		{
			name:  "object token",
			token: map[string]interface{}{"id": "task-1", "step": 5},
			expectedType: "map[string]interface {}",
			expectedValue: map[string]interface{}{"id": "task-1", "step": float64(5)}, // JSON number conversion
		},
		{
			name:          "array token",
			token:         []interface{}{"step1", "step2", "step3"},
			expectedType:  "[]interface {}",
			expectedValue: []interface{}{"step1", "step2", "step3"},
		},
		{
			name:          "null token",
			token:         nil,
			expectedType:  "<nil>",
			expectedValue: nil,
		},
	}

	for _, tt := range tokenTests {
		t.Run(tt.name, func(t *testing.T) {
			lastProgressToken = "UNSET" // Reset to detect if no token was set

			params := map[string]interface{}{
				"name":      "token-capture",
				"arguments": map[string]interface{}{},
			}

			if tt.token != nil || tt.name == "null token" {
				params["_meta"] = map[string]interface{}{
					"progressToken": tt.token,
				}
			}

			result, err := server.handleToolsCall(ctx, params)
			require.Nil(t, err)
			require.NotNil(t, result)

			if tt.token != nil {
				// Token should be captured
				assert.Equal(t, tt.expectedValue, lastProgressToken, "Progress token value should match")
				assert.Equal(t, tt.expectedType, fmt.Sprintf("%T", lastProgressToken), "Progress token type should match")
			} else if tt.name == "null token" {
				// Null token should still be set in context
				assert.Nil(t, lastProgressToken, "Null progress token should be nil")
			} else {
				// No token should be captured
				assert.Equal(t, "UNSET", lastProgressToken, "No progress token should be set")
			}
		})
	}
}

// TestMetaFieldEdgeCases tests edge cases and error scenarios
func TestMetaFieldEdgeCases(t *testing.T) {
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	config := ServerConfig{
		Name:                  "edge-cases-test",
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

	server.RegisterTool("edge-case-tool", "Tool for edge case testing", map[string]interface{}{
		"type": "object",
	}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		// Just return OK for edge case testing
		return []shared.Content{
			shared.TextContent{Type: "text", Text: "OK"},
		}, nil
	})

	// Test edge cases
	edgeCases := []struct {
		name         string
		params       interface{}
		expectError  bool
		expectedCode int
	}{
		{
			name: "invalid meta type",
			params: map[string]interface{}{
				"name":      "edge-case-tool",
				"arguments": map[string]interface{}{},
				"_meta":     "invalid-meta-not-object", // Should be object
			},
			expectError:  true, // Server correctly rejects invalid meta type
			expectedCode: -32602,
		},
		{
			name: "meta with complex nested objects",
			params: map[string]interface{}{
				"name":      "edge-case-tool",
				"arguments": map[string]interface{}{},
				"_meta": map[string]interface{}{
					"progressToken": map[string]interface{}{
						"complex": map[string]interface{}{
							"nested": map[string]interface{}{
								"deeply": map[string]interface{}{
									"data": []interface{}{1, 2, 3, "nested-array"},
								},
							},
						},
					},
				},
			},
			expectError:  false,
			expectedCode: 0,
		},
		{
			name: "meta with special characters",
			params: map[string]interface{}{
				"name":      "edge-case-tool",
				"arguments": map[string]interface{}{},
				"_meta": map[string]interface{}{
					"progressToken": "special-chars-!@#$%^&*()_+-=[]{}|;:,.<>?",
				},
			},
			expectError:  false,
			expectedCode: 0,
		},
		{
			name: "meta with unicode",
			params: map[string]interface{}{
				"name":      "edge-case-tool",
				"arguments": map[string]interface{}{},
				"_meta": map[string]interface{}{
					"progressToken": "unicode-😀🎉🌟测试🔥",
				},
			},
			expectError:  false,
			expectedCode: 0,
		},
	}

	for _, tc := range edgeCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := server.handleToolsCall(ctx, tc.params)

			if tc.expectError {
				assert.NotNil(t, err, "Should return an error")
				if tc.expectedCode != 0 {
					assert.Equal(t, tc.expectedCode, err.Code, "Error code should match")
				}
			} else {
				assert.Nil(t, err, "Should not return an error")
				assert.NotNil(t, result, "Should return a result")
			}
		})
	}
}

// TestConcurrentMetaFieldAccess tests meta field handling under concurrent load
func TestConcurrentMetaFieldAccess(t *testing.T) {
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	config := ServerConfig{
		Name:                  "concurrent-meta-test",
		Version:               "1.0.0",
		MaxConcurrentRequests: 50,
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

	// Track all progress tokens received
	var receivedTokens []interface{}
	var tokenMutex sync.Mutex

	server.RegisterTool("concurrent-meta-tool", "Tool for concurrent meta testing", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{"type": "number"},
		},
	}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		// Simulate some work
		time.Sleep(1 * time.Millisecond)

		token := ctx.Value("progressToken")
		tokenMutex.Lock()
		receivedTokens = append(receivedTokens, token)
		tokenMutex.Unlock()

		id := 0
		if idVal, ok := arguments["id"].(float64); ok {
			id = int(idVal)
		}

		return []shared.Content{
			shared.TextContent{Type: "text", Text: fmt.Sprintf("Processed request %d", id)},
		}, nil
	})

	// Launch concurrent requests with different progress tokens
	numRequests := 20
	var wg sync.WaitGroup

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			params := map[string]interface{}{
				"name":      "concurrent-meta-tool",
				"arguments": map[string]interface{}{"id": i},
				"_meta": map[string]interface{}{
					"progressToken": fmt.Sprintf("token-%d", i),
				},
			}

			result, err := server.handleToolsCall(ctx, params)
			assert.Nil(t, err, "Request %d should succeed", i)
			assert.NotNil(t, result, "Request %d should return result", i)
		}(i)
	}

	wg.Wait()

	// Verify all tokens were received
	tokenMutex.Lock()
	assert.Len(t, receivedTokens, numRequests, "Should have received all progress tokens")

	// Verify each unique token was received
	tokenSet := make(map[string]bool)
	for _, token := range receivedTokens {
		if tokenStr, ok := token.(string); ok {
			tokenSet[tokenStr] = true
		}
	}

	assert.Len(t, tokenSet, numRequests, "Should have received all unique progress tokens")

	// Verify all expected tokens are present
	for i := 0; i < numRequests; i++ {
		expectedToken := fmt.Sprintf("token-%d", i)
		assert.True(t, tokenSet[expectedToken], "Should have received token: %s", expectedToken)
	}
	tokenMutex.Unlock()
}
package client

import (
	"context"
	"testing"

	"github.com/rubys/mcp-go-sdk/shared"
)

func TestInProcessClientSimple(t *testing.T) {
	// Create in-process client
	client, transport := NewInProcessClient()
	defer client.Close()

	// Add test tool
	transport.AddTool(shared.Tool{
		Name:        "echo",
		Description: "Echo tool",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"message": map[string]interface{}{
					"type":        "string",
					"description": "Message to echo",
				},
			},
			"required": []string{"message"},
		},
	}, func(ctx context.Context, args map[string]interface{}) ([]shared.Content, error) {
		message := args["message"].(string)
		return []shared.Content{
			shared.TextContent{Type: "text", Text: "Echo: " + message},
		}, nil
	})

	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Failed to start client: %v", err)
	}

	// Test list tools
	result, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	if len(result.Tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(result.Tools))
	}
	if result.Tools[0].Name != "echo" {
		t.Errorf("Expected tool name 'echo', got '%s'", result.Tools[0].Name)
	}

	// Test call tool
	callResult, err := client.CallTool(ctx, shared.CallToolRequest{
		Name: "echo",
		Arguments: map[string]interface{}{
			"message": "Hello, World!",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if len(callResult.Content) != 1 {
		t.Errorf("Expected 1 content item, got %d", len(callResult.Content))
	}
	textContent, ok := callResult.Content[0].(shared.TextContent)
	if !ok {
		t.Errorf("Expected TextContent, got %T", callResult.Content[0])
	}
	if textContent.Text != "Echo: Hello, World!" {
		t.Errorf("Unexpected response: %s", textContent.Text)
	}
}

// Helper functions
func strPtr(s string) *string {
	return &s
}

var (
	trueVal  = true
	falseVal = false
)
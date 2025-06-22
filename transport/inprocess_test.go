package transport

import (
	"context"
	"testing"
	"time"

	"github.com/rubys/mcp-go-sdk/shared"
)

func TestInProcessTransport(t *testing.T) {
	// Create in-process transport
	transport := NewInProcessTransport()
	defer transport.Close()

	// Add a test tool
	transport.AddTool(shared.Tool{
		Name:        "test-tool",
		Description: "A test tool",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"message": map[string]interface{}{
					"type":        "string",
					"description": "Test message",
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

	// Add a test resource
	transport.AddResource(shared.Resource{
		URI:         "test://resource",
		Name:        "Test Resource",
		Description: "A test resource",
	}, func(ctx context.Context, uri string) ([]shared.ResourceContent, error) {
		return []shared.ResourceContent{
			{URI: uri, MimeType: "text/plain", Text: "Resource content for: " + uri},
		}, nil
	})

	// Add a test prompt
	transport.AddPrompt(shared.Prompt{
		Name:        "test-prompt",
		Description: "A test prompt",
		Arguments: []shared.PromptArgument{
			{
				Name:        "name",
				Description: "Your name",
				Required:    true,
			},
		},
	}, func(ctx context.Context, args map[string]string) (*shared.GetPromptResult, error) {
		name := args["name"]
		return &shared.GetPromptResult{
			Messages: []shared.PromptMessage{
				{
					Role:    "assistant",
					Content: shared.TextContent{Type: "text", Text: "Hello, " + name + "!"},
				},
			},
		}, nil
	})

	t.Run("BasicCommunication", func(t *testing.T) {

		// Send initialize request
		respChan, err := transport.SendRequest("initialize", map[string]interface{}{
			"protocolVersion": shared.ProtocolVersion,
			"capabilities":    shared.ClientCapabilities{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		})
		if err != nil {
			t.Fatalf("Failed to send initialize request: %v", err)
		}

		// Wait for response
		select {
		case resp := <-respChan:
			if resp.Error != nil {
				t.Fatalf("Initialize failed: %v", resp.Error)
			}
			if resp.Result == nil {
				t.Fatal("Initialize response has no result")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for initialize response")
		}
	})

	t.Run("ListTools", func(t *testing.T) {

		// Initialize first
		initResp, err := transport.SendRequest("initialize", map[string]interface{}{
			"protocolVersion": shared.ProtocolVersion,
			"capabilities":    shared.ClientCapabilities{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		})
		if err != nil {
			t.Fatalf("Failed to send initialize request: %v", err)
		}
		<-initResp

		// List tools
		respChan, err := transport.SendRequest("tools/list", map[string]interface{}{})
		if err != nil {
			t.Fatalf("Failed to send list tools request: %v", err)
		}

		select {
		case resp := <-respChan:
			if resp.Error != nil {
				t.Fatalf("List tools failed: %v", resp.Error)
			}
			if resp.Result == nil {
				t.Fatal("List tools response has no result")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for list tools response")
		}
	})

	t.Run("CallTool", func(t *testing.T) {

		// Initialize first
		initResp, err := transport.SendRequest("initialize", map[string]interface{}{
			"protocolVersion": shared.ProtocolVersion,
			"capabilities":    shared.ClientCapabilities{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		})
		if err != nil {
			t.Fatalf("Failed to send initialize request: %v", err)
		}
		<-initResp

		// Call tool
		respChan, err := transport.SendRequest("tools/call", map[string]interface{}{
			"name": "test-tool",
			"arguments": map[string]interface{}{
				"message": "Hello, World!",
			},
		})
		if err != nil {
			t.Fatalf("Failed to send call tool request: %v", err)
		}

		select {
		case resp := <-respChan:
			if resp.Error != nil {
				t.Fatalf("Call tool failed: %v", resp.Error)
			}
			if resp.Result == nil {
				t.Fatal("Call tool response has no result")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for call tool response")
		}
	})

	t.Run("SendNotification", func(t *testing.T) {

		// Send a notification (should not get a response)
		err := transport.SendNotification("notifications/cancelled", map[string]interface{}{
			"requestId": "test-request",
			"reason":    "test cancellation",
		})
		if err != nil {
			t.Fatalf("Failed to send notification: %v", err)
		}
	})

	t.Run("ConcurrentRequests", func(t *testing.T) {

		// Initialize first
		initResp, err := transport.SendRequest("initialize", map[string]interface{}{
			"protocolVersion": shared.ProtocolVersion,
			"capabilities":    shared.ClientCapabilities{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		})
		if err != nil {
			t.Fatalf("Failed to send initialize request: %v", err)
		}
		<-initResp

		// Send multiple concurrent requests
		numRequests := 10
		responses := make([]<-chan *shared.Response, numRequests)

		for i := 0; i < numRequests; i++ {
			respChan, err := transport.SendRequest("tools/call", map[string]interface{}{
				"name": "test-tool",
				"arguments": map[string]interface{}{
					"message": "Concurrent request",
				},
			})
			if err != nil {
				t.Fatalf("Failed to send request %d: %v", i, err)
			}
			responses[i] = respChan
		}

		// Wait for all responses
		for i, respChan := range responses {
			select {
			case resp := <-respChan:
				if resp.Error != nil {
					t.Errorf("Request %d failed: %v", i, resp.Error)
				}
			case <-time.After(2 * time.Second):
				t.Errorf("Timeout waiting for response %d", i)
			}
		}
	})
}

// Helper functions
func strPtr(s string) *string {
	return &s
}

var (
	trueVal  = true
	falseVal = false
)
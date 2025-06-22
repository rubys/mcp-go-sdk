package benchmarks

import (
	"context"
	"testing"
	"time"

	"github.com/rubys/mcp-go-sdk/client"
	"github.com/rubys/mcp-go-sdk/shared"
	"github.com/rubys/mcp-go-sdk/transport"
)

func TestSimpleBenchmark(t *testing.T) {
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	// Register tools directly on the transport instead of the server
	mockTransport.AddTool(shared.Tool{
		Name:        "test_tool",
		Description: "Simple test tool",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"input": map[string]interface{}{"type": "string"},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) ([]shared.Content, error) {
		return []shared.Content{
			shared.TextContent{Type: "text", Text: "Test response from transport"},
		}, nil
	})

	srv := setupBenchmarkServer(ctx, mockTransport)
	
	err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	client, err := client.NewClient(ctx, mockTransport, client.ClientConfig{
		ClientInfo: shared.ClientInfo{
			Name:    "test-client",
			Version: "1.0.0",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Give client time to initialize
	time.Sleep(100 * time.Millisecond)

	// Test listing tools
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Found %d tools", len(tools.Tools))
	for _, tool := range tools.Tools {
		t.Logf("Tool: %s", tool.Name)
	}

	// Test calling a tool
	if len(tools.Tools) > 0 {
		_, err := client.CallTool(ctx, shared.CallToolRequest{
			Name: tools.Tools[0].Name,
			Arguments: map[string]interface{}{
				"input": "test",
				"size":  100,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Log("Tool call successful")
	}
}
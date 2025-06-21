package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	mcp "github.com/rubys/mcp-go-sdk/compat"
)

func main() {
	fmt.Println("MCP Go SDK - SSE Transport Compatibility Example")

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down server...")
		cancel()
	}()

	// Create MCP server with SSE transport
	mcpServer := mcp.NewMCPServer(
		"sse-compat-server",
		"1.0.0",
		mcp.WithResourceCapabilities(true, true),
		mcp.WithToolCapabilities(true),
		mcp.WithConcurrency(100, 30), // High concurrency for SSE
	)

	// Initialize with SSE transport (dual endpoints)
	err := mcpServer.CreateWithSSE(ctx, 
		"http://localhost:8080/events",  // SSE endpoint for receiving
		"http://localhost:8080/send")    // HTTP POST endpoint for sending
	if err != nil {
		log.Fatalf("Failed to create SSE transport: %v", err)
	}

	// Register resources (identical API to mark3labs)
	mcpServer.AddResource(
		mcp.NewResource(
			"http://api.example.com/data",
			"External API Data",
			mcp.WithResourceDescription("Data from external API"),
			mcp.WithMIMEType("application/json"),
		),
		handleAPIResource,
	)

	// Register tools with HTTP-specific functionality
	mcpServer.AddTool(
		mcp.NewTool("webhook",
			mcp.WithDescription("Send webhook notification"),
			mcp.WithString("url",
				mcp.Required(),
				mcp.Description("Webhook URL"),
			),
			mcp.WithString("payload",
				mcp.Required(),
				mcp.Description("JSON payload to send"),
			),
		),
		handleWebhookTool,
	)

	// Start the server
	if err := mcpServer.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	log.SetOutput(os.Stderr)
	log.Println("MCP SSE Server started")
	log.Println("- SSE endpoint: :8080/events (receive)")
	log.Println("- HTTP endpoint: :8080/send (send)")
	log.Println("API: mark3labs/mcp-go compatible")
	log.Println("Transport: High-performance SSE with concurrency")
	log.Println("Capabilities:")
	log.Println("- Resources: http://api.example.com/data")
	log.Println("- Tools: webhook")

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	log.Println("Server shutting down...")
	if err := mcpServer.Close(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}
}

func handleAPIResource(ctx context.Context, req mcp.ResourceRequest) (mcp.ResourceResponse, error) {
	log.Printf("Fetching resource: %s", req.URI)
	
	// Simulate API call
	return mcp.ResourceResponse{
		Contents: []mcp.Content{
			mcp.TextContent{
				Type: mcp.ContentTypeText,
				Text: `{"status": "success", "data": {"users": 1250, "active": 890}, "transport": "HTTP"}`,
			},
		},
	}, nil
}

func handleWebhookTool(ctx context.Context, req mcp.ToolRequest) (mcp.ToolResponse, error) {
	url := req.Arguments["url"].(string)
	payload := req.Arguments["payload"].(string)
	
	log.Printf("Sending webhook to %s with payload: %s", url, payload)
	
	// Simulate webhook call (in real implementation, make HTTP request)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		return mcp.ToolResponse{}, fmt.Errorf("webhook failed: %w", err)
	}
	defer resp.Body.Close()
	
	return mcp.ToolResponse{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("Webhook sent successfully to %s (status: %s)", url, resp.Status),
			},
		},
	}, nil
}
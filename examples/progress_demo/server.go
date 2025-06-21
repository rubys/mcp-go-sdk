package main

import (
	"context"
	"fmt"
	"log"
	"time"

	mcp "github.com/rubys/mcp-go-sdk/compat"
)

func main() {
	ctx := context.Background()

	// Create server with mark3labs/mcp-go compatible API
	server := mcp.NewMCPServer(
		"progress-demo-server",
		"1.0.0",
		mcp.WithToolCapabilities(true),
		mcp.WithLogging(),
	)

	// Initialize with stdio transport
	err := server.CreateWithStdio(ctx)
	if err != nil {
		log.Fatal("Failed to create stdio transport:", err)
	}

	// Tool 1: Get current time
	server.AddTool(
		mcp.NewTool("get_time",
			mcp.WithDescription("Get the current time"),
		),
		func(ctx context.Context, request mcp.ToolRequest) (mcp.ToolResponse, error) {
			currentTime := time.Now().Format("2006-01-02 15:04:05 MST")
			return mcp.ToolResponse{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: mcp.ContentTypeText,
						Text: fmt.Sprintf("Current time: %s", currentTime),
					},
				},
			}, nil
		},
	)

	// Tool 2: Progress tool with cancellation support
	server.AddTool(
		mcp.NewTool("progress_tool",
			mcp.WithDescription("A tool that sends progress notifications for 10 seconds"),
			mcp.WithString("progress_token"), // Optional by default
		),
		func(ctx context.Context, request mcp.ToolRequest) (mcp.ToolResponse, error) {
			// Extract progress token from request metadata following MCP pattern
			var progressToken interface{}
			if request.Params.Meta != nil && request.Params.Meta.ProgressToken != nil {
				progressToken = request.Params.Meta.ProgressToken
			}
			if progressToken == nil {
				progressToken = "default-progress-token"
			}
			

			// Send progress notifications every second for 10 seconds
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()

			for i := 1; i <= 10; i++ {
				select {
				case <-ctx.Done():
					// Tool was cancelled
					return mcp.ToolResponse{
						Content: []mcp.Content{
							mcp.TextContent{
								Type: mcp.ContentTypeText,
								Text: "Tool was cancelled",
							},
						},
					}, nil
				case <-ticker.C:
					// Send progress notification
					if progressToken != nil {
						total := 10
						progress := mcp.ProgressNotification{
							Method: "notifications/progress",
							Params: mcp.ProgressParams{
								ProgressToken: progressToken,
								Progress:      i,
								Total:         &total,
								Message:       fmt.Sprintf("Processing step %d of %d", i, total),
							},
						}
						
						// Send progress notification via server
						err := server.SendProgressNotification(progress)
						if err != nil {
							log.Printf("Error sending progress notification: %v", err)
						}
					}
				}
			}

			return mcp.ToolResponse{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: mcp.ContentTypeText,
						Text: "Done",
					},
				},
			}, nil
		},
	)

	// Start the server
	log.Println("Starting MCP server with stdio transport...")
	server.Start()

	// Keep the server running
	select {}
}
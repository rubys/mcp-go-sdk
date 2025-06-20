package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rubys/mcp-go-sdk/server"
	"github.com/rubys/mcp-go-sdk/shared"
	"github.com/rubys/mcp-go-sdk/transport"
)

func main() {
	// Create context that can be cancelled
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

	// Create stdio transport with concurrent I/O
	baseTransport, err := transport.NewStdioTransport(ctx, transport.StdioConfig{
		RequestTimeout: 30 * time.Second,
		MessageBuffer:  100,
	})
	if err != nil {
		log.Fatalf("Failed to create transport: %v", err)
	}
	defer baseTransport.Close()

	// Create server with concurrency configuration
	serverConfig := server.ServerConfig{
		Name:                  "Example Go MCP Server",
		Version:               "1.0.0",
		MaxConcurrentRequests: 50,
		RequestTimeout:        30 * time.Second,
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

	mcpServer := server.NewServer(ctx, baseTransport, serverConfig)

	// Register example resource handler
	mcpServer.RegisterResource(
		"file://example.txt",
		"Example Text File",
		"An example text resource",
		func(ctx context.Context, uri string) ([]shared.Content, error) {
			// Simulate some processing time
			select {
			case <-time.After(100 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			return []shared.Content{
				shared.TextContent{
					Type: shared.ContentTypeText,
					Text: "This is example content from a concurrent Go MCP server!",
					URI:  uri,
				},
			}, nil
		},
	)

	// Register example tool handler
	mcpServer.RegisterTool(
		"echo",
		"Echo the input text",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"text": map[string]interface{}{
					"type":        "string",
					"description": "Text to echo back",
				},
			},
			"required": []string{"text"},
		},
		func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
			text, ok := arguments["text"].(string)
			if !ok {
				return nil, fmt.Errorf("text argument must be a string")
			}

			// Simulate some processing time
			select {
			case <-time.After(50 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			return []shared.Content{
				shared.TextContent{
					Type: shared.ContentTypeText,
					Text: fmt.Sprintf("Echo: %s", text),
				},
			}, nil
		},
	)

	// Register example prompt handler
	mcpServer.RegisterPrompt(
		"greeting",
		"Generate a personalized greeting",
		[]shared.PromptArgument{
			{
				Name:        "name",
				Description: "The name to greet",
				Required:    true,
			},
		},
		func(ctx context.Context, name string, arguments map[string]interface{}) (server.PromptMessage, error) {
			nameArg, ok := arguments["name"].(string)
			if !ok {
				return server.PromptMessage{}, fmt.Errorf("name argument must be a string")
			}

			return server.PromptMessage{
				Role: "user",
				Content: []shared.Content{
					shared.TextContent{
						Type: shared.ContentTypeText,
						Text: fmt.Sprintf("Hello, %s! Welcome to the concurrent Go MCP server.", nameArg),
					},
				},
			}, nil
		},
	)

	// Start the server
	if err := mcpServer.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	log.SetOutput(os.Stderr) // Log to stderr to not interfere with protocol
	log.Println("MCP Server started with concurrent stdio transport")
	log.Println("Capabilities:")
	log.Println("- Resources: file://example.txt")
	log.Println("- Tools: echo")
	log.Println("- Prompts: greeting")

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	log.Println("Server shutting down...")
	if err := mcpServer.Close(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}
}

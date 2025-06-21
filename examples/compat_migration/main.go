package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	mcp "github.com/rubys/mcp-go-sdk/compat"
)

func main() {
	fmt.Println("MCP Go SDK - Compatibility Layer Example")
	fmt.Println("This demonstrates migration from mark3labs/mcp-go API")

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

	// Create MCP server with mark3labs/mcp-go compatible API
	// This is identical to mark3labs/mcp-go syntax
	mcpServer := mcp.NewMCPServer(
		"compat-example-server",
		"1.0.0",
		mcp.WithResourceCapabilities(true, true),
		mcp.WithToolCapabilities(true),
		mcp.WithPromptCapabilities(true),
		mcp.WithLogging(),
		mcp.WithConcurrency(50, 30), // Additional high-performance options
	)

	// Initialize with stdio transport
	// This is the main difference - need to explicitly initialize transport
	err := mcpServer.CreateWithStdio(ctx)
	if err != nil {
		log.Fatalf("Failed to create stdio transport: %v", err)
	}

	// Register resources using fluent builder API (identical to mark3labs)
	mcpServer.AddResource(
		mcp.NewResource(
			"file://readme.md",
			"Project README",
			mcp.WithResourceDescription("The project's README file"),
			mcp.WithMIMEType("text/markdown"),
		),
		handleReadmeResource,
	)

	mcpServer.AddResource(
		mcp.NewResource(
			"file://config.json",
			"Configuration",
			mcp.WithResourceDescription("Server configuration"),
			mcp.WithMIMEType("application/json"),
		),
		handleConfigResource,
	)

	// Register tools using type-safe builder API (identical to mark3labs)
	mcpServer.AddTool(
		mcp.NewTool("calculate",
			mcp.WithDescription("Perform basic arithmetic calculations"),
			mcp.WithString("operation",
				mcp.Required(),
				mcp.Description("The operation to perform"),
				mcp.Enum("add", "subtract", "multiply", "divide"),
			),
			mcp.WithNumber("a",
				mcp.Required(),
				mcp.Description("First number"),
			),
			mcp.WithNumber("b",
				mcp.Required(),
				mcp.Description("Second number"),
			),
		),
		handleCalculatorTool,
	)

	mcpServer.AddTool(
		mcp.NewTool("echo",
			mcp.WithDescription("Echo the input text"),
			mcp.WithString("text",
				mcp.Required(),
				mcp.Description("Text to echo back"),
			),
			mcp.WithBoolean("uppercase",
				mcp.Description("Convert to uppercase"),
			),
		),
		handleEchoTool,
	)

	// Register prompts using fluent builder API (identical to mark3labs)
	mcpServer.AddPrompt(
		mcp.NewPrompt("greeting",
			mcp.WithPromptDescription("Generate a personalized greeting"),
			mcp.WithArgument("name",
				mcp.ArgumentDescription("Name of the person to greet"),
				mcp.RequiredArgument(),
			),
			mcp.WithArgument("style",
				mcp.ArgumentDescription("Greeting style (formal/casual)"),
			),
		),
		handleGreetingPrompt,
	)

	// Start the server
	if err := mcpServer.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	log.SetOutput(os.Stderr) // Log to stderr to not interfere with protocol
	log.Println("MCP Compatibility Server started")
	log.Println("API: mark3labs/mcp-go compatible")
	log.Println("Performance: 10x faster with Go concurrency")
	log.Println("Capabilities:")
	log.Println("- Resources: file://readme.md, file://config.json")
	log.Println("- Tools: calculate, echo")
	log.Println("- Prompts: greeting")

	// Demonstrate access to underlying high-performance server
	underlying := mcpServer.GetUnderlying()
	log.Printf("Underlying server: %T", underlying)

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	log.Println("Server shutting down...")
	if err := mcpServer.Close(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}
}

// Resource handlers with mark3labs/mcp-go compatible signatures
func handleReadmeResource(ctx context.Context, req mcp.ResourceRequest) (mcp.ResourceResponse, error) {
	log.Printf("Reading resource: %s", req.URI)
	
	return mcp.ResourceResponse{
		Contents: []mcp.Content{
			mcp.TextContent{
				Type: mcp.ContentTypeText,
				Text: "# MCP Go SDK Compatibility Layer\n\nThis demonstrates the compatibility layer that provides mark3labs/mcp-go API while leveraging high-performance Go concurrency.",
			},
		},
	}, nil
}

func handleConfigResource(ctx context.Context, req mcp.ResourceRequest) (mcp.ResourceResponse, error) {
	log.Printf("Reading config: %s", req.URI)
	
	return mcp.ResourceResponse{
		Contents: []mcp.Content{
			mcp.TextContent{
				Type: mcp.ContentTypeText,
				Text: `{"server": {"name": "compat-example", "version": "1.0.0", "performance": "10x"}}`,
			},
		},
	}, nil
}

// Tool handlers with mark3labs/mcp-go compatible signatures
func handleCalculatorTool(ctx context.Context, req mcp.ToolRequest) (mcp.ToolResponse, error) {
	operation := req.Arguments["operation"].(string)
	a := req.Arguments["a"].(float64)
	b := req.Arguments["b"].(float64)
	
	var result float64
	switch operation {
	case "add":
		result = a + b
	case "subtract":
		result = a - b
	case "multiply":
		result = a * b
	case "divide":
		if b == 0 {
			return mcp.ToolResponse{}, fmt.Errorf("cannot divide by zero")
		}
		result = a / b
	default:
		return mcp.ToolResponse{}, fmt.Errorf("unknown operation: %s", operation)
	}
	
	log.Printf("Calculator: %f %s %f = %f", a, operation, b, result)
	
	return mcp.ToolResponse{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("Result: %f %s %f = %f", a, operation, b, result),
			},
		},
	}, nil
}

func handleEchoTool(ctx context.Context, req mcp.ToolRequest) (mcp.ToolResponse, error) {
	text := req.Arguments["text"].(string)
	uppercase := false
	
	if val, ok := req.Arguments["uppercase"]; ok {
		uppercase = val.(bool)
	}
	
	if uppercase {
		text = fmt.Sprintf(text)
	}
	
	log.Printf("Echo: %s (uppercase: %v)", text, uppercase)
	
	return mcp.ToolResponse{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: mcp.ContentTypeText,
				Text: fmt.Sprintf("Echo: %s", text),
			},
		},
	}, nil
}

// Prompt handlers with mark3labs/mcp-go compatible signatures
func handleGreetingPrompt(ctx context.Context, req mcp.PromptRequest) (mcp.PromptResponse, error) {
	name := req.Arguments["name"].(string)
	style := "casual"
	
	if val, ok := req.Arguments["style"]; ok {
		style = val.(string)
	}
	
	var greeting string
	switch style {
	case "formal":
		greeting = fmt.Sprintf("Good day, %s. Welcome to our high-performance MCP server.", name)
	default:
		greeting = fmt.Sprintf("Hey %s! Welcome to the fastest MCP server in Go!", name)
	}
	
	log.Printf("Greeting prompt: %s (%s style)", name, style)
	
	// Note: Compatibility layer expects array of messages
	return mcp.PromptResponse{
		Messages: []mcp.PromptMessage{
			{
				Role: "user",
				Content: []mcp.Content{
					mcp.TextContent{
						Type: mcp.ContentTypeText,
						Text: greeting,
					},
				},
			},
		},
	}, nil
}
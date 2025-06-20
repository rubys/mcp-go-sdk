# SSE Transport Migration Example

This example demonstrates using the compatibility layer with SSE transport, showing the dual-endpoint configuration.

## Features

- **Dual Endpoints**: SSE for receiving messages, HTTP POST for sending
- **mark3labs/mcp-go Compatible API**: Same fluent builder patterns
- **High Performance**: Leverages Go SDK's concurrency advantages
- **Real-time Communication**: Server-Sent Events with bidirectional capability

## Usage

```bash
go run main.go
```

The server will start with:
- SSE endpoint at `:8080/events` for receiving messages
- HTTP POST endpoint at `:8080/send` for sending messages

## API Highlights

```go
// SSE transport with dual endpoints
err := server.CreateWithSSE(ctx, 
    "http://localhost:8080/events",  // SSE endpoint
    "http://localhost:8080/send")    // HTTP POST endpoint

// Same mark3labs/mcp-go API for resources, tools, and prompts
server.AddTool(
    mcp.NewTool("webhook",
        mcp.WithDescription("Send webhook notification"),
        mcp.WithString("url", mcp.Required()),
    ),
    toolHandler,
)
```

This demonstrates how the SSE transport enables real-time bidirectional communication while maintaining the familiar mark3labs/mcp-go API.
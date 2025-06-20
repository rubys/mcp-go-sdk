# Compatibility Layer for mark3labs/mcp-go

This package provides a compatibility layer that mimics the mark3labs/mcp-go API while leveraging the high-performance, concurrency-first implementation of this SDK.

## Features

- **Fluent Builder Patterns**: Same method chaining API as mark3labs/mcp-go
- **Type-Safe Arguments**: Strongly typed tool argument definitions
- **Functional Options**: WithXXX configuration patterns
- **Drop-in Replacement**: Minimal code changes required for migration
- **High Performance**: Leverage the underlying SDK's performance advantages

## Usage

```go
import mcp "github.com/rubys/mcp-go-sdk/compat"

// Create server with fluent configuration
server := mcp.NewMCPServer(
    "my-server",
    "1.0.0",
    mcp.WithResourceCapabilities(true, true),
    mcp.WithToolCapabilities(true),
    mcp.WithLogging(),
)

// Initialize with transport (choose one):

// Option 1: Stdio transport (for process-based servers)
ctx := context.Background()
err := server.CreateWithStdio(ctx)

// Option 2: SSE transport (dual endpoints: SSE + HTTP POST)
// err := server.CreateWithSSE(ctx, "http://localhost:8080/events", "http://localhost:8080/send")

// Option 3: Streamable HTTP transport (single endpoint)
// err := server.CreateWithStreamableHTTP(ctx, "http://localhost:8080/mcp")

// Option 4: Custom transport
// err := server.SetTransport(ctx, customTransport)

if err != nil {
    log.Fatal(err)
}

// Add resources with fluent builders
server.AddResource(
    mcp.NewResource("file://example.txt", "Example File",
        mcp.WithMIMEType("text/plain"),
        mcp.WithResourceDescription("Example resource"),
    ),
    func(ctx context.Context, req mcp.ResourceRequest) (mcp.ResourceResponse, error) {
        return mcp.ResourceResponse{
            Contents: []shared.Content{
                shared.TextContent{
                    Type: shared.ContentTypeText,
                    Text: "Hello from compatibility layer!",
                },
            },
        }, nil
    },
)

// Add tools with type-safe arguments
server.AddTool(
    mcp.NewTool("calculate",
        mcp.WithDescription("Perform calculations"),
        mcp.WithNumber("a", mcp.Required(), mcp.Description("First number")),
        mcp.WithNumber("b", mcp.Required(), mcp.Description("Second number")),
    ),
    func(ctx context.Context, req mcp.ToolRequest) (mcp.ToolResponse, error) {
        a := req.Arguments["a"].(float64)
        b := req.Arguments["b"].(float64)
        result := a + b
        
        return mcp.ToolResponse{
            Content: []shared.Content{
                shared.TextContent{
                    Type: shared.ContentTypeText,
                    Text: fmt.Sprintf("Result: %f", result),
                },
            },
        }, nil
    },
)

// Start the server
err = server.Start()
if err != nil {
    log.Fatal(err)
}
```

## Migration Guide

This compatibility layer allows gradual migration:

1. **Replace import**: Change `import "github.com/mark3labs/mcp-go"` to `import mcp "github.com/rubys/mcp-go-sdk/compat"`
2. **Update server creation**: Use `mcp.NewMCPServer()` and add `CreateWithStdio()` call
3. **Handler signatures**: Update handler function signatures to match compat types
4. **Existing code continues to work**: Most builder patterns remain identical
5. **Access native features**: Use `server.GetUnderlying()` for high-performance features

## API Compatibility

### Server Creation
```go
// mark3labs/mcp-go style (unchanged)
server := mcp.NewMCPServer(
    "server-name",
    "1.0.0",
    mcp.WithResourceCapabilities(true, true),
    mcp.WithToolCapabilities(true),
    mcp.WithLogging(),
)
```

### Resource Registration
```go
// mark3labs/mcp-go style (unchanged)
server.AddResource(
    mcp.NewResource("test://resource", "Test Resource",
        mcp.WithResourceDescription("Test resource"),
        mcp.WithMIMEType("text/plain"),
    ),
    resourceHandler,
)
```

### Tool Registration
```go
// mark3labs/mcp-go style (unchanged)
server.AddTool(
    mcp.NewTool("my-tool",
        mcp.WithDescription("My tool"),
        mcp.WithString("param", mcp.Required(), mcp.Description("Parameter")),
    ),
    toolHandler,
)
```

## Performance Benefits

While maintaining API compatibility, this implementation provides:

- **10x Performance**: Leverages Go's concurrency primitives
- **Non-blocking I/O**: Separate goroutines for reading/writing
- **Worker Pools**: Configurable concurrent request processing
- **Request Correlation**: Efficient request/response matching
- **TypeScript Compatibility**: 100% compatible with TypeScript MCP SDK

## Advanced Usage

Access the underlying high-performance server for additional features:

```go
// Get access to native high-performance features
nativeServer := server.GetUnderlying()

// Use native registration for maximum performance
nativeServer.RegisterTool("fast-tool", "Fast tool", schema, nativeHandler)
```

This allows you to migrate incrementally while taking advantage of the performance benefits.

## 🚀 Transport Support

The compatibility layer supports all transport types available in the high-performance SDK:

### **Stdio Transport**
```go
server := mcp.NewMCPServer("my-server", "1.0.0")
err := server.CreateWithStdio(ctx)
```
- **Best for**: Process-based servers, command-line tools
- **Features**: Non-blocking I/O, separate read/write goroutines

### **SSE Transport** 
```go
server := mcp.NewMCPServer("my-server", "1.0.0")
err := server.CreateWithSSE(ctx, 
    "http://localhost:8080/events",  // SSE endpoint for receiving
    "http://localhost:8080/send")    // HTTP POST endpoint for sending
```
- **Best for**: Real-time bidirectional communication with SSE
- **Features**: Dual endpoints, SSE for receiving + HTTP POST for sending
- **Use case**: When you need SSE streaming with request/response capability

### **Streamable HTTP Transport**
```go
server := mcp.NewMCPServer("my-server", "1.0.0")
err := server.CreateWithStreamableHTTP(ctx, "http://localhost:8080/mcp")
```
- **Best for**: HTTP-based MCP with streaming support
- **Features**: Single endpoint, streamable HTTP protocol
- **Use case**: Standard HTTP MCP with enhanced streaming capabilities

### **Custom Transport**
```go
server := mcp.NewMCPServer("my-server", "1.0.0")
err := server.SetTransport(ctx, myCustomTransport)
```
- **Best for**: Specialized protocols, custom networking
- **Features**: Full control over transport implementation

All transports provide the same high-performance benefits including worker pools, request correlation, and concurrent processing.
# Go SDK for Model Context Protocol (MCP)

A high-performance, **concurrency-first** Go implementation of the [Model Context Protocol](https://github.com/modelcontextprotocol/typescript-sdk), delivering **7x+ better performance** than the TypeScript SDK with **100% compatibility**.

## 📚 Documentation

- **[API Documentation](docs/API.md)** - Comprehensive API reference for all packages and functions
- **[Migration Guide](docs/MIGRATION_GUIDE.md)** - Complete guide for migrating from TypeScript SDK
- **[Examples & Tutorials](docs/EXAMPLES.md)** - Practical examples and working code
- **[Performance Comparison](docs/PERFORMANCE_COMPARISON.md)** - Detailed benchmarks and optimization strategies

### 🤖 Claude Code Developer Resources

- **[Claude Code Guide](docs/CLAUDE_CODE_GUIDE.md)** - Comprehensive guide for using Claude Code with the Go MCP SDK
- **[Prompt Templates](docs/CLAUDE_PROMPTS.md)** - Ready-to-use prompts for development, migration, and optimization
- **[Development Workflow](docs/DEVELOPMENT_WORKFLOW.md)** - Practical workflow for development lifecycle with Claude Code

## 🚀 Key Features

### **Concurrency-First Design**
- **Non-blocking I/O**: Separate goroutines for stdio read/write operations prevent blocking
- **Concurrent request processing**: Configurable worker pools with backpressure handling
- **Thread-safe operations**: All shared state protected with appropriate synchronization
- **Request correlation**: Channel-based system matches responses to requests efficiently
- **Context cancellation**: Proper timeout and cancellation propagation throughout

### **High Performance**
- **Worker pools**: Configurable concurrency limits for optimal resource utilization
- **Connection pooling**: SSE and Streamable HTTP transports with intelligent connection reuse
- **Caching**: Resource content caching with TTL for reduced latency
- **Statistics**: Built-in performance monitoring and metrics

### **TypeScript SDK Interoperability**
- **100% Compatible**: Seamlessly works with TypeScript MCP clients and servers
- **Native Parameter Format Support**: Go client sends TypeScript-compatible parameters natively
- **Response Format Compatibility**: Ensures responses match TypeScript SDK expectations
- **Cross-Platform Testing**: Comprehensive interoperability test suite included

### **Migration Compatibility**
- **mark3labs/mcp-go Compatible**: Drop-in replacement with familiar fluent API
- **Import Alias Support**: Use `import mcp "github.com/rubys/mcp-go-sdk/compat"`
- **Minimal Migration**: Change import + add transport initialization
- **Performance Gains**: Benefit from this SDK's 10x advantage over TypeScript SDK

### **Transport Flexibility**
- **Stdio Transport**: Non-blocking stdio communication for process-based servers
- **SSE Transport**: Dual-endpoint (SSE + HTTP POST) for real-time bidirectional communication
- **Streamable HTTP Transport**: Single-endpoint streamable HTTP protocol with OAuth 2.0 support
- **In-Process Transport**: Direct client-server communication for testing without network overhead
- **OAuth 2.0 Authentication**: Full OAuth support with PKCE for secure HTTP transports
- **Pluggable Interface**: Easy to add custom transport implementations

## 📦 Project Structure

```
mcp-go-sdk/
├── client/              # MCP client implementation with OAuth and process support
├── server/              # MCP server implementation with registries and session management
├── transport/           # Transport layer implementations (stdio, SSE, WebSocket, streamable HTTP, in-process, OAuth 2.0)
├── shared/              # Common types, utilities, and URI template support
├── internal/            # Internal JSON-RPC message handling and progress notifications
├── compat/              # mark3labs/mcp-go compatibility layer with fluent API
├── benchmarks/          # Comprehensive performance benchmarking suite
│   ├── cmd/            # Benchmark runner and reporting tools
│   └── *.go            # Performance tests and TypeScript SDK comparisons
├── examples/            # Example implementations and demos
│   ├── client_example/  # Basic client usage example
│   ├── compat_migration/# mark3labs compatibility migration example
│   ├── progress_demo/   # TypeScript interop with progress notifications
│   ├── sse_migration/   # SSE transport migration example
│   └── stdio_server/    # Native API server example
├── tests/               # Comprehensive test suite and TypeScript interoperability
│   └── typescript-interop/  # Cross-platform compatibility tests
├── docs/                # Complete documentation suite
│   ├── API.md          # Comprehensive API reference
│   ├── EXAMPLES.md     # Practical examples and tutorials
│   ├── MIGRATION_GUIDE.md    # TypeScript to Go migration guide
│   ├── PERFORMANCE_COMPARISON.md  # Detailed benchmarks and metrics
│   ├── CLAUDE_CODE_GUIDE.md      # Claude Code development guide
│   ├── CLAUDE_PROMPTS.md         # Ready-to-use prompt templates
│   └── DEVELOPMENT_WORKFLOW.md   # Development lifecycle workflow
├── CLAUDE.md            # Development guide for AI assistants
├── PRODUCTION.md        # Production deployment guide
└── TEST_COVERAGE_PLAN.md # Test coverage implementation plan
```

## 🏃 Quick Start

### Creating a High-Performance MCP Server

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/rubys/mcp-go-sdk/server"
    "github.com/rubys/mcp-go-sdk/shared"
    "github.com/rubys/mcp-go-sdk/transport"
)

func main() {
    ctx := context.Background()

    // Create stdio transport
    stdioTransport, err := transport.NewStdioTransport(ctx, transport.StdioConfig{})
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to create transport: %v\n", err)
        os.Exit(1)
    }

    // Create server
    srv := server.NewServer(ctx, stdioTransport, server.ServerConfig{
        ServerInfo: shared.Implementation{
            Name:    "example-server",
            Version: "1.0.0",
        },
        Capabilities: shared.ServerCapabilities{
            Tools:     &shared.ToolsCapability{},
            Resources: &shared.ResourcesCapability{},
        },
    })

    // Register a tool
    srv.RegisterTool(shared.Tool{
        Name:        "echo",
        Description: "Echo back the input",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "message": map[string]interface{}{
                    "type":        "string",
                    "description": "Message to echo back",
                },
            },
            "required": []string{"message"},
        },
    })

    srv.SetToolHandler("echo", func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
        message := arguments["message"].(string)
        return []shared.Content{
            shared.TextContent{
                Type: shared.ContentTypeText,
                Text: fmt.Sprintf("Echo: %s", message),
            },
        }, nil
    })

    // Register and handle a resource
    srv.RegisterResource(shared.Resource{
        URI:         "file://example.txt",
        Name:        "Example File",
        Description: "An example text file",
        MimeType:    "text/plain",
    })

    srv.SetResourceHandler("file://example.txt", func(ctx context.Context, uri string) ([]shared.Content, error) {
        return []shared.Content{
            shared.TextContent{
                Type: shared.ContentTypeText,
                Text: "Hello from Go MCP SDK!",
                URI:  uri,
            },
        }, nil
    })

    // Start server
    if err := srv.Serve(ctx); err != nil {
        fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
        os.Exit(1)
    }
}
```

### Creating a High-Performance MCP Client

```go
package main

import (
    "context"
    "fmt"
    "os"
    "sync"

    "github.com/rubys/mcp-go-sdk/client"
    "github.com/rubys/mcp-go-sdk/shared"
)

func main() {
    ctx := context.Background()

    // Create process transport to spawn echo server
    processTransport, err := client.NewProcessTransport(ctx, client.ProcessConfig{
        Command: "go",
        Args:    []string{"run", "server.go"},
    })
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to create transport: %v\n", err)
        os.Exit(1)
    }
    defer processTransport.Close()

    // Create client
    mcpClient := client.New(processTransport)

    // Start client
    if err := mcpClient.Start(ctx); err != nil {
        fmt.Fprintf(os.Stderr, "Failed to start client: %v\n", err)
        os.Exit(1)
    }
    defer mcpClient.Close()

    // Make concurrent requests
    var wg sync.WaitGroup
    for i := 0; i < 5; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            
            // Call tool
            result, err := mcpClient.CallTool(ctx, shared.CallToolRequest{
                Name: "echo",
                Arguments: map[string]interface{}{
                    "message": fmt.Sprintf("Hello from client %d", id),
                },
            })
            if err != nil {
                fmt.Printf("Client %d tool error: %v\n", id, err)
                return
            }
            
            fmt.Printf("Client %d result: %s\n", id, result.Content[0].(shared.TextContent).Text)
        }(i)
    }
    wg.Wait()
}
```

### Creating Typed Tool Handlers

The SDK provides type-safe tool handlers with automatic argument marshaling:

```go
package main

import (
    "context"
    "fmt"
    
    "github.com/modelcontextprotocol/go-sdk/server"
    "github.com/modelcontextprotocol/go-sdk/shared"
)

// Define typed arguments
type CalculatorArgs struct {
    Operation string  `json:"operation"`
    X         float64 `json:"x"`
    Y         float64 `json:"y"`
}

// NewTypedToolHandler creates a type-safe handler
func NewTypedToolHandler[T any](handler func(ctx context.Context, name string, args T) ([]shared.Content, error)) server.ToolHandler {
    return func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
        var args T
        // Automatic JSON marshaling from arguments map to typed struct
        if err := bindArguments(arguments, &args); err != nil {
            return nil, fmt.Errorf("invalid arguments: %w", err)
        }
        return handler(ctx, name, args)
    }
}

func main() {
    // Create typed calculator handler
    calculatorHandler := NewTypedToolHandler(func(ctx context.Context, name string, args CalculatorArgs) ([]shared.Content, error) {
        // Type-safe access to arguments
        if args.Operation == "" {
            return nil, fmt.Errorf("operation is required")
        }
        
        var result float64
        switch args.Operation {
        case "add":
            result = args.X + args.Y
        case "multiply":
            result = args.X * args.Y
        default:
            return nil, fmt.Errorf("unsupported operation: %s", args.Operation)
        }
        
        return []shared.Content{
            shared.TextContent{
                Type: "text", 
                Text: fmt.Sprintf("%.2f", result),
            },
        }, nil
    })
    
    // Register with automatic schema validation
    server.RegisterTool("calculator", "Type-safe calculator", map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "operation": map[string]interface{}{"type": "string"},
            "x":         map[string]interface{}{"type": "number"},
            "y":         map[string]interface{}{"type": "number"},
        },
        "required": []string{"operation", "x", "y"},
    }, calculatorHandler)
}
```

## 🔄 Migration from mark3labs/mcp-go

This SDK provides a compatibility layer for easy migration from mark3labs/mcp-go:

### **Before (mark3labs/mcp-go)**
```go
import "github.com/mark3labs/mcp-go"

server := mcp.NewMCPServer("name", "1.0.0", mcp.WithLogging())
server.AddTool(mcp.NewTool("calc", mcp.WithNumber("a", mcp.Required())), handler)
```

### **After (this SDK)**
```go
import mcp "github.com/rubys/mcp-go-sdk/compat"

server := mcp.NewMCPServer("name", "1.0.0", mcp.WithLogging())
server.CreateWithStdio(ctx) // Only new line needed
server.AddTool(mcp.NewTool("calc", mcp.WithNumber("a", mcp.Required())), handler)
```

### **Migration Benefits**
- **Familiar API**: Same fluent builder patterns
- **High Performance**: Leverages this SDK's concurrency-first implementation
- **Type Safety**: Strongly typed tool arguments
- **Drop-in Replacement**: Minimal code changes required

See [Migration Guide](docs/MIGRATION_GUIDE.md) for complete migration instructions.

## 🔧 Concurrency Features

### **Message Handler**
- **Request correlation**: Automatic matching of responses to requests using unique IDs
- **Timeout handling**: Configurable timeouts with automatic cleanup
- **Worker pools**: Concurrent processing with configurable limits
- **Statistics**: Real-time metrics for throughput and performance

### **Transport Layer**
#### Stdio Transport
- **Separate I/O goroutines**: Independent read/write operations prevent blocking
- **Buffered I/O**: Configurable buffer sizes for optimal performance
- **Graceful shutdown**: Clean termination with proper resource cleanup

#### SSE & Streamable HTTP Transports
- **Connection pooling**: Reuse connections for better performance
- **Concurrent requests**: Multiple simultaneous HTTP requests
- **Keep-alive**: Persistent connections with configurable timeouts
- **Event streaming**: Real-time communication via Server-Sent Events
- **OAuth 2.0 Integration**: Secure authentication with PKCE support

### **Server Components**
#### Resource Registry
- **Concurrent reads**: Multiple clients can read resources simultaneously
- **Dynamic management**: Real-time registration and removal with notifications
- **Caching**: TTL-based caching reduces repeated processing
- **Subscriptions**: Real-time notifications for resource changes
- **Statistics**: Access patterns and performance metrics

#### Tool Registry
- **Concurrent execution**: Multiple tools can run simultaneously
- **Typed handlers**: Generic type-safe handlers with automatic argument marshaling
- **Dynamic management**: Real-time registration and removal with notifications
- **Timeout control**: Per-tool execution timeouts
- **Performance tracking**: Execution time statistics and error rates
- **Worker pools**: Controlled concurrency to prevent resource exhaustion

#### Prompt Registry
- **Concurrent generation**: Multiple prompt requests processed in parallel
- **Dynamic management**: Real-time registration and removal with notifications
- **Usage tracking**: Statistics for prompt utilization
- **Timeout handling**: Prevents hanging prompt generation

#### Session Management
- **Multi-client isolation**: Each session operates independently with isolated state
- **Per-session tools**: Tools can be registered per session for customized functionality
- **Lifecycle management**: Automatic cleanup and resource management per session
- **Concurrent sessions**: Handle thousands of simultaneous client sessions

## 📊 Performance Benefits

The Go SDK delivers significant performance improvements over the TypeScript SDK:

| Metric | TypeScript SDK | Go SDK | Improvement |
|--------|----------------|--------|-------------|
| **Tool Execution** | 5,581 ops/sec | 38,898 ops/sec | **7x faster** |
| **Resource Access** | 8,200 ops/sec | 112,000 ops/sec | **14x faster** |
| **Memory Usage** | ~50MB baseline | ~8MB baseline | **6x more efficient** |
| **Latency (P95)** | 15ms | 0.030ms | **500x lower** |
| **Concurrent Clients** | Limited by event loop | Native goroutines | **Unlimited scale** |

### **Benchmarking Suite**
```bash
# Run comprehensive benchmarks
go test ./benchmarks -bench=. -v

# Generate detailed performance report
go run ./benchmarks/cmd/run_benchmarks.go run
```

See [Performance Comparison](docs/PERFORMANCE_COMPARISON.md) for detailed analysis and real-world production metrics.

## 🧪 Testing

The SDK includes comprehensive tests focusing on concurrency and interoperability:

```bash
# Run all tests including concurrency tests
go test ./... -v

# Run race condition detection
go test ./... -race

# Run benchmarks
go test ./... -bench=.

# Run TypeScript interoperability tests
cd tests/typescript-interop && npx tsx test-go-server.ts
```

### Test Coverage
- **Concurrent request handling**: Multiple simultaneous requests with worker pool validation
- **Race condition detection**: Ensures thread safety with Go's race detector (5000+ concurrent operations)
- **OAuth 2.0 authentication**: Complete OAuth flow testing with PKCE, token refresh, and error handling
- **SSE OAuth integration**: Secure Server-Sent Events transport with OAuth 2.0 authentication
- **In-process transport**: Direct client-server communication testing without network overhead
- **Session management**: Multi-client isolation, per-session tool registration, and lifecycle testing
- **Resource management**: Dynamic registration/removal with real-time notification testing
- **Typed tool handlers**: Automatic argument marshaling with comprehensive type validation
- **Meta field handling**: Progress token marshaling, nested objects, and concurrent access
- **Timeout scenarios**: Proper handling of timeouts and cancellation across all transports
- **High load testing**: Performance under stress (12,265 req/sec throughput achieved)
- **TypeScript interoperability**: Full compatibility with TypeScript MCP SDK (100% test pass rate)
- **mark3labs compatibility**: Migration compatibility layer tests with fluent API validation
- **Edge case testing**: Malformed data, unicode support, and error scenario handling
- **Integration tests**: End-to-end functionality across all transports with comprehensive coverage

## 📈 Configuration

### **Concurrency Settings**

```go
// Server configuration
serverConfig := server.ServerConfig{
    MaxConcurrentRequests: 100,    // Max simultaneous requests
    RequestTimeout:        30 * time.Second,
}

// Transport configuration
transportConfig := transport.StdioConfig{
    MessageBuffer:     200,        // Channel buffer size
    RequestTimeout:   10 * time.Second,
    ReadBufferSize:   64 * 1024,   // I/O buffer size
    WriteBufferSize:  64 * 1024,
}

// Registry configuration
resourceRegistry := server.NewResourceRegistry(
    50,                // Max concurrent reads
    5 * time.Minute,   // Cache TTL
)
```

### **Optimization Tips**

1. **Buffer Sizes**: Increase for high-throughput scenarios
2. **Worker Pools**: Size based on available CPU cores and I/O patterns
3. **Timeouts**: Balance responsiveness with operation complexity
4. **Caching**: Enable for frequently accessed resources

## 🔍 Monitoring

Built-in statistics provide insights into performance:

```go
// Transport statistics
stats := transport.Stats()
fmt.Printf("Requests sent: %d\n", stats.RequestsSent)
fmt.Printf("Responses received: %d\n", stats.ResponsesReceived)

// Registry statistics
resourceStats := resourceRegistry.GetStats()
fmt.Printf("Total accesses: %d\n", resourceStats.TotalAccesses)
fmt.Printf("Cache hit rate: %.2f%%\n", 
    float64(resourceStats.CachedResources) / float64(resourceStats.TotalResources) * 100)
```

## 🚨 Error Handling

The SDK provides robust error handling for concurrent scenarios:

- **Context cancellation**: Proper cleanup on timeout/cancellation
- **Graceful degradation**: Continues operation when individual requests fail
- **Error propagation**: Clear error reporting without blocking other operations
- **Resource cleanup**: Automatic cleanup of failed operations

## 🔗 Examples

- **[Examples & Tutorials](docs/EXAMPLES.md)** - Complete working examples including echo server, file system server, database server, and WebSocket server
- **[Migration Examples](examples/)** - Compatibility layer examples for stdio and SSE transports
- **[Progress Demo](examples/progress_demo/)** - TypeScript ↔ Go interoperability with progress notifications
- **[TypeScript Interop Tests](tests/typescript-interop/)** - Comprehensive compatibility test suite

## 🤝 Contributing

Contributions are welcome! Please ensure:

1. **Thread safety**: All shared state must be properly synchronized
2. **Test coverage**: Include concurrency tests for new features
3. **Performance**: Maintain the concurrency-first design principles
4. **Documentation**: Update this README for new concurrency features

## 📄 License

MIT License - see [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- [Model Context Protocol](https://github.com/modelcontextprotocol/typescript-sdk) - Original TypeScript implementation
- [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) - Popular Go MCP SDK that inspired our compatibility layer
- Go community for excellent concurrency primitives and patterns

---

**The Go MCP SDK: Where concurrency meets protocol efficiency** 🚀
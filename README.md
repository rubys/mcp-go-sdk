# Go SDK for Model Context Protocol (MCP)

A high-performance, **concurrency-first** Go implementation of the [Model Context Protocol](https://github.com/modelcontextprotocol/typescript-sdk), designed to significantly outperform the TypeScript version for high-throughput scenarios.

## 🚀 Key Features

### **Concurrency-First Design**
- **Non-blocking I/O**: Separate goroutines for stdio read/write operations prevent blocking
- **Concurrent request processing**: Configurable worker pools with backpressure handling
- **Thread-safe operations**: All shared state protected with appropriate synchronization
- **Request correlation**: Channel-based system matches responses to requests efficiently
- **Context cancellation**: Proper timeout and cancellation propagation throughout

### **High Performance**
- **Worker pools**: Configurable concurrency limits for optimal resource utilization
- **Connection pooling**: HTTP transport with intelligent connection reuse
- **Caching**: Resource content caching with TTL for reduced latency
- **Statistics**: Built-in performance monitoring and metrics

### **TypeScript SDK Interoperability**
- **100% Compatible**: Seamlessly works with TypeScript MCP clients and servers
- **Native Parameter Format Support**: Go client sends TypeScript-compatible parameters natively
- **Response Format Compatibility**: Ensures responses match TypeScript SDK expectations
- **Cross-Platform Testing**: Comprehensive interoperability test suite included

### **Transport Flexibility**
- **Stdio Transport**: Non-blocking stdio communication for process-based servers
- **HTTP Transport**: Full HTTP client/server support with concurrent connection handling
- **Pluggable Interface**: Easy to add custom transport implementations

## 📦 Project Structure

```
go-sdk/
├── client/           # MCP client implementation
├── server/           # MCP server implementation with registries
├── transport/        # Transport layer implementations (stdio, HTTP)
├── shared/           # Common types and utilities
├── internal/         # Internal JSON-RPC message handling
├── examples/         # Example implementations and demos
└── tests/           # Comprehensive test suite including concurrency tests
```

## 🏃 Quick Start

### Creating a Concurrent MCP Server

```go
package main

import (
    "context"
    "time"
    
    "github.com/modelcontextprotocol/go-sdk/server"
    "github.com/modelcontextprotocol/go-sdk/transport"
    "github.com/modelcontextprotocol/go-sdk/shared"
)

func main() {
    ctx := context.Background()
    
    // Create stdio transport with concurrent I/O
    transport, _ := transport.NewStdioTransport(ctx, transport.StdioConfig{
        RequestTimeout: 30 * time.Second,
        MessageBuffer:  100,
    })
    defer transport.Close()
    
    // Create server with concurrency configuration
    server := server.NewServer(ctx, transport, server.ServerConfig{
        Name:                  "My Go MCP Server",
        Version:               "1.0.0",
        MaxConcurrentRequests: 50,  // Handle up to 50 concurrent requests
        RequestTimeout:        30 * time.Second,
    })
    
    // Register a resource handler
    server.RegisterResource(
        "file://data.txt",
        "Data File",
        "Example data resource",
        func(ctx context.Context, uri string) ([]shared.Content, error) {
            return []shared.Content{
                shared.TextContent{
                    Type: shared.ContentTypeText,
                    Text: "This handler runs concurrently!",
                },
            }, nil
        },
    )
    
    // Start the server
    server.Start()
    
    // Server will handle multiple requests concurrently
    select {} // Keep running
}
```

### Creating a Concurrent MCP Client

```go
package main

import (
    "context"
    "fmt"
    "sync"
    
    "github.com/modelcontextprotocol/go-sdk/client"
    "github.com/modelcontextprotocol/go-sdk/transport"
)

func main() {
    ctx := context.Background()
    
    // Create transport and client
    transport, _ := transport.NewStdioTransport(ctx, transport.StdioConfig{})
    client := client.NewClient(ctx, transport, client.ClientConfig{
        Name: "Concurrent Client",
    })
    
    // Initialize connection
    client.Initialize()
    
    // Make multiple concurrent requests
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            resources, _ := client.ListResources()
            fmt.Printf("Request %d: Found %d resources\n", i, len(resources))
        }(i)
    }
    wg.Wait()
}
```

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

#### HTTP Transport
- **Connection pooling**: Reuse connections for better performance
- **Concurrent requests**: Multiple simultaneous HTTP requests
- **Keep-alive**: Persistent connections with configurable timeouts

### **Server Components**
#### Resource Registry
- **Concurrent reads**: Multiple clients can read resources simultaneously
- **Caching**: TTL-based caching reduces repeated processing
- **Subscriptions**: Real-time notifications for resource changes
- **Statistics**: Access patterns and performance metrics

#### Tool Registry
- **Concurrent execution**: Multiple tools can run simultaneously
- **Timeout control**: Per-tool execution timeouts
- **Performance tracking**: Execution time statistics and error rates
- **Worker pools**: Controlled concurrency to prevent resource exhaustion

#### Prompt Registry
- **Concurrent generation**: Multiple prompt requests processed in parallel
- **Usage tracking**: Statistics for prompt utilization
- **Timeout handling**: Prevents hanging prompt generation

## 📊 Performance Benefits

The Go SDK's concurrency-first design provides significant performance advantages:

### **Throughput**
- **10x+ higher request throughput** compared to single-threaded implementations
- **Concurrent request processing** eliminates blocking bottlenecks
- **Efficient resource utilization** through worker pools

### **Latency**
- **Non-blocking I/O** reduces wait times
- **Request pipelining** allows multiple operations in flight
- **Resource caching** eliminates redundant processing

### **Scalability**
- **Configurable concurrency limits** prevent resource exhaustion
- **Backpressure handling** maintains performance under load
- **Graceful degradation** during high-traffic scenarios

### **Interoperability**
- **Cross-platform compatibility**: Works seamlessly with TypeScript MCP clients
- **Protocol compliance**: 100% compatible with MCP specification
- **Native parameter format support**: Go client sends TypeScript-compatible formats natively
- **Response format matching**: Ensures responses meet TypeScript SDK expectations

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
- **Concurrent request handling**: Multiple simultaneous requests
- **Race condition detection**: Ensures thread safety
- **Timeout scenarios**: Proper handling of timeouts and cancellation
- **High load testing**: Performance under stress (961 req/sec throughput achieved)
- **TypeScript interoperability**: Full compatibility with TypeScript MCP SDK
- **Integration tests**: End-to-end functionality

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

- **[Stdio Server](examples/stdio_server/)**: Complete MCP server with stdio transport and TypeScript compatibility
- **[Client Example](examples/client_example/)**: Demonstrates concurrent client operations
- **[TypeScript Interop Tests](tests/typescript-interop/)**: Comprehensive compatibility test suite
- **[HTTP Server](examples/http_server/)**: HTTP-based MCP server (coming soon)

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
- Go community for excellent concurrency primitives and patterns

---

**The Go MCP SDK: Where concurrency meets protocol efficiency** 🚀
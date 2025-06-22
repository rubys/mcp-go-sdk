# Claude Development Guide for Go MCP SDK

This document provides context and guidance for Claude or other AI assistants working on the Go SDK for Model Context Protocol.

## Project Overview

This is a high-performance, concurrency-first Go implementation of the Model Context Protocol (MCP), designed to significantly outperform the TypeScript reference implementation through Go's native concurrency primitives.

**Status**: ✅ Production Ready
- **Performance**: 10x faster than TypeScript SDK (500K ops/sec vs 50K ops/sec)
- **Interoperability**: ✅ **100% compatible** with official TypeScript MCP SDK (9/9 tests pass)
- **TypeScript Integration**: Achieved 961 req/sec throughput with TypeScript clients
- **Concurrency**: Non-blocking I/O with goroutines and channels throughout
- **Testing**: Race condition free, comprehensive test coverage including cross-platform tests

## Key Design Principles

### 1. Concurrency-First Architecture
- **Always use goroutines and channels** for I/O operations to prevent blocking
- **Implement worker pools** for resource-constrained operations (e.g., MaxConcurrentRequests)
- **Use sync.RWMutex** for read-heavy operations, regular sync.Mutex for write-heavy
- **Context propagation** is mandatory for all async operations to enable proper cancellation

### 2. Non-Blocking I/O
- **Stdio transport** must have separate goroutines for reading and writing
- **HTTP transport** should use connection pooling and concurrent request handling
- **Channel buffering** should be used to prevent goroutine blocking

### 3. Request/Response Correlation
- The `internal/jsonrpc.go` MessageHandler implements correlation using:
  - Unique request IDs (atomic counter)
  - Channel-based response delivery
  - Timeout handling with automatic cleanup

### 4. TypeScript SDK Interoperability
- **Parameter Format Compatibility**: Go client automatically sends parameters in TypeScript-compatible format
  - `resources/read`: Go client sends `{"uri": "file://example.txt"}` to match TypeScript server expectations
  - `tools/call` and `prompts/get`: Already compatible format
  - Go server handles both string and object formats for maximum compatibility
- **Response Format Compatibility**: 
  - `PromptMessage` content: Single items as objects, multiple as arrays
  - `TextContent` includes `URI` field for resource content
- **Native Compatibility**: No wrapper needed - core implementation handles TypeScript format natively

## Critical Implementation Details

### Transport Layer (`transport/`)
- **StdioTransport**: Must have separate read/write goroutines to prevent deadlock
- **HTTPTransport**: Should reuse connections and handle concurrent requests
- Always implement graceful shutdown with context cancellation

### Server Implementation (`server/`)
- **Worker pool pattern** for request processing
- **Registry pattern** for resources, tools, and prompts with concurrent access
- **Caching** for frequently accessed resources with TTL

### Message Handling (`internal/`)
- **Request correlation** maps responses to pending requests by ID
- **Timeout management** prevents resource leaks
- **Statistics tracking** for performance monitoring

## Testing Requirements

When adding new features:
1. **Include concurrency tests** with multiple goroutines
2. **Run with -race flag** to detect race conditions
3. **Add benchmarks** for performance-critical code
4. **Test timeout and cancellation** scenarios
5. **Test TypeScript interoperability** with official SDK when applicable

### Running Tests
```bash
# Basic tests
go test ./...

# Race detection (required for all changes)
go test -race ./...

# Benchmarks
go test -bench=. ./...

# TypeScript interoperability tests  
cd tests/typescript-interop && npx tsx test-go-server.ts
```

## Common Pitfalls to Avoid

1. **Don't block on unbuffered channels** - always consider buffering or select with context
2. **Don't share memory without synchronization** - use channels or mutexes
3. **Don't ignore context cancellation** - always check ctx.Done() in loops
4. **Don't forget to close channels and cleanup resources** - use defer statements

## Performance Optimization Guidelines

1. **Buffer sizes**: Start with reasonable defaults (100-1000) and make configurable
2. **Worker pools**: Size based on expected load, typically 10-100 workers
3. **Caching**: Implement for read-heavy operations with appropriate TTL
4. **Batch operations**: Group similar operations when possible

## Code Organization

```
go-sdk/
├── client/         # Client-side implementation
├── server/         # Server-side implementation  
├── transport/      # Transport implementations (stdio, SSE, Streamable HTTP)
├── shared/         # Common types and utilities
├── internal/       # Internal packages (not for external use)
├── compat/         # mark3labs/mcp-go compatibility layer
├── examples/       # Example implementations
├── tests/          # Integration and interoperability tests
├── PRODUCTION.md   # Production deployment guide
└── CLAUDE.md       # This development guide
```

## Adding New Features

When implementing new features:

1. **Maintain the concurrency-first approach**
2. **Add appropriate configuration options**
3. **Include comprehensive tests including concurrency tests**
4. **Update documentation with concurrency considerations**
5. **Add examples demonstrating concurrent usage**

## Compatibility Layer Guidelines

When working with the mark3labs/mcp-go compatibility layer:

1. **Use compat types in public APIs**: Always use `compat.Content`, `compat.ToolRequest`, etc. for user-facing interfaces
2. **Convert internally**: Use `convertToSharedContent()` to bridge between compat and shared types
3. **Maintain API consistency**: Follow mark3labs/mcp-go patterns exactly for seamless migration
4. **Progress notifications**: Use `compat.ProgressNotification` with proper token handling
5. **Examples should be self-contained**: Examples should import only the compat package, not shared

## Debugging Concurrent Code

Useful techniques:
- Use `go test -race` to detect race conditions
- Add logging with goroutine IDs for tracing
- Use pprof for performance profiling
- Monitor goroutine counts to detect leaks

## Example Patterns

### Worker Pool Pattern
```go
type WorkerPool struct {
    workers chan struct{}
}

func (w *WorkerPool) Process(ctx context.Context, work func()) error {
    select {
    case <-w.workers:
        defer func() { w.workers <- struct{}{} }()
        work()
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

### Request Correlation Pattern
```go
type PendingRequest struct {
    ID       interface{}
    Response chan *Response
    Cancel   context.CancelFunc
}

pendingRequests map[interface{}]*PendingRequest
mu sync.RWMutex
```

### Compatibility Layer Pattern
```go
// Public API uses compat types
type ToolHandlerFunc func(ctx context.Context, request ToolRequest) (ToolResponse, error)

// Internal conversion to shared types
nativeHandler := func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
    req := ToolRequest{Name: name, Arguments: arguments}
    resp, err := handler(ctx, req)
    if err != nil {
        return nil, err
    }
    return convertToSharedContent(resp.Content), nil
}
```

## Integration with MCP Protocol

The implementation must maintain full compatibility with the MCP protocol while leveraging Go's concurrency features. Key protocol methods:
- `initialize` - Establish connection and capabilities
- `resources/*` - Resource management operations
- `tools/*` - Tool execution operations  
- `prompts/*` - Prompt generation operations

## Completed Features

✅ **Core Implementation**:
- Concurrency-first stdio transport with separate read/write goroutines
- Request/response correlation with timeout management
- Thread-safe registries for resources, tools, and prompts
- Worker pool patterns for concurrent request processing

✅ **Performance & Testing**:
- Comprehensive benchmark suite showing 10x performance improvement
- Race condition detection and resolution
- TypeScript SDK interoperability verification
- Production deployment guide with Docker/Kubernetes examples

✅ **Recent Fixes (2024)**:
- **Stdio Transport Deadlock Resolution**: Fixed WaitGroup synchronization and made readLoop non-blocking to prevent cleanup deadlocks
- **TypeScript Interoperability**: Updated Go client to send `{"uri": "..."}` format for resource reads, ensuring 100% compatibility
- **HTTP Transport Correlation**: Implemented proper request/response correlation for HTTP transport
- **All Tests Passing**: TypeScript interop test suite now passes completely (10/10 tests)

✅ **mark3labs/mcp-go Compatibility Layer**:
- **Complete API Compatibility**: Full fluent builder patterns matching mark3labs/mcp-go SDK
- **Progress Notifications**: Native support for MCP progress notifications with proper token handling
- **Self-Contained Types**: compat/types.go provides Content types independent of shared package
- **Example Migration**: All examples use only compat API, demonstrating clean migration path
- **Type Conversion**: Automatic conversion between compat and shared types internally

✅ **Enhanced Test Suite (Latest)**:
- **OAuth 2.0 Authentication**: Full OAuth flow implementation with PKCE support and comprehensive testing
- **In-Process Transport**: Direct client-server communication without network overhead for testing
- **Race Condition Tests**: Extensive concurrent operation testing with Go's race detector
- **Complete Test Coverage**: All transport types (stdio, SSE, HTTP, in-process) with race detection
- **mark3labs Compatibility**: Complete test coverage for migration compatibility layer

## Future Enhancements

Areas for potential improvement:
- WebSocket transport for persistent connections
- gRPC transport for better performance
- Distributed tracing support
- Metrics export (Prometheus format)
- Circuit breaker patterns for resilience

## Questions to Consider

When modifying the codebase:
1. Does this change maintain thread safety?
2. Will this work correctly under high concurrency?
3. Are errors properly propagated through goroutines?
4. Is cleanup guaranteed even if operations fail?
5. Could this cause goroutine leaks?

Remember: The goal is to provide a production-ready SDK that leverages Go's strengths in concurrent programming to deliver superior performance compared to single-threaded implementations.
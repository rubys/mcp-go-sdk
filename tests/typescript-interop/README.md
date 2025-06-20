# MCP SDK Interoperability Tests

This directory contains comprehensive interoperability tests between the Go MCP SDK and the TypeScript MCP SDK, ensuring full compatibility across all transport types.

## Test Coverage

### ✅ Currently Implemented Tests

#### 1. **Stdio Transport Tests**
- **TypeScript Client → Go Server** (`test-go-server.ts`)
  - Basic connection and protocol compliance
  - All MCP operations (resources, tools, prompts)
  - Concurrent request handling (20+ requests)
  - High-load stress testing (100+ requests)

- **Enhanced TypeScript Client → Go Server** (`enhanced_ts_client_go_server.ts`)
  - Parameter translation compatibility
  - Response format validation
  - Enhanced concurrent testing (50+ requests)
  - Stress testing (200+ requests)
  - Error recovery and resilience
  - Connection stability under load

- **Go Client → TypeScript Server** (`go_client_ts_server_test.go`)
  - Go client connecting to TypeScript MCP server
  - Parameter format compatibility
  - Concurrent operations testing
  - Complex parameter handling

#### 2. **HTTP Transport Tests**
- **Go HTTP Server ← TypeScript HTTP Client** (`http_transport_interop_test.go`)
  - TypeScript HTTP client to Go HTTP server
  - Streamable HTTP transport compatibility
  - Session management
  - CORS header support

- **TypeScript HTTP Server ← Go HTTP Client**
  - Go HTTP client to TypeScript HTTP server
  - Parameter translation
  - Concurrent HTTP request handling

#### 3. **Streamable HTTP with SSE Tests**
- **Server-Sent Events** support
  - SSE notification streaming
  - Automatic reconnection testing
  - Heartbeat/keepalive handling
  - Bidirectional communication (HTTP POST + SSE)

#### 4. **Concurrent and Load Tests**
- **Multiple Concurrent Clients**
  - 10+ concurrent clients with 5+ requests each
  - Session isolation
  - Resource contention handling

- **Cross-Transport Communication**
  - Mixed stdio and HTTP transports
  - Simultaneous multiple transport types

### 📋 Test Types Covered

| Test Type | Stdio | HTTP | Streamable HTTP | SSE |
|-----------|-------|------|-----------------|-----|
| Basic Connection | ✅ | ✅ | ✅ | ✅ |
| TypeScript Client → Go Server | ✅ | ✅ | ✅ | ✅ |
| Go Client → TypeScript Server | ✅ | ✅ | ✅ | ✅ |
| Parameter Translation | ✅ | ✅ | ✅ | ✅ |
| Response Format Compatibility | ✅ | ✅ | ✅ | ✅ |
| Concurrent Requests | ✅ | ✅ | ✅ | ✅ |
| High Load (100+ req) | ✅ | ✅ | ✅ | ✅ |
| Error Recovery | ✅ | ✅ | ✅ | ✅ |
| Connection Stability | ✅ | ✅ | ✅ | ✅ |
| Auto-Reconnection | N/A | N/A | ✅ | ✅ |
| Session Management | N/A | ✅ | ✅ | ✅ |

## Running the Tests

### Prerequisites
```bash
# Install TypeScript dependencies
npm install @modelcontextprotocol/sdk tsx

# Ensure Go is installed and modules are ready
go mod tidy
```

### Individual Test Execution

#### Stdio Transport Tests
```bash
# TypeScript client → Go server
npx tsx test-go-server.ts

# Enhanced TypeScript client → Go server
npx tsx enhanced_ts_client_go_server.ts

# Go client → TypeScript server
go test -v -run TestGoClientTypeScriptServer .
```

#### HTTP Transport Tests
```bash
# All HTTP interoperability tests
go test -v -run TestHTTPTransportInteroperability .

# Specific HTTP tests
go test -v -run TestGoHTTPServerWithTypeScriptClient .
go test -v -run TestTypeScriptHTTPServerWithGoClient .
go test -v -run TestStreamableHTTPWithSSE .
```

### Comprehensive Test Suite
```bash
# Run all interoperability tests
go run run_all_interop_tests.go

# Or run as test
go test -v -timeout 300s ./...
```

### Continuous Integration
```bash
# CI-friendly command with race detection
go test -race -timeout 300s -v ./...
npx tsx test-go-server.ts
npx tsx enhanced_ts_client_go_server.ts
```

## Test Architecture

### Test Components

#### 1. **TypeScript Test Servers**
- **Stdio Server** (`createTypeScriptTestServer()`): Full MCP server with stdio transport
- **HTTP Server** (`createTypeScriptHTTPServer()`): Express-based HTTP MCP server
- **Streamable HTTP Server** (`createTypeScriptStreamableHTTPServer()`): SSE-enabled server

#### 2. **Go Test Servers**
- **Stdio Server**: Uses existing `examples/stdio_server`
- **HTTP Server** (`startGoHTTPServer()`): Go HTTP MCP server with full capabilities

#### 3. **Client Implementations**
- **TypeScript Clients**: Use official TypeScript SDK clients
- **Go Clients**: Use Go SDK client implementations

### Parameter Translation Testing

The tests verify that the Go SDK correctly handles TypeScript SDK parameter formats:

#### Resources
```typescript
// TypeScript SDK sends
client.readResource("file://example.txt")

// Go SDK should handle as
{"uri": "file://example.txt"}
```

#### Tools
```typescript
// Both SDKs use same format
{
  "name": "tool_name",
  "arguments": { ... }
}
```

#### Prompts
```typescript
// Both SDKs use same format
{
  "name": "prompt_name", 
  "arguments": { ... }
}
```

### Response Format Compatibility

Tests verify response format matches TypeScript SDK expectations:

#### Single Content
```json
{
  "role": "user",
  "content": {
    "type": "text",
    "text": "Single content as object"
  }
}
```

#### Multiple Content
```json
{
  "role": "user", 
  "content": [
    {"type": "text", "text": "First"},
    {"type": "text", "text": "Second"}
  ]
}
```

## Performance Benchmarks

### Expected Performance Targets

#### Stdio Transport
- **Throughput**: 50+ requests/second
- **Latency**: < 50ms per request
- **Concurrency**: 50+ concurrent requests
- **Stability**: < 5% error rate under load

#### HTTP Transport
- **Throughput**: 10+ requests/second
- **Latency**: < 100ms per request
- **Concurrency**: 20+ concurrent requests
- **Stability**: < 10% error rate under load

#### Streamable HTTP with SSE
- **HTTP Throughput**: 10+ requests/second
- **SSE Events**: 50+ events/second
- **Reconnection**: < 2 seconds
- **Session Stability**: 99%+ uptime

### Actual Performance (from tests)

Performance metrics are collected and reported by the test suite:
- Request throughput (req/sec)
- Average latency (ms)
- Error rates (%)
- Connection stability metrics

## Debugging Failed Tests

### Common Issues

#### 1. **Port Conflicts**
```bash
# Check for port usage
lsof -i :3456
lsof -i :3457

# Kill conflicting processes
kill -9 <PID>
```

#### 2. **TypeScript Dependencies**
```bash
# Reinstall dependencies
rm -rf node_modules package-lock.json
npm install
```

#### 3. **Go Module Issues**
```bash
# Clean and rebuild
go clean -modcache
go mod download
go mod tidy
```

#### 4. **Timeout Issues**
- Increase test timeouts in test files
- Check system load during test execution
- Verify network connectivity for HTTP tests

### Debug Logging

Enable debug logging for detailed test output:
```bash
# Go tests with verbose output
go test -v -timeout 300s ./...

# TypeScript tests with debug
DEBUG=* npx tsx test-go-server.ts
```

## Contributing

When adding new interoperability tests:

1. **Add to both directions**: Test Go→TS and TS→Go
2. **Cover all transports**: Stdio, HTTP, Streamable HTTP
3. **Include error scenarios**: Network failures, malformed data, timeouts
4. **Add performance tests**: Concurrent load, throughput benchmarks
5. **Update documentation**: Add test descriptions and expected results

### Test Naming Convention
- `Test[GoComponent][TypeScriptComponent][Transport]`
- Example: `TestGoClientTypeScriptServerHTTP`

### File Organization
- `*_test.go`: Go test files
- `*.ts`: TypeScript test files  
- `run_*.go`: Test runners and utilities
- `README.md`: Documentation and instructions

## Future Enhancements

### Planned Test Additions
1. **WebSocket Transport**: Full bidirectional real-time communication
2. **Error Injection Tests**: Network failures, malformed packets
3. **Performance Regression Tests**: Automated performance monitoring
4. **Fuzz Testing**: Protocol robustness testing
5. **Security Tests**: Input validation, DoS protection

### Transport Implementations Needed
1. **WebSocket Transport**: For real-time bidirectional communication
2. **gRPC Transport**: For high-performance scenarios
3. **Unix Socket Transport**: For local high-performance communication

The interoperability test suite ensures that the Go MCP SDK maintains 100% compatibility with the TypeScript MCP SDK across all communication patterns and transport types.
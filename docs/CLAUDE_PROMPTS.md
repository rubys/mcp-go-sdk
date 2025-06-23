<p align="center">
  <img src="../logo.svg" alt="Go MCP SDK Logo" width="120" height="104" />
</p>

# Claude Code Prompt Templates for Go MCP SDK

Ready-to-use prompt templates for developing with the Go MCP SDK using Claude Code.

## 📋 Context Setup Prompt

Use this at the start of your Claude Code session:

```markdown
I'm developing with the Go MCP SDK (github.com/rubys/mcp-go-sdk), a high-performance Go implementation of the Model Context Protocol.

Key SDK Features:
- 7x+ faster than TypeScript MCP SDK
- 100% TypeScript SDK compatibility
- Concurrent, goroutine-based architecture
- Supports stdio, SSE, WebSocket, HTTP transports
- Production-ready with comprehensive testing

Documentation:
- API Docs: docs/API.md
- Migration Guide: docs/MIGRATION_GUIDE.md
- Examples: docs/EXAMPLES.md
- Performance Guide: docs/PERFORMANCE_COMPARISON.md

Please help with Go MCP SDK development using best practices for performance and concurrency.
```

## 🚀 Project Creation

### New MCP Server
```markdown
Create a production-ready MCP server using Go MCP SDK with:

Features:
- [List your required tools/resources/prompts]
- [Specify transport type: stdio/WebSocket/HTTP]
- [Authentication requirements: OAuth/basic/none]

Requirements:
- Handle [X] concurrent requests
- Include progress notifications for long operations
- Proper error handling and logging
- Docker deployment ready
- Comprehensive test coverage

Please include:
1. Complete project structure with go.mod
2. Server implementation with concurrent patterns
3. Tool/resource handlers with type safety
4. Configuration management
5. Basic tests and benchmarks
```

### New MCP Client
```markdown
Create an MCP client application using Go MCP SDK that:

Requirements:
- Connects to [specify servers/endpoints]
- Uses [transport type] transport
- Handles [list operations needed]
- Supports [concurrent requests/connection pooling]

Include:
1. Client setup with proper configuration
2. Connection management and retry logic
3. Error handling and logging
4. Example usage patterns
5. Integration tests
```

## 🔄 Migration Projects

### From TypeScript SDK
```markdown
Migrate this TypeScript MCP implementation to Go MCP SDK:

[Paste TypeScript code]

Migration Goals:
- Maintain 100% API compatibility with existing clients
- Achieve 7x+ performance improvement
- Use concurrent patterns appropriately
- Preserve all existing functionality
- Add proper error handling and logging

Please provide:
1. Direct Go translation maintaining compatibility
2. Performance optimizations using goroutines
3. Migration testing strategy
4. Before/after performance comparison
```

### From mark3labs/mcp-go
```markdown
Migrate this mark3labs/mcp-go code to the high-performance Go MCP SDK:

[Paste mark3labs code]

Use the compatibility layer (compat package) to:
- Minimize code changes required
- Maintain fluent builder API patterns
- Gain performance benefits of new SDK
- Add proper transport initialization

Include migration steps and compatibility verification.
```

## 🛠️ Development Tasks

### Add New Tool
```markdown
Add a new tool to my Go MCP SDK server:

Tool Requirements:
- Name: [tool name]
- Description: [what it does]
- Arguments: [list with types and descriptions]
- Operation: [describe the functionality]
- Performance: [concurrent/streaming/long-running]

Current Server Code:
[Paste existing server code]

Please:
1. Add typed tool handler with validation
2. Include progress notifications if long-running
3. Add comprehensive error handling
4. Include unit tests
5. Update tool registration
```

### Add Resource Handler
```markdown
Add resource handling to my Go MCP SDK server:

Resource Requirements:
- URI pattern: [pattern to handle]
- Content type: [text/binary/JSON/etc.]
- Source: [file system/database/API/etc.]
- Caching: [yes/no with TTL]
- Access pattern: [frequent/occasional/bulk]

Please implement:
1. Resource registration and metadata
2. Efficient handler with caching if needed
3. Proper content type handling
4. Error handling for missing resources
5. Performance optimizations for concurrent access
```

## 🔧 Optimization Tasks

### Performance Review
```markdown
Review this Go MCP SDK code for performance optimization:

[Paste code]

Focus on:
1. Goroutine usage and channel patterns
2. Memory allocation and GC pressure
3. Lock contention and synchronization
4. I/O efficiency and buffering
5. Connection pooling and reuse
6. Resource cleanup and leaks

Provide specific improvements with explanations and benchmark suggestions.
```

### Concurrency Audit
```markdown
Audit this Go MCP SDK code for concurrent safety:

[Paste code]

Check for:
1. Race conditions in shared state access
2. Proper mutex/RWMutex usage
3. Channel usage patterns and deadlocks
4. Goroutine lifecycle management
5. Context cancellation handling
6. Resource cleanup races

Provide fixes with proper synchronization patterns.
```

## 🧪 Testing

### Generate Test Suite
```markdown
Create comprehensive tests for this Go MCP SDK code:

[Paste code to test]

Include:
1. Unit tests for each handler function
2. Integration tests with real transport
3. Concurrent load testing with race detection
4. TypeScript SDK compatibility validation
5. Performance benchmarks vs baseline
6. Error scenario and edge case testing
7. Memory leak detection tests

Use Go testing best practices with testify assertions.
```

### Performance Benchmarks
```markdown
Create performance benchmarks for this Go MCP SDK implementation:

[Paste code]

Benchmark:
1. Tool execution throughput (ops/sec)
2. Resource access performance
3. Concurrent client handling
4. Memory usage under load
5. Latency measurements (P50/P95/P99)
6. Comparison with TypeScript SDK equivalent

Include benchmark analysis and optimization suggestions.
```

## 🚨 Debugging

### Troubleshoot Issue
```markdown
Help debug this Go MCP SDK issue:

Problem: [Describe the issue]
Error messages: [Paste error logs]
Expected behavior: [What should happen]
Actual behavior: [What's happening]

Code Context:
[Paste relevant code]

Please:
1. Identify the root cause
2. Provide debugging steps
3. Suggest fixes with explanations
4. Add logging/monitoring to prevent recurrence
5. Include tests to verify the fix
```

### Memory Issues
```markdown
Analyze this Go MCP SDK code for memory efficiency:

[Paste code with memory concerns]

Investigate:
1. Memory leaks and retention patterns
2. Excessive allocations in hot paths
3. Large object lifetime management
4. Buffer reuse opportunities
5. GC pressure and optimization

Suggest improvements with object pooling and efficient patterns.
```

## 🔐 Security Review

### Security Audit
```markdown
Review this Go MCP SDK server for security:

[Paste server code]

Audit for:
1. Input validation and sanitization
2. Authentication and authorization
3. Rate limiting and DoS protection
4. Secure transport configuration
5. Secrets and configuration management
6. Error information disclosure

Provide security improvements and best practices.
```

## 🚀 Production Deployment

### Production Readiness
```markdown
Make this Go MCP SDK application production-ready:

[Paste application code]

Add:
1. Comprehensive logging and monitoring
2. Health checks and readiness probes
3. Graceful shutdown and signal handling
4. Configuration management (env vars/files)
5. Docker containerization
6. Kubernetes deployment manifests
7. Observability (metrics, tracing, alerts)

Include deployment best practices and scaling guidelines.
```

### Docker & Kubernetes
```markdown
Create deployment configuration for this Go MCP SDK application:

[Paste application details]

Requirements:
- Container optimization for Go binaries
- Kubernetes manifests with scaling
- ConfigMaps and Secrets management
- Service mesh compatibility
- Monitoring and logging integration
- Rolling updates and health checks

Include multi-stage Dockerfile and complete K8s configuration.
```

## 📊 Architecture Design

### Design Review
```markdown
Review this Go MCP SDK architecture design:

[Describe or paste architecture]

Evaluate:
1. Scalability and performance characteristics
2. Fault tolerance and error handling
3. Resource utilization and efficiency
4. Maintainability and code organization
5. Security and compliance considerations
6. Monitoring and observability

Suggest improvements with Go SDK best practices.
```

### Scale Planning
```markdown
Design a scalable architecture using Go MCP SDK for:

Requirements:
- [X] concurrent clients
- [Y] requests per second
- [Z] data throughput
- Geographic distribution: [regions]
- Availability requirements: [SLA]

Include:
1. Load balancing strategies
2. Database/cache architecture
3. Transport selection and configuration
4. Monitoring and alerting
5. Deployment and scaling automation
6. Cost optimization recommendations
```

---

## 💡 Pro Tips

1. **Always provide context** about using Go MCP SDK and its performance characteristics
2. **Be specific** about performance requirements and constraints
3. **Request tests** alongside any code generation
4. **Ask for benchmarks** when optimizing performance
5. **Include error handling** in all requests
6. **Validate compatibility** when migrating from other SDKs

Copy and customize these prompts based on your specific needs!
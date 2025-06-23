# Go MCP SDK API Documentation

Comprehensive API reference for the high-performance Go MCP SDK.

## Table of Contents

- [Client Package](#client-package)
- [Server Package](#server-package)
- [Transport Package](#transport-package)
- [Shared Package](#shared-package)
- [Compat Package](#compat-package)
- [Configuration Examples](#configuration-examples)
- [Error Handling](#error-handling)
- [Performance Tuning](#performance-tuning)

## Client Package

The client package provides MCP client implementations with concurrent processing and TypeScript SDK compatibility.

### Types

#### `Client`

Main MCP client struct providing all client operations.

```go
type Client struct {
    // Contains filtered or unexported fields
}
```

#### `ClientConfig`

Configuration for MCP client initialization.

```go
type ClientConfig struct {
    ClientInfo   shared.ClientInfo        `json:"clientInfo"`
    Capabilities shared.ClientCapabilities `json:"capabilities"`
}
```

#### `ClientInfo`

Information about the client implementation.

```go
type ClientInfo struct {
    Name    string `json:"name"`
    Version string `json:"version"`
}
```

### Constructor Functions

#### `New(transport Transport) *Client`

Creates a basic MCP client with the specified transport.

```go
func New(transport Transport) *Client
```

**Parameters:**
- `transport` - Transport implementation (stdio, SSE, WebSocket, etc.)

**Returns:**
- Configured MCP client ready for initialization

**Example:**
```go
transport, _ := transport.NewStdioTransport(ctx, transport.StdioConfig{})
client := client.New(transport)
```

#### `NewClient(ctx context.Context, transport Transport, config ClientConfig) (*Client, error)`

Creates a fully configured MCP client with immediate initialization.

```go
func NewClient(ctx context.Context, transport Transport, config ClientConfig) (*Client, error)
```

**Parameters:**
- `ctx` - Context for initialization and cancellation
- `transport` - Transport implementation
- `config` - Client configuration including capabilities

**Returns:**
- Initialized MCP client or error

**Example:**
```go
client, err := client.NewClient(ctx, transport, client.ClientConfig{
    ClientInfo: shared.ClientInfo{
        Name:    "my-client",
        Version: "1.0.0",
    },
    Capabilities: shared.ClientCapabilities{
        Sampling: &shared.SamplingCapability{},
    },
})
```

### Core Methods

#### `Start(ctx context.Context) error`

Starts the client and underlying transport.

```go
func (c *Client) Start(ctx context.Context) error
```

**Parameters:**
- `ctx` - Context for operation cancellation

**Returns:**
- Error if startup fails

#### `Initialize(ctx context.Context) error`

Initializes the MCP connection with the server.

```go
func (c *Client) Initialize(ctx context.Context) error
```

**Parameters:**
- `ctx` - Context for operation cancellation

**Returns:**
- Error if initialization fails

#### `Close() error`

Closes the client connection and cleans up resources.

```go
func (c *Client) Close() error
```

**Returns:**
- Error if cleanup fails

### MCP Operations

#### `ListResources(ctx context.Context) (*shared.ListResourcesResult, error)`

Lists all available resources from the server.

```go
func (c *Client) ListResources(ctx context.Context) (*shared.ListResourcesResult, error)
```

**Parameters:**
- `ctx` - Context for operation cancellation

**Returns:**
- List of available resources or error

**Example:**
```go
resources, err := client.ListResources(ctx)
if err != nil {
    return err
}
fmt.Printf("Found %d resources\n", len(resources.Resources))
```

#### `ReadResource(ctx context.Context, uri string) (*shared.ReadResourceResult, error)`

Reads content from a specific resource.

```go
func (c *Client) ReadResource(ctx context.Context, uri string) (*shared.ReadResourceResult, error)
```

**Parameters:**
- `ctx` - Context for operation cancellation
- `uri` - Resource URI to read

**Returns:**
- Resource content or error

**Example:**
```go
content, err := client.ReadResource(ctx, "file://example.txt")
if err != nil {
    return err
}
for _, item := range content.Contents {
    fmt.Printf("Content: %s\n", item.(shared.TextContent).Text)
}
```

#### `ListTools(ctx context.Context) (*shared.ListToolsResult, error)`

Lists all available tools from the server.

```go
func (c *Client) ListTools(ctx context.Context) (*shared.ListToolsResult, error)
```

#### `CallTool(ctx context.Context, request shared.CallToolRequest) (*shared.CallToolResult, error)`

Executes a tool with the provided arguments.

```go
func (c *Client) CallTool(ctx context.Context, request shared.CallToolRequest) (*shared.CallToolResult, error)
```

**Parameters:**
- `ctx` - Context for operation cancellation
- `request` - Tool execution request

**Returns:**
- Tool execution result or error

**Example:**
```go
result, err := client.CallTool(ctx, shared.CallToolRequest{
    Name: "calculator",
    Arguments: map[string]interface{}{
        "operation": "add",
        "a": 5.0,
        "b": 3.0,
    },
})
```

#### `ListPrompts(ctx context.Context) (*shared.ListPromptsResult, error)`

Lists all available prompts from the server.

```go
func (c *Client) ListPrompts(ctx context.Context) (*shared.ListPromptsResult, error)
```

#### `GetPrompt(ctx context.Context, request shared.GetPromptRequest) (*shared.GetPromptResult, error)`

Generates a prompt with the provided arguments.

```go
func (c *Client) GetPrompt(ctx context.Context, request shared.GetPromptRequest) (*shared.GetPromptResult, error)
```

### OAuth Support

#### `OAuthConfig`

Configuration for OAuth 2.0 authentication.

```go
type OAuthConfig struct {
    ClientID     string `json:"client_id"`
    ClientSecret string `json:"client_secret,omitempty"`
    AuthURL      string `json:"auth_url"`
    TokenURL     string `json:"token_url"`
    RedirectURI  string `json:"redirect_uri"`
    Scopes       []string `json:"scopes,omitempty"`
    UsePKCE      bool   `json:"use_pkce"`
}
```

#### `NewOAuthStreamableHttpClient(url string, oauthConfig OAuthConfig) (*Client, error)`

Creates an OAuth-enabled HTTP client.

```go
func NewOAuthStreamableHttpClient(url string, oauthConfig OAuthConfig) (*Client, error)
```

## Server Package

The server package provides MCP server implementations with concurrent request processing and dynamic registration.

### Types

#### `Server`

Main MCP server struct handling all server operations.

```go
type Server struct {
    // Contains filtered or unexported fields
}
```

#### `ServerConfig`

Configuration for MCP server initialization.

```go
type ServerConfig struct {
    ServerInfo            shared.Implementation      `json:"serverInfo"`
    Capabilities          shared.ServerCapabilities  `json:"capabilities"`
    MaxConcurrentRequests int                        `json:"maxConcurrentRequests,omitempty"`
    RequestTimeout        time.Duration              `json:"requestTimeout,omitempty"`
}
```

### Handler Types

#### `ResourceHandler`

Handler function for resource requests.

```go
type ResourceHandler func(ctx context.Context, uri string) ([]shared.Content, error)
```

#### `ToolHandler`

Handler function for tool execution requests.

```go
type ToolHandler func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error)
```

#### `PromptHandler`

Handler function for prompt generation requests.

```go
type PromptHandler func(ctx context.Context, name string, arguments map[string]interface{}) (PromptMessage, error)
```

### Constructor Functions

#### `NewServer(ctx context.Context, transport Transport, config ServerConfig) *Server`

Creates a new MCP server with the specified configuration.

```go
func NewServer(ctx context.Context, transport Transport, config ServerConfig) *Server
```

**Parameters:**
- `ctx` - Context for server lifecycle
- `transport` - Transport implementation
- `config` - Server configuration

**Returns:**
- Configured MCP server

**Example:**
```go
srv := server.NewServer(ctx, transport, server.ServerConfig{
    ServerInfo: shared.Implementation{
        Name:    "my-server",
        Version: "1.0.0",
    },
    Capabilities: shared.ServerCapabilities{
        Tools:     &shared.ToolsCapability{},
        Resources: &shared.ResourcesCapability{},
    },
    MaxConcurrentRequests: 100,
    RequestTimeout:        30 * time.Second,
})
```

### Server Management

#### `Serve(ctx context.Context) error`

Starts the server and begins processing requests.

```go
func (s *Server) Serve(ctx context.Context) error
```

**Parameters:**
- `ctx` - Context for server lifecycle

**Returns:**
- Error if server fails to start or during operation

#### `Close() error`

Gracefully shuts down the server.

```go
func (s *Server) Close() error
```

### Registration Methods

#### `RegisterResource(resource shared.Resource)`

Registers a resource with metadata.

```go
func (s *Server) RegisterResource(resource shared.Resource)
```

**Parameters:**
- `resource` - Resource metadata

**Example:**
```go
srv.RegisterResource(shared.Resource{
    URI:         "file://data.txt",
    Name:        "Data File",
    Description: "Sample data file",
    MimeType:    "text/plain",
})
```

#### `SetResourceHandler(pattern string, handler ResourceHandler)`

Sets a handler for resources matching the pattern.

```go
func (s *Server) SetResourceHandler(pattern string, handler ResourceHandler)
```

**Parameters:**
- `pattern` - URI pattern to match (exact match or prefix)
- `handler` - Handler function

**Example:**
```go
srv.SetResourceHandler("file://", func(ctx context.Context, uri string) ([]shared.Content, error) {
    // Handle all file:// URIs
    content := fmt.Sprintf("Content for %s", uri)
    return []shared.Content{
        shared.TextContent{
            Type: shared.ContentTypeText,
            Text: content,
            URI:  uri,
        },
    }, nil
})
```

#### `RegisterTool(tool shared.Tool)`

Registers a tool with metadata.

```go
func (s *Server) RegisterTool(tool shared.Tool)
```

#### `SetToolHandler(name string, handler ToolHandler)`

Sets a handler for a specific tool.

```go
func (s *Server) SetToolHandler(name string, handler ToolHandler)
```

**Example:**
```go
srv.SetToolHandler("calculator", func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
    a := arguments["a"].(float64)
    b := arguments["b"].(float64)
    result := a + b
    
    return []shared.Content{
        shared.TextContent{
            Type: shared.ContentTypeText,
            Text: fmt.Sprintf("Result: %.2f", result),
        },
    }, nil
})
```

#### `RegisterPrompt(prompt shared.Prompt)`

Registers a prompt with metadata.

```go
func (s *Server) RegisterPrompt(prompt shared.Prompt)
```

#### `SetPromptHandler(name string, handler PromptHandler)`

Sets a handler for a specific prompt.

```go
func (s *Server) SetPromptHandler(name string, handler PromptHandler)
```

## Transport Package

The transport package provides various transport implementations for MCP communication.

### Core Interfaces

#### `Transport`

Universal transport interface for both client and server usage.

```go
type Transport interface {
    SendRequest(method string, params interface{}) (<-chan *shared.Response, error)
    SendNotification(method string, params interface{}) error
    SendResponse(id interface{}, result interface{}, err *shared.RPCError) error
    Channels() (<-chan *shared.Request, <-chan *shared.Notification)
    Close() error
    Stats() interface{}
}
```

### Stdio Transport

#### `StdioTransport`

Concurrent stdio transport with separate read/write goroutines.

```go
type StdioTransport struct {
    // Contains filtered or unexported fields
}
```

#### `StdioConfig`

Configuration for stdio transport.

```go
type StdioConfig struct {
    ReadBufferSize   int           `json:"readBufferSize,omitempty"`
    WriteBufferSize  int           `json:"writeBufferSize,omitempty"`
    RequestTimeout   time.Duration `json:"requestTimeout,omitempty"`
    MessageBuffer    int           `json:"messageBuffer,omitempty"`
}
```

#### `NewStdioTransport(ctx context.Context, config StdioConfig) (*StdioTransport, error)`

Creates a new stdio transport.

```go
func NewStdioTransport(ctx context.Context, config StdioConfig) (*StdioTransport, error)
```

**Example:**
```go
transport, err := transport.NewStdioTransport(ctx, transport.StdioConfig{
    ReadBufferSize:  64 * 1024,
    WriteBufferSize: 64 * 1024,
    RequestTimeout:  30 * time.Second,
    MessageBuffer:   100,
})
```

### SSE Transport

#### `SSETransport`

Server-Sent Events transport with dual endpoints.

```go
type SSETransport struct {
    // Contains filtered or unexported fields
}
```

#### `SSEConfig`

Configuration for SSE transport.

```go
type SSEConfig struct {
    Endpoint        string                 `json:"endpoint"`
    Writer          http.ResponseWriter    `json:"-"`
    Request         *http.Request          `json:"-"`
    Headers         map[string]string      `json:"headers,omitempty"`
    RequestTimeout  time.Duration          `json:"requestTimeout,omitempty"`
}
```

#### `NewSSETransport(config SSEConfig) (*SSETransport, error)`

Creates a new SSE transport.

```go
func NewSSETransport(config SSEConfig) (*SSETransport, error)
```

### WebSocket Transport

#### `WebSocketTransport`

WebSocket transport with reconnection support.

```go
type WebSocketTransport struct {
    // Contains filtered or unexported fields
}
```

#### `WebSocketConfig`

Configuration for WebSocket transport.

```go
type WebSocketConfig struct {
    URL                  string        `json:"url"`
    ReconnectInterval    time.Duration `json:"reconnectInterval,omitempty"`
    MaxReconnectAttempts int           `json:"maxReconnectAttempts,omitempty"`
    PingInterval         time.Duration `json:"pingInterval,omitempty"`
    RequestTimeout       time.Duration `json:"requestTimeout,omitempty"`
    Headers              http.Header   `json:"headers,omitempty"`
}
```

#### `NewWebSocketTransport(ctx context.Context, config WebSocketConfig) (*WebSocketTransport, error)`

Creates a new WebSocket transport.

```go
func NewWebSocketTransport(ctx context.Context, config WebSocketConfig) (*WebSocketTransport, error)
```

### HTTP Transport

#### `StreamableHTTPTransport`

HTTP transport with optional Server-Sent Events.

```go
type StreamableHTTPTransport struct {
    // Contains filtered or unexported fields
}
```

#### `StreamableHTTPConfig`

Configuration for HTTP transport.

```go
type StreamableHTTPConfig struct {
    URL            string            `json:"url"`
    Headers        map[string]string `json:"headers,omitempty"`
    RequestTimeout time.Duration     `json:"requestTimeout,omitempty"`
    UseSSE         bool             `json:"useSSE,omitempty"`
}
```

## Shared Package

The shared package contains common types, constants, and utilities used across the SDK.

### Protocol Constants

```go
const (
    LATEST_PROTOCOL_VERSION = "2024-11-05"
    ProtocolVersion        = LATEST_PROTOCOL_VERSION
)

var SUPPORTED_PROTOCOL_VERSIONS = []string{
    "2024-11-05",
    "2024-10-07",
}
```

### Message Types

#### `Request`

JSON-RPC request structure.

```go
type Request struct {
    JSONRPC string      `json:"jsonrpc"`
    ID      interface{} `json:"id"`
    Method  string      `json:"method"`
    Params  interface{} `json:"params,omitempty"`
}
```

#### `Response`

JSON-RPC response structure.

```go
type Response struct {
    JSONRPC string      `json:"jsonrpc"`
    ID      interface{} `json:"id"`
    Result  interface{} `json:"result,omitempty"`
    Error   *RPCError   `json:"error,omitempty"`
}
```

#### `Notification`

JSON-RPC notification structure.

```go
type Notification struct {
    JSONRPC string      `json:"jsonrpc"`
    Method  string      `json:"method"`
    Params  interface{} `json:"params,omitempty"`
}
```

### Content Types

#### `Content`

Interface for all content types.

```go
type Content interface {
    GetType() ContentType
}
```

#### `TextContent`

Text content implementation.

```go
type TextContent struct {
    Type        ContentType `json:"type"`
    Text        string      `json:"text"`
    URI         string      `json:"uri,omitempty"`
    Annotations *Annotations `json:"annotations,omitempty"`
}
```

#### `ImageContent`

Image content implementation.

```go
type ImageContent struct {
    Type        ContentType `json:"type"`
    Data        string      `json:"data"`
    MimeType    string      `json:"mimeType"`
    URI         string      `json:"uri,omitempty"`
    Annotations *Annotations `json:"annotations,omitempty"`
}
```

### MCP Types

#### `Resource`

Resource metadata structure.

```go
type Resource struct {
    URI         string      `json:"uri"`
    Name        string      `json:"name"`
    Description string      `json:"description,omitempty"`
    MimeType    string      `json:"mimeType,omitempty"`
    Annotations *Annotations `json:"annotations,omitempty"`
}
```

#### `Tool`

Tool metadata structure.

```go
type Tool struct {
    Name        string                 `json:"name"`
    Description string                 `json:"description,omitempty"`
    InputSchema map[string]interface{} `json:"inputSchema"`
}
```

#### `Prompt`

Prompt metadata structure.

```go
type Prompt struct {
    Name        string           `json:"name"`
    Description string           `json:"description,omitempty"`
    Arguments   []PromptArgument `json:"arguments,omitempty"`
}
```

### Capabilities

#### `ServerCapabilities`

Server capability declarations.

```go
type ServerCapabilities struct {
    Experimental map[string]interface{} `json:"experimental,omitempty"`
    Logging      *LoggingCapability     `json:"logging,omitempty"`
    Prompts      *PromptsCapability     `json:"prompts,omitempty"`
    Resources    *ResourcesCapability   `json:"resources,omitempty"`
    Tools        *ToolsCapability       `json:"tools,omitempty"`
}
```

#### `ClientCapabilities`

Client capability declarations.

```go
type ClientCapabilities struct {
    Experimental map[string]interface{} `json:"experimental,omitempty"`
    Sampling     *SamplingCapability    `json:"sampling,omitempty"`
    Roots        *RootsCapability       `json:"roots,omitempty"`
}
```

## Compat Package

The compat package provides compatibility with mark3labs/mcp-go for easy migration.

### Types

#### `MCPServer`

Compatible MCP server with fluent API.

```go
type MCPServer struct {
    // Contains filtered or unexported fields
}
```

### Constructor Functions

#### `NewMCPServer(name, version string, options ...ServerOption) *MCPServer`

Creates a new compatible MCP server.

```go
func NewMCPServer(name, version string, options ...ServerOption) *MCPServer
```

**Example:**
```go
server := compat.NewMCPServer("my-server", "1.0.0",
    compat.WithLogging(),
    compat.WithConcurrency(50, 30*time.Second),
)
```

### Server Options

#### `WithLogging() ServerOption`

Enables logging capability.

```go
func WithLogging() ServerOption
```

#### `WithConcurrency(maxRequests int, timeout time.Duration) ServerOption`

Configures concurrency settings.

```go
func WithConcurrency(maxRequests int, timeout time.Duration) ServerOption
```

### Transport Creation

#### `CreateWithStdio(ctx context.Context) error`

Creates stdio transport for the server.

```go
func (s *MCPServer) CreateWithStdio(ctx context.Context) error
```

#### `CreateWithSSE(ctx context.Context, sseEndpoint, httpEndpoint string) error`

Creates SSE transport for the server.

```go
func (s *MCPServer) CreateWithSSE(ctx context.Context, sseEndpoint, httpEndpoint string) error
```

### Builder Patterns

#### `NewTool(name string, options ...ToolOption) *Tool`

Creates a new tool with fluent API.

```go
func NewTool(name string, options ...ToolOption) *Tool
```

**Example:**
```go
tool := compat.NewTool("calculator",
    compat.WithDescription("Simple calculator"),
    compat.WithString("operation", compat.Required(), compat.Description("Math operation")),
    compat.WithNumber("a", compat.Required()),
    compat.WithNumber("b", compat.Required()),
)
```

## Configuration Examples

### Basic Server Setup

```go
package main

import (
    "context"
    "github.com/rubys/mcp-go-sdk/server"
    "github.com/rubys/mcp-go-sdk/shared"
    "github.com/rubys/mcp-go-sdk/transport"
)

func main() {
    ctx := context.Background()
    
    // Create transport
    transport, _ := transport.NewStdioTransport(ctx, transport.StdioConfig{
        RequestTimeout: 30 * time.Second,
    })
    
    // Create server
    srv := server.NewServer(ctx, transport, server.ServerConfig{
        ServerInfo: shared.Implementation{
            Name:    "example-server",
            Version: "1.0.0",
        },
        Capabilities: shared.ServerCapabilities{
            Tools: &shared.ToolsCapability{},
        },
        MaxConcurrentRequests: 100,
    })
    
    // Start server
    srv.Serve(ctx)
}
```

### High-Performance Configuration

```go
// Optimized for high throughput
transport, _ := transport.NewStdioTransport(ctx, transport.StdioConfig{
    ReadBufferSize:  128 * 1024,  // Large read buffer
    WriteBufferSize: 128 * 1024,  // Large write buffer
    MessageBuffer:   1000,        // High message buffer
    RequestTimeout:  10 * time.Second,
})

srv := server.NewServer(ctx, transport, server.ServerConfig{
    MaxConcurrentRequests: 500,   // High concurrency
    RequestTimeout:        5 * time.Second,
    // ... other config
})
```

## Error Handling

### Common Error Types

```go
// JSON-RPC errors
type RPCError struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
}

// OAuth errors
func IsOAuthAuthorizationRequiredError(err error) bool

// Validation errors
type ValidationError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
}
```

### Error Handling Patterns

```go
// Tool handler with proper error handling
srv.SetToolHandler("risky-tool", func(ctx context.Context, name string, args map[string]interface{}) ([]shared.Content, error) {
    if err := validateArgs(args); err != nil {
        return nil, &shared.RPCError{
            Code:    shared.ErrorCodeInvalidParams,
            Message: "Invalid arguments",
            Data:    err,
        }
    }
    
    result, err := performOperation(ctx, args)
    if err != nil {
        return nil, &shared.RPCError{
            Code:    shared.ErrorCodeInternalError,
            Message: "Operation failed",
        }
    }
    
    return result, nil
})
```

## Performance Tuning

### Concurrency Configuration

```go
// Balance based on workload
config := server.ServerConfig{
    MaxConcurrentRequests: runtime.NumCPU() * 10,  // CPU-bound: NumCPU
    MaxConcurrentRequests: 1000,                   // I/O-bound: Higher value
    RequestTimeout:        30 * time.Second,       // Generous for complex operations
}
```

### Memory Optimization

```go
// Use object pooling for frequent allocations
var contentPool = sync.Pool{
    New: func() interface{} {
        return &shared.TextContent{}
    },
}

func reuseContent() *shared.TextContent {
    content := contentPool.Get().(*shared.TextContent)
    // Reset content
    *content = shared.TextContent{}
    return content
}

func recycleContent(content *shared.TextContent) {
    contentPool.Put(content)
}
```

### Transport Optimization

```go
// Stdio: Large buffers for high throughput
transport.StdioConfig{
    ReadBufferSize:  256 * 1024,
    WriteBufferSize: 256 * 1024,
    MessageBuffer:   2000,
}

// WebSocket: Tuned for real-time
transport.WebSocketConfig{
    PingInterval:      15 * time.Second,
    ReconnectInterval: 5 * time.Second,
    RequestTimeout:    10 * time.Second,
}

// HTTP: Connection pooling
transport.StreamableHTTPConfig{
    RequestTimeout: 30 * time.Second,
    UseSSE:        true,  // For real-time notifications
}
```

---

This API documentation covers all public interfaces of the Go MCP SDK. For additional examples and migration guides, see the [Examples](EXAMPLES.md) and [Migration Guide](MIGRATION_GUIDE.md) documentation.
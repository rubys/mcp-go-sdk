package compat

import (
	"context"
	"fmt"
	"time"

	"github.com/rubys/mcp-go-sdk/server"
	"github.com/rubys/mcp-go-sdk/shared"
	"github.com/rubys/mcp-go-sdk/transport"
)

// MCPServer provides a mark3labs/mcp-go compatible API
type MCPServer struct {
	underlying *server.Server
	config     ServerConfig
	transport  transport.Transport
	ctx        context.Context
}

// ServerConfig holds server configuration
type ServerConfig struct {
	name                  string
	version               string
	capabilities          shared.ServerCapabilities
	maxConcurrentRequests int
	requestTimeout        time.Duration
	withLogging           bool
	hooks                 *ServerHooks
}

// ServerHooks provides callback hooks (placeholder for compatibility)
type ServerHooks struct {
	// Add hooks as needed for compatibility
}

// ServerOption configures the MCP server
type ServerOption func(*ServerConfig)

// WithResourceCapabilities enables resource capabilities
func WithResourceCapabilities(subscribe, listChanged bool) ServerOption {
	return func(c *ServerConfig) {
		c.capabilities.Resources = &shared.ResourcesCapability{
			Subscribe:   subscribe,
			ListChanged: listChanged,
		}
	}
}

// WithToolCapabilities enables tool capabilities
func WithToolCapabilities(listChanged bool) ServerOption {
	return func(c *ServerConfig) {
		c.capabilities.Tools = &shared.ToolsCapability{
			ListChanged: listChanged,
		}
	}
}

// WithPromptCapabilities enables prompt capabilities
func WithPromptCapabilities(listChanged bool) ServerOption {
	return func(c *ServerConfig) {
		c.capabilities.Prompts = &shared.PromptsCapability{
			ListChanged: listChanged,
		}
	}
}

// WithLogging enables logging (placeholder for compatibility)
func WithLogging() ServerOption {
	return func(c *ServerConfig) {
		c.withLogging = true
	}
}

// WithHooks sets server hooks (placeholder for compatibility)
func WithHooks(hooks *ServerHooks) ServerOption {
	return func(c *ServerConfig) {
		c.hooks = hooks
	}
}

// WithConcurrency sets concurrency limits
func WithConcurrency(maxRequests int, timeout time.Duration) ServerOption {
	return func(c *ServerConfig) {
		c.maxConcurrentRequests = maxRequests
		c.requestTimeout = timeout
	}
}

// NewMCPServer creates a new MCP server with mark3labs-compatible API
func NewMCPServer(name, version string, options ...ServerOption) *MCPServer {
	config := ServerConfig{
		name:                  name,
		version:               version,
		maxConcurrentRequests: 100,
		requestTimeout:        30 * time.Second,
	}
	
	for _, opt := range options {
		opt(&config)
	}
	
	return &MCPServer{
		config: config,
	}
}

// SetTransport sets the transport for the server
func (s *MCPServer) SetTransport(ctx context.Context, transport transport.Transport) error {
	s.ctx = ctx
	s.transport = transport
	
	// Create the underlying server
	serverConfig := server.ServerConfig{
		Name:                  s.config.name,
		Version:               s.config.version,
		MaxConcurrentRequests: s.config.maxConcurrentRequests,
		RequestTimeout:        s.config.requestTimeout,
		Capabilities:          s.config.capabilities,
	}
	
	s.underlying = server.NewServer(ctx, transport, serverConfig)
	return nil
}

// CreateWithStdio creates a server with stdio transport (convenience method)
func (s *MCPServer) CreateWithStdio(ctx context.Context) error {
	stdioTransport, err := transport.NewStdioTransport(ctx, transport.StdioConfig{
		RequestTimeout: s.config.requestTimeout,
		MessageBuffer:  100,
	})
	if err != nil {
		return fmt.Errorf("failed to create stdio transport: %w", err)
	}
	
	return s.SetTransport(ctx, stdioTransport)
}

// CreateWithSSE creates a server with SSE transport (dual endpoints)
func (s *MCPServer) CreateWithSSE(ctx context.Context, sseEndpoint, httpEndpoint string) error {
	sseTransport, err := transport.NewSSETransport(ctx, transport.SSEConfig{
		SSEEndpoint:    sseEndpoint,   // SSE endpoint for receiving messages
		HTTPEndpoint:   httpEndpoint,  // HTTP POST endpoint for sending messages
		RequestTimeout: s.config.requestTimeout,
		MessageBuffer:  100,
		MaxReconnects:  10,
	})
	if err != nil {
		return fmt.Errorf("failed to create SSE transport: %w", err)
	}
	
	return s.SetTransport(ctx, sseTransport)
}

// CreateWithStreamableHTTP creates a server with streamable HTTP transport (single endpoint)
func (s *MCPServer) CreateWithStreamableHTTP(ctx context.Context, endpoint string) error {
	streamTransport, err := transport.NewStreamableHTTPTransport(ctx, transport.StreamableHTTPConfig{
		ClientEndpoint:        endpoint,  // Single endpoint for streamable HTTP
		RequestTimeout:        s.config.requestTimeout,
		MaxConcurrentRequests: s.config.maxConcurrentRequests,
		ConnectionPoolSize:    20,
		MessageBuffer:         100,
	})
	if err != nil {
		return fmt.Errorf("failed to create streamable HTTP transport: %w", err)
	}
	
	return s.SetTransport(ctx, streamTransport)
}

// ResourceHandlerFunc matches mark3labs signature
type ResourceHandlerFunc func(ctx context.Context, request ResourceRequest) (ResourceResponse, error)

// ResourceRequest represents a resource request
type ResourceRequest struct {
	URI string
}

// ResourceResponse represents a resource response
type ResourceResponse struct {
	Contents []shared.Content
}

// ToolHandlerFunc matches mark3labs signature
type ToolHandlerFunc func(ctx context.Context, request ToolRequest) (ToolResponse, error)

// ToolRequest represents a tool request
type ToolRequest struct {
	Name      string
	Arguments map[string]interface{}
}

// ToolResponse represents a tool response
type ToolResponse struct {
	Content []shared.Content
}

// PromptHandlerFunc matches mark3labs signature
type PromptHandlerFunc func(ctx context.Context, request PromptRequest) (PromptResponse, error)

// PromptRequest represents a prompt request
type PromptRequest struct {
	Name      string
	Arguments map[string]interface{}
}

// PromptResponse represents a prompt response
type PromptResponse struct {
	Messages []server.PromptMessage
}

// AddResource adds a resource with mark3labs-compatible API
func (s *MCPServer) AddResource(resource *Resource, handler ResourceHandlerFunc) {
	if s.underlying == nil {
		panic("Server not initialized. Call SetTransport or CreateWithStdio first.")
	}
	
	// Convert mark3labs handler to native handler
	nativeHandler := func(ctx context.Context, uri string) ([]shared.Content, error) {
		req := ResourceRequest{URI: uri}
		resp, err := handler(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp.Contents, nil
	}
	
	s.underlying.RegisterResource(
		resource.uri,
		resource.name,
		resource.description,
		nativeHandler,
	)
}

// AddTool adds a tool with mark3labs-compatible API
func (s *MCPServer) AddTool(tool *Tool, handler ToolHandlerFunc) {
	if s.underlying == nil {
		panic("Server not initialized. Call SetTransport or CreateWithStdio first.")
	}
	
	// Convert mark3labs handler to native handler
	nativeHandler := func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		req := ToolRequest{Name: name, Arguments: arguments}
		resp, err := handler(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp.Content, nil
	}
	
	s.underlying.RegisterTool(
		tool.name,
		tool.description,
		tool.inputSchema,
		nativeHandler,
	)
}

// AddPrompt adds a prompt with mark3labs-compatible API
func (s *MCPServer) AddPrompt(prompt *Prompt, handler PromptHandlerFunc) {
	if s.underlying == nil {
		panic("Server not initialized. Call SetTransport or CreateWithStdio first.")
	}
	
	// Convert mark3labs handler to native handler
	nativeHandler := func(ctx context.Context, name string, arguments map[string]interface{}) (server.PromptMessage, error) {
		req := PromptRequest{Name: name, Arguments: arguments}
		resp, err := handler(ctx, req)
		if err != nil {
			return server.PromptMessage{}, err
		}
		if len(resp.Messages) == 0 {
			return server.PromptMessage{}, fmt.Errorf("no messages in prompt response")
		}
		return resp.Messages[0], nil
	}
	
	s.underlying.RegisterPrompt(
		prompt.name,
		prompt.description,
		prompt.arguments,
		nativeHandler,
	)
}

// Start starts the server
func (s *MCPServer) Start() error {
	if s.underlying == nil {
		return fmt.Errorf("server not initialized. Call SetTransport or CreateWithStdio first")
	}
	return s.underlying.Start()
}

// Close closes the server
func (s *MCPServer) Close() error {
	if s.underlying == nil {
		return nil
	}
	return s.underlying.Close()
}

// GetUnderlying returns the underlying high-performance server
// This allows access to native features while maintaining compatibility
func (s *MCPServer) GetUnderlying() *server.Server {
	return s.underlying
}

// SendProgressNotification sends a progress notification to the client
func (s *MCPServer) SendProgressNotification(notification shared.ProgressNotification) error {
	if s.underlying == nil {
		return fmt.Errorf("server not initialized")
	}
	
	// Send notification via the underlying server
	return s.underlying.SendNotification(notification.Method, notification.Params)
}
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/shared"
)

// HandlerFunc represents a generic handler function
type HandlerFunc func(ctx context.Context, params interface{}) (interface{}, error)

// ResourceHandler handles resource requests
type ResourceHandler func(ctx context.Context, uri string) ([]shared.Content, error)

// ToolHandler handles tool execution
type ToolHandler func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error)

// PromptHandler handles prompt requests
type PromptHandler func(ctx context.Context, name string, arguments map[string]interface{}) (PromptMessage, error)

// Server implements a concurrent MCP server
type Server struct {
	// Transport layer
	transport Transport

	// Server info and capabilities
	info         shared.Implementation
	capabilities shared.ServerCapabilities

	// Concurrent handler registries
	resourceHandlers map[string]ResourceHandler
	resourceMeta     map[string]shared.Resource
	toolHandlers     map[string]ToolHandler
	toolMeta         map[string]shared.Tool
	promptHandlers   map[string]PromptHandler
	promptMeta       map[string]shared.Prompt
	requestHandlers  map[string]HandlerFunc

	// Thread-safe access
	resourceMu sync.RWMutex
	toolMu     sync.RWMutex
	promptMu   sync.RWMutex
	handlerMu  sync.RWMutex

	// Context and lifecycle
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once

	// Configuration
	maxConcurrentRequests int
	requestTimeout        time.Duration

	// Worker pool for request processing
	requestChan chan *requestWork
	workerPool  chan struct{}

	// Statistics
	requestsProcessed int64
	requestsActive    int64
	errors            int64
}

// Transport interface for server communication
type Transport interface {
	Channels() (<-chan *shared.Request, <-chan *shared.Notification)
	SendResponse(id interface{}, result interface{}, err *shared.RPCError) error
	SendNotification(method string, params interface{}) error
	Close() error
}

// ServerConfig configures the MCP server
type ServerConfig struct {
	Name                  string
	Version               string
	MaxConcurrentRequests int
	RequestTimeout        time.Duration
	Capabilities          shared.ServerCapabilities
}

// requestWork represents work to be processed by the worker pool
type requestWork struct {
	request *shared.Request
	ctx     context.Context
}

// PromptMessage represents a prompt response message
type PromptMessage struct {
	Role    string           `json:"role"`
	Content []shared.Content `json:"content"`
}

// MarshalJSON implements custom JSON marshaling for PromptMessage
// to ensure compatibility with TypeScript SDK expectations.
// When there's only one content item, it serializes as a single object.
// When there are multiple content items, it serializes as an array.
func (pm PromptMessage) MarshalJSON() ([]byte, error) {
	type Alias PromptMessage

	// Create a temporary struct for JSON marshaling
	temp := struct {
		Role    string      `json:"role"`
		Content interface{} `json:"content"`
	}{
		Role: pm.Role,
	}

	// If there's exactly one content item, serialize as single object
	// This matches TypeScript SDK expectations
	if len(pm.Content) == 1 {
		temp.Content = pm.Content[0]
	} else {
		// Multiple items or empty - serialize as array
		temp.Content = pm.Content
	}

	return json.Marshal(temp)
}

// NewServer creates a new concurrent MCP server
func NewServer(ctx context.Context, transport Transport, config ServerConfig) *Server {
	if config.MaxConcurrentRequests == 0 {
		config.MaxConcurrentRequests = 100
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 30 * time.Second
	}

	ctx, cancel := context.WithCancel(ctx)

	server := &Server{
		transport: transport,
		info: shared.Implementation{
			Name:    config.Name,
			Version: config.Version,
		},
		capabilities:          config.Capabilities,
		resourceHandlers:      make(map[string]ResourceHandler),
		resourceMeta:          make(map[string]shared.Resource),
		toolHandlers:          make(map[string]ToolHandler),
		toolMeta:              make(map[string]shared.Tool),
		promptHandlers:        make(map[string]PromptHandler),
		promptMeta:            make(map[string]shared.Prompt),
		requestHandlers:       make(map[string]HandlerFunc),
		ctx:                   ctx,
		cancel:                cancel,
		maxConcurrentRequests: config.MaxConcurrentRequests,
		requestTimeout:        config.RequestTimeout,
		requestChan:           make(chan *requestWork, config.MaxConcurrentRequests),
		workerPool:            make(chan struct{}, config.MaxConcurrentRequests),
	}

	// Initialize worker pool
	for i := 0; i < config.MaxConcurrentRequests; i++ {
		server.workerPool <- struct{}{}
	}

	// Register built-in handlers
	server.registerBuiltinHandlers()

	return server
}

// Start starts the server and begins processing requests
func (s *Server) Start() error {
	// Start worker goroutines
	s.wg.Add(2)
	go s.processRequests()
	go s.processNotifications()

	// Start request dispatcher
	s.wg.Add(1)
	go s.dispatchRequests()

	return nil
}

// RegisterResource registers a resource handler
func (s *Server) RegisterResource(uri, name, description string, handler ResourceHandler) {
	s.resourceMu.Lock()
	defer s.resourceMu.Unlock()
	s.resourceHandlers[uri] = handler
	s.resourceMeta[uri] = shared.Resource{
		URI:         uri,
		Name:        name,
		Description: description,
	}
}

// RegisterTool registers a tool handler
func (s *Server) RegisterTool(name, description string, inputSchema map[string]interface{}, handler ToolHandler) {
	s.toolMu.Lock()
	defer s.toolMu.Unlock()
	s.toolHandlers[name] = handler
	s.toolMeta[name] = shared.Tool{
		Name:        name,
		Description: description,
		InputSchema: inputSchema,
	}
}

// RegisterPrompt registers a prompt handler
func (s *Server) RegisterPrompt(name, description string, arguments []shared.PromptArgument, handler PromptHandler) {
	s.promptMu.Lock()
	defer s.promptMu.Unlock()
	s.promptHandlers[name] = handler
	s.promptMeta[name] = shared.Prompt{
		Name:        name,
		Description: description,
		Arguments:   arguments,
	}
}

// RegisterHandler registers a custom request handler
func (s *Server) RegisterHandler(method string, handler HandlerFunc) {
	s.handlerMu.Lock()
	defer s.handlerMu.Unlock()
	s.requestHandlers[method] = handler
}

// processRequests processes incoming requests concurrently
func (s *Server) processRequests() {
	defer s.wg.Done()

	requests, _ := s.transport.Channels()

	for {
		select {
		case request := <-requests:
			if request != nil {
				// Create context with timeout for this request
				reqCtx, cancel := context.WithTimeout(s.ctx, s.requestTimeout)

				work := &requestWork{
					request: request,
					ctx:     reqCtx,
				}

				select {
				case s.requestChan <- work:
					// Request queued for processing
				case <-s.ctx.Done():
					cancel()
					return
				default:
					// Request queue full, send error response
					s.transport.SendResponse(request.ID, nil, &shared.RPCError{
						Code:    -32000,
						Message: "Server overloaded",
					})
					cancel()
				}
			}
		case <-s.ctx.Done():
			return
		}
	}
}

// processNotifications processes incoming notifications
func (s *Server) processNotifications() {
	defer s.wg.Done()

	_, notifications := s.transport.Channels()

	for {
		select {
		case notification := <-notifications:
			if notification != nil {
				go s.handleNotification(notification)
			}
		case <-s.ctx.Done():
			return
		}
	}
}

// dispatchRequests dispatches requests to worker goroutines
func (s *Server) dispatchRequests() {
	defer s.wg.Done()

	for {
		select {
		case work := <-s.requestChan:
			// Acquire worker from pool
			select {
			case <-s.workerPool:
				go s.handleRequest(work)
			case <-s.ctx.Done():
				return
			}
		case <-s.ctx.Done():
			return
		}
	}
}

// handleRequest processes a single request
func (s *Server) handleRequest(work *requestWork) {
	defer func() {
		// Return worker to pool
		s.workerPool <- struct{}{}
	}()

	request := work.request
	ctx := work.ctx

	var result interface{}
	var err *shared.RPCError

	// Route request to appropriate handler
	switch request.Method {
	case "initialize":
		result, err = s.handleInitialize(ctx, request.Params)
	case "resources/list":
		result, err = s.handleResourcesList(ctx, request.Params)
	case "resources/read":
		result, err = s.handleResourcesRead(ctx, request.Params)
	case "tools/list":
		result, err = s.handleToolsList(ctx, request.Params)
	case "tools/call":
		result, err = s.handleToolsCall(ctx, request.Params)
	case "prompts/list":
		result, err = s.handlePromptsList(ctx, request.Params)
	case "prompts/get":
		result, err = s.handlePromptsGet(ctx, request.Params)
	default:
		// Check custom handlers
		s.handlerMu.RLock()
		handler, exists := s.requestHandlers[request.Method]
		s.handlerMu.RUnlock()

		if exists {
			resultVal, handlerErr := handler(ctx, request.Params)
			if handlerErr != nil {
				err = &shared.RPCError{
					Code:    -32603,
					Message: handlerErr.Error(),
				}
			} else {
				result = resultVal
			}
		} else {
			err = &shared.RPCError{
				Code:    -32601,
				Message: fmt.Sprintf("Method not found: %s", request.Method),
			}
		}
	}

	// Send response
	s.transport.SendResponse(request.ID, result, err)
}

// handleNotification processes notifications
func (s *Server) handleNotification(notification *shared.Notification) {
	// Handle notifications (no response required)
	switch notification.Method {
	case "initialized":
		// Server initialization complete
	case "notifications/cancelled":
		// Handle cancellation
	}
}

// Built-in handler implementations
func (s *Server) registerBuiltinHandlers() {
	// Built-in handlers are registered automatically
}

func (s *Server) handleInitialize(ctx context.Context, params interface{}) (interface{}, *shared.RPCError) {
	return shared.InitializeResponse{
		ProtocolVersion: shared.ProtocolVersion,
		Capabilities:    s.capabilities,
		ServerInfo:      s.info,
	}, nil
}

func (s *Server) handleResourcesList(ctx context.Context, params interface{}) (interface{}, *shared.RPCError) {
	s.resourceMu.RLock()
	defer s.resourceMu.RUnlock()

	resources := make([]shared.Resource, 0, len(s.resourceMeta))
	for _, resource := range s.resourceMeta {
		resources = append(resources, resource)
	}

	return map[string]interface{}{
		"resources": resources,
	}, nil
}

func (s *Server) handleResourcesRead(ctx context.Context, params interface{}) (interface{}, *shared.RPCError) {
	var req struct {
		URI string `json:"uri"`
	}

	if err := s.unmarshalParams(params, &req); err != nil {
		return nil, &shared.RPCError{
			Code:    -32602,
			Message: "Invalid params",
		}
	}

	s.resourceMu.RLock()
	handler, exists := s.resourceHandlers[req.URI]
	s.resourceMu.RUnlock()

	if !exists {
		return nil, &shared.RPCError{
			Code:    -32602,
			Message: fmt.Sprintf("Resource not found: %s", req.URI),
		}
	}

	contents, err := handler(ctx, req.URI)
	if err != nil {
		return nil, &shared.RPCError{
			Code:    -32603,
			Message: err.Error(),
		}
	}

	return map[string]interface{}{
		"contents": contents,
	}, nil
}

func (s *Server) handleToolsList(ctx context.Context, params interface{}) (interface{}, *shared.RPCError) {
	s.toolMu.RLock()
	defer s.toolMu.RUnlock()

	tools := make([]shared.Tool, 0, len(s.toolMeta))
	for _, tool := range s.toolMeta {
		tools = append(tools, tool)
	}

	return map[string]interface{}{
		"tools": tools,
	}, nil
}

func (s *Server) handleToolsCall(ctx context.Context, params interface{}) (interface{}, *shared.RPCError) {
	var req struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}

	if err := s.unmarshalParams(params, &req); err != nil {
		return nil, &shared.RPCError{
			Code:    -32602,
			Message: "Invalid params",
		}
	}

	s.toolMu.RLock()
	handler, exists := s.toolHandlers[req.Name]
	s.toolMu.RUnlock()

	if !exists {
		return nil, &shared.RPCError{
			Code:    -32602,
			Message: fmt.Sprintf("Tool not found: %s", req.Name),
		}
	}

	contents, err := handler(ctx, req.Name, req.Arguments)
	if err != nil {
		return nil, &shared.RPCError{
			Code:    -32603,
			Message: err.Error(),
		}
	}

	return map[string]interface{}{
		"content": contents,
	}, nil
}

func (s *Server) handlePromptsList(ctx context.Context, params interface{}) (interface{}, *shared.RPCError) {
	s.promptMu.RLock()
	defer s.promptMu.RUnlock()

	prompts := make([]shared.Prompt, 0, len(s.promptMeta))
	for _, prompt := range s.promptMeta {
		prompts = append(prompts, prompt)
	}

	return map[string]interface{}{
		"prompts": prompts,
	}, nil
}

func (s *Server) handlePromptsGet(ctx context.Context, params interface{}) (interface{}, *shared.RPCError) {
	var req struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}

	if err := s.unmarshalParams(params, &req); err != nil {
		return nil, &shared.RPCError{
			Code:    -32602,
			Message: "Invalid params",
		}
	}

	s.promptMu.RLock()
	handler, exists := s.promptHandlers[req.Name]
	s.promptMu.RUnlock()

	if !exists {
		return nil, &shared.RPCError{
			Code:    -32602,
			Message: fmt.Sprintf("Prompt not found: %s", req.Name),
		}
	}

	message, err := handler(ctx, req.Name, req.Arguments)
	if err != nil {
		return nil, &shared.RPCError{
			Code:    -32603,
			Message: err.Error(),
		}
	}

	return map[string]interface{}{
		"messages": []PromptMessage{message},
	}, nil
}

// unmarshalParams unmarshals request parameters
func (s *Server) unmarshalParams(params interface{}, dest interface{}) error {
	if params == nil {
		return nil
	}

	data, err := json.Marshal(params)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, dest)
}

// Close gracefully shuts down the server
func (s *Server) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.cancel()
		s.wg.Wait()

		if closeErr := s.transport.Close(); closeErr != nil {
			err = closeErr
		}
	})
	return err
}

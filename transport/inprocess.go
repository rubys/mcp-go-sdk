package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rubys/mcp-go-sdk/internal"
	"github.com/rubys/mcp-go-sdk/shared"
)

// InProcessTransport provides direct in-process communication between client and server
type InProcessTransport struct {
	messageHandler *internal.MessageHandler
	
	// Server-side channels (we'll implement a mini server in this transport)
	serverRequestChan      chan *shared.Request
	serverNotificationChan chan *shared.Notification
	
	requestChan      chan *shared.Request
	notificationChan chan *shared.Notification
	
	// Resource, tool, and prompt handlers for the in-process server
	resourceHandlers map[string]func(ctx context.Context, uri string) ([]shared.ResourceContent, error)
	toolHandlers     map[string]func(ctx context.Context, args map[string]interface{}) ([]shared.Content, error)
	promptHandlers   map[string]func(ctx context.Context, args map[string]string) (*shared.GetPromptResult, error)
	
	resourceMeta map[string]shared.Resource
	toolMeta     map[string]shared.Tool
	promptMeta   map[string]shared.Prompt
	
	mu sync.RWMutex
	
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	
	serverInfo shared.Implementation
}

// NewInProcessTransport creates a new in-process transport with default server info
func NewInProcessTransport() *InProcessTransport {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Create message handler for client-side correlation
	messageHandler := internal.NewMessageHandler(ctx, internal.MessageHandlerConfig{
		RequestTimeout: 30 * time.Second,
		BufferSize:     100,
	})
	
	transport := &InProcessTransport{
		messageHandler:         messageHandler,
		serverRequestChan:      make(chan *shared.Request, 100),
		serverNotificationChan: make(chan *shared.Notification, 100),
		requestChan:            make(chan *shared.Request, 100),
		notificationChan:       make(chan *shared.Notification, 100),
		resourceHandlers:       make(map[string]func(ctx context.Context, uri string) ([]shared.ResourceContent, error)),
		toolHandlers:           make(map[string]func(ctx context.Context, args map[string]interface{}) ([]shared.Content, error)),
		promptHandlers:         make(map[string]func(ctx context.Context, args map[string]string) (*shared.GetPromptResult, error)),
		resourceMeta:           make(map[string]shared.Resource),
		toolMeta:               make(map[string]shared.Tool),
		promptMeta:             make(map[string]shared.Prompt),
		ctx:                    ctx,
		cancel:                 cancel,
		serverInfo: shared.Implementation{
			Name:    "in-process-server",
			Version: "1.0.0",
		},
	}
	
	// Get channels from message handler
	incoming, outgoing, clientRequests, clientNotifications := messageHandler.Channels()
	
	// Start goroutines to bridge channels
	transport.wg.Add(4)
	go transport.bridgeOutgoing(outgoing, incoming)
	go transport.bridgeRequests(clientRequests)
	go transport.bridgeNotifications(clientNotifications)
	go transport.processServerRequests()
	
	return transport
}

// Channels returns the transport's communication channels
func (t *InProcessTransport) Channels() (
	requests <-chan *shared.Request,
	notifications <-chan *shared.Notification,
) {
	return t.requestChan, t.notificationChan
}

// SendRequest sends a request directly to the server
func (t *InProcessTransport) SendRequest(method string, params interface{}) (<-chan *shared.Response, error) {
	return t.messageHandler.SendRequest(method, params)
}

// SendNotification sends a notification directly to the server
func (t *InProcessTransport) SendNotification(method string, params interface{}) error {
	return t.messageHandler.SendNotification(method, params)
}

// SendResponse sends a response (not used by client transport)
func (t *InProcessTransport) SendResponse(id interface{}, result interface{}, err *shared.RPCError) error {
	return fmt.Errorf("client transport does not send responses")
}

// bridgeOutgoing processes messages from the message handler and routes them to the server
func (t *InProcessTransport) bridgeOutgoing(outgoing <-chan []byte, incoming chan<- []byte) {
	defer t.wg.Done()
	
	for {
		select {
		case data := <-outgoing:
			if data == nil {
				return
			}
			
			// Parse the JSON-RPC message to see if it's a request or notification
			var msg struct {
				JSONRPC string      `json:"jsonrpc"`
				ID      interface{} `json:"id,omitempty"`
				Method  string      `json:"method,omitempty"`
				Params  interface{} `json:"params,omitempty"`
			}
			
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			
			// If it has an ID, it's a request - handle it and send response
			if msg.ID != nil {
				response := t.handleRequest(msg.Method, msg.Params, msg.ID)
				if response != nil {
					responseData, _ := json.Marshal(response)
					select {
					case incoming <- responseData:
					case <-t.ctx.Done():
						return
					}
				}
			} else {
				// It's a notification - handle it without response
				t.handleNotification(msg.Method, msg.Params)
			}
			
		case <-t.ctx.Done():
			return
		}
	}
}

// bridgeRequests forwards requests from the message handler to the channel
func (t *InProcessTransport) bridgeRequests(clientRequests <-chan *shared.Request) {
	defer t.wg.Done()
	defer close(t.requestChan)
	
	for {
		select {
		case req := <-clientRequests:
			if req == nil {
				return
			}
			select {
			case t.requestChan <- req:
			case <-t.ctx.Done():
				return
			}
		case <-t.ctx.Done():
			return
		}
	}
}

// bridgeNotifications forwards notifications from the message handler to the channel
func (t *InProcessTransport) bridgeNotifications(clientNotifications <-chan *shared.Notification) {
	defer t.wg.Done()
	defer close(t.notificationChan)
	
	for {
		select {
		case notif := <-clientNotifications:
			if notif == nil {
				return
			}
			select {
			case t.notificationChan <- notif:
			case <-t.ctx.Done():
				return
			}
		case <-t.ctx.Done():
			return
		}
	}
}

// processServerRequests handles server-side request processing (not needed for client)
func (t *InProcessTransport) processServerRequests() {
	defer t.wg.Done()
	
	for {
		select {
		case <-t.ctx.Done():
			return
		}
	}
}

// Start starts the transport (no-op for in-process)
func (t *InProcessTransport) Start(ctx context.Context) error {
	// Already started in constructor
	return nil
}

// Close closes the transport
func (t *InProcessTransport) Close() error {
	t.cancel()
	t.wg.Wait()
	return t.messageHandler.Close()
}

// Port returns 0 as in-process transport doesn't use a port
func (t *InProcessTransport) Port() int {
	return 0
}

// Stats returns transport statistics
func (t *InProcessTransport) Stats() interface{} {
	return struct {
		Type string
	}{
		Type: "in-process",
	}
}

// AddTool adds a tool handler to the in-process server
func (t *InProcessTransport) AddTool(tool shared.Tool, handler func(ctx context.Context, args map[string]interface{}) ([]shared.Content, error)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.toolMeta[tool.Name] = tool
	t.toolHandlers[tool.Name] = handler
}

// AddResource adds a resource handler to the in-process server
func (t *InProcessTransport) AddResource(resource shared.Resource, handler func(ctx context.Context, uri string) ([]shared.ResourceContent, error)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.resourceMeta[resource.URI] = resource
	t.resourceHandlers[resource.URI] = handler
}

// AddPrompt adds a prompt handler to the in-process server
func (t *InProcessTransport) AddPrompt(prompt shared.Prompt, handler func(ctx context.Context, args map[string]string) (*shared.GetPromptResult, error)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.promptMeta[prompt.Name] = prompt
	t.promptHandlers[prompt.Name] = handler
}

// handleRequest processes incoming requests
func (t *InProcessTransport) handleRequest(method string, params interface{}, id interface{}) *shared.Response {
	ctx := context.Background()
	
	switch method {
	case "initialize":
		return &shared.Response{
			JSONRPC: "2.0",
			ID:      id,
			Result: map[string]interface{}{
				"protocolVersion": shared.ProtocolVersion,
				"serverInfo":      t.serverInfo,
				"capabilities": map[string]interface{}{
					"resources": map[string]interface{}{
						"subscribe":    false,
						"listChanged": true,
					},
					"tools": map[string]interface{}{
						"listChanged": true,
					},
					"prompts": map[string]interface{}{
						"listChanged": true,
					},
				},
			},
		}
		
	case "tools/list":
		t.mu.RLock()
		tools := make([]shared.Tool, 0, len(t.toolMeta))
		for _, tool := range t.toolMeta {
			tools = append(tools, tool)
		}
		t.mu.RUnlock()
		
		return &shared.Response{
			JSONRPC: "2.0",
			ID:      id,
			Result: &shared.ListToolsResult{
				Tools: tools,
			},
		}
		
	case "tools/call":
		paramsMap, ok := params.(map[string]interface{})
		if !ok {
			return &shared.Response{
				JSONRPC: "2.0",
				ID:      id,
				Error: &shared.RPCError{
					Code:    -32602,
					Message: "Invalid params",
				},
			}
		}
		
		name, ok := paramsMap["name"].(string)
		if !ok {
			return &shared.Response{
				JSONRPC: "2.0",
				ID:      id,
				Error: &shared.RPCError{
					Code:    -32602,
					Message: "Missing tool name",
				},
			}
		}
		
		args, _ := paramsMap["arguments"].(map[string]interface{})
		
		t.mu.RLock()
		handler, exists := t.toolHandlers[name]
		t.mu.RUnlock()
		
		if !exists {
			return &shared.Response{
				JSONRPC: "2.0",
				ID:      id,
				Error: &shared.RPCError{
					Code:    -32601,
					Message: "Tool not found: " + name,
				},
			}
		}
		
		content, err := handler(ctx, args)
		if err != nil {
			return &shared.Response{
				JSONRPC: "2.0",
				ID:      id,
				Error: &shared.RPCError{
					Code:    -32603,
					Message: err.Error(),
				},
			}
		}
		
		return &shared.Response{
			JSONRPC: "2.0",
			ID:      id,
			Result: &shared.CallToolResult{
				Content: content,
			},
		}
		
	case "resources/list":
		t.mu.RLock()
		resources := make([]shared.Resource, 0, len(t.resourceMeta))
		for _, resource := range t.resourceMeta {
			resources = append(resources, resource)
		}
		t.mu.RUnlock()
		
		return &shared.Response{
			JSONRPC: "2.0",
			ID:      id,
			Result: &shared.ListResourcesResult{
				Resources: resources,
			},
		}
		
	case "resources/read":
		paramsMap, ok := params.(map[string]interface{})
		if !ok {
			return &shared.Response{
				JSONRPC: "2.0",
				ID:      id,
				Error: &shared.RPCError{
					Code:    -32602,
					Message: "Invalid params",
				},
			}
		}
		
		uri, ok := paramsMap["uri"].(string)
		if !ok {
			return &shared.Response{
				JSONRPC: "2.0",
				ID:      id,
				Error: &shared.RPCError{
					Code:    -32602,
					Message: "Missing URI",
				},
			}
		}
		
		t.mu.RLock()
		handler, exists := t.resourceHandlers[uri]
		t.mu.RUnlock()
		
		if !exists {
			return &shared.Response{
				JSONRPC: "2.0",
				ID:      id,
				Error: &shared.RPCError{
					Code:    -32601,
					Message: "Resource not found: " + uri,
				},
			}
		}
		
		content, err := handler(ctx, uri)
		if err != nil {
			return &shared.Response{
				JSONRPC: "2.0",
				ID:      id,
				Error: &shared.RPCError{
					Code:    -32603,
					Message: err.Error(),
				},
			}
		}
		
		return &shared.Response{
			JSONRPC: "2.0",
			ID:      id,
			Result: &shared.ReadResourceResult{
				Contents: content,
			},
		}
		
	case "prompts/list":
		t.mu.RLock()
		prompts := make([]shared.Prompt, 0, len(t.promptMeta))
		for _, prompt := range t.promptMeta {
			prompts = append(prompts, prompt)
		}
		t.mu.RUnlock()
		
		return &shared.Response{
			JSONRPC: "2.0",
			ID:      id,
			Result: &shared.ListPromptsResult{
				Prompts: prompts,
			},
		}
		
	case "prompts/get":
		paramsMap, ok := params.(map[string]interface{})
		if !ok {
			return &shared.Response{
				JSONRPC: "2.0",
				ID:      id,
				Error: &shared.RPCError{
					Code:    -32602,
					Message: "Invalid params",
				},
			}
		}
		
		name, ok := paramsMap["name"].(string)
		if !ok {
			return &shared.Response{
				JSONRPC: "2.0",
				ID:      id,
				Error: &shared.RPCError{
					Code:    -32602,
					Message: "Missing prompt name",
				},
			}
		}
		
		argsInterface, _ := paramsMap["arguments"]
		args := make(map[string]string)
		if argsMap, ok := argsInterface.(map[string]interface{}); ok {
			for k, v := range argsMap {
				if s, ok := v.(string); ok {
					args[k] = s
				}
			}
		}
		
		t.mu.RLock()
		handler, exists := t.promptHandlers[name]
		t.mu.RUnlock()
		
		if !exists {
			return &shared.Response{
				JSONRPC: "2.0",
				ID:      id,
				Error: &shared.RPCError{
					Code:    -32601,
					Message: "Prompt not found: " + name,
				},
			}
		}
		
		result, err := handler(ctx, args)
		if err != nil {
			return &shared.Response{
				JSONRPC: "2.0",
				ID:      id,
				Error: &shared.RPCError{
					Code:    -32603,
					Message: err.Error(),
				},
			}
		}
		
		return &shared.Response{
			JSONRPC: "2.0",
			ID:      id,
			Result:  result,
		}
		
	default:
		return &shared.Response{
			JSONRPC: "2.0",
			ID:      id,
			Error: &shared.RPCError{
				Code:    -32601,
				Message: "Method not found: " + method,
			},
		}
	}
}

// handleNotification processes incoming notifications
func (t *InProcessTransport) handleNotification(method string, params interface{}) {
	// For now, just ignore notifications
	// In a real implementation, you might want to handle notifications like cancellation
}
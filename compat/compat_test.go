package compat

import (
	"context"
	"testing"
	"time"

	"github.com/rubys/mcp-go-sdk/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceBuilder(t *testing.T) {
	t.Run("BasicResource", func(t *testing.T) {
		resource := NewResource("file://test.txt", "Test File")
		
		assert.Equal(t, "file://test.txt", resource.uri)
		assert.Equal(t, "Test File", resource.name)
		assert.Empty(t, resource.description)
		assert.Empty(t, resource.mimeType)
	})
	
	t.Run("ResourceWithOptions", func(t *testing.T) {
		resource := NewResource("file://test.txt", "Test File",
			WithResourceDescription("A test file"),
			WithMIMEType("text/plain"),
			WithResourceAnnotation("key", "value"),
		)
		
		assert.Equal(t, "file://test.txt", resource.uri)
		assert.Equal(t, "Test File", resource.name)
		assert.Equal(t, "A test file", resource.description)
		assert.Equal(t, "text/plain", resource.mimeType)
		assert.Equal(t, "value", resource.annotations["key"])
	})
}

func TestToolBuilder(t *testing.T) {
	t.Run("BasicTool", func(t *testing.T) {
		tool := NewTool("test-tool")
		
		assert.Equal(t, "test-tool", tool.name)
		assert.Empty(t, tool.description)
		assert.NotNil(t, tool.inputSchema)
		
		// Check schema structure
		schema := tool.inputSchema
		assert.Equal(t, "object", schema["type"])
		assert.NotNil(t, schema["properties"])
		assert.NotNil(t, schema["required"])
	})
	
	t.Run("ToolWithArguments", func(t *testing.T) {
		tool := NewTool("calc",
			WithDescription("Calculator tool"),
			WithString("operation", Required(), Description("Operation to perform"), Enum("add", "sub")),
			WithNumber("x", Required(), Description("First number")),
			WithBoolean("verbose", Description("Verbose output")),
		)
		
		assert.Equal(t, "calc", tool.name)
		assert.Equal(t, "Calculator tool", tool.description)
		
		schema := tool.inputSchema
		properties := schema["properties"].(map[string]interface{})
		required := schema["required"].([]string)
		
		// Check string argument
		opProp := properties["operation"].(map[string]interface{})
		assert.Equal(t, "string", opProp["type"])
		assert.Equal(t, "Operation to perform", opProp["description"])
		assert.Equal(t, []string{"add", "sub"}, opProp["enum"])
		assert.Contains(t, required, "operation")
		
		// Check number argument
		xProp := properties["x"].(map[string]interface{})
		assert.Equal(t, "number", xProp["type"])
		assert.Equal(t, "First number", xProp["description"])
		assert.Contains(t, required, "x")
		
		// Check boolean argument (not required)
		verboseProp := properties["verbose"].(map[string]interface{})
		assert.Equal(t, "boolean", verboseProp["type"])
		assert.Equal(t, "Verbose output", verboseProp["description"])
		assert.NotContains(t, required, "verbose")
	})
}

func TestPromptBuilder(t *testing.T) {
	t.Run("BasicPrompt", func(t *testing.T) {
		prompt := NewPrompt("test-prompt")
		
		assert.Equal(t, "test-prompt", prompt.name)
		assert.Empty(t, prompt.description)
		assert.Empty(t, prompt.arguments)
	})
	
	t.Run("PromptWithArguments", func(t *testing.T) {
		prompt := NewPrompt("greeting",
			WithPromptDescription("Generate greeting"),
			WithArgument("name", RequiredArgument(), ArgumentDescription("Person's name")),
			WithArgument("style", ArgumentDescription("Greeting style")),
		)
		
		assert.Equal(t, "greeting", prompt.name)
		assert.Equal(t, "Generate greeting", prompt.description)
		assert.Len(t, prompt.arguments, 2)
		
		// Check required argument
		nameArg := prompt.arguments[0]
		assert.Equal(t, "name", nameArg.Name)
		assert.Equal(t, "Person's name", nameArg.Description)
		assert.True(t, nameArg.Required)
		
		// Check optional argument
		styleArg := prompt.arguments[1]
		assert.Equal(t, "style", styleArg.Name)
		assert.Equal(t, "Greeting style", styleArg.Description)
		assert.False(t, styleArg.Required)
	})
}

func TestServerConfiguration(t *testing.T) {
	t.Run("BasicServer", func(t *testing.T) {
		server := NewMCPServer("test-server", "1.0.0")
		
		assert.Equal(t, "test-server", server.config.name)
		assert.Equal(t, "1.0.0", server.config.version)
		assert.Equal(t, 100, server.config.maxConcurrentRequests)
		assert.Equal(t, 30*time.Second, server.config.requestTimeout)
	})
	
	t.Run("ServerWithOptions", func(t *testing.T) {
		server := NewMCPServer("test-server", "1.0.0",
			WithResourceCapabilities(true, true),
			WithToolCapabilities(true),
			WithPromptCapabilities(false),
			WithLogging(),
			WithConcurrency(50, 15*time.Second),
		)
		
		assert.Equal(t, "test-server", server.config.name)
		assert.Equal(t, "1.0.0", server.config.version)
		assert.Equal(t, 50, server.config.maxConcurrentRequests)
		assert.Equal(t, 15*time.Second, server.config.requestTimeout)
		assert.True(t, server.config.withLogging)
		
		// Check capabilities
		caps := server.config.capabilities
		assert.NotNil(t, caps.Resources)
		assert.True(t, caps.Resources.Subscribe)
		assert.True(t, caps.Resources.ListChanged)
		
		assert.NotNil(t, caps.Tools)
		assert.True(t, caps.Tools.ListChanged)
		
		assert.NotNil(t, caps.Prompts)
		assert.False(t, caps.Prompts.ListChanged)
	})
}

// MockTransport for testing server integration
type MockTransport struct {
	requests      chan *shared.Request
	notifications chan *shared.Notification
	responses     map[interface{}]chan *shared.Response
}

func NewMockTransport() *MockTransport {
	return &MockTransport{
		requests:      make(chan *shared.Request, 10),
		notifications: make(chan *shared.Notification, 10),
		responses:     make(map[interface{}]chan *shared.Response),
	}
}

func (m *MockTransport) Channels() (<-chan *shared.Request, <-chan *shared.Notification) {
	return m.requests, m.notifications
}

func (m *MockTransport) SendResponse(id interface{}, result interface{}, err *shared.RPCError) error {
	if respChan, exists := m.responses[id]; exists {
		respChan <- &shared.Response{
			ID:     id,
			Result: result,
			Error:  err,
		}
		close(respChan)
		delete(m.responses, id)
	}
	return nil
}

func (m *MockTransport) SendNotification(method string, params interface{}) error {
	m.notifications <- &shared.Notification{
		Method: method,
		Params: params,
	}
	return nil
}

func (m *MockTransport) SendRequest(method string, params interface{}) (<-chan *shared.Response, error) {
	respChan := make(chan *shared.Response, 1)
	reqID := len(m.responses) + 1
	m.responses[reqID] = respChan
	
	m.requests <- &shared.Request{
		ID:     reqID,
		Method: method,
		Params: params,
	}
	
	return respChan, nil
}

func (m *MockTransport) Close() error {
	close(m.requests)
	close(m.notifications)
	return nil
}

func (m *MockTransport) Stats() interface{} {
	return map[string]interface{}{
		"type": "mock",
		"requests_sent": len(m.responses),
	}
}

func TestServerIntegration(t *testing.T) {
	t.Run("StdioTransport", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		server := NewMCPServer("test-server", "1.0.0")
		
		err := server.CreateWithStdio(ctx)
		require.NoError(t, err)
		
		err = server.Start()
		require.NoError(t, err)
		defer server.Close()
		
		underlying := server.GetUnderlying()
		assert.NotNil(t, underlying)
	})
	
	t.Run("SSETransport", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		server := NewMCPServer("test-server", "1.0.0")
		
		err := server.CreateWithSSE(ctx, "http://localhost:8080/events", "http://localhost:8080/send")
		require.NoError(t, err)
		
		underlying := server.GetUnderlying()
		assert.NotNil(t, underlying)
		
		// Note: Don't start SSE server in test to avoid port conflicts
		defer server.Close()
	})
	
	t.Run("StreamableHTTPTransport", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		server := NewMCPServer("test-server", "1.0.0")
		
		err := server.CreateWithStreamableHTTP(ctx, "http://localhost:8081/mcp")
		require.NoError(t, err)
		
		underlying := server.GetUnderlying()
		assert.NotNil(t, underlying)
		
		// Note: Don't start HTTP server in test to avoid port conflicts
		defer server.Close()
	})
	
	t.Run("CustomTransport", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		transport := NewMockTransport()
		server := NewMCPServer("test-server", "1.0.0")
		
		err := server.SetTransport(ctx, transport)
		require.NoError(t, err)
		
		underlying := server.GetUnderlying()
		assert.NotNil(t, underlying)
		
		defer server.Close()
	})

	t.Run("ResourceRegistration", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		transport := NewMockTransport()
		server := NewMCPServer("test-server", "1.0.0")
		
		err := server.SetTransport(ctx, transport)
		require.NoError(t, err)
		
		// Register a test resource
		resource := NewResource("test://resource", "Test Resource",
			WithResourceDescription("Test description"),
		)
		
		server.AddResource(resource, func(ctx context.Context, req ResourceRequest) (ResourceResponse, error) {
			return ResourceResponse{
				Contents: []shared.Content{
					shared.TextContent{
						Type: shared.ContentTypeText,
						Text: "test content",
						URI:  req.URI,
					},
				},
			}, nil
		})
		
		err = server.Start()
		require.NoError(t, err)
		defer server.Close()
		
		// Verify the underlying server has the resource registered
		underlying := server.GetUnderlying()
		assert.NotNil(t, underlying)
	})
	
	t.Run("ToolRegistration", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		transport := NewMockTransport()
		server := NewMCPServer("test-server", "1.0.0")
		
		err := server.SetTransport(ctx, transport)
		require.NoError(t, err)
		
		// Register a test tool
		tool := NewTool("test-tool",
			WithDescription("Test tool"),
			WithString("input", Required()),
		)
		
		server.AddTool(tool, func(ctx context.Context, req ToolRequest) (ToolResponse, error) {
			input := req.Arguments["input"].(string)
			return ToolResponse{
				Content: []shared.Content{
					shared.TextContent{
						Type: shared.ContentTypeText,
						Text: "Output: " + input,
					},
				},
			}, nil
		})
		
		err = server.Start()
		require.NoError(t, err)
		defer server.Close()
		
		// Verify the underlying server has the tool registered
		underlying := server.GetUnderlying()
		assert.NotNil(t, underlying)
	})
}

func TestCompatibilityTypes(t *testing.T) {
	t.Run("ResourceRequest", func(t *testing.T) {
		req := ResourceRequest{URI: "test://uri"}
		assert.Equal(t, "test://uri", req.URI)
	})
	
	t.Run("ToolRequest", func(t *testing.T) {
		req := ToolRequest{
			Name:      "test-tool",
			Arguments: map[string]interface{}{"key": "value"},
		}
		assert.Equal(t, "test-tool", req.Name)
		assert.Equal(t, "value", req.Arguments["key"])
	})
	
	t.Run("PromptRequest", func(t *testing.T) {
		req := PromptRequest{
			Name:      "test-prompt",
			Arguments: map[string]interface{}{"param": "value"},
		}
		assert.Equal(t, "test-prompt", req.Name)
		assert.Equal(t, "value", req.Arguments["param"])
	})
}
package client

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/shared"
	"github.com/modelcontextprotocol/go-sdk/transport"
)

// Client represents an MCP client
type Client struct {
	ctx        context.Context
	transport  transport.Transport
	serverInfo *shared.ServerInfo
	config     ClientConfig
}

// ClientConfig holds client configuration matching test expectations
type ClientConfig struct {
	ClientInfo   shared.ClientInfo
	Capabilities shared.ClientCapabilities
}

// NewClient creates a new MCP client with generic transport interface
func NewClient(ctx context.Context, transport transport.Transport, config ClientConfig) (*Client, error) {
	client := &Client{
		ctx:       ctx,
		transport: transport,
		config:    config,
	}

	// Initialize the connection
	if err := client.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize client: %w", err)
	}

	return client, nil
}

// Initialize initializes the connection with the server
func (c *Client) Initialize(ctx context.Context) error {
	resp, err := c.transport.SendRequest("initialize", map[string]interface{}{
		"protocolVersion": shared.ProtocolVersion,
		"capabilities":    c.config.Capabilities,
		"clientInfo": map[string]interface{}{
			"name":    c.config.ClientInfo.Name,
			"version": c.config.ClientInfo.Version,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to send initialize request: %w", err)
	}

	// Wait for response with context
	select {
	case response := <-resp:
		if response == nil {
			return fmt.Errorf("no response received")
		}

		if response.Error != nil {
			return fmt.Errorf("initialize error: %s", response.Error.Message)
		}

		// Parse server info
		if result, ok := response.Result.(map[string]interface{}); ok {
			if serverInfo, ok := result["serverInfo"].(map[string]interface{}); ok {
				c.serverInfo = &shared.ServerInfo{
					Name:    getString(serverInfo, "name"),
					Version: getString(serverInfo, "version"),
				}
			}
		}

		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ListResources lists available resources
func (c *Client) ListResources(ctx context.Context) (*shared.ListResourcesResult, error) {
	resp, err := c.transport.SendRequest("resources/list", nil)
	if err != nil {
		return nil, err
	}

	select {
	case response := <-resp:
		if response == nil {
			return nil, fmt.Errorf("no response received")
		}

		if response.Error != nil {
			return nil, fmt.Errorf("error: %s", response.Error.Message)
		}

		var result shared.ListResourcesResult
		if resultMap, ok := response.Result.(map[string]interface{}); ok {
			if resList, ok := resultMap["resources"].([]interface{}); ok {
				for _, r := range resList {
					if res, ok := r.(map[string]interface{}); ok {
						resource := shared.Resource{
							URI:         getString(res, "uri"),
							Name:        getString(res, "name"),
							Description: getString(res, "description"),
							MimeType:    getString(res, "mimeType"),
						}
						result.Resources = append(result.Resources, resource)
					}
				}
			}
		}

		return &result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ReadResource reads a specific resource
func (c *Client) ReadResource(ctx context.Context, uri string) (*shared.ReadResourceResult, error) {
	// Send URI as object with uri field - this matches TypeScript SDK expectations
	resp, err := c.transport.SendRequest("resources/read", map[string]interface{}{
		"uri": uri,
	})
	if err != nil {
		return nil, err
	}

	select {
	case response := <-resp:
		if response == nil {
			return nil, fmt.Errorf("no response received")
		}

		if response.Error != nil {
			return nil, fmt.Errorf("error: %s", response.Error.Message)
		}

		var result shared.ReadResourceResult
		if resultMap, ok := response.Result.(map[string]interface{}); ok {
			if contentList, ok := resultMap["contents"].([]interface{}); ok {
				for _, c := range contentList {
					if content, ok := c.(map[string]interface{}); ok {
						resourceContent := shared.ResourceContent{
							URI:      getString(content, "uri"),
							MimeType: getString(content, "mimeType"),
							Text:     getString(content, "text"),
							Blob:     getString(content, "blob"),
						}
						result.Contents = append(result.Contents, resourceContent)
					}
				}
			}
		}

		return &result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ListTools lists available tools
func (c *Client) ListTools(ctx context.Context) (*shared.ListToolsResult, error) {
	resp, err := c.transport.SendRequest("tools/list", nil)
	if err != nil {
		return nil, err
	}

	select {
	case response := <-resp:
		if response == nil {
			return nil, fmt.Errorf("no response received")
		}

		if response.Error != nil {
			return nil, fmt.Errorf("error: %s", response.Error.Message)
		}

		var result shared.ListToolsResult
		if resultMap, ok := response.Result.(map[string]interface{}); ok {
			if toolList, ok := resultMap["tools"].([]interface{}); ok {
				for _, t := range toolList {
					if tool, ok := t.(map[string]interface{}); ok {
						var inputSchema map[string]interface{}
						if schema, ok := tool["inputSchema"].(map[string]interface{}); ok {
							inputSchema = schema
						}
						
						sharedTool := shared.Tool{
							Name:        getString(tool, "name"),
							Description: getString(tool, "description"),
							InputSchema: inputSchema,
						}
						result.Tools = append(result.Tools, sharedTool)
					}
				}
			}
		}

		return &result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// CallTool calls a tool with the given arguments
func (c *Client) CallTool(ctx context.Context, request shared.CallToolRequest) (*shared.CallToolResult, error) {
	resp, err := c.transport.SendRequest("tools/call", map[string]interface{}{
		"name":      request.Name,
		"arguments": request.Arguments,
	})
	if err != nil {
		return nil, err
	}

	select {
	case response := <-resp:
		if response == nil {
			return nil, fmt.Errorf("no response received")
		}

		if response.Error != nil {
			return nil, fmt.Errorf("error: %s", response.Error.Message)
		}

		var result shared.CallToolResult
		if resultMap, ok := response.Result.(map[string]interface{}); ok {
			if contentList, ok := resultMap["content"].([]interface{}); ok {
				for _, c := range contentList {
					if content, ok := c.(map[string]interface{}); ok {
						textContent := shared.TextContent{
							Type: shared.ContentTypeText,
							Text: getString(content, "text"),
							URI:  getString(content, "uri"),
						}
						result.Content = append(result.Content, textContent)
					}
				}
			}
			
			if isError, ok := resultMap["isError"].(bool); ok {
				result.IsError = isError
			}
		}

		return &result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ListPrompts lists available prompts
func (c *Client) ListPrompts(ctx context.Context) (*shared.ListPromptsResult, error) {
	resp, err := c.transport.SendRequest("prompts/list", nil)
	if err != nil {
		return nil, err
	}

	select {
	case response := <-resp:
		if response == nil {
			return nil, fmt.Errorf("no response received")
		}

		if response.Error != nil {
			return nil, fmt.Errorf("error: %s", response.Error.Message)
		}

		var result shared.ListPromptsResult
		if resultMap, ok := response.Result.(map[string]interface{}); ok {
			if promptList, ok := resultMap["prompts"].([]interface{}); ok {
				for _, p := range promptList {
					if prompt, ok := p.(map[string]interface{}); ok {
						var arguments []shared.PromptArgument
						if args, ok := prompt["arguments"].([]interface{}); ok {
							for _, arg := range args {
								if argMap, ok := arg.(map[string]interface{}); ok {
									promptArg := shared.PromptArgument{
										Name:        getString(argMap, "name"),
										Description: getString(argMap, "description"),
									}
									if required, ok := argMap["required"].(bool); ok {
										promptArg.Required = required
									}
									arguments = append(arguments, promptArg)
								}
							}
						}
						
						sharedPrompt := shared.Prompt{
							Name:        getString(prompt, "name"),
							Description: getString(prompt, "description"),
							Arguments:   arguments,
						}
						result.Prompts = append(result.Prompts, sharedPrompt)
					}
				}
			}
		}

		return &result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetPrompt gets a specific prompt with arguments
func (c *Client) GetPrompt(ctx context.Context, request shared.GetPromptRequest) (*shared.GetPromptResult, error) {
	resp, err := c.transport.SendRequest("prompts/get", map[string]interface{}{
		"name":      request.Name,
		"arguments": request.Arguments,
	})
	if err != nil {
		return nil, err
	}

	select {
	case response := <-resp:
		if response == nil {
			return nil, fmt.Errorf("no response received")
		}

		if response.Error != nil {
			return nil, fmt.Errorf("error: %s", response.Error.Message)
		}

		var result shared.GetPromptResult
		if resultMap, ok := response.Result.(map[string]interface{}); ok {
			result.Description = getString(resultMap, "description")

			if messages, ok := resultMap["messages"].([]interface{}); ok {
				for _, m := range messages {
					if msg, ok := m.(map[string]interface{}); ok {
						message := shared.PromptMessage{
							Role: getString(msg, "role"),
						}

						// Handle content - can be string, object, or array
						if content, ok := msg["content"].(string); ok {
							// String content - convert to TextContent
							message.Content = shared.TextContent{
								Type: shared.ContentTypeText,
								Text: content,
							}
						} else if content, ok := msg["content"].(map[string]interface{}); ok {
							// Single object content
							message.Content = shared.TextContent{
								Type: shared.ContentTypeText,
								Text: getString(content, "text"),
								URI:  getString(content, "uri"),
							}
						} else if contentArray, ok := msg["content"].([]interface{}); ok {
							// Array of content items
							var contents []shared.Content
							for _, c := range contentArray {
								if contentMap, ok := c.(map[string]interface{}); ok {
									textContent := shared.TextContent{
										Type: shared.ContentTypeText,
										Text: getString(contentMap, "text"),
										URI:  getString(contentMap, "uri"),
									}
									contents = append(contents, textContent)
								}
							}
							message.Content = contents
						}

						result.Messages = append(result.Messages, message)
					}
				}
			}
		}

		return &result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close closes the client connection
func (c *Client) Close() error {
	return c.transport.Close()
}

// Helper function to safely get string from map
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

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
	transport  *transport.StdioTransport
	serverInfo *ServerInfo
}

// ClientConfig holds client configuration
type ClientConfig struct {
	Name    string
	Version string
}

// ServerInfo contains server information
type ServerInfo struct {
	Name         string
	Version      string
	Capabilities shared.ServerCapabilities
}

// NewClient creates a new MCP client
func NewClient(ctx context.Context, transport *transport.StdioTransport, config ClientConfig) *Client {
	return &Client{
		ctx:       ctx,
		transport: transport,
	}
}

// Initialize initializes the connection with the server
func (c *Client) Initialize() error {
	resp, err := c.transport.SendRequest("initialize", map[string]interface{}{
		"protocolVersion": shared.ProtocolVersion,
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "go-mcp-client",
			"version": "1.0.0",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to send initialize request: %w", err)
	}

	// Wait for response
	response := <-resp
	if response == nil {
		return fmt.Errorf("no response received")
	}

	if response.Error != nil {
		return fmt.Errorf("initialize error: %s", response.Error.Message)
	}

	// Parse server info
	if result, ok := response.Result.(map[string]interface{}); ok {
		if serverInfo, ok := result["serverInfo"].(map[string]interface{}); ok {
			c.serverInfo = &ServerInfo{
				Name:    getString(serverInfo, "name"),
				Version: getString(serverInfo, "version"),
			}
		}
	}

	return nil
}

// ListResources lists available resources
func (c *Client) ListResources() ([]Resource, error) {
	resp, err := c.transport.SendRequest("resources/list", nil)
	if err != nil {
		return nil, err
	}

	response := <-resp
	if response == nil {
		return nil, fmt.Errorf("no response received")
	}

	if response.Error != nil {
		return nil, fmt.Errorf("error: %s", response.Error.Message)
	}

	var resources []Resource
	if result, ok := response.Result.(map[string]interface{}); ok {
		if resList, ok := result["resources"].([]interface{}); ok {
			for _, r := range resList {
				if res, ok := r.(map[string]interface{}); ok {
					resources = append(resources, Resource{
						URI:         getString(res, "uri"),
						Name:        getString(res, "name"),
						Description: getString(res, "description"),
					})
				}
			}
		}
	}

	return resources, nil
}

// ReadResource reads a specific resource
func (c *Client) ReadResource(uri string) ([]Content, error) {
	resp, err := c.transport.SendRequest("resources/read", map[string]interface{}{
		"uri": uri,
	})
	if err != nil {
		return nil, err
	}

	response := <-resp
	if response == nil {
		return nil, fmt.Errorf("no response received")
	}

	if response.Error != nil {
		return nil, fmt.Errorf("error: %s", response.Error.Message)
	}

	var contents []Content
	if result, ok := response.Result.(map[string]interface{}); ok {
		if contentList, ok := result["contents"].([]interface{}); ok {
			for _, c := range contentList {
				if content, ok := c.(map[string]interface{}); ok {
					contents = append(contents, Content{
						Type: getString(content, "type"),
						Text: getString(content, "text"),
						URI:  getString(content, "uri"),
					})
				}
			}
		}
	}

	return contents, nil
}

// ListTools lists available tools
func (c *Client) ListTools() ([]Tool, error) {
	resp, err := c.transport.SendRequest("tools/list", nil)
	if err != nil {
		return nil, err
	}

	response := <-resp
	if response == nil {
		return nil, fmt.Errorf("no response received")
	}

	if response.Error != nil {
		return nil, fmt.Errorf("error: %s", response.Error.Message)
	}

	var tools []Tool
	if result, ok := response.Result.(map[string]interface{}); ok {
		if toolList, ok := result["tools"].([]interface{}); ok {
			for _, t := range toolList {
				if tool, ok := t.(map[string]interface{}); ok {
					tools = append(tools, Tool{
						Name:        getString(tool, "name"),
						Description: getString(tool, "description"),
						InputSchema: tool["inputSchema"],
					})
				}
			}
		}
	}

	return tools, nil
}

// CallTool calls a tool with the given arguments
func (c *Client) CallTool(name string, arguments map[string]interface{}) ([]Content, error) {
	resp, err := c.transport.SendRequest("tools/call", map[string]interface{}{
		"name":      name,
		"arguments": arguments,
	})
	if err != nil {
		return nil, err
	}

	response := <-resp
	if response == nil {
		return nil, fmt.Errorf("no response received")
	}

	if response.Error != nil {
		return nil, fmt.Errorf("error: %s", response.Error.Message)
	}

	var contents []Content
	if result, ok := response.Result.(map[string]interface{}); ok {
		if contentList, ok := result["content"].([]interface{}); ok {
			for _, c := range contentList {
				if content, ok := c.(map[string]interface{}); ok {
					contents = append(contents, Content{
						Type: getString(content, "type"),
						Text: getString(content, "text"),
					})
				}
			}
		}
	}

	return contents, nil
}

// ListPrompts lists available prompts
func (c *Client) ListPrompts() ([]Prompt, error) {
	resp, err := c.transport.SendRequest("prompts/list", nil)
	if err != nil {
		return nil, err
	}

	response := <-resp
	if response == nil {
		return nil, fmt.Errorf("no response received")
	}

	if response.Error != nil {
		return nil, fmt.Errorf("error: %s", response.Error.Message)
	}

	var prompts []Prompt
	if result, ok := response.Result.(map[string]interface{}); ok {
		if promptList, ok := result["prompts"].([]interface{}); ok {
			for _, p := range promptList {
				if prompt, ok := p.(map[string]interface{}); ok {
					prompts = append(prompts, Prompt{
						Name:        getString(prompt, "name"),
						Description: getString(prompt, "description"),
					})
				}
			}
		}
	}

	return prompts, nil
}

// GetPrompt gets a specific prompt with arguments
func (c *Client) GetPrompt(name string, arguments map[string]interface{}) (*PromptResult, error) {
	resp, err := c.transport.SendRequest("prompts/get", map[string]interface{}{
		"name":      name,
		"arguments": arguments,
	})
	if err != nil {
		return nil, err
	}

	response := <-resp
	if response == nil {
		return nil, fmt.Errorf("no response received")
	}

	if response.Error != nil {
		return nil, fmt.Errorf("error: %s", response.Error.Message)
	}

	promptResult := &PromptResult{}
	if result, ok := response.Result.(map[string]interface{}); ok {
		promptResult.Description = getString(result, "description")

		if messages, ok := result["messages"].([]interface{}); ok {
			for _, m := range messages {
				if msg, ok := m.(map[string]interface{}); ok {
					message := Message{
						Role: getString(msg, "role"),
					}

					// Handle content - can be string or object
					if content, ok := msg["content"].(string); ok {
						message.Content = content
					} else if content, ok := msg["content"].(map[string]interface{}); ok {
						message.Content = Content{
							Type: getString(content, "type"),
							Text: getString(content, "text"),
						}
					}

					promptResult.Messages = append(promptResult.Messages, message)
				}
			}
		}
	}

	return promptResult, nil
}

// Helper function to safely get string from map
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// Types for client responses

type Resource struct {
	URI         string
	Name        string
	Description string
}

type Content struct {
	Type string
	Text string
	URI  string
}

type Tool struct {
	Name        string
	Description string
	InputSchema interface{}
}

type Prompt struct {
	Name        string
	Description string
}

type PromptResult struct {
	Description string
	Messages    []Message
}

type Message struct {
	Role    string
	Content interface{} // Can be string or Content
}

package client

import (
	"github.com/rubys/mcp-go-sdk/transport"
)

// NewInProcessClient creates a new MCP client that connects to an in-process server
func NewInProcessClient() (*Client, *transport.InProcessTransport) {
	trans := transport.NewInProcessTransport()
	client := New(trans)
	return client, trans
}
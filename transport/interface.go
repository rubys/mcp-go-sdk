package transport

import (
	"github.com/modelcontextprotocol/go-sdk/shared"
)

// Transport interface defines the common interface for all MCP transports
type Transport interface {
	// SendRequest sends a request and returns a channel for the response
	SendRequest(method string, params interface{}) (<-chan *shared.Response, error)

	// SendNotification sends a notification (no response expected)
	SendNotification(method string, params interface{}) error

	// SendResponse sends a response to a request (for server implementations)
	SendResponse(id interface{}, result interface{}, err *shared.RPCError) error

	// Channels returns channels for receiving incoming requests and notifications
	Channels() (requests <-chan *shared.Request, notifications <-chan *shared.Notification)

	// Close gracefully shuts down the transport
	Close() error

	// Stats returns transport statistics
	Stats() interface{}
}

// ClientTransport is a transport that can only send requests and notifications
type ClientTransport interface {
	// SendRequest sends a request and returns a channel for the response
	SendRequest(method string, params interface{}) (<-chan *shared.Response, error)

	// SendNotification sends a notification (no response expected)
	SendNotification(method string, params interface{}) error

	// Close gracefully shuts down the transport
	Close() error

	// Stats returns transport statistics
	Stats() interface{}
}

// ServerTransport is a transport that can receive requests and send responses
type ServerTransport interface {
	// SendResponse sends a response to a request
	SendResponse(id interface{}, result interface{}, err *shared.RPCError) error

	// SendNotification sends a notification (server-to-client)
	SendNotification(method string, params interface{}) error

	// Channels returns channels for receiving incoming requests and notifications
	Channels() (requests <-chan *shared.Request, notifications <-chan *shared.Notification)

	// Close gracefully shuts down the transport
	Close() error

	// Stats returns transport statistics
	Stats() interface{}
}
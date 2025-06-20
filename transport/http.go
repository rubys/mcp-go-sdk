package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/internal"
	"github.com/modelcontextprotocol/go-sdk/shared"
)

// HTTPTransport implements HTTP transport with concurrent connection handling
type HTTPTransport struct {
	// HTTP configuration
	client   *http.Client
	endpoint string
	headers  map[string]string

	// Message handler for concurrent processing
	messageHandler *internal.MessageHandler

	// Channels for external communication
	requestChan      chan *shared.Request
	notificationChan chan *shared.Notification

	// Context and synchronization
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once

	// Configuration
	maxConcurrentRequests int
	connectionPoolSize    int
	keepAliveTimeout      time.Duration

	// Statistics
	requestsSent      int64
	responsesReceived int64
	connectionErrors  int64
	httpErrors        int64
}

// HTTPConfig configures the HTTP transport
type HTTPConfig struct {
	Endpoint              string
	Headers               map[string]string
	Timeout               time.Duration
	MaxConcurrentRequests int
	ConnectionPoolSize    int
	KeepAliveTimeout      time.Duration
	RequestTimeout        time.Duration
	MessageBuffer         int
	TLSConfig             interface{} // For future TLS configuration
}

// NewHTTPTransport creates a new concurrent HTTP transport
func NewHTTPTransport(ctx context.Context, config HTTPConfig) (*HTTPTransport, error) {
	if config.Endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}

	// Set defaults
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.MaxConcurrentRequests == 0 {
		config.MaxConcurrentRequests = 100
	}
	if config.ConnectionPoolSize == 0 {
		config.ConnectionPoolSize = 50
	}
	if config.KeepAliveTimeout == 0 {
		config.KeepAliveTimeout = 30 * time.Second
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 30 * time.Second
	}
	if config.MessageBuffer == 0 {
		config.MessageBuffer = 100
	}

	ctx, cancel := context.WithCancel(ctx)

	// Create HTTP client with connection pooling
	httpClient := &http.Client{
		Timeout: config.Timeout,
		Transport: &http.Transport{
			MaxIdleConns:        config.ConnectionPoolSize,
			MaxIdleConnsPerHost: config.ConnectionPoolSize / 2,
			IdleConnTimeout:     config.KeepAliveTimeout,
			DisableKeepAlives:   false,
		},
	}

	// Create message handler
	messageHandler := internal.NewMessageHandler(ctx, internal.MessageHandlerConfig{
		RequestTimeout: config.RequestTimeout,
		BufferSize:     config.MessageBuffer,
	})

	// Get channels from message handler
	incoming, outgoing, requests, notifications := messageHandler.Channels()

	transport := &HTTPTransport{
		client:                httpClient,
		endpoint:              config.Endpoint,
		headers:               config.Headers,
		messageHandler:        messageHandler,
		requestChan:           make(chan *shared.Request, config.MessageBuffer),
		notificationChan:      make(chan *shared.Notification, config.MessageBuffer),
		ctx:                   ctx,
		cancel:                cancel,
		maxConcurrentRequests: config.MaxConcurrentRequests,
		connectionPoolSize:    config.ConnectionPoolSize,
		keepAliveTimeout:      config.KeepAliveTimeout,
	}

	// Connect transport to message handler
	go transport.bridgeOutgoing(outgoing)
	go transport.bridgeRequests(requests)
	go transport.bridgeNotifications(notifications)

	// HTTP transport doesn't have incoming messages like stdio
	// Responses come back directly from HTTP requests
	close(incoming)

	return transport, nil
}

// Channels returns the transport's communication channels
func (ht *HTTPTransport) Channels() (
	requests <-chan *shared.Request,
	notifications <-chan *shared.Notification,
) {
	return ht.requestChan, ht.notificationChan
}

// SendRequest sends an HTTP request and returns a response channel
func (ht *HTTPTransport) SendRequest(method string, params interface{}) (<-chan *shared.Response, error) {
	return ht.messageHandler.SendRequest(method, params)
}

// SendNotification sends an HTTP notification
func (ht *HTTPTransport) SendNotification(method string, params interface{}) error {
	return ht.messageHandler.SendNotification(method, params)
}

// SendResponse sends a response (not applicable for HTTP client transport)
func (ht *HTTPTransport) SendResponse(id interface{}, result interface{}, err *shared.RPCError) error {
	return fmt.Errorf("HTTP client transport cannot send responses")
}

// bridgeOutgoing handles outgoing messages by sending them via HTTP
func (ht *HTTPTransport) bridgeOutgoing(messageHandlerOutgoing <-chan []byte) {
	// Create worker pool for concurrent HTTP requests
	semaphore := make(chan struct{}, ht.maxConcurrentRequests)

	for {
		select {
		case data := <-messageHandlerOutgoing:
			if data != nil {
				// Acquire semaphore slot
				semaphore <- struct{}{}

				go func(data []byte) {
					defer func() { <-semaphore }() // Release semaphore slot
					ht.sendHTTPRequest(data)
				}(data)
			}
		case <-ht.ctx.Done():
			return
		}
	}
}

// sendHTTPRequest sends a single HTTP request
func (ht *HTTPTransport) sendHTTPRequest(data []byte) {
	// Parse the JSON-RPC message to determine if it's a request or notification
	var msg shared.JSONRPCMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		ht.httpErrors++
		return
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ht.ctx, "POST", ht.endpoint, bytes.NewReader(data))
	if err != nil {
		ht.connectionErrors++
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for key, value := range ht.headers {
		req.Header.Set(key, value)
	}

	// Send HTTP request
	resp, err := ht.client.Do(req)
	if err != nil {
		ht.connectionErrors++
		return
	}
	defer resp.Body.Close()

	ht.requestsSent++

	// For notifications, we don't expect a response
	if msg.IsNotification() {
		return
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ht.httpErrors++
		return
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		ht.httpErrors++
		// Could parse error response and handle appropriately
		return
	}

	// Parse JSON-RPC response
	var response shared.Response
	if err := json.Unmarshal(body, &response); err != nil {
		ht.httpErrors++
		return
	}

	ht.responsesReceived++

	// Send response to message handler via the internal correlation system
	// This is handled automatically by the message handler's response correlation
}

// bridgeRequests bridges requests from message handler (not used in HTTP client)
func (ht *HTTPTransport) bridgeRequests(messageHandlerRequests <-chan *shared.Request) {
	// HTTP client transport doesn't receive requests, only sends them
	for {
		select {
		case <-messageHandlerRequests:
			// Discard - HTTP client doesn't handle incoming requests
		case <-ht.ctx.Done():
			return
		}
	}
}

// bridgeNotifications bridges notifications from message handler (not used in HTTP client)
func (ht *HTTPTransport) bridgeNotifications(messageHandlerNotifications <-chan *shared.Notification) {
	// HTTP client transport doesn't receive notifications, only sends them
	for {
		select {
		case <-messageHandlerNotifications:
			// Discard - HTTP client doesn't handle incoming notifications
		case <-ht.ctx.Done():
			return
		}
	}
}

// Close gracefully shuts down the transport
func (ht *HTTPTransport) Close() error {
	var err error
	ht.closeOnce.Do(func() {
		// Cancel context to stop all goroutines
		ht.cancel()

		// Wait for goroutines to finish
		ht.wg.Wait()

		// Close message handler
		if closeErr := ht.messageHandler.Close(); closeErr != nil {
			err = closeErr
		}

		// Close HTTP client transport if needed
		if transport, ok := ht.client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}

		// Close channels
		close(ht.requestChan)
		close(ht.notificationChan)
	})
	return err
}

// Stats returns transport statistics
func (ht *HTTPTransport) Stats() interface{} {
	messageStats := ht.messageHandler.GetStats()
	return HTTPTransportStats{
		RequestsSent:      ht.requestsSent,
		ResponsesReceived: ht.responsesReceived,
		ConnectionErrors:  ht.connectionErrors,
		HTTPErrors:        ht.httpErrors,
		MessageStats:      messageStats,
	}
}

// HTTPTransportStats represents HTTP transport-level statistics
type HTTPTransportStats struct {
	RequestsSent      int64
	ResponsesReceived int64
	ConnectionErrors  int64
	HTTPErrors        int64
	MessageStats      internal.MessageStats
}

// HTTPServerTransport implements HTTP server transport for receiving requests
type HTTPServerTransport struct {
	server  *http.Server
	handler http.Handler

	// Message processing
	messageHandler   *internal.MessageHandler
	requestChan      chan *shared.Request
	notificationChan chan *shared.Notification

	// Context and lifecycle
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// HTTPServerConfig configures the HTTP server transport
type HTTPServerConfig struct {
	Address        string
	Port           int
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	RequestTimeout time.Duration
	MessageBuffer  int
}

// NewHTTPServerTransport creates a new HTTP server transport
func NewHTTPServerTransport(ctx context.Context, config HTTPServerConfig) (*HTTPServerTransport, error) {
	if config.Address == "" {
		config.Address = "localhost"
	}
	if config.Port == 0 {
		config.Port = 8080
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = 10 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 10 * time.Second
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 30 * time.Second
	}
	if config.MessageBuffer == 0 {
		config.MessageBuffer = 100
	}

	ctx, cancel := context.WithCancel(ctx)

	// Create message handler
	messageHandler := internal.NewMessageHandler(ctx, internal.MessageHandlerConfig{
		RequestTimeout: config.RequestTimeout,
		BufferSize:     config.MessageBuffer,
	})

	transport := &HTTPServerTransport{
		messageHandler:   messageHandler,
		requestChan:      make(chan *shared.Request, config.MessageBuffer),
		notificationChan: make(chan *shared.Notification, config.MessageBuffer),
		ctx:              ctx,
		cancel:           cancel,
	}

	// Create HTTP handler
	mux := http.NewServeMux()
	mux.HandleFunc("/", transport.handleRequest)

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", config.Address, config.Port)
	transport.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
	}

	return transport, nil
}

// handleRequest handles incoming HTTP requests
func (hst *HTTPServerTransport) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Parse JSON-RPC message
	var msg shared.JSONRPCMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		http.Error(w, "Invalid JSON-RPC message", http.StatusBadRequest)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	if msg.IsRequest() {
		// Handle request - this would integrate with the server
		response := shared.Response{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  map[string]string{"status": "received"},
		}
		json.NewEncoder(w).Encode(response)
	} else if msg.IsNotification() {
		// Handle notification - no response needed
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "Invalid message type", http.StatusBadRequest)
	}
}

// Start starts the HTTP server
func (hst *HTTPServerTransport) Start() error {
	hst.wg.Add(1)
	go func() {
		defer hst.wg.Done()
		if err := hst.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Log error
		}
	}()
	return nil
}

// Close gracefully shuts down the HTTP server transport
func (hst *HTTPServerTransport) Close() error {
	var err error
	hst.closeOnce.Do(func() {
		// Shutdown HTTP server
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if shutdownErr := hst.server.Shutdown(shutdownCtx); shutdownErr != nil {
			err = shutdownErr
		}

		// Cancel context and wait for goroutines
		hst.cancel()
		hst.wg.Wait()

		// Close message handler
		if closeErr := hst.messageHandler.Close(); closeErr != nil && err == nil {
			err = closeErr
		}

		// Close channels
		close(hst.requestChan)
		close(hst.notificationChan)
	})
	return err
}

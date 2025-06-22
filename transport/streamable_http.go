package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rubys/mcp-go-sdk/internal"
	"github.com/rubys/mcp-go-sdk/shared"
)

// StreamableHTTP is a type alias for backward compatibility
type StreamableHTTP = StreamableHTTPTransport

// StreamableHTTPTransport implements the Streamable HTTP transport with SSE support
type StreamableHTTPTransport struct {
	// HTTP configuration
	client         *http.Client
	clientEndpoint string
	sseEndpoint    string
	sessionID      string
	headers        map[string]string

	// SSE configuration
	enableSSE      bool
	reconnectDelay time.Duration
	maxReconnects  int
	heartbeatInterval time.Duration

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
	maxConcurrentRequests   int
	connectionPoolSize      int
	keepAliveTimeout        time.Duration
	typeScriptCompatibility bool

	// Statistics
	requestsSent      int64
	responsesReceived int64
	connectionErrors  int64
	httpErrors        int64
	sseConnected      int64
	sseReconnects     int64
	requestCounter    int64

	// SSE state
	sseActive       int64
	sseConnection   *http.Response
	sseReader       *bufio.Scanner
	sseMu           sync.RWMutex

	// OAuth support
	oauthHandler *OAuthHandler
	oauthEnabled bool
	oauthMu      sync.RWMutex
}

// StreamableHTTPConfig configures the Streamable HTTP transport
type StreamableHTTPConfig struct {
	ClientEndpoint          string
	SSEEndpoint             string
	SessionID               string
	Headers                 map[string]string
	Timeout                 time.Duration
	MaxConcurrentRequests   int
	ConnectionPoolSize      int
	KeepAliveTimeout        time.Duration
	RequestTimeout          time.Duration
	MessageBuffer           int
	EnableSSE               bool
	ReconnectDelay          time.Duration
	MaxReconnects           int
	HeartbeatInterval       time.Duration
	TypeScriptCompatibility bool
	TLSConfig               interface{} // For future TLS configuration
}

// NewStreamableHTTPTransport creates a new Streamable HTTP transport with a simple URL
func NewStreamableHTTPTransport(url string) *StreamableHTTPTransport {
	config := StreamableHTTPConfig{
		ClientEndpoint: url,
		EnableSSE:      false,
	}
	transport, _ := NewStreamableHTTPTransportWithConfig(context.Background(), config)
	return transport
}

// NewStreamableHTTPTransportWithConfig creates a new Streamable HTTP transport
func NewStreamableHTTPTransportWithConfig(ctx context.Context, config StreamableHTTPConfig) (*StreamableHTTPTransport, error) {
	if config.ClientEndpoint == "" {
		return nil, fmt.Errorf("client endpoint is required")
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
	if config.ReconnectDelay == 0 {
		config.ReconnectDelay = 1 * time.Second
	}
	if config.MaxReconnects == 0 {
		config.MaxReconnects = 5
	}
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = 30 * time.Second
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

	transport := &StreamableHTTPTransport{
		client:                  httpClient,
		clientEndpoint:          config.ClientEndpoint,
		sseEndpoint:             config.SSEEndpoint,
		sessionID:               config.SessionID,
		headers:                 config.Headers,
		enableSSE:               config.EnableSSE,
		reconnectDelay:          config.ReconnectDelay,
		maxReconnects:           config.MaxReconnects,
		heartbeatInterval:       config.HeartbeatInterval,
		messageHandler:          messageHandler,
		requestChan:             make(chan *shared.Request, config.MessageBuffer),
		notificationChan:        make(chan *shared.Notification, config.MessageBuffer),
		ctx:                     ctx,
		cancel:                  cancel,
		maxConcurrentRequests:   config.MaxConcurrentRequests,
		connectionPoolSize:      config.ConnectionPoolSize,
		keepAliveTimeout:        config.KeepAliveTimeout,
		typeScriptCompatibility: config.TypeScriptCompatibility,
	}

	// Get channels from message handler
	incoming, outgoing, requests, notifications := messageHandler.Channels()

	// Connect transport to message handler
	go transport.bridgeOutgoing(outgoing)
	go transport.bridgeRequests(requests)
	go transport.bridgeNotifications(notifications)

	// Start SSE connection if enabled
	if config.EnableSSE && config.SSEEndpoint != "" {
		go transport.startSSEConnection()
	}

	// HTTP client doesn't receive incoming messages directly
	close(incoming)

	return transport, nil
}

// Channels returns the transport's communication channels
func (sht *StreamableHTTPTransport) Channels() (
	requests <-chan *shared.Request,
	notifications <-chan *shared.Notification,
) {
	return sht.requestChan, sht.notificationChan
}

// SendRequest sends an HTTP request and returns a response channel
func (sht *StreamableHTTPTransport) SendRequest(method string, params interface{}) (<-chan *shared.Response, error) {
	// Create request ID
	id := atomic.AddInt64(&sht.requestCounter, 1)
	
	// Create JSON-RPC request
	request := shared.Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	
	// Marshal request
	data, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	// Create response channel
	responseChan := make(chan *shared.Response, 1)
	
	// Send HTTP request in goroutine
	go func() {
		defer close(responseChan)
		
		// Create HTTP request
		req, err := http.NewRequestWithContext(sht.ctx, "POST", sht.clientEndpoint, bytes.NewReader(data))
		if err != nil {
			atomic.AddInt64(&sht.httpErrors, 1)
			return
		}
		
		// Set headers
		req.Header.Set("Content-Type", "application/json")
		for key, value := range sht.headers {
			req.Header.Set(key, value)
		}
		
		// Send request
		resp, err := sht.client.Do(req)
		if err != nil {
			atomic.AddInt64(&sht.httpErrors, 1)
			return
		}
		defer resp.Body.Close()
		
		// Read response
		responseData, err := io.ReadAll(resp.Body)
		if err != nil {
			atomic.AddInt64(&sht.httpErrors, 1)
			return
		}
		
		// Parse response
		var jsonRPCResponse shared.Response
		if err := json.Unmarshal(responseData, &jsonRPCResponse); err != nil {
			atomic.AddInt64(&sht.httpErrors, 1)
			return
		}
		
		// Send response to channel
		atomic.AddInt64(&sht.responsesReceived, 1)
		responseChan <- &jsonRPCResponse
	}()
	
	atomic.AddInt64(&sht.requestsSent, 1)
	return responseChan, nil
}

// SendNotification sends an HTTP notification
func (sht *StreamableHTTPTransport) SendNotification(method string, params interface{}) error {
	return sht.messageHandler.SendNotification(method, params)
}

// SendResponse sends a response (not applicable for HTTP client transport)
func (sht *StreamableHTTPTransport) SendResponse(id interface{}, result interface{}, err *shared.RPCError) error {
	return fmt.Errorf("HTTP client transport cannot send responses")
}

// bridgeOutgoing handles outgoing messages by sending them via HTTP
func (sht *StreamableHTTPTransport) bridgeOutgoing(messageHandlerOutgoing <-chan []byte) {
	// Create worker pool for concurrent HTTP requests
	semaphore := make(chan struct{}, sht.maxConcurrentRequests)

	for {
		select {
		case data := <-messageHandlerOutgoing:
			if data != nil {
				// Acquire semaphore slot
				semaphore <- struct{}{}

				go func(data []byte) {
					defer func() { <-semaphore }() // Release semaphore slot
					sht.sendHTTPRequest(data)
				}(data)
			}
		case <-sht.ctx.Done():
			return
		}
	}
}

// sendHTTPRequest sends a single HTTP request
func (sht *StreamableHTTPTransport) sendHTTPRequest(data []byte) {
	// Parse the JSON-RPC message to determine if it's a request or notification
	var msg shared.JSONRPCMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		atomic.AddInt64(&sht.httpErrors, 1)
		return
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(sht.ctx, "POST", sht.clientEndpoint, bytes.NewReader(data))
	if err != nil {
		atomic.AddInt64(&sht.connectionErrors, 1)
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	
	// Add session ID if configured
	if sht.sessionID != "" {
		req.Header.Set("X-Session-ID", sht.sessionID)
	}
	
	// Add OAuth authorization header if enabled
	sht.oauthMu.RLock()
	oauthEnabled := sht.oauthEnabled
	oauthHandler := sht.oauthHandler
	sht.oauthMu.RUnlock()
	
	if oauthEnabled && oauthHandler != nil {
		authHeader, err := oauthHandler.GetAuthorizationHeader(sht.ctx)
		if err != nil {
			// Handle OAuth error - this might require authorization
			atomic.AddInt64(&sht.httpErrors, 1)
			return
		}
		req.Header.Set("Authorization", authHeader)
	}
	
	// Add custom headers
	for key, value := range sht.headers {
		req.Header.Set(key, value)
	}

	// Send HTTP request
	resp, err := sht.client.Do(req)
	if err != nil {
		atomic.AddInt64(&sht.connectionErrors, 1)
		return
	}
	defer resp.Body.Close()

	atomic.AddInt64(&sht.requestsSent, 1)

	// For notifications, we don't expect a response
	if msg.IsNotification() {
		return
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		atomic.AddInt64(&sht.httpErrors, 1)
		return
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		atomic.AddInt64(&sht.httpErrors, 1)
		return
	}

	// Parse JSON-RPC response
	var response shared.Response
	if err := json.Unmarshal(body, &response); err != nil {
		atomic.AddInt64(&sht.httpErrors, 1)
		return
	}

	atomic.AddInt64(&sht.responsesReceived, 1)

	// The response correlation is handled by the message handler automatically
}

// startSSEConnection establishes and maintains SSE connection
func (sht *StreamableHTTPTransport) startSSEConnection() {
	sht.wg.Add(1)
	defer sht.wg.Done()

	reconnectCount := 0
	
	for {
		select {
		case <-sht.ctx.Done():
			return
		default:
		}

		err := sht.connectSSE()
		if err == nil {
			// Connection succeeded, reset reconnect count
			reconnectCount = 0
			atomic.StoreInt64(&sht.sseConnected, 1)
		} else {
			atomic.StoreInt64(&sht.sseConnected, 0)
			
			if reconnectCount >= sht.maxReconnects {
				fmt.Printf("SSE: Max reconnection attempts reached (%d)\n", sht.maxReconnects)
				return
			}
			
			reconnectCount++
			atomic.AddInt64(&sht.sseReconnects, 1)
			
			// Wait before reconnecting
			select {
			case <-time.After(sht.reconnectDelay):
			case <-sht.ctx.Done():
				return
			}
		}
	}
}

// connectSSE establishes a single SSE connection
func (sht *StreamableHTTPTransport) connectSSE() error {
	req, err := http.NewRequestWithContext(sht.ctx, "GET", sht.sseEndpoint, nil)
	if err != nil {
		return fmt.Errorf("creating SSE request: %w", err)
	}

	// Set SSE headers
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")
	
	// Add session ID if configured
	if sht.sessionID != "" {
		req.Header.Set("X-Session-ID", sht.sessionID)
	}
	
	// Add OAuth authorization header if enabled
	sht.oauthMu.RLock()
	oauthEnabled := sht.oauthEnabled
	oauthHandler := sht.oauthHandler
	sht.oauthMu.RUnlock()
	
	if oauthEnabled && oauthHandler != nil {
		authHeader, err := oauthHandler.GetAuthorizationHeader(sht.ctx)
		if err != nil {
			return fmt.Errorf("OAuth authorization failed: %w", err)
		}
		req.Header.Set("Authorization", authHeader)
	}
	
	// Add custom headers
	for key, value := range sht.headers {
		req.Header.Set(key, value)
	}

	resp, err := sht.client.Do(req)
	if err != nil {
		return fmt.Errorf("SSE connection failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("SSE connection failed with status: %d", resp.StatusCode)
	}

	sht.sseMu.Lock()
	sht.sseConnection = resp
	sht.sseReader = bufio.NewScanner(resp.Body)
	sht.sseMu.Unlock()

	defer func() {
		sht.sseMu.Lock()
		if sht.sseConnection != nil {
			sht.sseConnection.Body.Close()
			sht.sseConnection = nil
			sht.sseReader = nil
		}
		sht.sseMu.Unlock()
	}()

	// Read SSE events
	return sht.readSSEEvents()
}

// readSSEEvents reads and processes SSE events
func (sht *StreamableHTTPTransport) readSSEEvents() error {
	sht.sseMu.RLock()
	scanner := sht.sseReader
	sht.sseMu.RUnlock()

	if scanner == nil {
		return fmt.Errorf("SSE scanner not initialized")
	}

	for scanner.Scan() {
		select {
		case <-sht.ctx.Done():
			return sht.ctx.Err()
		default:
		}

		line := scanner.Text()
		
		// Handle SSE event format
		if strings.HasPrefix(line, "data: ") {
			data := line[6:] // Remove "data: " prefix
			
			if err := sht.processSSEData(data); err != nil {
				fmt.Printf("SSE: Error processing data: %v\n", err)
			}
		} else if strings.HasPrefix(line, ": ") {
			// Heartbeat or comment, ignore
			continue
		} else if line == "" {
			// End of event, continue
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("SSE reading error: %w", err)
	}

	return fmt.Errorf("SSE connection closed")
}

// processSSEData processes SSE data as JSON-RPC message
func (sht *StreamableHTTPTransport) processSSEData(data string) error {
	if data == "" {
		return nil
	}

	// Parse as JSON-RPC message
	var msg shared.JSONRPCMessage
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return fmt.Errorf("invalid JSON-RPC in SSE data: %w", err)
	}

	// Handle different message types
	if msg.IsNotification() {
		// Convert to notification
		notification := &shared.Notification{
			JSONRPC: msg.JSONRPC,
			Method:  msg.Method,
			Params:  msg.Params,
		}
		
		select {
		case sht.notificationChan <- notification:
		case <-sht.ctx.Done():
			return sht.ctx.Err()
		}
	} else if msg.IsRequest() {
		// Convert to request (if server-to-client requests are supported)
		request := &shared.Request{
			JSONRPC: msg.JSONRPC,
			ID:      msg.ID,
			Method:  msg.Method,
			Params:  msg.Params,
		}
		
		select {
		case sht.requestChan <- request:
		case <-sht.ctx.Done():
			return sht.ctx.Err()
		}
	}
	// Response messages are handled by the message handler's correlation system

	return nil
}

// bridgeRequests bridges requests from message handler (not used in HTTP client)
func (sht *StreamableHTTPTransport) bridgeRequests(messageHandlerRequests <-chan *shared.Request) {
	// HTTP client transport doesn't normally receive requests, only sends them
	// However, with SSE, server might send requests to client
	for {
		select {
		case req := <-messageHandlerRequests:
			if req != nil {
				// Forward to external request channel
				select {
				case sht.requestChan <- req:
				case <-sht.ctx.Done():
					return
				}
			}
		case <-sht.ctx.Done():
			return
		}
	}
}

// bridgeNotifications bridges notifications from message handler (not used in HTTP client)
func (sht *StreamableHTTPTransport) bridgeNotifications(messageHandlerNotifications <-chan *shared.Notification) {
	// HTTP client transport doesn't normally receive notifications, only sends them
	// However, with SSE, server sends notifications to client
	for {
		select {
		case notif := <-messageHandlerNotifications:
			if notif != nil {
				// Forward to external notification channel
				select {
				case sht.notificationChan <- notif:
				case <-sht.ctx.Done():
					return
				}
			}
		case <-sht.ctx.Done():
			return
		}
	}
}

// Close gracefully shuts down the transport
func (sht *StreamableHTTPTransport) Close() error {
	var err error
	sht.closeOnce.Do(func() {
		// Cancel context to stop all goroutines
		sht.cancel()

		// Close SSE connection
		sht.sseMu.Lock()
		if sht.sseConnection != nil {
			sht.sseConnection.Body.Close()
		}
		sht.sseMu.Unlock()

		// Wait for goroutines to finish
		sht.wg.Wait()

		// Close message handler
		if closeErr := sht.messageHandler.Close(); closeErr != nil {
			err = closeErr
		}

		// Close HTTP client transport if needed
		if transport, ok := sht.client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}

		// Close channels
		close(sht.requestChan)
		close(sht.notificationChan)
	})
	return err
}

// Stats returns transport statistics
func (sht *StreamableHTTPTransport) Stats() interface{} {
	messageStats := sht.messageHandler.GetStats()
	return StreamableHTTPTransportStats{
		RequestsSent:      atomic.LoadInt64(&sht.requestsSent),
		ResponsesReceived: atomic.LoadInt64(&sht.responsesReceived),
		ConnectionErrors:  atomic.LoadInt64(&sht.connectionErrors),
		HTTPErrors:        atomic.LoadInt64(&sht.httpErrors),
		SSEConnected:      atomic.LoadInt64(&sht.sseConnected) == 1,
		SSEReconnects:     atomic.LoadInt64(&sht.sseReconnects),
		MessageStats:      messageStats,
	}
}

// StreamableHTTPTransportStats represents Streamable HTTP transport statistics
type StreamableHTTPTransportStats struct {
	RequestsSent      int64
	ResponsesReceived int64
	ConnectionErrors  int64
	HTTPErrors        int64
	SSEConnected      bool
	SSEReconnects     int64
	MessageStats      internal.MessageStats
}

// Port returns the port number (not applicable for client transport)
func (sht *StreamableHTTPTransport) Port() int {
	return 0 // Client transport doesn't have a port
}

// EnableOAuth enables OAuth authentication for the transport
func (sht *StreamableHTTPTransport) EnableOAuth(handler *OAuthHandler) {
	sht.oauthMu.Lock()
	defer sht.oauthMu.Unlock()
	sht.oauthHandler = handler
	sht.oauthEnabled = true
}

// IsOAuthEnabled returns whether OAuth is enabled
func (sht *StreamableHTTPTransport) IsOAuthEnabled() bool {
	sht.oauthMu.RLock()
	defer sht.oauthMu.RUnlock()
	return sht.oauthEnabled
}

// GetOAuthHandler returns the OAuth handler if enabled
func (sht *StreamableHTTPTransport) GetOAuthHandler() *OAuthHandler {
	sht.oauthMu.RLock()
	defer sht.oauthMu.RUnlock()
	if sht.oauthEnabled {
		return sht.oauthHandler
	}
	return nil
}

// Start starts the transport (no-op for StreamableHTTP as it starts automatically)
func (sht *StreamableHTTPTransport) Start(ctx context.Context) error {
	// Transport is already started in the constructor
	return nil
}
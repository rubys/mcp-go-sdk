package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/internal"
	"github.com/modelcontextprotocol/go-sdk/shared"
)

// StreamableHTTPServerTransport implements HTTP server transport with SSE support
type StreamableHTTPServerTransport struct {
	// HTTP server
	server     *http.Server
	mux        *http.ServeMux
	httpServer *http.Server

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
	address           string
	port              int
	actualPort        int
	readTimeout       time.Duration
	writeTimeout      time.Duration
	enableSSE         bool
	corsEnabled       bool
	maxRequestSize    int64
	requestTimeout    time.Duration

	// SSE connections
	sseClients map[string]*SSEClient
	sseMu      sync.RWMutex

	// HTTP request correlation
	pendingHTTPRequests map[interface{}]*PendingHTTPRequest
	httpMu              sync.RWMutex

	// Statistics
	requestsReceived int64
	responsesSent    int64
	sseConnections   int64
	httpErrors       int64
}

// SSEClient represents an SSE client connection
type SSEClient struct {
	ID       string
	Writer   http.ResponseWriter
	Flusher  http.Flusher
	Done     chan struct{}
	Headers  map[string]string
	LastPing time.Time
}

// PendingHTTPRequest represents a pending HTTP request waiting for response
type PendingHTTPRequest struct {
	Writer     http.ResponseWriter
	Request    *http.Request
	ResponseCh chan *shared.Response
	Timer      *time.Timer
}

// StreamableHTTPServerConfig configures the HTTP server transport
type StreamableHTTPServerConfig struct {
	Address        string
	Port           int
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	RequestTimeout time.Duration
	MessageBuffer  int
	EnableSSE      bool
	CORSEnabled    bool
	MaxRequestSize int64
}

// NewStreamableHTTPServerTransport creates a new HTTP server transport
func NewStreamableHTTPServerTransport(ctx context.Context, config StreamableHTTPServerConfig) (*StreamableHTTPServerTransport, error) {
	// Set defaults
	if config.Address == "" {
		config.Address = "localhost"
	}
	if config.Port == 0 {
		config.Port = 0 // Use any available port
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
	if config.MaxRequestSize == 0 {
		config.MaxRequestSize = 10 * 1024 * 1024 // 10MB
	}

	ctx, cancel := context.WithCancel(ctx)

	// Create message handler
	messageHandler := internal.NewMessageHandler(ctx, internal.MessageHandlerConfig{
		RequestTimeout: config.RequestTimeout,
		BufferSize:     config.MessageBuffer,
	})

	transport := &StreamableHTTPServerTransport{
		messageHandler:   messageHandler,
		requestChan:      make(chan *shared.Request, config.MessageBuffer),
		notificationChan: make(chan *shared.Notification, config.MessageBuffer),
		ctx:              ctx,
		cancel:           cancel,
		address:          config.Address,
		port:             config.Port,
		readTimeout:      config.ReadTimeout,
		writeTimeout:     config.WriteTimeout,
		enableSSE:        config.EnableSSE,
		corsEnabled:      config.CORSEnabled,
		maxRequestSize:      config.MaxRequestSize,
		requestTimeout:      config.RequestTimeout,
		sseClients:          make(map[string]*SSEClient),
		pendingHTTPRequests: make(map[interface{}]*PendingHTTPRequest),
	}

	// Setup HTTP handlers
	transport.setupHandlers()

	// Get channels from message handler
	_, outgoing, requests, notifications := messageHandler.Channels()

	// Bridge channels - no incoming for server transport
	go transport.bridgeOutgoing(outgoing)
	go transport.bridgeRequests(requests)
	go transport.bridgeNotifications(notifications)

	return transport, nil
}

// setupHandlers sets up HTTP route handlers
func (hst *StreamableHTTPServerTransport) setupHandlers() {
	hst.mux = http.NewServeMux()

	// JSON-RPC endpoint
	hst.mux.HandleFunc("/rpc", hst.handleRPC)

	// SSE endpoint
	if hst.enableSSE {
		hst.mux.HandleFunc("/sse", hst.handleSSE)
	}

	// Health check endpoint
	hst.mux.HandleFunc("/health", hst.handleHealth)

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", hst.address, hst.port)
	hst.httpServer = &http.Server{
		Addr:         addr,
		Handler:      hst.mux,
		ReadTimeout:  hst.readTimeout,
		WriteTimeout: hst.writeTimeout,
	}
}

// handleRPC handles JSON-RPC requests
func (hst *StreamableHTTPServerTransport) handleRPC(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers if enabled
	if hst.corsEnabled {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Session-ID")
	}

	// Handle preflight
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check request size
	r.Body = http.MaxBytesReader(w, r.Body, hst.maxRequestSize)

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		atomic.AddInt64(&hst.httpErrors, 1)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	atomic.AddInt64(&hst.requestsReceived, 1)

	// Parse JSON-RPC message
	var msg shared.JSONRPCMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		atomic.AddInt64(&hst.httpErrors, 1)
		http.Error(w, "Invalid JSON-RPC message", http.StatusBadRequest)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	if msg.IsRequest() {
		// Handle request
		request := msg.ToRequest()
		if request == nil {
			atomic.AddInt64(&hst.httpErrors, 1)
			http.Error(w, "Invalid request format", http.StatusBadRequest)
			return
		}

		// Create pending request for response correlation
		responseCh := make(chan *shared.Response, 1)
		timer := time.AfterFunc(hst.requestTimeout, func() {
			hst.removePendingHTTPRequest(request.ID)
			close(responseCh)
		})

		pendingReq := &PendingHTTPRequest{
			Writer:     w,
			Request:    r,
			ResponseCh: responseCh,
			Timer:      timer,
		}

		// Store pending request
		hst.httpMu.Lock()
		hst.pendingHTTPRequests[request.ID] = pendingReq
		hst.httpMu.Unlock()

		// Send to incoming channel for processing
		select {
		case hst.requestChan <- request:
		case <-hst.ctx.Done():
			hst.removePendingHTTPRequest(request.ID)
			http.Error(w, "Server shutting down", http.StatusServiceUnavailable)
			return
		}

		// Wait for response
		select {
		case response := <-responseCh:
			if response != nil {
				if err := json.NewEncoder(w).Encode(response); err != nil {
					atomic.AddInt64(&hst.httpErrors, 1)
				} else {
					atomic.AddInt64(&hst.responsesSent, 1)
				}
			} else {
				// Timeout occurred
				http.Error(w, "Request timeout", http.StatusGatewayTimeout)
			}
		case <-hst.ctx.Done():
			http.Error(w, "Server shutting down", http.StatusServiceUnavailable)
		}

	} else if msg.IsNotification() {
		// Handle notification
		notification := msg.ToNotification()
		if notification == nil {
			atomic.AddInt64(&hst.httpErrors, 1)
			http.Error(w, "Invalid notification format", http.StatusBadRequest)
			return
		}

		select {
		case hst.notificationChan <- notification:
		case <-hst.ctx.Done():
			http.Error(w, "Server shutting down", http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
	} else {
		atomic.AddInt64(&hst.httpErrors, 1)
		http.Error(w, "Invalid message type", http.StatusBadRequest)
	}
}

// handleSSE handles Server-Sent Events connections
func (hst *StreamableHTTPServerTransport) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Check if connection supports SSE
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Set CORS headers if enabled
	if hst.corsEnabled {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")
	}

	// Create SSE client
	clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())
	client := &SSEClient{
		ID:      clientID,
		Writer:  w,
		Flusher: flusher,
		Done:    make(chan struct{}),
		Headers: make(map[string]string),
		LastPing: time.Now(),
	}

	// Extract headers
	sessionID := r.Header.Get("X-Session-ID")
	if sessionID != "" {
		client.Headers["X-Session-ID"] = sessionID
	}

	// Register SSE client
	hst.sseMu.Lock()
	hst.sseClients[clientID] = client
	hst.sseMu.Unlock()

	atomic.AddInt64(&hst.sseConnections, 1)

	// Send initial connection event
	hst.sendSSEEvent(client, "connected", map[string]interface{}{
		"clientId":  clientID,
		"timestamp": time.Now().Unix(),
	})

	// Handle client disconnection
	defer func() {
		hst.sseMu.Lock()
		delete(hst.sseClients, clientID)
		hst.sseMu.Unlock()
		close(client.Done)
	}()

	// Keep connection alive and listen for client disconnect
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Send heartbeat
			if err := hst.sendSSEHeartbeat(client); err != nil {
				return // Client disconnected
			}
		case <-r.Context().Done():
			return // Client disconnected
		case <-hst.ctx.Done():
			return // Server shutting down
		}
	}
}

// handleHealth handles health check requests
func (hst *StreamableHTTPServerTransport) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"transport": "streamable-http",
	})
}

// sendSSEEvent sends an event to an SSE client
func (hst *StreamableHTTPServerTransport) sendSSEEvent(client *SSEClient, eventType string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// Write SSE event format
	if eventType != "" {
		fmt.Fprintf(client.Writer, "event: %s\n", eventType)
	}
	fmt.Fprintf(client.Writer, "data: %s\n\n", string(jsonData))
	
	client.Flusher.Flush()
	client.LastPing = time.Now()

	return nil
}

// sendSSEHeartbeat sends a heartbeat to an SSE client
func (hst *StreamableHTTPServerTransport) sendSSEHeartbeat(client *SSEClient) error {
	fmt.Fprintf(client.Writer, ": heartbeat\n\n")
	client.Flusher.Flush()
	client.LastPing = time.Now()
	return nil
}

// broadcastSSEEvent broadcasts an event to all SSE clients
func (hst *StreamableHTTPServerTransport) broadcastSSEEvent(eventType string, data interface{}) {
	hst.sseMu.RLock()
	clients := make([]*SSEClient, 0, len(hst.sseClients))
	for _, client := range hst.sseClients {
		clients = append(clients, client)
	}
	hst.sseMu.RUnlock()

	for _, client := range clients {
		if err := hst.sendSSEEvent(client, eventType, data); err != nil {
			// Client disconnected, remove it
			hst.sseMu.Lock()
			delete(hst.sseClients, client.ID)
			hst.sseMu.Unlock()
		}
	}
}

// Start starts the HTTP server
func (hst *StreamableHTTPServerTransport) Start() error {
	listener, err := net.Listen("tcp", hst.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// Get actual port
	if addr, ok := listener.Addr().(*net.TCPAddr); ok {
		hst.actualPort = addr.Port
	}

	hst.wg.Add(1)
	go func() {
		defer hst.wg.Done()
		if err := hst.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Printf("HTTP server error: %v\n", err)
		}
	}()

	return nil
}

// Port returns the actual port the server is listening on
func (hst *StreamableHTTPServerTransport) Port() int {
	return hst.actualPort
}

// Channels returns the transport's communication channels
func (hst *StreamableHTTPServerTransport) Channels() (
	requests <-chan *shared.Request,
	notifications <-chan *shared.Notification,
) {
	return hst.requestChan, hst.notificationChan
}

// SendRequest sends a request (not typically used by server transport)
func (hst *StreamableHTTPServerTransport) SendRequest(method string, params interface{}) (<-chan *shared.Response, error) {
	return hst.messageHandler.SendRequest(method, params)
}

// SendNotification sends a notification via SSE
func (hst *StreamableHTTPServerTransport) SendNotification(method string, params interface{}) error {
	if hst.enableSSE {
		notification := shared.Notification{
			JSONRPC: "2.0",
			Method:  method,
			Params:  params,
		}
		hst.broadcastSSEEvent("notification", notification)
	}
	return hst.messageHandler.SendNotification(method, params)
}

// SendResponse sends a response to a request
func (hst *StreamableHTTPServerTransport) SendResponse(id interface{}, result interface{}, err *shared.RPCError) error {
	// First check if this is a pending HTTP request
	hst.httpMu.RLock()
	pendingReq, exists := hst.pendingHTTPRequests[id]
	hst.httpMu.RUnlock()

	if exists {
		// This is an HTTP request, send response directly to the HTTP client
		response := &shared.Response{
			JSONRPC: "2.0",
			ID:      id,
			Result:  result,
			Error:   err,
		}

		// Send to the pending request's response channel
		select {
		case pendingReq.ResponseCh <- response:
			// Clean up
			hst.removePendingHTTPRequest(id)
		default:
			// Channel might be closed due to timeout
		}
		return nil
	}

	// Not an HTTP request, delegate to message handler
	return hst.messageHandler.SendResponse(id, result, err)
}

// removePendingHTTPRequest removes a pending HTTP request
func (hst *StreamableHTTPServerTransport) removePendingHTTPRequest(id interface{}) {
	hst.httpMu.Lock()
	defer hst.httpMu.Unlock()
	
	if pendingReq, exists := hst.pendingHTTPRequests[id]; exists {
		if pendingReq.Timer != nil {
			pendingReq.Timer.Stop()
		}
		delete(hst.pendingHTTPRequests, id)
	}
}


// bridgeOutgoing handles outgoing messages to SSE clients
func (hst *StreamableHTTPServerTransport) bridgeOutgoing(outgoing <-chan []byte) {
	for {
		select {
		case data := <-outgoing:
			if data != nil && hst.enableSSE {
				// Parse and send via SSE
				var msg shared.JSONRPCMessage
				if err := json.Unmarshal(data, &msg); err != nil {
					continue
				}

				if msg.IsNotification() {
					hst.broadcastSSEEvent("notification", msg)
				} else if msg.IsResponse() {
					hst.broadcastSSEEvent("response", msg)
				}
			}
		case <-hst.ctx.Done():
			return
		}
	}
}

// bridgeRequests forwards requests from message handler
func (hst *StreamableHTTPServerTransport) bridgeRequests(requests <-chan *shared.Request) {
	for {
		select {
		case req := <-requests:
			if req != nil {
				select {
				case hst.requestChan <- req:
				case <-hst.ctx.Done():
					return
				}
			}
		case <-hst.ctx.Done():
			return
		}
	}
}

// bridgeNotifications forwards notifications from message handler
func (hst *StreamableHTTPServerTransport) bridgeNotifications(notifications <-chan *shared.Notification) {
	for {
		select {
		case notif := <-notifications:
			if notif != nil {
				select {
				case hst.notificationChan <- notif:
				case <-hst.ctx.Done():
					return
				}
			}
		case <-hst.ctx.Done():
			return
		}
	}
}

// Close gracefully shuts down the HTTP server transport
func (hst *StreamableHTTPServerTransport) Close() error {
	var err error
	hst.closeOnce.Do(func() {
		// Shutdown HTTP server
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if shutdownErr := hst.httpServer.Shutdown(shutdownCtx); shutdownErr != nil {
			err = shutdownErr
		}

		// Close all SSE connections
		hst.sseMu.Lock()
		for _, client := range hst.sseClients {
			close(client.Done)
		}
		hst.sseMu.Unlock()

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

// Stats returns transport statistics
func (hst *StreamableHTTPServerTransport) Stats() interface{} {
	messageStats := hst.messageHandler.GetStats()
	
	hst.sseMu.RLock()
	activeSSEClients := len(hst.sseClients)
	hst.sseMu.RUnlock()

	return StreamableHTTPServerTransportStats{
		RequestsReceived: atomic.LoadInt64(&hst.requestsReceived),
		ResponsesSent:    atomic.LoadInt64(&hst.responsesSent),
		SSEConnections:   atomic.LoadInt64(&hst.sseConnections),
		ActiveSSEClients: activeSSEClients,
		HTTPErrors:       atomic.LoadInt64(&hst.httpErrors),
		MessageStats:     messageStats,
	}
}

// StreamableHTTPServerTransportStats represents server transport statistics
type StreamableHTTPServerTransportStats struct {
	RequestsReceived int64
	ResponsesSent    int64
	SSEConnections   int64
	ActiveSSEClients int
	HTTPErrors       int64
	MessageStats     internal.MessageStats
}
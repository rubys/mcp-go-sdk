package transport

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rubys/mcp-go-sdk/internal"
	"github.com/rubys/mcp-go-sdk/shared"
)

// WebSocketTransport implements WebSocket transport for MCP
type WebSocketTransport struct {
	// WebSocket connection
	conn   *websocket.Conn
	connMu sync.RWMutex

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
	url              string
	requestHeader    http.Header
	reconnectEnabled bool
	reconnectDelay   time.Duration
	maxReconnects    int
	pingInterval     time.Duration
	pongTimeout      time.Duration
	writeTimeout     time.Duration
	readTimeout      time.Duration

	// Connection state
	connected      int32 // atomic
	reconnectCount int
	lastConnectTime time.Time

	// Statistics
	messagesSent     int64
	messagesReceived int64
	reconnects       int64
	connectionErrors int64
}

// WebSocketConfig configures the WebSocket transport
type WebSocketConfig struct {
	URL              string
	RequestHeader    http.Header
	RequestTimeout   time.Duration
	MessageBuffer    int
	ReconnectEnabled bool
	ReconnectDelay   time.Duration
	MaxReconnects    int
	PingInterval     time.Duration
	PongTimeout      time.Duration
	WriteTimeout     time.Duration
	ReadTimeout      time.Duration
}

// WebSocketStats represents WebSocket transport statistics
type WebSocketStats struct {
	MessagesSent     int64 `json:"messagesSent"`
	MessagesReceived int64 `json:"messagesReceived"`
	Reconnects       int64 `json:"reconnects"`
	ConnectionErrors int64 `json:"connectionErrors"`
	Connected        bool  `json:"connected"`
	ReconnectCount   int   `json:"reconnectCount"`
	LastConnectTime  time.Time `json:"lastConnectTime"`
}

// NewWebSocketTransport creates a new WebSocket transport
func NewWebSocketTransport(ctx context.Context, config WebSocketConfig) (*WebSocketTransport, error) {
	// Validate URL
	if config.URL == "" {
		return nil, fmt.Errorf("WebSocket URL is required")
	}
	
	parsedURL, err := url.Parse(config.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid WebSocket URL: %w", err)
	}
	
	// Check if URL scheme is valid for WebSocket
	if parsedURL.Scheme != "ws" && parsedURL.Scheme != "wss" {
		return nil, fmt.Errorf("invalid WebSocket URL: scheme must be ws or wss")
	}

	// Set defaults
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 30 * time.Second
	}
	if config.MessageBuffer == 0 {
		config.MessageBuffer = 100
	}
	if config.ReconnectDelay == 0 {
		config.ReconnectDelay = 5 * time.Second
	}
	if config.MaxReconnects == 0 {
		config.MaxReconnects = 10
	}
	if config.PingInterval == 0 {
		config.PingInterval = 30 * time.Second
	}
	if config.PongTimeout == 0 {
		config.PongTimeout = 10 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 10 * time.Second
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = 60 * time.Second
	}

	ctx, cancel := context.WithCancel(ctx)

	// Create message handler
	messageHandler := internal.NewMessageHandler(ctx, internal.MessageHandlerConfig{
		RequestTimeout: config.RequestTimeout,
		BufferSize:     config.MessageBuffer,
	})

	// Get channels from message handler
	_, _, requests, notifications := messageHandler.Channels()

	transport := &WebSocketTransport{
		messageHandler:   messageHandler,
		requestChan:      make(chan *shared.Request, config.MessageBuffer),
		notificationChan: make(chan *shared.Notification, config.MessageBuffer),
		ctx:              ctx,
		cancel:           cancel,
		url:              config.URL,
		requestHeader:    config.RequestHeader,
		reconnectEnabled: config.ReconnectEnabled,
		reconnectDelay:   config.ReconnectDelay,
		maxReconnects:    config.MaxReconnects,
		pingInterval:     config.PingInterval,
		pongTimeout:      config.PongTimeout,
		writeTimeout:     config.WriteTimeout,
		readTimeout:      config.ReadTimeout,
	}

	// Copy channels from message handler
	go func() {
		for {
			select {
			case req := <-requests:
				if req != nil {
					select {
					case transport.requestChan <- req:
					case <-ctx.Done():
						return
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		for {
			select {
			case notif := <-notifications:
				if notif != nil {
					select {
					case transport.notificationChan <- notif:
					case <-ctx.Done():
						return
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return transport, nil
}

// Connect establishes the WebSocket connection
func (wst *WebSocketTransport) Connect() error {
	wst.connMu.Lock()
	defer wst.connMu.Unlock()

	if wst.conn != nil {
		return fmt.Errorf("already connected")
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(wst.url, wst.requestHeader)
	if err != nil {
		atomic.AddInt64(&wst.connectionErrors, 1)
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	wst.conn = conn
	wst.lastConnectTime = time.Now()
	atomic.StoreInt32(&wst.connected, 1)

	// Configure connection
	conn.SetReadDeadline(time.Now().Add(wst.readTimeout))
	conn.SetWriteDeadline(time.Now().Add(wst.writeTimeout))

	// Set up ping/pong handling
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(wst.readTimeout))
		return nil
	})

	// Start connection handlers
	wst.wg.Add(3)
	go wst.readLoop()
	go wst.writeLoop()
	go wst.pingLoop()

	return nil
}

// Start starts the WebSocket transport (alias for Connect for compatibility)
func (wst *WebSocketTransport) Start(ctx context.Context) error {
	return wst.Connect()
}

// readLoop reads messages from the WebSocket connection
func (wst *WebSocketTransport) readLoop() {
	defer wst.wg.Done()
	defer wst.handleDisconnection()

	incoming, _, _, _ := wst.messageHandler.Channels()

	for {
		select {
		case <-wst.ctx.Done():
			return
		default:
		}

		wst.connMu.RLock()
		conn := wst.conn
		wst.connMu.RUnlock()

		if conn == nil {
			return
		}

		// Read message
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				atomic.AddInt64(&wst.connectionErrors, 1)
			}
			return
		}

		atomic.AddInt64(&wst.messagesReceived, 1)

		// Forward to message handler
		select {
		case incoming <- message:
		case <-wst.ctx.Done():
			return
		}
	}
}

// writeLoop writes messages to the WebSocket connection
func (wst *WebSocketTransport) writeLoop() {
	defer wst.wg.Done()

	_, outgoing, _, _ := wst.messageHandler.Channels()

	for {
		select {
		case message := <-outgoing:
			if err := wst.writeMessage(message); err != nil {
				atomic.AddInt64(&wst.connectionErrors, 1)
				return
			}
		case <-wst.ctx.Done():
			return
		}
	}
}

// writeMessage writes a message to the WebSocket connection
func (wst *WebSocketTransport) writeMessage(message []byte) error {
	wst.connMu.Lock()
	conn := wst.conn
	wst.connMu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	conn.SetWriteDeadline(time.Now().Add(wst.writeTimeout))
	
	if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
		return err
	}

	atomic.AddInt64(&wst.messagesSent, 1)
	return nil
}

// pingLoop sends periodic ping messages
func (wst *WebSocketTransport) pingLoop() {
	defer wst.wg.Done()

	ticker := time.NewTicker(wst.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := wst.ping(); err != nil {
				return
			}
		case <-wst.ctx.Done():
			return
		}
	}
}

// ping sends a ping message
func (wst *WebSocketTransport) ping() error {
	wst.connMu.Lock()
	conn := wst.conn
	wst.connMu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	conn.SetWriteDeadline(time.Now().Add(wst.writeTimeout))
	return conn.WriteMessage(websocket.PingMessage, nil)
}

// handleDisconnection handles connection loss and potential reconnection
func (wst *WebSocketTransport) handleDisconnection() {
	atomic.StoreInt32(&wst.connected, 0)

	wst.connMu.Lock()
	if wst.conn != nil {
		wst.conn.Close()
		wst.conn = nil
	}
	wst.connMu.Unlock()

	// Attempt reconnection if enabled
	if wst.reconnectEnabled && wst.reconnectCount < wst.maxReconnects {
		wst.reconnectCount++
		atomic.AddInt64(&wst.reconnects, 1)

		go func() {
			time.Sleep(wst.reconnectDelay)
			
			select {
			case <-wst.ctx.Done():
				return
			default:
			}

			if err := wst.Connect(); err != nil {
				atomic.AddInt64(&wst.connectionErrors, 1)
			} else {
				wst.reconnectCount = 0 // Reset on successful reconnection
			}
		}()
	}
}

// SendRequest sends a request and returns a channel for the response
func (wst *WebSocketTransport) SendRequest(method string, params interface{}) (<-chan *shared.Response, error) {
	if atomic.LoadInt32(&wst.connected) == 0 {
		return nil, fmt.Errorf("not connected")
	}
	return wst.messageHandler.SendRequest(method, params)
}

// SendNotification sends a notification
func (wst *WebSocketTransport) SendNotification(method string, params interface{}) error {
	if atomic.LoadInt32(&wst.connected) == 0 {
		return fmt.Errorf("not connected")
	}
	return wst.messageHandler.SendNotification(method, params)
}

// SendResponse sends a response to a request
func (wst *WebSocketTransport) SendResponse(id interface{}, result interface{}, err *shared.RPCError) error {
	if atomic.LoadInt32(&wst.connected) == 0 {
		return fmt.Errorf("not connected")
	}
	return wst.messageHandler.SendResponse(id, result, err)
}

// Channels returns channels for receiving incoming requests and notifications
func (wst *WebSocketTransport) Channels() (requests <-chan *shared.Request, notifications <-chan *shared.Notification) {
	return wst.requestChan, wst.notificationChan
}

// Close gracefully shuts down the transport
func (wst *WebSocketTransport) Close() error {
	var err error
	wst.closeOnce.Do(func() {
		// Cancel context to stop all goroutines
		wst.cancel()

		// Close WebSocket connection
		wst.connMu.Lock()
		conn := wst.conn
		if conn != nil {
			wst.conn = nil
		}
		wst.connMu.Unlock()

		// Close connection outside of mutex to prevent race
		if conn != nil {
			// Send close message - ignore errors since connection might be broken
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			conn.Close()
		}

		// Wait for goroutines to finish
		done := make(chan struct{})
		go func() {
			wst.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			err = fmt.Errorf("timeout waiting for goroutines to finish")
		}

		// Close message handler
		if wst.messageHandler != nil {
			wst.messageHandler.Close()
		}

		atomic.StoreInt32(&wst.connected, 0)
	})
	return err
}

// Stats returns transport statistics
func (wst *WebSocketTransport) Stats() interface{} {
	return WebSocketStats{
		MessagesSent:     atomic.LoadInt64(&wst.messagesSent),
		MessagesReceived: atomic.LoadInt64(&wst.messagesReceived),
		Reconnects:       atomic.LoadInt64(&wst.reconnects),
		ConnectionErrors: atomic.LoadInt64(&wst.connectionErrors),
		Connected:        atomic.LoadInt32(&wst.connected) == 1,
		ReconnectCount:   wst.reconnectCount,
		LastConnectTime:  wst.lastConnectTime,
	}
}

// IsConnected returns whether the transport is connected
func (wst *WebSocketTransport) IsConnected() bool {
	return atomic.LoadInt32(&wst.connected) == 1
}

// GetURL returns the WebSocket URL
func (wst *WebSocketTransport) GetURL() string {
	return wst.url
}

// SetReconnectEnabled enables or disables automatic reconnection
func (wst *WebSocketTransport) SetReconnectEnabled(enabled bool) {
	wst.reconnectEnabled = enabled
}

// ResetReconnectCount resets the reconnection counter
func (wst *WebSocketTransport) ResetReconnectCount() {
	wst.reconnectCount = 0
}
package transport

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rubys/mcp-go-sdk/internal"
	"github.com/rubys/mcp-go-sdk/shared"
)

// SSETransport implements Server-Sent Events transport
type SSETransport struct {
	// SSE configuration
	sseEndpoint       string
	httpEndpoint      string
	headers           map[string]string
	reconnectDelay    time.Duration
	maxReconnects     int
	heartbeatInterval time.Duration

	// HTTP client for both SSE and POST
	client     *http.Client
	connection *http.Response
	reader     *bufio.Scanner

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
	requestTimeout time.Duration
	messageBuffer  int

	// Connection state
	connected     int64
	reconnectCount int64

	// Statistics
	messagesReceived int64
	reconnects       int64
	connectionErrors int64
	messagesSent     int64
}

// SSEConfig configures the SSE transport (dual endpoints)
type SSEConfig struct {
	SSEEndpoint       string            // SSE endpoint for receiving messages
	HTTPEndpoint      string            // HTTP POST endpoint for sending messages
	Headers           map[string]string
	ReconnectDelay    time.Duration
	MaxReconnects     int
	HeartbeatInterval time.Duration
	RequestTimeout    time.Duration
	MessageBuffer     int
	Timeout           time.Duration
}

// NewSSETransport creates a new SSE transport
func NewSSETransport(ctx context.Context, config SSEConfig) (*SSETransport, error) {
	if config.SSEEndpoint == "" {
		return nil, fmt.Errorf("SSE endpoint is required")
	}
	if config.HTTPEndpoint == "" {
		return nil, fmt.Errorf("HTTP endpoint is required")
	}

	// Set defaults
	if config.ReconnectDelay == 0 {
		config.ReconnectDelay = 1 * time.Second
	}
	if config.MaxReconnects == 0 {
		config.MaxReconnects = 10
	}
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = 30 * time.Second
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 30 * time.Second
	}
	if config.MessageBuffer == 0 {
		config.MessageBuffer = 100
	}
	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}

	ctx, cancel := context.WithCancel(ctx)

	transport := &SSETransport{
		sseEndpoint:       config.SSEEndpoint,
		httpEndpoint:      config.HTTPEndpoint,
		headers:           config.Headers,
		reconnectDelay:    config.ReconnectDelay,
		maxReconnects:     config.MaxReconnects,
		heartbeatInterval: config.HeartbeatInterval,
		requestTimeout:    config.RequestTimeout,
		messageBuffer:     config.MessageBuffer,
		ctx:               ctx,
		cancel:            cancel,
		requestChan:       make(chan *shared.Request, config.MessageBuffer),
		notificationChan:  make(chan *shared.Notification, config.MessageBuffer),
	}

	// Create HTTP client
	transport.client = &http.Client{
		Timeout: config.Timeout,
	}

	// Create message handler
	messageHandler := internal.NewMessageHandler(ctx, internal.MessageHandlerConfig{
		RequestTimeout: config.RequestTimeout,
		BufferSize:     config.MessageBuffer,
	})
	transport.messageHandler = messageHandler

	// Start SSE connection for receiving
	transport.wg.Add(1)
	go transport.connectSSE()

	// Start HTTP sender for outgoing messages
	transport.wg.Add(1)
	go transport.handleOutgoingMessages()

	return transport, nil
}

// connectSSE establishes and maintains SSE connection
func (s *SSETransport) connectSSE() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		if err := s.establishConnection(); err != nil {
			atomic.AddInt64(&s.connectionErrors, 1)
			
			if atomic.LoadInt64(&s.reconnectCount) >= int64(s.maxReconnects) {
				return
			}

			atomic.AddInt64(&s.reconnectCount, 1)
			
			select {
			case <-time.After(s.reconnectDelay):
				continue
			case <-s.ctx.Done():
				return
			}
		}

		// Connection established, start reading events
		s.readEvents()
	}
}

// establishConnection creates SSE connection
func (s *SSETransport) establishConnection() error {
	req, err := http.NewRequestWithContext(s.ctx, "GET", s.sseEndpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set SSE headers
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	
	// Add custom headers
	for key, value := range s.headers {
		req.Header.Set(key, value)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	s.connection = resp
	s.reader = bufio.NewScanner(resp.Body)
	atomic.StoreInt64(&s.connected, 1)

	return nil
}

// readEvents reads SSE events
func (s *SSETransport) readEvents() {
	defer func() {
		if s.connection != nil {
			s.connection.Body.Close()
		}
		atomic.StoreInt64(&s.connected, 0)
	}()

	for s.reader.Scan() {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		line := s.reader.Text()
		
		// Handle SSE event format
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "" {
				continue // Skip empty data lines
			}

			atomic.AddInt64(&s.messagesReceived, 1)

			// Parse JSON-RPC message
			msg, err := shared.ParseMessage([]byte(data))
			if err != nil {
				continue // Skip invalid messages
			}

			// Route message
			if msg.IsRequest() {
				req := msg.ToRequest()
				select {
				case s.requestChan <- req:
				case <-s.ctx.Done():
					return
				}
			} else if msg.IsNotification() {
				notif := msg.ToNotification()
				select {
				case s.notificationChan <- notif:
				case <-s.ctx.Done():
					return
				}
			} else if msg.IsResponse() {
				// Handle responses through message handler by sending raw data
				// The message handler will parse and correlate responses
				incomingChan, _, _, _ := s.messageHandler.Channels()
				select {
				case incomingChan <- []byte(data):
				case <-s.ctx.Done():
					return
				}
			}
		}
	}

	// Connection closed or error
	if err := s.reader.Err(); err != nil {
		atomic.AddInt64(&s.connectionErrors, 1)
	}
}

// handleOutgoingMessages processes outgoing messages and sends them via HTTP POST
func (s *SSETransport) handleOutgoingMessages() {
	defer s.wg.Done()

	_, outgoingChan, _, _ := s.messageHandler.Channels()

	for {
		select {
		case data := <-outgoingChan:
			if err := s.sendHTTPMessage(data); err != nil {
				// Log error but continue processing
				atomic.AddInt64(&s.connectionErrors, 1)
			}
		case <-s.ctx.Done():
			return
		}
	}
}

// SendRequest sends a request via HTTP POST
func (s *SSETransport) SendRequest(method string, params interface{}) (<-chan *shared.Response, error) {
	return s.messageHandler.SendRequest(method, params)
}

// SendNotification sends a notification via HTTP POST
func (s *SSETransport) SendNotification(method string, params interface{}) error {
	return s.messageHandler.SendNotification(method, params)
}

// SendResponse sends a response via HTTP POST
func (s *SSETransport) SendResponse(id interface{}, result interface{}, err *shared.RPCError) error {
	return s.messageHandler.SendResponse(id, result, err)
}

// sendHTTPMessage sends a message via HTTP POST to the HTTP endpoint
func (s *SSETransport) sendHTTPMessage(data []byte) error {
	req, err := http.NewRequestWithContext(s.ctx, "POST", s.httpEndpoint, strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range s.headers {
		req.Header.Set(key, value)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP request failed with status: %d", resp.StatusCode)
	}

	atomic.AddInt64(&s.messagesSent, 1)
	return nil
}

// Channels returns the communication channels
func (s *SSETransport) Channels() (<-chan *shared.Request, <-chan *shared.Notification) {
	return s.requestChan, s.notificationChan
}

// Stats returns transport statistics
func (s *SSETransport) Stats() interface{} {
	return map[string]interface{}{
		"type":              "sse",
		"sse_endpoint":      s.sseEndpoint,
		"http_endpoint":     s.httpEndpoint,
		"connected":         atomic.LoadInt64(&s.connected) == 1,
		"messages_received": atomic.LoadInt64(&s.messagesReceived),
		"messages_sent":     atomic.LoadInt64(&s.messagesSent),
		"reconnects":        atomic.LoadInt64(&s.reconnects),
		"connection_errors": atomic.LoadInt64(&s.connectionErrors),
	}
}

// Close gracefully shuts down the transport
func (s *SSETransport) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.cancel()
		
		if s.connection != nil {
			s.connection.Body.Close()
		}
		
		if s.messageHandler != nil {
			s.messageHandler.Close()
		}
		
		s.wg.Wait()
	})
	return err
}
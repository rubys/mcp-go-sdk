package transport

import (
	"bufio"
	"context"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/internal"
	"github.com/modelcontextprotocol/go-sdk/shared"
)

// StdioTransport implements non-blocking stdio transport with concurrent I/O
type StdioTransport struct {
	// I/O streams
	reader io.Reader
	writer io.Writer

	// Message handler for concurrent processing
	messageHandler *internal.MessageHandler

	// Channels for external communication
	incomingChan     chan []byte
	outgoingChan     chan []byte
	requestChan      chan *shared.Request
	notificationChan chan *shared.Notification

	// Context and synchronization
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once

	// Configuration
	readBufferSize  int
	writeBufferSize int
	flushInterval   time.Duration

	// Statistics
	bytesRead    int64
	bytesWritten int64
	linesRead    int64
	linesWritten int64
}

// StdioConfig configures the stdio transport
type StdioConfig struct {
	Reader          io.Reader
	Writer          io.Writer
	ReadBufferSize  int
	WriteBufferSize int
	FlushInterval   time.Duration
	RequestTimeout  time.Duration
	MessageBuffer   int
}

// NewStdioTransport creates a new concurrent stdio transport
func NewStdioTransport(ctx context.Context, config StdioConfig) (*StdioTransport, error) {
	// Set defaults
	if config.Reader == nil {
		config.Reader = os.Stdin
	}
	if config.Writer == nil {
		config.Writer = os.Stdout
	}
	if config.ReadBufferSize == 0 {
		config.ReadBufferSize = 64 * 1024 // 64KB
	}
	if config.WriteBufferSize == 0 {
		config.WriteBufferSize = 64 * 1024 // 64KB
	}
	if config.FlushInterval == 0 {
		config.FlushInterval = 10 * time.Millisecond
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

	// Get channels from message handler
	incoming, outgoing, requests, notifications := messageHandler.Channels()

	transport := &StdioTransport{
		reader:           config.Reader,
		writer:           config.Writer,
		messageHandler:   messageHandler,
		incomingChan:     make(chan []byte, config.MessageBuffer),
		outgoingChan:     make(chan []byte, config.MessageBuffer),
		requestChan:      make(chan *shared.Request, config.MessageBuffer),
		notificationChan: make(chan *shared.Notification, config.MessageBuffer),
		ctx:              ctx,
		cancel:           cancel,
		readBufferSize:   config.ReadBufferSize,
		writeBufferSize:  config.WriteBufferSize,
		flushInterval:    config.FlushInterval,
	}

	// Start all goroutines
	transport.wg.Add(6) // 2 I/O + 4 bridge goroutines

	// Connect transport channels to message handler
	go func() {
		defer transport.wg.Done()
		transport.bridgeIncoming(incoming)
	}()
	go func() {
		defer transport.wg.Done()
		transport.bridgeOutgoing(outgoing)
	}()
	go func() {
		defer transport.wg.Done()
		transport.bridgeRequests(requests)
	}()
	go func() {
		defer transport.wg.Done()
		transport.bridgeNotifications(notifications)
	}()

	// Start I/O goroutines
	go transport.readLoop()
	go transport.writeLoop()

	return transport, nil
}

// Channels returns the transport's communication channels
func (st *StdioTransport) Channels() (
	requests <-chan *shared.Request,
	notifications <-chan *shared.Notification,
) {
	return st.requestChan, st.notificationChan
}

// SendRequest sends a request and returns a response channel
func (st *StdioTransport) SendRequest(method string, params interface{}) (<-chan *shared.Response, error) {
	return st.messageHandler.SendRequest(method, params)
}

// SendNotification sends a notification
func (st *StdioTransport) SendNotification(method string, params interface{}) error {
	return st.messageHandler.SendNotification(method, params)
}

// SendResponse sends a response to a request
func (st *StdioTransport) SendResponse(id interface{}, result interface{}, err *shared.RPCError) error {
	return st.messageHandler.SendResponse(id, result, err)
}

// readLoop continuously reads from stdin in a separate goroutine
func (st *StdioTransport) readLoop() {
	defer st.wg.Done()

	scanner := bufio.NewScanner(st.reader)
	scanner.Buffer(make([]byte, st.readBufferSize), st.readBufferSize)

	for {
		select {
		case <-st.ctx.Done():
			return
		default:
			if scanner.Scan() {
				line := scanner.Bytes()
				if len(line) > 0 {
					// Make a copy since scanner reuses the buffer
					data := make([]byte, len(line))
					copy(data, line)

					atomic.AddInt64(&st.linesRead, 1)
					atomic.AddInt64(&st.bytesRead, int64(len(data)))

					select {
					case st.incomingChan <- data:
					case <-st.ctx.Done():
						return
					}
				}
			} else {
				if err := scanner.Err(); err != nil {
					// Handle read error - could send error notification
					if err != io.EOF {
						// Log error or send error notification
					}
				}
				return // EOF or error
			}
		}
	}
}

// writeLoop continuously writes to stdout in a separate goroutine
func (st *StdioTransport) writeLoop() {
	defer st.wg.Done()

	writer := bufio.NewWriterSize(st.writer, st.writeBufferSize)
	ticker := time.NewTicker(st.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case data := <-st.outgoingChan:
			// Write the message followed by newline
			if _, err := writer.Write(data); err != nil {
				// Handle write error
				continue
			}
			if err := writer.WriteByte('\n'); err != nil {
				// Handle write error
				continue
			}

			atomic.AddInt64(&st.linesWritten, 1)
			atomic.AddInt64(&st.bytesWritten, int64(len(data)+1))

			// Flush immediately for interactive use
			writer.Flush()

		case <-ticker.C:
			// Periodic flush to ensure data is sent
			writer.Flush()

		case <-st.ctx.Done():
			// Final flush before exit
			writer.Flush()
			return
		}
	}
}

// Bridge functions to connect transport channels to message handler

func (st *StdioTransport) bridgeIncoming(messageHandlerIncoming chan<- []byte) {
	for {
		select {
		case data := <-st.incomingChan:
			select {
			case messageHandlerIncoming <- data:
			case <-st.ctx.Done():
				return
			}
		case <-st.ctx.Done():
			return
		}
	}
}

func (st *StdioTransport) bridgeOutgoing(messageHandlerOutgoing <-chan []byte) {
	for {
		select {
		case data := <-messageHandlerOutgoing:
			select {
			case st.outgoingChan <- data:
			case <-st.ctx.Done():
				return
			}
		case <-st.ctx.Done():
			return
		}
	}
}

func (st *StdioTransport) bridgeRequests(messageHandlerRequests <-chan *shared.Request) {
	for {
		select {
		case req := <-messageHandlerRequests:
			select {
			case st.requestChan <- req:
			case <-st.ctx.Done():
				return
			}
		case <-st.ctx.Done():
			return
		}
	}
}

func (st *StdioTransport) bridgeNotifications(messageHandlerNotifications <-chan *shared.Notification) {
	for {
		select {
		case notif := <-messageHandlerNotifications:
			select {
			case st.notificationChan <- notif:
			case <-st.ctx.Done():
				return
			}
		case <-st.ctx.Done():
			return
		}
	}
}

// Close gracefully shuts down the transport
func (st *StdioTransport) Close() error {
	var err error
	st.closeOnce.Do(func() {
		// Cancel context to stop all goroutines
		st.cancel()

		// Wait for I/O goroutines to finish
		st.wg.Wait()

		// Close message handler first
		if closeErr := st.messageHandler.Close(); closeErr != nil {
			err = closeErr
		}

		// Close channels - the wg.Wait() ensures goroutines are done
		close(st.incomingChan)
		close(st.outgoingChan)
		close(st.requestChan)
		close(st.notificationChan)
	})
	return err
}

// Stats returns transport statistics
func (st *StdioTransport) Stats() TransportStats {
	messageStats := st.messageHandler.GetStats()
	return TransportStats{
		BytesRead:         atomic.LoadInt64(&st.bytesRead),
		BytesWritten:      atomic.LoadInt64(&st.bytesWritten),
		LinesRead:         atomic.LoadInt64(&st.linesRead),
		LinesWritten:      atomic.LoadInt64(&st.linesWritten),
		RequestsSent:      messageStats.RequestsSent,
		ResponsesReceived: messageStats.ResponsesReceived,
		NotificationsSent: messageStats.NotificationsSent,
		Errors:            messageStats.Errors,
		Timeouts:          messageStats.Timeouts,
	}
}

// TransportStats represents transport-level statistics
type TransportStats struct {
	BytesRead         int64
	BytesWritten      int64
	LinesRead         int64
	LinesWritten      int64
	RequestsSent      int64
	ResponsesReceived int64
	NotificationsSent int64
	Errors            int64
	Timeouts          int64
}

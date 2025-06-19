package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/shared"
)

// RequestID generates unique request IDs
type RequestID struct {
	counter int64
}

func NewRequestID() *RequestID {
	return &RequestID{}
}

func (r *RequestID) Next() interface{} {
	return atomic.AddInt64(&r.counter, 1)
}

// PendingRequest represents a request waiting for response
type PendingRequest struct {
	ID       interface{}
	Response chan *shared.Response
	Cancel   context.CancelFunc
	Timer    *time.Timer
}

// MessageHandler handles concurrent JSON-RPC message processing
type MessageHandler struct {
	// Channels for message flow
	incomingChan     chan []byte
	outgoingChan     chan []byte
	requestChan      chan *shared.Request
	notificationChan chan *shared.Notification
	responseChan     chan *shared.Response

	// Request correlation
	pendingRequests map[interface{}]*PendingRequest
	requestsMu      sync.RWMutex
	requestID       *RequestID

	// Context and cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Configuration
	requestTimeout time.Duration
	bufferSize     int

	// Statistics
	stats MessageStats
}

// MessageStats tracks message handling statistics
type MessageStats struct {
	RequestsSent      int64
	ResponsesReceived int64
	NotificationsSent int64
	Errors            int64
	Timeouts          int64
}

// MessageHandlerConfig configures the message handler
type MessageHandlerConfig struct {
	RequestTimeout time.Duration
	BufferSize     int
}

// NewMessageHandler creates a new concurrent message handler
func NewMessageHandler(ctx context.Context, config MessageHandlerConfig) *MessageHandler {
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 30 * time.Second
	}
	if config.BufferSize == 0 {
		config.BufferSize = 100
	}

	ctx, cancel := context.WithCancel(ctx)

	mh := &MessageHandler{
		incomingChan:     make(chan []byte, config.BufferSize),
		outgoingChan:     make(chan []byte, config.BufferSize),
		requestChan:      make(chan *shared.Request, config.BufferSize),
		notificationChan: make(chan *shared.Notification, config.BufferSize),
		responseChan:     make(chan *shared.Response, config.BufferSize),
		pendingRequests:  make(map[interface{}]*PendingRequest),
		requestID:        NewRequestID(),
		ctx:              ctx,
		cancel:           cancel,
		requestTimeout:   config.RequestTimeout,
		bufferSize:       config.BufferSize,
	}

	// Start message processing goroutines
	go mh.processIncomingMessages()
	go mh.handleResponses()

	return mh
}

// Channels returns the message handler channels for external use
func (mh *MessageHandler) Channels() (
	incoming chan<- []byte,
	outgoing <-chan []byte,
	requests <-chan *shared.Request,
	notifications <-chan *shared.Notification,
) {
	return mh.incomingChan, mh.outgoingChan, mh.requestChan, mh.notificationChan
}

// SendRequest sends a request and returns a channel for the response
func (mh *MessageHandler) SendRequest(method string, params interface{}) (<-chan *shared.Response, error) {
	select {
	case <-mh.ctx.Done():
		return nil, mh.ctx.Err()
	default:
	}

	id := mh.requestID.Next()
	request := &shared.Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	// Create response channel and pending request
	responseChan := make(chan *shared.Response, 1)
	_, cancel := context.WithTimeout(mh.ctx, mh.requestTimeout)

	timer := time.AfterFunc(mh.requestTimeout, func() {
		atomic.AddInt64(&mh.stats.Timeouts, 1)
		mh.removePendingRequest(id)
		cancel()
		close(responseChan)
	})

	pendingReq := &PendingRequest{
		ID:       id,
		Response: responseChan,
		Cancel:   cancel,
		Timer:    timer,
	}

	// Store pending request
	mh.requestsMu.Lock()
	mh.pendingRequests[id] = pendingReq
	mh.requestsMu.Unlock()

	// Marshal and send request
	data, err := json.Marshal(request)
	if err != nil {
		mh.removePendingRequest(id)
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	select {
	case mh.outgoingChan <- data:
		atomic.AddInt64(&mh.stats.RequestsSent, 1)
		return responseChan, nil
	case <-mh.ctx.Done():
		mh.removePendingRequest(id)
		return nil, mh.ctx.Err()
	}
}

// SendNotification sends a notification (no response expected)
func (mh *MessageHandler) SendNotification(method string, params interface{}) error {
	select {
	case <-mh.ctx.Done():
		return mh.ctx.Err()
	default:
	}

	notification := &shared.Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	select {
	case mh.outgoingChan <- data:
		atomic.AddInt64(&mh.stats.NotificationsSent, 1)
		return nil
	case <-mh.ctx.Done():
		return mh.ctx.Err()
	}
}

// SendResponse sends a response to a request
func (mh *MessageHandler) SendResponse(id interface{}, result interface{}, err *shared.RPCError) error {
	select {
	case <-mh.ctx.Done():
		return mh.ctx.Err()
	default:
	}

	response := &shared.Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   err,
	}

	data, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal response: %w", marshalErr)
	}

	select {
	case mh.outgoingChan <- data:
		return nil
	case <-mh.ctx.Done():
		return mh.ctx.Err()
	}
}

// processIncomingMessages processes incoming JSON-RPC messages
func (mh *MessageHandler) processIncomingMessages() {
	for {
		select {
		case data := <-mh.incomingChan:
			mh.handleIncomingMessage(data)
		case <-mh.ctx.Done():
			return
		}
	}
}

// handleIncomingMessage parses and routes incoming messages
func (mh *MessageHandler) handleIncomingMessage(data []byte) {
	msg, err := shared.ParseMessage(data)
	if err != nil {
		atomic.AddInt64(&mh.stats.Errors, 1)
		return
	}

	if msg.IsRequest() {
		if req := msg.ToRequest(); req != nil {
			select {
			case mh.requestChan <- req:
			case <-mh.ctx.Done():
				return
			}
		}
	} else if msg.IsNotification() {
		if notif := msg.ToNotification(); notif != nil {
			select {
			case mh.notificationChan <- notif:
			case <-mh.ctx.Done():
				return
			}
		}
	} else if msg.IsResponse() {
		if resp := msg.ToResponse(); resp != nil {
			select {
			case mh.responseChan <- resp:
			case <-mh.ctx.Done():
				return
			}
		}
	}
}

// handleResponses correlates responses with pending requests
func (mh *MessageHandler) handleResponses() {
	for {
		select {
		case response := <-mh.responseChan:
			mh.correlateResponse(response)
		case <-mh.ctx.Done():
			return
		}
	}
}

// correlateResponse matches responses to pending requests
func (mh *MessageHandler) correlateResponse(response *shared.Response) {
	mh.requestsMu.Lock()

	// Handle ID type conversion: JSON parsing converts numbers to float64,
	// but we generate IDs as int64. Try both the original ID and converted ID.
	var pending *PendingRequest
	var exists bool

	// First try the original ID
	pending, exists = mh.pendingRequests[response.ID]

	// If not found and ID is float64, try converting to int64
	if !exists {
		if floatID, ok := response.ID.(float64); ok {
			intID := int64(floatID)
			pending, exists = mh.pendingRequests[intID]
			if exists {
				// Update the response ID to match the original type
				response.ID = intID
			}
		}
	}

	if exists {
		delete(mh.pendingRequests, pending.ID)
	}
	mh.requestsMu.Unlock()

	if exists {
		pending.Timer.Stop()
		pending.Cancel()

		select {
		case pending.Response <- response:
			atomic.AddInt64(&mh.stats.ResponsesReceived, 1)
		default:
			// Channel was closed (timeout occurred)
		}
		close(pending.Response)
	}
}

// removePendingRequest removes a pending request (used for timeouts)
func (mh *MessageHandler) removePendingRequest(id interface{}) {
	mh.requestsMu.Lock()
	if pending, exists := mh.pendingRequests[id]; exists {
		delete(mh.pendingRequests, id)
		pending.Timer.Stop()
		pending.Cancel()
	}
	mh.requestsMu.Unlock()
}

// Close shuts down the message handler gracefully
func (mh *MessageHandler) Close() error {
	mh.cancel()

	// Cancel all pending requests
	mh.requestsMu.Lock()
	for _, pending := range mh.pendingRequests {
		pending.Timer.Stop()
		pending.Cancel()
		close(pending.Response)
	}
	mh.pendingRequests = make(map[interface{}]*PendingRequest)
	mh.requestsMu.Unlock()

	return nil
}

// GetStats returns current message handling statistics
func (mh *MessageHandler) GetStats() MessageStats {
	return MessageStats{
		RequestsSent:      atomic.LoadInt64(&mh.stats.RequestsSent),
		ResponsesReceived: atomic.LoadInt64(&mh.stats.ResponsesReceived),
		NotificationsSent: atomic.LoadInt64(&mh.stats.NotificationsSent),
		Errors:            atomic.LoadInt64(&mh.stats.Errors),
		Timeouts:          atomic.LoadInt64(&mh.stats.Timeouts),
	}
}

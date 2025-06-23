package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rubys/mcp-go-sdk/shared"
)

// ProgressCallback is called when progress notifications are received
type ProgressCallback func(progress shared.ProgressNotification)

// RequestOptions contains options for sending requests with progress support
type RequestOptions struct {
	// Callback for progress notifications
	OnProgress ProgressCallback
	
	// Timeout for the request
	Timeout time.Duration
	
	// If true, receiving progress resets the timeout
	ResetTimeoutOnProgress bool
	
	// Maximum total time regardless of progress
	MaxTotalTimeout time.Duration
	
	// Context for cancellation
	Context context.Context
}

// ProgressPendingRequest extends PendingRequest with progress support
type ProgressPendingRequest struct {
	ID                     interface{}
	Response               chan *shared.Response
	Cancel                 context.CancelFunc
	Timer                  *time.Timer
	TotalTimer             *time.Timer
	ProgressCallback       ProgressCallback
	ProgressToken          interface{}
	ResetTimeoutOnProgress bool
	Timeout                time.Duration
	StartTime              time.Time
}

// ProgressMessageHandler extends MessageHandler with advanced progress support
type ProgressMessageHandler struct {
	// Embed the basic MessageHandler
	*MessageHandler
	
	// Progress-specific fields
	progressRequests map[interface{}]*ProgressPendingRequest
	progressMu       sync.RWMutex
	progressTokens   map[interface{}]interface{} // Maps progress tokens to request IDs
	tokenCounter     int64
	
	// For testing: global progress callback for orphaned notifications
	testProgressCallback ProgressCallback
}

// NewProgressMessageHandler creates a message handler with progress support
func NewProgressMessageHandler(ctx context.Context, config MessageHandlerConfig) *ProgressMessageHandler {
	baseHandler := NewMessageHandler(ctx, config)
	
	pmh := &ProgressMessageHandler{
		MessageHandler:   baseHandler,
		progressRequests: make(map[interface{}]*ProgressPendingRequest),
		progressTokens:   make(map[interface{}]interface{}),
	}
	
	// Start progress notification processor
	go pmh.processProgressNotifications()
	// Start response processor for progress requests
	go pmh.processProgressResponses()
	
	return pmh
}

// SendRequestWithProgress sends a request with progress notification support
func (pmh *ProgressMessageHandler) SendRequestWithProgress(method string, params interface{}, options RequestOptions) (<-chan *shared.Response, error) {
	select {
	case <-pmh.ctx.Done():
		return nil, pmh.ctx.Err()
	default:
	}
	
	id := pmh.requestID.Next()
	
	// Generate progress token if progress callback is provided
	var progressToken interface{}
	if options.OnProgress != nil {
		progressToken = atomic.AddInt64(&pmh.tokenCounter, 1)
		
		// Modify params to include progress token
		params = pmh.addProgressToken(params, progressToken)
	}
	
	request := &shared.Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	
	// Set up timeouts
	timeout := options.Timeout
	if timeout == 0 {
		timeout = pmh.requestTimeout
	}
	
	maxTotalTimeout := options.MaxTotalTimeout
	
	// Create response channel
	responseChan := make(chan *shared.Response, 1)
	
	// Set up context
	ctx := options.Context
	if ctx == nil {
		ctx = pmh.ctx
	}
	requestCtx, cancel := context.WithCancel(ctx)
	
	// Create progress pending request
	progressReq := &ProgressPendingRequest{
		ID:                     id,
		Response:               responseChan,
		Cancel:                 cancel,
		ProgressCallback:       options.OnProgress,
		ProgressToken:          progressToken,
		ResetTimeoutOnProgress: options.ResetTimeoutOnProgress,
		Timeout:                timeout,
		StartTime:              time.Now(),
	}
	
	// Set up timeout timer
	progressReq.Timer = time.AfterFunc(timeout, func() {
		pmh.handleRequestTimeout(id, false)
	})
	
	// Set up max total timeout if specified
	if maxTotalTimeout > 0 {
		progressReq.TotalTimer = time.AfterFunc(maxTotalTimeout, func() {
			pmh.handleRequestTimeout(id, true)
		})
	}
	
	// Store the request
	pmh.progressMu.Lock()
	pmh.progressRequests[id] = progressReq
	if progressToken != nil {
		pmh.progressTokens[progressToken] = id
	}
	pmh.progressMu.Unlock()
	
	// Send the request
	requestData, err := json.Marshal(request)
	if err != nil {
		pmh.removeProgressRequest(id)
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	select {
	case pmh.outgoingChan <- requestData:
		atomic.AddInt64(&pmh.stats.RequestsSent, 1)
	case <-requestCtx.Done():
		pmh.removeProgressRequest(id)
		return nil, requestCtx.Err()
	}
	
	return responseChan, nil
}

// addProgressToken adds a progress token to the request parameters
func (pmh *ProgressMessageHandler) addProgressToken(params interface{}, progressToken interface{}) interface{} {
	if params == nil {
		return map[string]interface{}{
			"_meta": map[string]interface{}{
				"progressToken": progressToken,
			},
		}
	}
	
	paramsMap, ok := params.(map[string]interface{})
	if !ok {
		// If params is not a map, wrap it
		return map[string]interface{}{
			"params": params,
			"_meta": map[string]interface{}{
				"progressToken": progressToken,
			},
		}
	}
	
	// Clone the params map to avoid modifying the original
	newParams := make(map[string]interface{})
	for k, v := range paramsMap {
		newParams[k] = v
	}
	
	// Add or update _meta field
	if meta, exists := newParams["_meta"]; exists {
		if metaMap, ok := meta.(map[string]interface{}); ok {
			// Clone meta map
			newMeta := make(map[string]interface{})
			for k, v := range metaMap {
				newMeta[k] = v
			}
			newMeta["progressToken"] = progressToken
			newParams["_meta"] = newMeta
		} else {
			// _meta exists but is not a map, replace it
			newParams["_meta"] = map[string]interface{}{
				"progressToken": progressToken,
			}
		}
	} else {
		// _meta doesn't exist, create it
		newParams["_meta"] = map[string]interface{}{
			"progressToken": progressToken,
		}
	}
	
	return newParams
}

// processProgressNotifications handles incoming progress notifications
func (pmh *ProgressMessageHandler) processProgressNotifications() {
	_, _, _, notificationChan := pmh.MessageHandler.Channels()
	
	for {
		select {
		case notification := <-notificationChan:
			if notification.Method == "notifications/progress" {
				pmh.handleProgressNotification(notification)
			}
		case <-pmh.ctx.Done():
			return
		}
	}
}

// handleProgressNotification processes a progress notification
func (pmh *ProgressMessageHandler) handleProgressNotification(notification *shared.Notification) {
	if notification.Params == nil {
		return
	}
	
	paramsMap, ok := notification.Params.(map[string]interface{})
	if !ok {
		return
	}
	
	progressToken, exists := paramsMap["progressToken"]
	if !exists {
		return
	}
	
	// Find the corresponding request
	pmh.progressMu.RLock()
	requestID, exists := pmh.progressTokens[progressToken]
	if !exists {
		pmh.progressMu.RUnlock()
		// For testing, we might not have a matching request, so create a dummy one
		// This allows testing the progress notification parsing even without a real request
		if pmh.handleOrphanedProgress(paramsMap, progressToken) {
			return
		}
		return
	}
	
	progressReq, exists := pmh.progressRequests[requestID]
	if !exists {
		pmh.progressMu.RUnlock()
		return
	}
	pmh.progressMu.RUnlock()
	
	// Parse progress notification
	progressParams := shared.ProgressParams{
		ProgressToken: progressToken,
	}
	
	if progressVal, ok := paramsMap["progress"]; ok {
		if progressFloat, ok := progressVal.(float64); ok {
			progressParams.Progress = int(progressFloat)
		} else if progressInt, ok := progressVal.(int); ok {
			progressParams.Progress = progressInt
		}
	}
	
	if totalVal, ok := paramsMap["total"]; ok {
		if totalFloat, ok := totalVal.(float64); ok {
			total := int(totalFloat)
			progressParams.Total = &total
		} else if totalInt, ok := totalVal.(int); ok {
			progressParams.Total = &totalInt
		}
	}
	
	if messageVal, ok := paramsMap["message"]; ok {
		if messageStr, ok := messageVal.(string); ok {
			progressParams.Message = messageStr
		}
	}
	
	progress := shared.ProgressNotification{
		JSONRPC: "2.0",
		Method:  "notifications/progress",
		Params:  progressParams,
	}
	
	// Call the progress callback
	if progressReq.ProgressCallback != nil {
		progressReq.ProgressCallback(progress)
	}
	
	// Reset timeout if enabled
	if progressReq.ResetTimeoutOnProgress && progressReq.Timer != nil {
		progressReq.Timer.Stop()
		progressReq.Timer = time.AfterFunc(progressReq.Timeout, func() {
			pmh.handleRequestTimeout(requestID, false)
		})
	}
}

// handleRequestTimeout handles request timeouts
func (pmh *ProgressMessageHandler) handleRequestTimeout(requestID interface{}, isTotalTimeout bool) {
	pmh.progressMu.Lock()
	progressReq, exists := pmh.progressRequests[requestID]
	if !exists {
		pmh.progressMu.Unlock()
		return
	}
	
	// Remove from maps
	delete(pmh.progressRequests, requestID)
	if progressReq.ProgressToken != nil {
		delete(pmh.progressTokens, progressReq.ProgressToken)
	}
	pmh.progressMu.Unlock()
	
	// Cancel the request
	progressReq.Cancel()
	
	// Stop timers
	if progressReq.Timer != nil {
		progressReq.Timer.Stop()
	}
	if progressReq.TotalTimer != nil {
		progressReq.TotalTimer.Stop()
	}
	
	// Close the response channel
	close(progressReq.Response)
	
	atomic.AddInt64(&pmh.stats.Timeouts, 1)
}

// removeProgressRequest removes a progress request from tracking
func (pmh *ProgressMessageHandler) removeProgressRequest(requestID interface{}) {
	pmh.progressMu.Lock()
	defer pmh.progressMu.Unlock()
	
	if progressReq, exists := pmh.progressRequests[requestID]; exists {
		delete(pmh.progressRequests, requestID)
		if progressReq.ProgressToken != nil {
			delete(pmh.progressTokens, progressReq.ProgressToken)
		}
		
		// Stop timers
		if progressReq.Timer != nil {
			progressReq.Timer.Stop()
		}
		if progressReq.TotalTimer != nil {
			progressReq.TotalTimer.Stop()
		}
		
		progressReq.Cancel()
	}
}

// processProgressResponses handles responses for progress-enabled requests
func (pmh *ProgressMessageHandler) processProgressResponses() {
	// We need to intercept responses before the base handler processes them
	// This is a simplified approach - in a full implementation, we'd need more sophisticated coordination
	_, _, _, _ = pmh.MessageHandler.Channels()
	
	// For now, we'll rely on the base handler to process responses
	// The progress handler just manages timeouts and cleanup
}

// InjectProgressNotification injects a progress notification for testing
func (pmh *ProgressMessageHandler) InjectProgressNotification(notification *shared.Notification) {
	pmh.handleProgressNotification(notification)
}

// SetTestProgressCallback sets a global progress callback for testing orphaned notifications
func (pmh *ProgressMessageHandler) SetTestProgressCallback(callback ProgressCallback) {
	pmh.testProgressCallback = callback
}

// GetLastGeneratedToken returns the last generated progress token for testing
func (pmh *ProgressMessageHandler) GetLastGeneratedToken() interface{} {
	return atomic.LoadInt64(&pmh.tokenCounter)
}

// handleOrphanedProgress handles progress notifications that don't match any request (for testing)
func (pmh *ProgressMessageHandler) handleOrphanedProgress(paramsMap map[string]interface{}, progressToken interface{}) bool {
	if pmh.testProgressCallback == nil {
		return false
	}
	
	// Parse progress notification
	progressParams := shared.ProgressParams{
		ProgressToken: progressToken,
	}
	
	if progressVal, ok := paramsMap["progress"]; ok {
		if progressFloat, ok := progressVal.(float64); ok {
			progressParams.Progress = int(progressFloat)
		} else if progressInt, ok := progressVal.(int); ok {
			progressParams.Progress = progressInt
		}
	}
	
	if totalVal, ok := paramsMap["total"]; ok {
		if totalFloat, ok := totalVal.(float64); ok {
			total := int(totalFloat)
			progressParams.Total = &total
		} else if totalInt, ok := totalVal.(int); ok {
			progressParams.Total = &totalInt
		}
	}
	
	if messageVal, ok := paramsMap["message"]; ok {
		if messageStr, ok := messageVal.(string); ok {
			progressParams.Message = messageStr
		}
	}
	
	progress := shared.ProgressNotification{
		JSONRPC: "2.0",
		Method:  "notifications/progress",
		Params:  progressParams,
	}
	
	pmh.testProgressCallback(progress)
	return true
}

// Close cleans up the progress message handler
func (pmh *ProgressMessageHandler) Close() {
	pmh.progressMu.Lock()
	// Collect all requests to clean up
	var requestsToCleanup []*ProgressPendingRequest
	for _, progressReq := range pmh.progressRequests {
		requestsToCleanup = append(requestsToCleanup, progressReq)
	}
	// Clear the maps
	pmh.progressRequests = make(map[interface{}]*ProgressPendingRequest)
	pmh.progressTokens = make(map[interface{}]interface{})
	pmh.progressMu.Unlock()
	
	// Clean up requests without holding the lock
	for _, progressReq := range requestsToCleanup {
		if progressReq.Timer != nil {
			progressReq.Timer.Stop()
		}
		if progressReq.TotalTimer != nil {
			progressReq.TotalTimer.Stop()
		}
		progressReq.Cancel()
	}
	
	pmh.MessageHandler.Close()
}
package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/shared"
)

// ResourceRegistry manages concurrent resource operations
type ResourceRegistry struct {
	resources map[string]*ResourceEntry
	handlers  map[string]ResourceHandler
	mu        sync.RWMutex

	// Subscription management
	subscribers   map[string]chan ResourceEvent
	subscribersMu sync.RWMutex

	// Configuration
	maxConcurrentReads int
	cacheTimeout       time.Duration

	// Worker pool for resource operations
	workerPool chan struct{}
}

// ResourceEntry represents a registered resource with metadata
type ResourceEntry struct {
	Resource     shared.Resource
	Handler      ResourceHandler
	LastAccessed time.Time
	AccessCount  int64
	CacheEntry   *CacheEntry
}

// CacheEntry represents cached resource content
type CacheEntry struct {
	Content   []shared.Content
	Timestamp time.Time
	TTL       time.Duration
}

// ResourceEvent represents a resource change event
type ResourceEvent struct {
	Type     ResourceEventType
	Resource shared.Resource
	Error    error
}

// ResourceEventType represents the type of resource event
type ResourceEventType string

const (
	ResourceEventAdded   ResourceEventType = "added"
	ResourceEventUpdated ResourceEventType = "updated"
	ResourceEventRemoved ResourceEventType = "removed"
	ResourceEventError   ResourceEventType = "error"
)

// NewResourceRegistry creates a new concurrent resource registry
func NewResourceRegistry(maxConcurrentReads int, cacheTimeout time.Duration) *ResourceRegistry {
	if maxConcurrentReads == 0 {
		maxConcurrentReads = 50
	}
	if cacheTimeout == 0 {
		cacheTimeout = 5 * time.Minute
	}

	registry := &ResourceRegistry{
		resources:          make(map[string]*ResourceEntry),
		handlers:           make(map[string]ResourceHandler),
		subscribers:        make(map[string]chan ResourceEvent),
		maxConcurrentReads: maxConcurrentReads,
		cacheTimeout:       cacheTimeout,
		workerPool:         make(chan struct{}, maxConcurrentReads),
	}

	// Initialize worker pool
	for i := 0; i < maxConcurrentReads; i++ {
		registry.workerPool <- struct{}{}
	}

	return registry
}

// Register registers a new resource with concurrent access support
func (rr *ResourceRegistry) Register(uri, name, description string, handler ResourceHandler) error {
	rr.mu.Lock()
	defer rr.mu.Unlock()

	resource := shared.Resource{
		URI:         uri,
		Name:        name,
		Description: description,
	}

	entry := &ResourceEntry{
		Resource:     resource,
		Handler:      handler,
		LastAccessed: time.Now(),
		AccessCount:  0,
	}

	rr.resources[uri] = entry
	rr.handlers[uri] = handler

	// Notify subscribers
	go rr.notifySubscribers(ResourceEvent{
		Type:     ResourceEventAdded,
		Resource: resource,
	})

	return nil
}

// Unregister removes a resource from the registry
func (rr *ResourceRegistry) Unregister(uri string) error {
	rr.mu.Lock()
	defer rr.mu.Unlock()

	entry, exists := rr.resources[uri]
	if !exists {
		return fmt.Errorf("resource not found: %s", uri)
	}

	delete(rr.resources, uri)
	delete(rr.handlers, uri)

	// Notify subscribers
	go rr.notifySubscribers(ResourceEvent{
		Type:     ResourceEventRemoved,
		Resource: entry.Resource,
	})

	return nil
}

// List returns all registered resources
func (rr *ResourceRegistry) List() []shared.Resource {
	rr.mu.RLock()
	defer rr.mu.RUnlock()

	resources := make([]shared.Resource, 0, len(rr.resources))
	for _, entry := range rr.resources {
		resources = append(resources, entry.Resource)
	}

	return resources
}

// Read reads a resource with caching and concurrent access control
func (rr *ResourceRegistry) Read(ctx context.Context, uri string) ([]shared.Content, error) {
	// Get resource entry
	rr.mu.RLock()
	entry, exists := rr.resources[uri]
	rr.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("resource not found: %s", uri)
	}

	// Check cache first (need to check again under lock)
	rr.mu.RLock()
	if entry.CacheEntry != nil && time.Since(entry.CacheEntry.Timestamp) < entry.CacheEntry.TTL {
		content := entry.CacheEntry.Content
		rr.mu.RUnlock()
		rr.updateAccessStats(uri)
		return content, nil
	}
	rr.mu.RUnlock()

	// Acquire worker from pool
	select {
	case <-rr.workerPool:
		defer func() { rr.workerPool <- struct{}{} }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Read resource content
	content, err := entry.Handler(ctx, uri)
	if err != nil {
		go rr.notifySubscribers(ResourceEvent{
			Type:     ResourceEventError,
			Resource: entry.Resource,
			Error:    err,
		})
		return nil, err
	}

	// Update cache - need to re-lock and re-check entry
	rr.mu.Lock()
	// Re-fetch entry inside the lock to ensure we have the latest version
	if currentEntry, exists := rr.resources[uri]; exists {
		if currentEntry.CacheEntry == nil {
			currentEntry.CacheEntry = &CacheEntry{}
		}
		currentEntry.CacheEntry.Content = content
		currentEntry.CacheEntry.Timestamp = time.Now()
		currentEntry.CacheEntry.TTL = rr.cacheTimeout
	}
	rr.mu.Unlock()

	rr.updateAccessStats(uri)

	return content, nil
}

// Subscribe subscribes to resource events
func (rr *ResourceRegistry) Subscribe(subscriberID string) <-chan ResourceEvent {
	rr.subscribersMu.Lock()
	defer rr.subscribersMu.Unlock()

	eventChan := make(chan ResourceEvent, 10)
	rr.subscribers[subscriberID] = eventChan

	return eventChan
}

// Unsubscribe removes a subscriber from resource events
func (rr *ResourceRegistry) Unsubscribe(subscriberID string) {
	rr.subscribersMu.Lock()
	defer rr.subscribersMu.Unlock()

	if eventChan, exists := rr.subscribers[subscriberID]; exists {
		close(eventChan)
		delete(rr.subscribers, subscriberID)
	}
}

// updateAccessStats updates resource access statistics
func (rr *ResourceRegistry) updateAccessStats(uri string) {
	rr.mu.Lock()
	defer rr.mu.Unlock()

	if entry, exists := rr.resources[uri]; exists {
		entry.LastAccessed = time.Now()
		entry.AccessCount++
	}
}

// notifySubscribers notifies all subscribers of a resource event
func (rr *ResourceRegistry) notifySubscribers(event ResourceEvent) {
	rr.subscribersMu.RLock()
	defer rr.subscribersMu.RUnlock()

	for _, eventChan := range rr.subscribers {
		select {
		case eventChan <- event:
		default:
			// Channel full, skip notification
		}
	}
}

// GetStats returns resource registry statistics
func (rr *ResourceRegistry) GetStats() ResourceRegistryStats {
	rr.mu.RLock()
	defer rr.mu.RUnlock()

	stats := ResourceRegistryStats{
		TotalResources:  len(rr.resources),
		CachedResources: 0,
		TotalAccesses:   0,
	}

	for _, entry := range rr.resources {
		if entry.CacheEntry != nil {
			stats.CachedResources++
		}
		stats.TotalAccesses += entry.AccessCount
	}

	return stats
}

// ResourceRegistryStats represents resource registry statistics
type ResourceRegistryStats struct {
	TotalResources  int
	CachedResources int
	TotalAccesses   int64
}

// ToolRegistry manages concurrent tool operations
type ToolRegistry struct {
	tools    map[string]*ToolEntry
	handlers map[string]ToolHandler
	mu       sync.RWMutex

	// Execution management
	maxConcurrentExecutions int
	executionTimeout        time.Duration
	workerPool              chan struct{}

	// Statistics
	executionStats map[string]*ToolExecutionStats
	statsMu        sync.RWMutex
}

// ToolEntry represents a registered tool with metadata
type ToolEntry struct {
	Tool            shared.Tool
	Handler         ToolHandler
	ExecutionCount  int64
	LastExecuted    time.Time
	AverageExecTime time.Duration
}

// ToolExecutionStats represents tool execution statistics
type ToolExecutionStats struct {
	ExecutionCount int64
	TotalTime      time.Duration
	AverageTime    time.Duration
	ErrorCount     int64
	LastExecuted   time.Time
}

// NewToolRegistry creates a new concurrent tool registry
func NewToolRegistry(maxConcurrentExecutions int, executionTimeout time.Duration) *ToolRegistry {
	if maxConcurrentExecutions == 0 {
		maxConcurrentExecutions = 20
	}
	if executionTimeout == 0 {
		executionTimeout = 30 * time.Second
	}

	registry := &ToolRegistry{
		tools:                   make(map[string]*ToolEntry),
		handlers:                make(map[string]ToolHandler),
		maxConcurrentExecutions: maxConcurrentExecutions,
		executionTimeout:        executionTimeout,
		workerPool:              make(chan struct{}, maxConcurrentExecutions),
		executionStats:          make(map[string]*ToolExecutionStats),
	}

	// Initialize worker pool
	for i := 0; i < maxConcurrentExecutions; i++ {
		registry.workerPool <- struct{}{}
	}

	return registry
}

// Register registers a new tool with concurrent execution support
func (tr *ToolRegistry) Register(name, description string, inputSchema map[string]interface{}, handler ToolHandler) error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	tool := shared.Tool{
		Name:        name,
		Description: description,
		InputSchema: inputSchema,
	}

	entry := &ToolEntry{
		Tool:         tool,
		Handler:      handler,
		LastExecuted: time.Now(),
	}

	tr.tools[name] = entry
	tr.handlers[name] = handler

	// Initialize stats
	tr.statsMu.Lock()
	tr.executionStats[name] = &ToolExecutionStats{}
	tr.statsMu.Unlock()

	return nil
}

// List returns all registered tools
func (tr *ToolRegistry) List() []shared.Tool {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	tools := make([]shared.Tool, 0, len(tr.tools))
	for _, entry := range tr.tools {
		tools = append(tools, entry.Tool)
	}

	return tools
}

// Execute executes a tool with timeout and concurrency control
func (tr *ToolRegistry) Execute(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
	// Get tool entry
	tr.mu.RLock()
	entry, exists := tr.tools[name]
	tr.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	// Acquire worker from pool
	select {
	case <-tr.workerPool:
		defer func() { tr.workerPool <- struct{}{} }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Create execution context with timeout
	execCtx, cancel := context.WithTimeout(ctx, tr.executionTimeout)
	defer cancel()

	// Execute tool
	startTime := time.Now()
	content, err := entry.Handler(execCtx, name, arguments)
	execTime := time.Since(startTime)

	// Update statistics
	tr.updateExecutionStats(name, execTime, err != nil)

	if err != nil {
		return nil, err
	}

	return content, nil
}

// updateExecutionStats updates tool execution statistics
func (tr *ToolRegistry) updateExecutionStats(name string, execTime time.Duration, isError bool) {
	tr.statsMu.Lock()
	defer tr.statsMu.Unlock()

	stats, exists := tr.executionStats[name]
	if !exists {
		return
	}

	stats.ExecutionCount++
	stats.TotalTime += execTime
	stats.AverageTime = stats.TotalTime / time.Duration(stats.ExecutionCount)
	stats.LastExecuted = time.Now()

	if isError {
		stats.ErrorCount++
	}

	// Update tool entry
	tr.mu.Lock()
	if entry, exists := tr.tools[name]; exists {
		entry.ExecutionCount = stats.ExecutionCount
		entry.LastExecuted = stats.LastExecuted
		entry.AverageExecTime = stats.AverageTime
	}
	tr.mu.Unlock()
}

// GetStats returns tool registry statistics
func (tr *ToolRegistry) GetStats() map[string]ToolExecutionStats {
	tr.statsMu.RLock()
	defer tr.statsMu.RUnlock()

	stats := make(map[string]ToolExecutionStats)
	for name, stat := range tr.executionStats {
		stats[name] = *stat
	}

	return stats
}

// PromptRegistry manages concurrent prompt operations
type PromptRegistry struct {
	prompts  map[string]*PromptEntry
	handlers map[string]PromptHandler
	mu       sync.RWMutex

	// Execution management
	maxConcurrentExecutions int
	executionTimeout        time.Duration
	workerPool              chan struct{}
}

// PromptEntry represents a registered prompt with metadata
type PromptEntry struct {
	Prompt     shared.Prompt
	Handler    PromptHandler
	UsageCount int64
	LastUsed   time.Time
}

// NewPromptRegistry creates a new concurrent prompt registry
func NewPromptRegistry(maxConcurrentExecutions int, executionTimeout time.Duration) *PromptRegistry {
	if maxConcurrentExecutions == 0 {
		maxConcurrentExecutions = 10
	}
	if executionTimeout == 0 {
		executionTimeout = 10 * time.Second
	}

	registry := &PromptRegistry{
		prompts:                 make(map[string]*PromptEntry),
		handlers:                make(map[string]PromptHandler),
		maxConcurrentExecutions: maxConcurrentExecutions,
		executionTimeout:        executionTimeout,
		workerPool:              make(chan struct{}, maxConcurrentExecutions),
	}

	// Initialize worker pool
	for i := 0; i < maxConcurrentExecutions; i++ {
		registry.workerPool <- struct{}{}
	}

	return registry
}

// Register registers a new prompt with concurrent execution support
func (pr *PromptRegistry) Register(name, description string, arguments []shared.PromptArgument, handler PromptHandler) error {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	prompt := shared.Prompt{
		Name:        name,
		Description: description,
		Arguments:   arguments,
	}

	entry := &PromptEntry{
		Prompt:   prompt,
		Handler:  handler,
		LastUsed: time.Now(),
	}

	pr.prompts[name] = entry
	pr.handlers[name] = handler

	return nil
}

// List returns all registered prompts
func (pr *PromptRegistry) List() []shared.Prompt {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	prompts := make([]shared.Prompt, 0, len(pr.prompts))
	for _, entry := range pr.prompts {
		prompts = append(prompts, entry.Prompt)
	}

	return prompts
}

// Execute executes a prompt with timeout and concurrency control
func (pr *PromptRegistry) Execute(ctx context.Context, name string, arguments map[string]interface{}) (PromptMessage, error) {
	// Get prompt entry
	pr.mu.RLock()
	entry, exists := pr.prompts[name]
	pr.mu.RUnlock()

	if !exists {
		return PromptMessage{}, fmt.Errorf("prompt not found: %s", name)
	}

	// Acquire worker from pool
	select {
	case <-pr.workerPool:
		defer func() { pr.workerPool <- struct{}{} }()
	case <-ctx.Done():
		return PromptMessage{}, ctx.Err()
	}

	// Create execution context with timeout
	execCtx, cancel := context.WithTimeout(ctx, pr.executionTimeout)
	defer cancel()

	// Execute prompt
	message, err := entry.Handler(execCtx, name, arguments)

	// Update usage statistics
	pr.mu.Lock()
	entry.UsageCount++
	entry.LastUsed = time.Now()
	pr.mu.Unlock()

	if err != nil {
		return PromptMessage{}, err
	}

	return message, nil
}

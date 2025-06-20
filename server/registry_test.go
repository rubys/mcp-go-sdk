package server

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/rubys/mcp-go-sdk/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewResourceRegistry(10, 1*time.Minute)

	// Register a test resource
	handler := func(ctx context.Context, uri string) ([]shared.Content, error) {
		// Simulate some processing time
		time.Sleep(10 * time.Millisecond)
		return []shared.Content{
			shared.TextContent{
				Type: shared.ContentTypeText,
				Text: "Test content for " + uri,
			},
		}, nil
	}

	err := registry.Register("test://resource1", "Test Resource", "A test resource", handler)
	require.NoError(t, err)

	// Test concurrent reads
	const numConcurrentReads = 50
	var wg sync.WaitGroup
	results := make([][]shared.Content, numConcurrentReads)
	errors := make([]error, numConcurrentReads)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	startTime := time.Now()

	for i := 0; i < numConcurrentReads; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			content, err := registry.Read(ctx, "test://resource1")
			results[i] = content
			errors[i] = err
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	// Verify all reads succeeded
	successCount := 0
	for i := 0; i < numConcurrentReads; i++ {
		if errors[i] == nil {
			successCount++
			assert.Len(t, results[i], 1)
			assert.Contains(t, results[i][0].(shared.TextContent).Text, "Test content")
		}
	}

	assert.Equal(t, numConcurrentReads, successCount)
	t.Logf("Completed %d concurrent reads in %v", successCount, duration)

	// Verify statistics
	stats := registry.GetStats()
	assert.Equal(t, 1, stats.TotalResources)
	assert.Equal(t, int64(numConcurrentReads), stats.TotalAccesses)
}

func TestResourceRegistry_Caching(t *testing.T) {
	registry := NewResourceRegistry(5, 100*time.Millisecond) // Short cache timeout

	callCount := 0
	handler := func(ctx context.Context, uri string) ([]shared.Content, error) {
		callCount++
		return []shared.Content{
			shared.TextContent{
				Type: shared.ContentTypeText,
				Text: "Cached content",
			},
		}, nil
	}

	err := registry.Register("test://cached", "Cached Resource", "A cached resource", handler)
	require.NoError(t, err)

	ctx := context.Background()

	// First read - should call handler
	content1, err := registry.Read(ctx, "test://cached")
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// Second read - should use cache
	content2, err := registry.Read(ctx, "test://cached")
	require.NoError(t, err)
	assert.Equal(t, 1, callCount) // Handler not called again
	assert.Equal(t, content1, content2)

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Third read - should call handler again
	content3, err := registry.Read(ctx, "test://cached")
	require.NoError(t, err)
	assert.Equal(t, 2, callCount) // Handler called again
	assert.Equal(t, content1, content3)
}

func TestResourceRegistry_Subscription(t *testing.T) {
	registry := NewResourceRegistry(5, 1*time.Minute)

	// Subscribe to events
	events := registry.Subscribe("test-subscriber")

	// Register a resource - should trigger event
	handler := func(ctx context.Context, uri string) ([]shared.Content, error) {
		return []shared.Content{}, nil
	}

	err := registry.Register("test://sub", "Sub Resource", "A subscription test", handler)
	require.NoError(t, err)

	// Wait for event
	select {
	case event := <-events:
		assert.Equal(t, ResourceEventAdded, event.Type)
		assert.Equal(t, "test://sub", event.Resource.URI)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for resource event")
	}

	// Unregister resource - should trigger event
	err = registry.Unregister("test://sub")
	require.NoError(t, err)

	select {
	case event := <-events:
		assert.Equal(t, ResourceEventRemoved, event.Type)
		assert.Equal(t, "test://sub", event.Resource.URI)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for resource removal event")
	}

	// Unsubscribe
	registry.Unsubscribe("test-subscriber")
}

func TestToolRegistry_ConcurrentExecution(t *testing.T) {
	registry := NewToolRegistry(10, 5*time.Second)

	// Register a test tool
	handler := func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		// Simulate processing time
		time.Sleep(20 * time.Millisecond)

		input, _ := arguments["input"].(string)
		return []shared.Content{
			shared.TextContent{
				Type: shared.ContentTypeText,
				Text: "Processed: " + input,
			},
		}, nil
	}

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"input": map[string]interface{}{
				"type": "string",
			},
		},
	}

	err := registry.Register("test_tool", "Test Tool", schema, handler)
	require.NoError(t, err)

	// Test concurrent executions
	const numExecutions = 20
	var wg sync.WaitGroup
	results := make([][]shared.Content, numExecutions)
	errors := make([]error, numExecutions)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	startTime := time.Now()

	for i := 0; i < numExecutions; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := map[string]interface{}{
				"input": fmt.Sprintf("test-%d", i),
			}
			content, err := registry.Execute(ctx, "test_tool", args)
			results[i] = content
			errors[i] = err
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	// Verify all executions succeeded
	successCount := 0
	for i := 0; i < numExecutions; i++ {
		if errors[i] == nil {
			successCount++
			assert.Len(t, results[i], 1)
			assert.Contains(t, results[i][0].(shared.TextContent).Text, "Processed:")
		}
	}

	assert.Equal(t, numExecutions, successCount)
	t.Logf("Completed %d concurrent tool executions in %v", successCount, duration)

	// Verify statistics
	stats := registry.GetStats()
	toolStats, exists := stats["test_tool"]
	assert.True(t, exists)
	assert.Equal(t, int64(numExecutions), toolStats.ExecutionCount)
	assert.Equal(t, int64(0), toolStats.ErrorCount)
}

func TestToolRegistry_ExecutionTimeout(t *testing.T) {
	registry := NewToolRegistry(5, 50*time.Millisecond) // Very short timeout

	// Register a slow tool
	handler := func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		// Simulate slow processing
		select {
		case <-time.After(200 * time.Millisecond):
			return []shared.Content{}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	schema := map[string]interface{}{"type": "object"}
	err := registry.Register("slow_tool", "Slow Tool", schema, handler)
	require.NoError(t, err)

	ctx := context.Background()

	// Execute tool - should timeout
	_, err = registry.Execute(ctx, "slow_tool", map[string]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")

	// Verify error was recorded in statistics
	stats := registry.GetStats()
	toolStats, exists := stats["slow_tool"]
	assert.True(t, exists)
	assert.Equal(t, int64(1), toolStats.ErrorCount)
}

func TestPromptRegistry_ConcurrentExecution(t *testing.T) {
	registry := NewPromptRegistry(10, 5*time.Second)

	// Register a test prompt
	handler := func(ctx context.Context, name string, arguments map[string]interface{}) (PromptMessage, error) {
		// Simulate processing time
		time.Sleep(10 * time.Millisecond)

		userName, _ := arguments["name"].(string)
		return PromptMessage{
			Role: "user",
			Content: []shared.Content{
				shared.TextContent{
					Type: shared.ContentTypeText,
					Text: "Hello " + userName + "!",
				},
			},
		}, nil
	}

	args := []shared.PromptArgument{
		{
			Name:        "name",
			Description: "The name to greet",
			Required:    true,
		},
	}

	err := registry.Register("greeting", "Greeting Prompt", args, handler)
	require.NoError(t, err)

	// Test concurrent executions
	const numExecutions = 15
	var wg sync.WaitGroup
	results := make([]PromptMessage, numExecutions)
	errors := make([]error, numExecutions)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 0; i < numExecutions; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := map[string]interface{}{
				"name": fmt.Sprintf("User%d", i),
			}
			result, err := registry.Execute(ctx, "greeting", args)
			results[i] = result
			errors[i] = err
		}(i)
	}

	wg.Wait()

	// Verify all executions succeeded
	successCount := 0
	for i := 0; i < numExecutions; i++ {
		if errors[i] == nil {
			successCount++
			assert.Equal(t, "user", results[i].Role)
			assert.Len(t, results[i].Content, 1)
			assert.Contains(t, results[i].Content[0].(shared.TextContent).Text, "Hello User")
		}
	}

	assert.Equal(t, numExecutions, successCount)
}

func BenchmarkResourceRegistry_ConcurrentReads(b *testing.B) {
	registry := NewResourceRegistry(50, 5*time.Minute)

	handler := func(ctx context.Context, uri string) ([]shared.Content, error) {
		return []shared.Content{
			shared.TextContent{
				Type: shared.ContentTypeText,
				Text: "Benchmark content",
			},
		}, nil
	}

	registry.Register("bench://resource", "Benchmark Resource", "A benchmark resource", handler)

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := registry.Read(ctx, "bench://resource")
			if err != nil {
				b.Error(err)
			}
		}
	})

	stats := registry.GetStats()
	b.Logf("Total accesses: %d, Cached resources: %d", stats.TotalAccesses, stats.CachedResources)
}

func BenchmarkToolRegistry_ConcurrentExecutions(b *testing.B) {
	registry := NewToolRegistry(50, 30*time.Second)

	handler := func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		return []shared.Content{
			shared.TextContent{
				Type: shared.ContentTypeText,
				Text: "Benchmark result",
			},
		}, nil
	}

	schema := map[string]interface{}{"type": "object"}
	registry.Register("bench_tool", "Benchmark Tool", schema, handler)

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := registry.Execute(ctx, "bench_tool", map[string]interface{}{})
			if err != nil {
				b.Error(err)
			}
		}
	})

	stats := registry.GetStats()
	if toolStats, exists := stats["bench_tool"]; exists {
		b.Logf("Total executions: %d, Average time: %v", toolStats.ExecutionCount, toolStats.AverageTime)
	}
}

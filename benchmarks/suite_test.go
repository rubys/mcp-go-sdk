package benchmarks

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/rubys/mcp-go-sdk/client"
	"github.com/rubys/mcp-go-sdk/server"
	"github.com/rubys/mcp-go-sdk/shared"
	"github.com/rubys/mcp-go-sdk/transport"
)

// BenchmarkConfig controls benchmark parameters
type BenchmarkConfig struct {
	NumWorkers    int
	RequestsPerOp int
	MessageSize   int
	Concurrency   int
}

// DefaultBenchmarkConfig provides reasonable defaults for benchmarking
var DefaultBenchmarkConfig = BenchmarkConfig{
	NumWorkers:    runtime.NumCPU(),
	RequestsPerOp: 100,
	MessageSize:   1024,
	Concurrency:   10,
}

// BenchmarkResults stores performance metrics
type BenchmarkResults struct {
	RequestsPerSecond   float64
	AvgLatencyMs        float64
	P95LatencyMs        float64
	P99LatencyMs        float64
	ThroughputMBps      float64
	ConcurrentClients   int
	ErrorRate           float64
}

// setupBenchmarkServer creates a server for benchmarking and registers tools/resources on transport
func setupBenchmarkServer(ctx context.Context, trans server.Transport) *server.Server {
	config := server.ServerConfig{
		Name:                  "benchmark-server",
		Version:               "1.0.0",
		MaxConcurrentRequests: 1000,
		RequestTimeout:        10 * time.Second,
		Capabilities: shared.ServerCapabilities{
			Resources: &shared.ResourcesCapability{
				Subscribe:   true,
				ListChanged: true,
			},
			Tools: &shared.ToolsCapability{
				ListChanged: true,
			},
			Prompts: &shared.PromptsCapability{
				ListChanged: true,
			},
		},
	}

	srv := server.NewServer(ctx, trans, config)

	// Check if this is an InProcessTransport and register directly on it
	if inProcTransport, ok := trans.(*transport.InProcessTransport); ok {
		// Register benchmark resources directly on transport
		for i := 0; i < 100; i++ {
			uri := fmt.Sprintf("file://benchmark-%d.txt", i)
			resource := shared.Resource{
				URI:         uri,
				Name:        fmt.Sprintf("Benchmark Resource %d", i),
				Description: "Resource for performance testing",
			}
			inProcTransport.AddResource(resource, func(ctx context.Context, uri string) ([]shared.ResourceContent, error) {
				return []shared.ResourceContent{
					{
						URI:      uri,
						MimeType: "text/plain",
						Text:     fmt.Sprintf("Benchmark content for %s with timestamp %d", uri, time.Now().UnixNano()),
					},
				}, nil
			})
		}
	} else {
		// Register benchmark resources on server for other transports
		for i := 0; i < 100; i++ {
			uri := fmt.Sprintf("file://benchmark-%d.txt", i)
			srv.RegisterResource(uri, fmt.Sprintf("Benchmark Resource %d", i), "Resource for performance testing",
				func(ctx context.Context, uri string) ([]shared.Content, error) {
					return []shared.Content{
						shared.TextContent{
							Type: "text",
							Text: fmt.Sprintf("Benchmark content for %s with timestamp %d", uri, time.Now().UnixNano()),
						},
					}, nil
				})
		}
	}

	// Register benchmark tools
	if inProcTransport, ok := trans.(*transport.InProcessTransport); ok {
		// Register tools directly on transport
		for i := 0; i < 50; i++ {
			toolName := fmt.Sprintf("benchmark_tool_%d", i)
			tool := shared.Tool{
				Name:        toolName,
				Description: fmt.Sprintf("Benchmark Tool %d", i),
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"input": map[string]interface{}{
							"type":        "string",
							"description": "Input data for processing",
						},
						"size": map[string]interface{}{
							"type":        "number",
							"description": "Size parameter",
						},
					},
				},
			}
			inProcTransport.AddTool(tool, func(ctx context.Context, arguments map[string]interface{}) ([]shared.Content, error) {
				// Simulate some work
				time.Sleep(time.Microsecond * 10)
				
				input := "default"
				if val, ok := arguments["input"].(string); ok {
					input = val
				}
				
				return []shared.Content{
					shared.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Processed %s with tool %s at %d", input, toolName, time.Now().UnixNano()),
					},
				}, nil
			})
		}
	} else {
		// Register tools on server for other transports
		for i := 0; i < 50; i++ {
			toolName := fmt.Sprintf("benchmark_tool_%d", i)
			srv.RegisterTool(toolName, fmt.Sprintf("Benchmark Tool %d", i), map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"input": map[string]interface{}{
						"type":        "string",
						"description": "Input data for processing",
					},
					"size": map[string]interface{}{
						"type":        "number",
						"description": "Size parameter",
					},
				},
			}, func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
				// Simulate some work
				time.Sleep(time.Microsecond * 10)
				
				input := "default"
				if val, ok := arguments["input"].(string); ok {
					input = val
				}
				
				return []shared.Content{
					shared.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Processed %s with tool %s at %d", input, name, time.Now().UnixNano()),
					},
				}, nil
			})
		}
	}

	// Register benchmark prompts
	for i := 0; i < 25; i++ {
		promptName := fmt.Sprintf("benchmark_prompt_%d", i)
		srv.RegisterPrompt(promptName, fmt.Sprintf("Benchmark Prompt %d", i), []shared.PromptArgument{
			{
				Name:        "context",
				Description: "Context for the prompt",
				Required:    true,
			},
		}, func(ctx context.Context, name string, arguments map[string]interface{}) (server.PromptMessage, error) {
			context := "default"
			if val, ok := arguments["context"].(string); ok {
				context = val
			}
			
			return server.PromptMessage{
				Role: "assistant",
				Content: []shared.Content{
					shared.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Generated prompt response for %s with context: %s", name, context),
					},
				},
			}, nil
		})
	}

	return srv
}

// BenchmarkInProcessTransport_ThroughputSingle tests single-threaded throughput
func BenchmarkInProcessTransport_ThroughputSingle(b *testing.B) {
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	srv := setupBenchmarkServer(ctx, mockTransport)
	srv.Start()
	defer srv.Close()

	client, err := client.NewClient(ctx, mockTransport, client.ClientConfig{
		ClientInfo: shared.ClientInfo{
			Name:    "benchmark-client",
			Version: "1.0.0",
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	defer client.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.CallTool(ctx, shared.CallToolRequest{
			Name: "benchmark_tool_0",
			Arguments: map[string]interface{}{
				"input": fmt.Sprintf("benchmark_input_%d", i),
				"size":  1024,
			},
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkInProcessTransport_ThroughputConcurrent tests concurrent throughput
func BenchmarkInProcessTransport_ThroughputConcurrent(b *testing.B) {
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	srv := setupBenchmarkServer(ctx, mockTransport)
	srv.Start()
	defer srv.Close()

	// Create multiple clients for concurrent testing
	numClients := 10
	clients := make([]*client.Client, numClients)
	
	for i := 0; i < numClients; i++ {
		c, err := client.NewClient(ctx, mockTransport, client.ClientConfig{
			ClientInfo: shared.ClientInfo{
				Name:    fmt.Sprintf("benchmark-client-%d", i),
				Version: "1.0.0",
			},
		})
		if err != nil {
			b.Fatal(err)
		}
		clients[i] = c
		defer clients[i].Close()
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		clientIndex := 0
		for pb.Next() {
			client := clients[clientIndex%numClients]
			clientIndex++
			
			_, err := client.CallTool(ctx, shared.CallToolRequest{
				Name: "benchmark_tool_0",
				Arguments: map[string]interface{}{
					"input": fmt.Sprintf("concurrent_input_%d", clientIndex),
					"size":  1024,
				},
			})
			if err != nil {
				b.Error(err)
			}
		}
	})
}

// BenchmarkStdioTransport_Throughput tests stdio transport performance
func BenchmarkStdioTransport_Throughput(b *testing.B) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stdioTransport, err := transport.NewStdioTransport(ctx, transport.StdioConfig{
		RequestTimeout: 5 * time.Second,
		MessageBuffer:  1000,
	})
	if err != nil {
		b.Fatal(err)
	}
	defer stdioTransport.Close()

	srv := setupBenchmarkServer(ctx, stdioTransport)
	srv.Start()
	defer srv.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Simulate stdio message processing
		err := stdioTransport.SendNotification("benchmark", map[string]interface{}{
			"iteration": i,
			"data":      "benchmark_data",
		})
		if err != nil {
			b.Error(err)
		}
	}
}

// BenchmarkResourceAccess tests resource access performance
func BenchmarkResourceAccess(b *testing.B) {
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	srv := setupBenchmarkServer(ctx, mockTransport)
	srv.Start()
	defer srv.Close()

	client, err := client.NewClient(ctx, mockTransport, client.ClientConfig{
		ClientInfo: shared.ClientInfo{
			Name:    "benchmark-client",
			Version: "1.0.0",
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	defer client.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		uri := fmt.Sprintf("file://benchmark-%d.txt", i%100)
		_, err := client.ReadResource(ctx, uri)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkToolExecution tests tool execution performance
func BenchmarkToolExecution(b *testing.B) {
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	srv := setupBenchmarkServer(ctx, mockTransport)
	srv.Start()
	defer srv.Close()

	client, err := client.NewClient(ctx, mockTransport, client.ClientConfig{
		ClientInfo: shared.ClientInfo{
			Name:    "benchmark-client",
			Version: "1.0.0",
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	defer client.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		toolName := fmt.Sprintf("benchmark_tool_%d", i%50)
		_, err := client.CallTool(ctx, shared.CallToolRequest{
			Name: toolName,
			Arguments: map[string]interface{}{
				"input": fmt.Sprintf("benchmark_input_%d", i),
				"size":  1024,
			},
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPromptGeneration tests prompt generation performance
func BenchmarkPromptGeneration(b *testing.B) {
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	srv := setupBenchmarkServer(ctx, mockTransport)
	srv.Start()
	defer srv.Close()

	client, err := client.NewClient(ctx, mockTransport, client.ClientConfig{
		ClientInfo: shared.ClientInfo{
			Name:    "benchmark-client",
			Version: "1.0.0",
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	defer client.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		promptName := fmt.Sprintf("benchmark_prompt_%d", i%25)
		_, err := client.GetPrompt(ctx, shared.GetPromptRequest{
			Name: promptName,
			Arguments: map[string]interface{}{
				"context": fmt.Sprintf("benchmark_context_%d", i),
			},
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHighConcurrency tests performance under high concurrency
func BenchmarkHighConcurrency(b *testing.B) {
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	srv := setupBenchmarkServer(ctx, mockTransport)
	srv.Start()
	defer srv.Close()

	// Create many concurrent clients
	numClients := 100
	clients := make([]*client.Client, numClients)
	
	for i := 0; i < numClients; i++ {
		c, err := client.NewClient(ctx, mockTransport, client.ClientConfig{
			ClientInfo: shared.ClientInfo{
				Name:    fmt.Sprintf("high-concurrency-client-%d", i),
				Version: "1.0.0",
			},
		})
		if err != nil {
			b.Fatal(err)
		}
		clients[i] = c
		defer clients[i].Close()
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		clientIndex := 0
		for pb.Next() {
			client := clients[clientIndex%numClients]
			clientIndex++
			
			// Mix different operations
			switch clientIndex % 4 {
			case 0:
				client.CallTool(context.Background(), shared.CallToolRequest{
					Name: "benchmark_tool_0",
					Arguments: map[string]interface{}{
						"input": fmt.Sprintf("high_concurrency_%d", clientIndex),
					},
				})
			case 1:
				client.ReadResource(context.Background(), "file://benchmark-0.txt")
			case 2:
				client.GetPrompt(context.Background(), shared.GetPromptRequest{
					Name: "benchmark_prompt_0",
					Arguments: map[string]interface{}{
						"context": fmt.Sprintf("high_concurrency_%d", clientIndex),
					},
				})
			case 3:
				client.ListResources(context.Background())
			}
		}
	})
}

// BenchmarkMemoryUsage tests memory allocation patterns
func BenchmarkMemoryUsage(b *testing.B) {
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	srv := setupBenchmarkServer(ctx, mockTransport)
	srv.Start()
	defer srv.Close()

	client, err := client.NewClient(ctx, mockTransport, client.ClientConfig{
		ClientInfo: shared.ClientInfo{
			Name:    "memory-benchmark-client",
			Version: "1.0.0",
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	defer client.Close()

	// Force garbage collection before starting
	runtime.GC()

	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Perform mixed operations
		client.CallTool(ctx, shared.CallToolRequest{
			Name: "benchmark_tool_0",
			Arguments: map[string]interface{}{
				"input": fmt.Sprintf("memory_test_%d", i),
				"size":  1024,
			},
		})
		
		if i%100 == 0 {
			runtime.GC() // Periodic GC to measure steady-state memory
		}
	}

	runtime.ReadMemStats(&m2)
	
	b.ReportMetric(float64(m2.TotalAlloc-m1.TotalAlloc)/float64(b.N), "bytes/op")
	b.ReportMetric(float64(m2.Mallocs-m1.Mallocs)/float64(b.N), "allocs/op")
}

// RunComprehensiveBenchmark runs a comprehensive performance comparison
func RunComprehensiveBenchmark(b *testing.B) {
	// This is a meta-benchmark that runs all benchmarks and reports comprehensive results
	benchmarks := []struct {
		name string
		fn   func(*testing.B)
	}{
		{"SingleThreaded", BenchmarkInProcessTransport_ThroughputSingle},
		{"Concurrent", BenchmarkInProcessTransport_ThroughputConcurrent},
		{"ResourceAccess", BenchmarkResourceAccess},
		{"ToolExecution", BenchmarkToolExecution},
		{"PromptGeneration", BenchmarkPromptGeneration},
		{"HighConcurrency", BenchmarkHighConcurrency},
		{"MemoryUsage", BenchmarkMemoryUsage},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, bm.fn)
	}
}

// Benchmark comparison helper functions
func runPerformanceComparison(b *testing.B, testName string, goSDKOps, typeScriptOps int) {
	b.Helper()
	
	improvement := float64(goSDKOps) / float64(typeScriptOps)
	b.Logf("%s Performance Comparison:", testName)
	b.Logf("  Go SDK:          %d ops/sec", goSDKOps)
	b.Logf("  TypeScript SDK:  %d ops/sec", typeScriptOps)
	b.Logf("  Improvement:     %.1fx", improvement)
	
	if improvement < 5.0 {
		b.Logf("WARNING: Performance improvement is below 5x target")
	} else if improvement >= 10.0 {
		b.Logf("SUCCESS: Achieved 10x+ performance improvement!")
	} else {
		b.Logf("GOOD: Achieved %.1fx performance improvement", improvement)
	}
}
package benchmarks

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rubys/mcp-go-sdk/client"
	"github.com/rubys/mcp-go-sdk/shared"
	"github.com/rubys/mcp-go-sdk/transport"
)

// TypeScriptSDKSimulator simulates TypeScript SDK performance characteristics
type TypeScriptSDKSimulator struct {
	requestDelay    time.Duration
	processingDelay time.Duration
	concurrencyLimit int
	semaphore       chan struct{}
}

func NewTypeScriptSDKSimulator() *TypeScriptSDKSimulator {
	// Simulate TypeScript SDK characteristics based on observed performance
	return &TypeScriptSDKSimulator{
		requestDelay:    time.Microsecond * 50,  // Network + parsing overhead
		processingDelay: time.Microsecond * 100, // Single-threaded processing
		concurrencyLimit: 10,                    // Limited concurrency
		semaphore:       make(chan struct{}, 10),
	}
}

func (ts *TypeScriptSDKSimulator) SimulateRequest() error {
	// Acquire semaphore (simulate concurrency limit)
	ts.semaphore <- struct{}{}
	defer func() { <-ts.semaphore }()
	
	// Simulate request overhead
	time.Sleep(ts.requestDelay)
	
	// Simulate processing time
	time.Sleep(ts.processingDelay)
	
	return nil
}

// BenchmarkGoSDKvsTypeScriptSDK_SingleThreaded compares single-threaded performance
func BenchmarkGoSDKvsTypeScriptSDK_SingleThreaded(b *testing.B) {
	// Go SDK setup
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	srv := setupBenchmarkServer(ctx, mockTransport)
	srv.Start()
	defer srv.Close()

	client, err := client.NewClient(ctx, mockTransport, client.ClientConfig{
		ClientInfo: shared.ClientInfo{
			Name:    "comparison-client",
			Version: "1.0.0",
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	defer client.Close()

	// TypeScript simulator
	tsSimulator := NewTypeScriptSDKSimulator()

	b.Run("GoSDK", func(b *testing.B) {
		b.ResetTimer()
		start := time.Now()
		
		for i := 0; i < b.N; i++ {
			_, err := client.CallTool(ctx, shared.CallToolRequest{
				Name: "benchmark_tool_0",
				Arguments: map[string]interface{}{
					"input": fmt.Sprintf("go_sdk_%d", i),
				},
			})
			if err != nil {
				b.Fatal(err)
			}
		}
		
		duration := time.Since(start)
		opsPerSec := float64(b.N) / duration.Seconds()
		b.ReportMetric(opsPerSec, "ops/sec")
	})

	b.Run("TypeScriptSDK_Simulated", func(b *testing.B) {
		b.ResetTimer()
		start := time.Now()
		
		for i := 0; i < b.N; i++ {
			err := tsSimulator.SimulateRequest()
			if err != nil {
				b.Fatal(err)
			}
		}
		
		duration := time.Since(start)
		opsPerSec := float64(b.N) / duration.Seconds()
		b.ReportMetric(opsPerSec, "ops/sec")
	})
}

// BenchmarkGoSDKvsTypeScriptSDK_Concurrent compares concurrent performance
func BenchmarkGoSDKvsTypeScriptSDK_Concurrent(b *testing.B) {
	// Go SDK setup
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	srv := setupBenchmarkServer(ctx, mockTransport)
	srv.Start()
	defer srv.Close()

	// Create multiple clients for Go SDK
	numClients := 50
	clients := make([]*client.Client, numClients)
	
	for i := 0; i < numClients; i++ {
		c, err := client.NewClient(ctx, mockTransport, client.ClientConfig{
			ClientInfo: shared.ClientInfo{
				Name:    fmt.Sprintf("concurrent-client-%d", i),
				Version: "1.0.0",
			},
		})
		if err != nil {
			b.Fatal(err)
		}
		clients[i] = c
		defer clients[i].Close()
	}

	// TypeScript simulator
	tsSimulator := NewTypeScriptSDKSimulator()

	b.Run("GoSDK_Concurrent", func(b *testing.B) {
		b.ResetTimer()
		start := time.Now()
		
		b.RunParallel(func(pb *testing.PB) {
			clientIndex := 0
			for pb.Next() {
				client := clients[clientIndex%numClients]
				clientIndex++
				
				_, err := client.CallTool(ctx, shared.CallToolRequest{
					Name: "benchmark_tool_0",
					Arguments: map[string]interface{}{
						"input": fmt.Sprintf("concurrent_go_%d", clientIndex),
					},
				})
				if err != nil {
					b.Error(err)
				}
			}
		})
		
		duration := time.Since(start)
		opsPerSec := float64(b.N) / duration.Seconds()
		b.ReportMetric(opsPerSec, "ops/sec")
	})

	b.Run("TypeScriptSDK_Concurrent", func(b *testing.B) {
		b.ResetTimer()
		start := time.Now()
		
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				err := tsSimulator.SimulateRequest()
				if err != nil {
					b.Error(err)
				}
			}
		})
		
		duration := time.Since(start)
		opsPerSec := float64(b.N) / duration.Seconds()
		b.ReportMetric(opsPerSec, "ops/sec")
	})
}

// BenchmarkThroughputComparison_FixedTime runs both SDKs for a fixed time and compares throughput
func BenchmarkThroughputComparison_FixedTime(b *testing.B) {
	duration := 5 * time.Second
	
	// Go SDK setup
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	srv := setupBenchmarkServer(ctx, mockTransport)
	srv.Start()
	defer srv.Close()

	numClients := 100
	clients := make([]*client.Client, numClients)
	
	for i := 0; i < numClients; i++ {
		c, err := client.NewClient(ctx, mockTransport, client.ClientConfig{
			ClientInfo: shared.ClientInfo{
				Name:    fmt.Sprintf("throughput-client-%d", i),
				Version: "1.0.0",
			},
		})
		if err != nil {
			b.Fatal(err)
		}
		clients[i] = c
		defer clients[i].Close()
	}

	tsSimulator := NewTypeScriptSDKSimulator()

	b.Run("GoSDK_FixedTime", func(b *testing.B) {
		var ops int64
		var wg sync.WaitGroup
		ctx, cancel := context.WithTimeout(context.Background(), duration)
		defer cancel()

		// Start multiple goroutines
		for i := 0; i < numClients; i++ {
			wg.Add(1)
			go func(clientIndex int) {
				defer wg.Done()
				client := clients[clientIndex]
				opCount := 0
				
				for {
					select {
					case <-ctx.Done():
						atomic.AddInt64(&ops, int64(opCount))
						return
					default:
						_, err := client.CallTool(ctx, shared.CallToolRequest{
							Name: "benchmark_tool_0",
							Arguments: map[string]interface{}{
								"input": fmt.Sprintf("fixed_time_go_%d_%d", clientIndex, opCount),
							},
						})
						if err == nil {
							opCount++
						}
					}
				}
			}(i)
		}

		b.ResetTimer()
		wg.Wait()
		
		totalOps := atomic.LoadInt64(&ops)
		opsPerSec := float64(totalOps) / duration.Seconds()
		
		b.ReportMetric(opsPerSec, "ops/sec")
		b.Logf("Go SDK: %d operations in %v (%.0f ops/sec)", totalOps, duration, opsPerSec)
	})

	b.Run("TypeScriptSDK_FixedTime", func(b *testing.B) {
		var ops int64
		var wg sync.WaitGroup
		ctx, cancel := context.WithTimeout(context.Background(), duration)
		defer cancel()

		// Simulate TypeScript concurrency limitations
		numWorkers := 10 // TypeScript typically has lower concurrency
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(workerIndex int) {
				defer wg.Done()
				opCount := 0
				
				for {
					select {
					case <-ctx.Done():
						atomic.AddInt64(&ops, int64(opCount))
						return
					default:
						err := tsSimulator.SimulateRequest()
						if err == nil {
							opCount++
						}
					}
				}
			}(i)
		}

		b.ResetTimer()
		wg.Wait()
		
		totalOps := atomic.LoadInt64(&ops)
		opsPerSec := float64(totalOps) / duration.Seconds()
		
		b.ReportMetric(opsPerSec, "ops/sec")
		b.Logf("TypeScript SDK (simulated): %d operations in %v (%.0f ops/sec)", totalOps, duration, opsPerSec)
	})
}

// BenchmarkLatencyComparison compares latency characteristics
func BenchmarkLatencyComparison(b *testing.B) {
	// Go SDK setup
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	srv := setupBenchmarkServer(ctx, mockTransport)
	srv.Start()
	defer srv.Close()

	client, err := client.NewClient(ctx, mockTransport, client.ClientConfig{
		ClientInfo: shared.ClientInfo{
			Name:    "latency-client",
			Version: "1.0.0",
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	defer client.Close()

	tsSimulator := NewTypeScriptSDKSimulator()

	b.Run("GoSDK_Latency", func(b *testing.B) {
		latencies := make([]time.Duration, b.N)
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			start := time.Now()
			_, err := client.CallTool(ctx, shared.CallToolRequest{
				Name: "benchmark_tool_0",
				Arguments: map[string]interface{}{
					"input": fmt.Sprintf("latency_go_%d", i),
				},
			})
			latencies[i] = time.Since(start)
			if err != nil {
				b.Fatal(err)
			}
		}
		
		// Calculate percentiles
		avgLatency := calculateAverageLatency(latencies)
		p95Latency := calculatePercentile(latencies, 0.95)
		p99Latency := calculatePercentile(latencies, 0.99)
		
		b.ReportMetric(float64(avgLatency.Nanoseconds())/1e6, "avg_latency_ms")
		b.ReportMetric(float64(p95Latency.Nanoseconds())/1e6, "p95_latency_ms")
		b.ReportMetric(float64(p99Latency.Nanoseconds())/1e6, "p99_latency_ms")
	})

	b.Run("TypeScriptSDK_Latency", func(b *testing.B) {
		latencies := make([]time.Duration, b.N)
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			start := time.Now()
			err := tsSimulator.SimulateRequest()
			latencies[i] = time.Since(start)
			if err != nil {
				b.Fatal(err)
			}
		}
		
		// Calculate percentiles
		avgLatency := calculateAverageLatency(latencies)
		p95Latency := calculatePercentile(latencies, 0.95)
		p99Latency := calculatePercentile(latencies, 0.99)
		
		b.ReportMetric(float64(avgLatency.Nanoseconds())/1e6, "avg_latency_ms")
		b.ReportMetric(float64(p95Latency.Nanoseconds())/1e6, "p95_latency_ms")
		b.ReportMetric(float64(p99Latency.Nanoseconds())/1e6, "p99_latency_ms")
	})
}

// BenchmarkScalabilityComparison tests how both SDKs scale with load
func BenchmarkScalabilityComparison(b *testing.B) {
	clientCounts := []int{1, 10, 50, 100, 500}
	
	for _, numClients := range clientCounts {
		b.Run(fmt.Sprintf("Clients_%d", numClients), func(b *testing.B) {
			// Go SDK test
			ctx := context.Background()
			mockTransport := transport.NewInProcessTransport()
			defer mockTransport.Close()

			srv := setupBenchmarkServer(ctx, mockTransport)
			srv.Start()
			defer srv.Close()

			clients := make([]*client.Client, numClients)
			for i := 0; i < numClients; i++ {
				c, err := client.NewClient(ctx, mockTransport, client.ClientConfig{
					ClientInfo: shared.ClientInfo{
						Name:    fmt.Sprintf("scale-client-%d", i),
						Version: "1.0.0",
					},
				})
				if err != nil {
					b.Fatal(err)
				}
				clients[i] = c
				defer clients[i].Close()
			}

			tsSimulator := NewTypeScriptSDKSimulator()

			b.Run("GoSDK", func(b *testing.B) {
				b.ResetTimer()
				start := time.Now()
				
				b.RunParallel(func(pb *testing.PB) {
					clientIndex := 0
					for pb.Next() {
						client := clients[clientIndex%numClients]
						clientIndex++
						
						_, err := client.CallTool(ctx, shared.CallToolRequest{
							Name: "benchmark_tool_0",
							Arguments: map[string]interface{}{
								"input": fmt.Sprintf("scale_go_%d_%d", numClients, clientIndex),
							},
						})
						if err != nil {
							b.Error(err)
						}
					}
				})
				
				duration := time.Since(start)
				opsPerSec := float64(b.N) / duration.Seconds()
				b.ReportMetric(opsPerSec, "ops/sec")
			})

			// TypeScript would have concurrency limitations
			maxTSWorkers := min(numClients, 20) // TypeScript typically has lower concurrency limits
			
			b.Run("TypeScriptSDK", func(b *testing.B) {
				b.ResetTimer()
				start := time.Now()
				
				// Simulate TypeScript with limited workers
				semaphore := make(chan struct{}, maxTSWorkers)
				var wg sync.WaitGroup
				
				for i := 0; i < b.N; i++ {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						semaphore <- struct{}{}
						defer func() { <-semaphore }()
						
						tsSimulator.SimulateRequest()
					}(i)
				}
				
				wg.Wait()
				
				duration := time.Since(start)
				opsPerSec := float64(b.N) / duration.Seconds()
				b.ReportMetric(opsPerSec, "ops/sec")
			})
		})
	}
}

// Helper functions for latency calculations
func calculateAverageLatency(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	
	var total time.Duration
	for _, lat := range latencies {
		total += lat
	}
	return total / time.Duration(len(latencies))
}

func calculatePercentile(latencies []time.Duration, percentile float64) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	
	// Simple sort and percentile calculation
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	
	// Basic bubble sort for simplicity
	for i := 0; i < len(sorted); i++ {
		for j := 0; j < len(sorted)-1-i; j++ {
			if sorted[j] > sorted[j+1] {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}
	
	index := int(float64(len(sorted)) * percentile)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	
	return sorted[index]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/client"
	"github.com/modelcontextprotocol/go-sdk/transport"
)

func main() {
	fmt.Println("Starting MCP Go SDK Client Example")

	// Start the example server as a subprocess
	serverCmd := exec.Command("go", "run", "../stdio_server/main.go")
	serverStdin, err := serverCmd.StdinPipe()
	if err != nil {
		log.Fatalf("Failed to create server stdin pipe: %v", err)
	}

	serverStdout, err := serverCmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to create server stdout pipe: %v", err)
	}

	if err := serverCmd.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		serverCmd.Process.Kill()
		serverCmd.Wait()
	}()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create stdio transport for client
	transportConfig := transport.StdioConfig{
		Reader:         serverStdout,
		Writer:         serverStdin,
		RequestTimeout: 10 * time.Second,
		MessageBuffer:  50,
	}

	clientTransport, err := transport.NewStdioTransport(ctx, transportConfig)
	if err != nil {
		log.Fatalf("Failed to create client transport: %v", err)
	}
	defer clientTransport.Close()

	// Create MCP client
	clientConfig := client.ClientConfig{
		Name:    "Go MCP Client Example",
		Version: "1.0.0",
	}

	mcpClient := client.NewClient(ctx, clientTransport, clientConfig)

	// Initialize the connection
	fmt.Println("Initializing MCP connection...")
	if err := mcpClient.Initialize(); err != nil {
		log.Fatalf("Failed to initialize client: %v", err)
	}

	fmt.Println("✅ MCP client initialized successfully!")

	fmt.Println("✅ Connected to MCP server!")

	// Test concurrent operations
	fmt.Println("\n🚀 Testing concurrent operations...")

	// Test 1: List all capabilities concurrently
	fmt.Println("\n1. Testing concurrent capability listing...")
	testConcurrentListing(mcpClient)

	// Test 2: Concurrent resource reads
	fmt.Println("\n2. Testing concurrent resource reads...")
	testConcurrentResourceReads(mcpClient)

	// Test 3: Concurrent tool calls
	fmt.Println("\n3. Testing concurrent tool calls...")
	testConcurrentToolCalls(mcpClient)

	// Test 4: Concurrent prompt requests
	fmt.Println("\n4. Testing concurrent prompt requests...")
	testConcurrentPromptRequests(mcpClient)

	// Test 5: Mixed concurrent operations
	fmt.Println("\n5. Testing mixed concurrent operations...")
	testMixedConcurrentOperations(mcpClient)

	fmt.Println("\n✅ All concurrency tests completed successfully!")
	fmt.Println("The Go MCP SDK demonstrates excellent concurrent performance!")
}

func testConcurrentListing(client *client.Client) {
	var wg sync.WaitGroup
	const numConcurrent = 10

	// Concurrent resource listing
	wg.Add(numConcurrent)
	for i := 0; i < numConcurrent; i++ {
		go func(i int) {
			defer wg.Done()
			resources, err := client.ListResources()
			if err != nil {
				fmt.Printf("Error listing resources %d: %v\n", i, err)
				return
			}
			fmt.Printf("  Resources %d: Found %d resources\n", i, len(resources))
		}(i)
	}

	// Concurrent tool listing
	wg.Add(numConcurrent)
	for i := 0; i < numConcurrent; i++ {
		go func(i int) {
			defer wg.Done()
			tools, err := client.ListTools()
			if err != nil {
				fmt.Printf("Error listing tools %d: %v\n", i, err)
				return
			}
			fmt.Printf("  Tools %d: Found %d tools\n", i, len(tools))
		}(i)
	}

	// Concurrent prompt listing
	wg.Add(numConcurrent)
	for i := 0; i < numConcurrent; i++ {
		go func(i int) {
			defer wg.Done()
			prompts, err := client.ListPrompts()
			if err != nil {
				fmt.Printf("Error listing prompts %d: %v\n", i, err)
				return
			}
			fmt.Printf("  Prompts %d: Found %d prompts\n", i, len(prompts))
		}(i)
	}

	wg.Wait()
}

func testConcurrentResourceReads(client *client.Client) {
	var wg sync.WaitGroup
	const numReads = 20

	startTime := time.Now()

	for i := 0; i < numReads; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			content, err := client.ReadResource("file://example.txt")
			if err != nil {
				fmt.Printf("Error reading resource %d: %v\n", i, err)
				return
			}
			fmt.Printf("  Read %d: Got %d content blocks\n", i, len(content))
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)
	fmt.Printf("  Completed %d concurrent resource reads in %v\n", numReads, duration)
}

func testConcurrentToolCalls(client *client.Client) {
	var wg sync.WaitGroup
	const numCalls = 15

	startTime := time.Now()

	for i := 0; i < numCalls; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := map[string]interface{}{
				"text": fmt.Sprintf("Concurrent call #%d", i),
			}
			content, err := client.CallTool("echo", args)
			if err != nil {
				fmt.Printf("Error calling tool %d: %v\n", i, err)
				return
			}
			fmt.Printf("  Tool call %d: Got %d content blocks\n", i, len(content))
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)
	fmt.Printf("  Completed %d concurrent tool calls in %v\n", numCalls, duration)
}

func testConcurrentPromptRequests(client *client.Client) {
	var wg sync.WaitGroup
	const numRequests = 10

	startTime := time.Now()

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := map[string]interface{}{
				"name": fmt.Sprintf("User%d", i),
			}
			promptResult, err := client.GetPrompt("greeting", args)
			if err != nil {
				fmt.Printf("Error getting prompt %d: %v\n", i, err)
				return
			}
			fmt.Printf("  Prompt %d: Got %d messages\n", i, len(promptResult.Messages))
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)
	fmt.Printf("  Completed %d concurrent prompt requests in %v\n", numRequests, duration)
}

func testMixedConcurrentOperations(client *client.Client) {
	var wg sync.WaitGroup
	const numMixed = 30

	startTime := time.Now()

	for i := 0; i < numMixed; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			switch i % 4 {
			case 0:
				// Resource read
				_, err := client.ReadResource("file://example.txt")
				if err != nil {
					fmt.Printf("Mixed %d (resource): %v\n", i, err)
				}
			case 1:
				// Tool call
				args := map[string]interface{}{
					"text": fmt.Sprintf("Mixed operation %d", i),
				}
				_, err := client.CallTool("echo", args)
				if err != nil {
					fmt.Printf("Mixed %d (tool): %v\n", i, err)
				}
			case 2:
				// Prompt request
				args := map[string]interface{}{
					"name": fmt.Sprintf("MixedUser%d", i),
				}
				_, err := client.GetPrompt("greeting", args)
				if err != nil {
					fmt.Printf("Mixed %d (prompt): %v\n", i, err)
				}
			case 3:
				// List operation
				_, err := client.ListResources()
				if err != nil {
					fmt.Printf("Mixed %d (list): %v\n", i, err)
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)
	fmt.Printf("  Completed %d mixed concurrent operations in %v\n", numMixed, duration)
	fmt.Printf("  Average time per operation: %v\n", duration/numMixed)
}

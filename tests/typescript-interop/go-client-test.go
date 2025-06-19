package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/client"
	"github.com/modelcontextprotocol/go-sdk/transport"
)

type TestResult struct {
	Name    string
	Passed  bool
	Error   string
	Details interface{}
}

func main() {
	log.SetOutput(os.Stderr) // Log to stderr to not interfere with protocol
	log.Println("=== Go Client → TypeScript Server Interoperability Tests ===")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start TypeScript server
	cmd := exec.CommandContext(ctx, "npx", "tsx", "typescript-server.ts")
	cmd.Dir = "."          // Current directory
	cmd.Stderr = os.Stderr // Show server logs

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		log.Fatalf("Failed to get stdin pipe: %v", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start TypeScript server: %v", err)
	}
	defer cmd.Process.Kill()

	// Give server time to start
	time.Sleep(2 * time.Second)

	// Create transport using the pipes
	transport, err := transport.NewStdioTransport(ctx, transport.StdioConfig{
		Reader:         stdoutPipe,
		Writer:         stdinPipe,
		RequestTimeout: 5 * time.Second,
		MessageBuffer:  100,
	})
	if err != nil {
		log.Fatalf("Failed to create transport: %v", err)
	}
	defer transport.Close()

	// Create client
	client := client.NewClient(ctx, transport, client.ClientConfig{
		Name:    "go-test-client",
		Version: "1.0.0",
	})

	results := []TestResult{}

	// Test 1: Initialize
	log.Println("Test 1: Initialize connection...")
	err = client.Initialize()
	if err != nil {
		results = append(results, TestResult{Name: "Initialize", Passed: false, Error: err.Error()})
	} else {
		results = append(results, TestResult{Name: "Initialize", Passed: true})
		log.Println("✓ Initialize successful")
	}

	// Test 2: List Resources
	log.Println("Test 2: List Resources...")
	resources, err := client.ListResources()
	if err != nil {
		results = append(results, TestResult{Name: "List Resources", Passed: false, Error: err.Error()})
	} else {
		results = append(results, TestResult{Name: "List Resources", Passed: true, Details: resources})
		log.Printf("✓ List Resources: %d resources found", len(resources))
	}

	// Test 3: Read Resource
	log.Println("Test 3: Read Resource...")
	contents, err := client.ReadResource("file://example.txt")
	if err != nil {
		results = append(results, TestResult{Name: "Read Resource", Passed: false, Error: err.Error()})
	} else {
		results = append(results, TestResult{Name: "Read Resource", Passed: true, Details: contents})
		log.Printf("✓ Read Resource: %d content items", len(contents))
	}

	// Test 4: List Tools
	log.Println("Test 4: List Tools...")
	tools, err := client.ListTools()
	if err != nil {
		results = append(results, TestResult{Name: "List Tools", Passed: false, Error: err.Error()})
	} else {
		results = append(results, TestResult{Name: "List Tools", Passed: true, Details: tools})
		log.Printf("✓ List Tools: %d tools found", len(tools))
	}

	// Test 5: Call Tool
	log.Println("Test 5: Call Tool...")
	toolResult, err := client.CallTool("echo", map[string]interface{}{
		"text": "Hello from Go client!",
	})
	if err != nil {
		results = append(results, TestResult{Name: "Call Tool", Passed: false, Error: err.Error()})
	} else {
		results = append(results, TestResult{Name: "Call Tool", Passed: true, Details: toolResult})
		log.Println("✓ Call Tool successful")
	}

	// Test 6: List Prompts
	log.Println("Test 6: List Prompts...")
	prompts, err := client.ListPrompts()
	if err != nil {
		results = append(results, TestResult{Name: "List Prompts", Passed: false, Error: err.Error()})
	} else {
		results = append(results, TestResult{Name: "List Prompts", Passed: true, Details: prompts})
		log.Printf("✓ List Prompts: %d prompts found", len(prompts))
	}

	// Test 7: Get Prompt
	log.Println("Test 7: Get Prompt...")
	prompt, err := client.GetPrompt("greeting", map[string]interface{}{
		"name": "Go User",
	})
	if err != nil {
		results = append(results, TestResult{Name: "Get Prompt", Passed: false, Error: err.Error()})
	} else {
		results = append(results, TestResult{Name: "Get Prompt", Passed: true, Details: prompt})
		log.Println("✓ Get Prompt successful")
	}

	// Print summary
	log.Println("\n=== Test Summary ===")
	passed := 0
	failed := 0
	for _, result := range results {
		if result.Passed {
			passed++
			log.Printf("✓ %s", result.Name)
		} else {
			failed++
			log.Printf("✗ %s: %s", result.Name, result.Error)
		}
	}

	log.Printf("\nTotal tests: %d", len(results))
	log.Printf("Passed: %d", passed)
	log.Printf("Failed: %d", failed)

	// Print detailed results for debugging
	if failed > 0 {
		log.Println("\n=== Detailed Results ===")
		for _, result := range results {
			if result.Details != nil {
				data, _ := json.MarshalIndent(result.Details, "", "  ")
				log.Printf("%s result: %s", result.Name, string(data))
			}
		}
	}

	if failed == 0 {
		log.Println("\n✅ All tests passed! Go client is compatible with TypeScript server.")
	} else {
		log.Println("\n❌ Some tests failed.")
		os.Exit(1)
	}
}

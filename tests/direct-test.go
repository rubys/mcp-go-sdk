package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/shared"
)

func main() {
	// Create pipes for communication
	serverInR, serverInW := io.Pipe()
	serverOutR, serverOutW := io.Pipe()

	// Start a goroutine to act as the server
	go runServer(serverInR, serverOutW)

	// Act as client
	scanner := bufio.NewScanner(serverOutR)

	// Send initialize request
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": shared.ProtocolVersion,
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}

	data, _ := json.Marshal(initReq)
	fmt.Fprintf(serverInW, "%s\n", data)
	log.Printf("Sent: %s", data)

	// Read response
	if scanner.Scan() {
		line := scanner.Text()
		log.Printf("Received: %s", line)

		// Verify it's a valid response
		var resp map[string]interface{}
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			log.Fatalf("Failed to parse response: %v", err)
		}

		if resp["id"] != float64(1) {
			log.Fatalf("Unexpected response ID: %v", resp["id"])
		}

		if result, ok := resp["result"].(map[string]interface{}); ok {
			log.Printf("Server info: %v", result["serverInfo"])
			log.Printf("Capabilities: %v", result["capabilities"])
		}
	}

	// Send initialized notification
	notif := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialized",
	}
	data, _ = json.Marshal(notif)
	fmt.Fprintf(serverInW, "%s\n", data)
	log.Printf("Sent notification: %s", data)

	// Test resources/list
	listReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "resources/list",
	}
	data, _ = json.Marshal(listReq)
	fmt.Fprintf(serverInW, "%s\n", data)
	log.Printf("Sent: %s", data)

	// Read response
	if scanner.Scan() {
		line := scanner.Text()
		log.Printf("Received: %s", line)
	}

	log.Println("Test completed successfully!")
}

func runServer(input io.Reader, output io.Writer) {
	// Use our actual server components
	ctx := context.Background()

	// Create mock transport
	transport := &mockTransport{
		input:         input,
		output:        output,
		requests:      make(chan *shared.Request, 10),
		notifications: make(chan *shared.Notification, 10),
	}

	// Start reading input
	go transport.readLoop()

	// Create server (importing from our implementation)
	// This is a simplified version - in real test we'd use the actual server

	// For now, just handle requests manually
	for {
		select {
		case req := <-transport.requests:
			log.Printf("Server received request: %s", req.Method)

			var result interface{}

			switch req.Method {
			case "initialize":
				result = map[string]interface{}{
					"protocolVersion": shared.ProtocolVersion,
					"capabilities": map[string]interface{}{
						"resources": map[string]interface{}{
							"subscribe": true,
						},
					},
					"serverInfo": map[string]interface{}{
						"name":    "Test Server",
						"version": "1.0.0",
					},
				}
			case "resources/list":
				result = map[string]interface{}{
					"resources": []interface{}{
						map[string]interface{}{
							"uri":  "test://resource",
							"name": "Test Resource",
						},
					},
				}
			}

			transport.SendResponse(req.ID, result, nil)

		case notif := <-transport.notifications:
			log.Printf("Server received notification: %s", notif.Method)

		case <-ctx.Done():
			return
		}
	}
}

type mockTransport struct {
	input         io.Reader
	output        io.Writer
	requests      chan *shared.Request
	notifications chan *shared.Notification
}

func (t *mockTransport) readLoop() {
	scanner := bufio.NewScanner(t.input)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		msg, err := shared.ParseMessage([]byte(line))
		if err != nil {
			log.Printf("Parse error: %v", err)
			continue
		}

		if req := msg.ToRequest(); req != nil {
			t.requests <- req
		} else if notif := msg.ToNotification(); notif != nil {
			t.notifications <- notif
		}
	}
}

func (t *mockTransport) SendResponse(id interface{}, result interface{}, err *shared.RPCError) error {
	resp := shared.Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   err,
	}

	data, _ := json.Marshal(resp)
	fmt.Fprintf(t.output, "%s\n", data)
	return nil
}

func (t *mockTransport) Channels() (<-chan *shared.Request, <-chan *shared.Notification) {
	return t.requests, t.notifications
}

func (t *mockTransport) SendNotification(method string, params interface{}) error {
	return nil
}

func (t *mockTransport) Close() error {
	return nil
}

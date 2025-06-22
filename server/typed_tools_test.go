package server

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/rubys/mcp-go-sdk/shared"
	"github.com/rubys/mcp-go-sdk/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TypedToolHandlerFunc is a function that handles a tool call with typed arguments
type TypedToolHandlerFunc[T any] func(ctx context.Context, name string, args T) ([]shared.Content, error)

// NewTypedToolHandler creates a ToolHandler that automatically binds arguments to a typed struct
func NewTypedToolHandler[T any](handler TypedToolHandlerFunc[T]) ToolHandler {
	return func(ctx context.Context, name string, arguments map[string]interface{}) ([]shared.Content, error) {
		var args T
		if err := bindArguments(arguments, &args); err != nil {
			return nil, fmt.Errorf("failed to bind arguments: %w", err)
		}
		return handler(ctx, name, args)
	}
}

// bindArguments unmarshals the arguments map into the provided struct
func bindArguments(arguments map[string]interface{}, target interface{}) error {
	if target == nil || reflect.ValueOf(target).Kind() != reflect.Ptr {
		return fmt.Errorf("target must be a non-nil pointer")
	}

	data, err := json.Marshal(arguments)
	if err != nil {
		return fmt.Errorf("failed to marshal arguments: %w", err)
	}

	return json.Unmarshal(data, target)
}

// Test structs for various complexity levels

// Simple struct with basic types
type SimpleArgs struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

// Complex struct with nested objects and arrays
type Address struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	Country string `json:"country"`
	ZipCode string `json:"zip_code"`
}

type UserPreferences struct {
	Theme       string   `json:"theme"`
	Timezone    string   `json:"timezone"`
	Newsletters []string `json:"newsletters"`
}

type ComplexArgs struct {
	Name        string          `json:"name"`
	Email       string          `json:"email"`
	Age         int             `json:"age"`
	IsVerified  bool            `json:"is_verified"`
	Balance     float64         `json:"balance"`
	Address     Address         `json:"address"`
	Preferences UserPreferences `json:"preferences"`
	Tags        []string        `json:"tags"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   time.Time       `json:"created_at"`
}

// Calculation struct for mathematical operations
type CalculatorArgs struct {
	Operation string  `json:"operation"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
}

// TestSimpleTypedToolHandler tests basic typed tool functionality
func TestSimpleTypedToolHandler(t *testing.T) {
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	config := ServerConfig{
		Name:                  "typed-tool-test",
		Version:               "1.0.0",
		MaxConcurrentRequests: 10,
		RequestTimeout:        5 * time.Second,
		Capabilities: shared.ServerCapabilities{
			Tools: &shared.ToolsCapability{
				ListChanged: true,
			},
		},
	}

	server := NewServer(ctx, mockTransport, config)
	require.NoError(t, server.Start())
	defer server.Close()

	// Create a typed handler for simple arguments
	typedHandler := NewTypedToolHandler(func(ctx context.Context, name string, args SimpleArgs) ([]shared.Content, error) {
		if args.Name == "" {
			return nil, fmt.Errorf("name is required")
		}
		if args.Age < 0 {
			return nil, fmt.Errorf("age must be non-negative")
		}

		message := fmt.Sprintf("Hello, %s! You are %d years old.", args.Name, args.Age)
		return []shared.Content{
			shared.TextContent{Type: "text", Text: message},
		}, nil
	})

	// Register the typed tool
	server.RegisterTool("greeting", "Generate a personalized greeting", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the person to greet",
			},
			"age": map[string]interface{}{
				"type":        "number",
				"description": "Age of the person",
			},
		},
		"required": []string{"name"},
	}, typedHandler)

	// Test valid arguments
	result, err := typedHandler(ctx, "greeting", map[string]interface{}{
		"name": "Alice",
		"age":  30,
	})
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "Hello, Alice! You are 30 years old.", result[0].(shared.TextContent).Text)

	// Test missing required field
	_, err = typedHandler(ctx, "greeting", map[string]interface{}{
		"age": 25,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")

	// Test invalid age
	_, err = typedHandler(ctx, "greeting", map[string]interface{}{
		"name": "Bob",
		"age":  -5,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "age must be non-negative")
}

// TestComplexTypedToolHandler tests nested objects and complex data structures
func TestComplexTypedToolHandler(t *testing.T) {
	ctx := context.Background()

	// Create a typed handler for complex arguments
	typedHandler := NewTypedToolHandler(func(ctx context.Context, name string, args ComplexArgs) ([]shared.Content, error) {
		// Validate required fields
		if args.Name == "" {
			return nil, fmt.Errorf("name is required")
		}
		if args.Email == "" {
			return nil, fmt.Errorf("email is required")
		}

		// Build comprehensive response
		response := fmt.Sprintf("User Profile: %s (%s)", args.Name, args.Email)
		
		if args.Age > 0 {
			response += fmt.Sprintf(", Age: %d", args.Age)
		}
		
		if args.IsVerified {
			response += ", Verified Account"
		}
		
		if args.Balance > 0 {
			response += fmt.Sprintf(", Balance: $%.2f", args.Balance)
		}
		
		if args.Address.City != "" {
			response += fmt.Sprintf(", Location: %s, %s", args.Address.City, args.Address.Country)
		}
		
		if args.Preferences.Theme != "" {
			response += fmt.Sprintf(", Theme: %s", args.Preferences.Theme)
		}
		
		if len(args.Tags) > 0 {
			response += fmt.Sprintf(", Tags: %v", args.Tags)
		}

		return []shared.Content{
			shared.TextContent{Type: "text", Text: response},
		}, nil
	})

	// Test with complex nested data
	complexArgs := map[string]interface{}{
		"name":        "John Doe",
		"email":       "john@example.com",
		"age":         35,
		"is_verified": true,
		"balance":     1250.75,
		"address": map[string]interface{}{
			"street":   "123 Main St",
			"city":     "New York",
			"country":  "USA",
			"zip_code": "10001",
		},
		"preferences": map[string]interface{}{
			"theme":       "dark",
			"timezone":    "UTC-5",
			"newsletters": []string{"tech", "finance"},
		},
		"tags": []string{"premium", "developer", "early-adopter"},
		"metadata": map[string]interface{}{
			"source":      "web",
			"campaign_id": "summer2024",
		},
		"created_at": "2024-01-15T10:30:00Z",
	}

	result, err := typedHandler(ctx, "profile", complexArgs)
	require.NoError(t, err)
	require.Len(t, result, 1)

	expectedText := "User Profile: John Doe (john@example.com), Age: 35, Verified Account, Balance: $1250.75, Location: New York, USA, Theme: dark, Tags: [premium developer early-adopter]"
	assert.Equal(t, expectedText, result[0].(shared.TextContent).Text)

	// Test with missing required field
	invalidArgs := map[string]interface{}{
		"email": "test@example.com",
		// missing name
	}

	_, err = typedHandler(ctx, "profile", invalidArgs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

// TestTypedCalculatorTool tests mathematical operations with validation
func TestTypedCalculatorTool(t *testing.T) {
	ctx := context.Background()

	// Create a typed calculator handler
	calculatorHandler := NewTypedToolHandler(func(ctx context.Context, name string, args CalculatorArgs) ([]shared.Content, error) {
		// Validate operation
		if args.Operation == "" {
			return nil, fmt.Errorf("operation is required")
		}

		var result float64

		switch args.Operation {
		case "add":
			result = args.X + args.Y
		case "subtract":
			result = args.X - args.Y
		case "multiply":
			result = args.X * args.Y
		case "divide":
			if args.Y == 0 {
				return nil, fmt.Errorf("division by zero")
			}
			result = args.X / args.Y
		case "power":
			if args.X == 0 && args.Y < 0 {
				return nil, fmt.Errorf("cannot raise zero to negative power")
			}
			result = 1
			for i := 0; i < int(args.Y); i++ {
				result *= args.X
			}
		default:
			return nil, fmt.Errorf("unsupported operation: %s", args.Operation)
		}

		return []shared.Content{
			shared.TextContent{
				Type: "text", 
				Text: fmt.Sprintf("%.2f %s %.2f = %.2f", args.X, args.Operation, args.Y, result),
			},
		}, nil
	})

	// Test valid operations
	testCases := []struct {
		name      string
		operation string
		x, y      float64
		expected  string
	}{
		{"addition", "add", 10, 5, "10.00 add 5.00 = 15.00"},
		{"subtraction", "subtract", 10, 3, "10.00 subtract 3.00 = 7.00"},
		{"multiplication", "multiply", 4, 6, "4.00 multiply 6.00 = 24.00"},
		{"division", "divide", 15, 3, "15.00 divide 3.00 = 5.00"},
		{"power", "power", 2, 3, "2.00 power 3.00 = 8.00"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := calculatorHandler(ctx, "calculator", map[string]interface{}{
				"operation": tc.operation,
				"x":         tc.x,
				"y":         tc.y,
			})
			require.NoError(t, err)
			require.Len(t, result, 1)
			assert.Equal(t, tc.expected, result[0].(shared.TextContent).Text)
		})
	}

	// Test error cases
	errorCases := []struct {
		name     string
		args     map[string]interface{}
		errorMsg string
	}{
		{
			"missing operation",
			map[string]interface{}{"x": 5, "y": 3},
			"operation is required",
		},
		{
			"division by zero",
			map[string]interface{}{"operation": "divide", "x": 10, "y": 0},
			"division by zero",
		},
		{
			"unsupported operation",
			map[string]interface{}{"operation": "modulo", "x": 10, "y": 3},
			"unsupported operation: modulo",
		},
		{
			"zero to negative power",
			map[string]interface{}{"operation": "power", "x": 0, "y": -1},
			"cannot raise zero to negative power",
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := calculatorHandler(ctx, "calculator", tc.args)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.errorMsg)
		})
	}
}

// TestTypedToolWithMalformedJSON tests error handling for invalid JSON types
func TestTypedToolWithMalformedData(t *testing.T) {
	ctx := context.Background()

	typedHandler := NewTypedToolHandler(func(ctx context.Context, name string, args SimpleArgs) ([]shared.Content, error) {
		return []shared.Content{
			shared.TextContent{Type: "text", Text: fmt.Sprintf("Hello, %s!", args.Name)},
		}, nil
	})

	// Test with wrong type for age field
	_, err := typedHandler(ctx, "test", map[string]interface{}{
		"name": "Alice",
		"age":  "thirty", // should be number, not string
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to bind arguments")

	// Test with nil arguments
	_, err = typedHandler(ctx, "test", nil)
	assert.NoError(t, err) // Should work with nil, binding to zero values

	// Test with empty arguments
	result, err := typedHandler(ctx, "test", map[string]interface{}{})
	assert.NoError(t, err) // Should work with empty args, using zero values
	require.Len(t, result, 1)
	assert.Equal(t, "Hello, !", result[0].(shared.TextContent).Text) // Empty name
}

// TestTypedToolHandlerIntegration tests typed handlers in a full server setup
func TestTypedToolHandlerIntegration(t *testing.T) {
	ctx := context.Background()
	mockTransport := transport.NewInProcessTransport()
	defer mockTransport.Close()

	config := ServerConfig{
		Name:                  "typed-integration-test",
		Version:               "1.0.0",
		MaxConcurrentRequests: 10,
		RequestTimeout:        5 * time.Second,
		Capabilities: shared.ServerCapabilities{
			Tools: &shared.ToolsCapability{
				ListChanged: true,
			},
		},
	}

	server := NewServer(ctx, mockTransport, config)
	require.NoError(t, server.Start())
	defer server.Close()

	// Register multiple typed tools
	greetingHandler := NewTypedToolHandler(func(ctx context.Context, name string, args SimpleArgs) ([]shared.Content, error) {
		return []shared.Content{
			shared.TextContent{Type: "text", Text: fmt.Sprintf("Hello, %s!", args.Name)},
		}, nil
	})

	calculatorHandler := NewTypedToolHandler(func(ctx context.Context, name string, args CalculatorArgs) ([]shared.Content, error) {
		result := args.X + args.Y
		return []shared.Content{
			shared.TextContent{Type: "text", Text: fmt.Sprintf("%.2f", result)},
		}, nil
	})

	server.RegisterTool("greeting", "Greeting tool", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{"type": "string"},
			"age":  map[string]interface{}{"type": "number"},
		},
	}, greetingHandler)

	server.RegisterTool("calculator", "Calculator tool", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"operation": map[string]interface{}{"type": "string"},
			"x":         map[string]interface{}{"type": "number"},
			"y":         map[string]interface{}{"type": "number"},
		},
	}, calculatorHandler)

	// Verify tools are registered
	server.toolMu.RLock()
	assert.Contains(t, server.toolHandlers, "greeting")
	assert.Contains(t, server.toolHandlers, "calculator")
	assert.Contains(t, server.toolMeta, "greeting")
	assert.Contains(t, server.toolMeta, "calculator")
	server.toolMu.RUnlock()

	// Test greeting tool
	result, err := greetingHandler(ctx, "greeting", map[string]interface{}{
		"name": "Integration Test",
		"age":  1,
	})
	require.NoError(t, err)
	assert.Equal(t, "Hello, Integration Test!", result[0].(shared.TextContent).Text)

	// Test calculator tool
	result, err = calculatorHandler(ctx, "calculator", map[string]interface{}{
		"operation": "add",
		"x":         10.5,
		"y":         20.3,
	})
	require.NoError(t, err)
	assert.Equal(t, "30.80", result[0].(shared.TextContent).Text)
}

// TestConcurrentTypedToolHandlers tests typed handlers under concurrent load
func TestConcurrentTypedToolHandlers(t *testing.T) {
	ctx := context.Background()

	// Create a thread-safe typed handler
	typedHandler := NewTypedToolHandler(func(ctx context.Context, name string, args CalculatorArgs) ([]shared.Content, error) {
		// Simulate some work
		time.Sleep(1 * time.Millisecond)
		
		result := args.X + args.Y
		return []shared.Content{
			shared.TextContent{Type: "text", Text: fmt.Sprintf("%.2f", result)},
		}, nil
	})

	// Run concurrent executions
	numGoroutines := 50
	results := make(chan []shared.Content, numGoroutines)
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(i int) {
			result, err := typedHandler(ctx, "concurrent-test", map[string]interface{}{
				"operation": "add",
				"x":         float64(i),
				"y":         float64(i * 2),
			})
			
			if err != nil {
				errors <- err
			} else {
				results <- result
			}
		}(i)
	}

	// Collect results
	var successCount int
	for i := 0; i < numGoroutines; i++ {
		select {
		case result := <-results:
			successCount++
			assert.Len(t, result, 1)
			assert.Contains(t, result[0].(shared.TextContent).Text, ".")
		case err := <-errors:
			t.Errorf("Unexpected error: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("Test timed out")
		}
	}

	assert.Equal(t, numGoroutines, successCount, "All concurrent executions should succeed")
}
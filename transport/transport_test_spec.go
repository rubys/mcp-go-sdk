package transport

// This file defines the test specifications for MCP transport implementations
// based on the official TypeScript SDK behavior and protocol requirements.
//
// All transport implementations MUST pass these tests to ensure compatibility
// with the TypeScript SDK and adherence to the MCP protocol.

import (
	"testing"
	"time"

	"github.com/rubys/mcp-go-sdk/shared"
)

// TransportTestSuite defines the common test interface that all transports must satisfy
type TransportTestSuite interface {
	// TestBasicCommunication verifies basic request/response and notification handling
	TestBasicCommunication(t *testing.T, transport Transport)

	// TestConcurrentRequests verifies handling of multiple concurrent requests
	TestConcurrentRequests(t *testing.T, transport Transport)

	// TestConcurrentNotifications verifies handling of multiple concurrent notifications
	TestConcurrentNotifications(t *testing.T, transport Transport)

	// TestRequestResponseCorrelation verifies correct request/response matching
	TestRequestResponseCorrelation(t *testing.T, transport Transport)

	// TestRequestTimeout verifies timeout handling for pending requests
	TestRequestTimeout(t *testing.T, transport Transport)

	// TestLargeMessages verifies handling of large JSON-RPC messages
	TestLargeMessages(t *testing.T, transport Transport)

	// TestMalformedMessages verifies error handling for invalid JSON-RPC messages
	TestMalformedMessages(t *testing.T, transport Transport)

	// TestConnectionFailure verifies behavior during connection failures
	TestConnectionFailure(t *testing.T, transport Transport)

	// TestGracefulShutdown verifies proper cleanup during shutdown
	TestGracefulShutdown(t *testing.T, transport Transport)

	// TestBackpressure verifies handling when message buffers are full
	TestBackpressure(t *testing.T, transport Transport)

	// TestContextCancellation verifies proper handling of context cancellation
	TestContextCancellation(t *testing.T, transport Transport)

	// TestTypeScriptCompatibility verifies parameter translation for TypeScript SDK
	TestTypeScriptCompatibility(t *testing.T, transport Transport)
}

// StdioTransportTests defines specific tests for stdio transport
type StdioTransportTests struct {
	// TestLineDelimitedJSON verifies proper handling of newline-delimited JSON
	TestLineDelimitedJSON func(t *testing.T)

	// TestPartialReads verifies handling of partial message reads
	TestPartialReads func(t *testing.T)

	// TestIOErrors verifies handling of read/write errors
	TestIOErrors func(t *testing.T)

	// TestSeparateReadWriteGoroutines verifies non-blocking I/O
	TestSeparateReadWriteGoroutines func(t *testing.T)
}

// StreamableHTTPTransportTests defines specific tests for Streamable HTTP transport
type StreamableHTTPTransportTests struct {
	// TestHTTPPostRequests verifies client-to-server communication via POST
	TestHTTPPostRequests func(t *testing.T)

	// TestSSEServerToClient verifies server-to-client SSE streaming
	TestSSEServerToClient func(t *testing.T)

	// TestSessionManagement verifies stateful session handling
	TestSessionManagement func(t *testing.T)

	// TestConcurrentClients verifies handling of multiple concurrent clients
	TestConcurrentClients func(t *testing.T)

	// TestConnectionReuse verifies HTTP connection pooling
	TestConnectionReuse func(t *testing.T)

	// TestChunkedTransferEncoding verifies support for chunked responses
	TestChunkedTransferEncoding func(t *testing.T)

	// TestHTTPHeaders verifies proper header handling (Content-Type, etc.)
	TestHTTPHeaders func(t *testing.T)

	// TestHTTPStatusCodes verifies proper status code handling
	TestHTTPStatusCodes func(t *testing.T)

	// TestSSEReconnection verifies SSE auto-reconnection behavior
	TestSSEReconnection func(t *testing.T)

	// TestSSEHeartbeat verifies SSE keepalive/heartbeat messages
	TestSSEHeartbeat func(t *testing.T)

	// TestRequestBodySizeLimit verifies handling of large request bodies
	TestRequestBodySizeLimit func(t *testing.T)

	// TestCORS verifies CORS header support for web clients
	TestCORS func(t *testing.T)
}

// ExpectedBehaviors defines the expected behaviors for TypeScript SDK compatibility
type ExpectedBehaviors struct {
	// ParameterTranslation defines how parameters should be translated
	ParameterTranslation struct {
		// ResourceRead: string URI -> {uri: string} object
		ResourceReadTranslation bool

		// ToolCall: parameters passed as-is
		ToolCallPassthrough bool

		// PromptGet: parameters passed as-is
		PromptGetPassthrough bool
	}

	// MessageFormat defines expected message formatting
	MessageFormat struct {
		// Single content items as objects, multiple as arrays
		PromptMessageContentFormat bool

		// TextContent includes URI field for resources
		TextContentURIField bool
	}

	// ErrorHandling defines expected error behaviors
	ErrorHandling struct {
		// Timeout errors return specific error code
		TimeoutErrorCode int

		// Malformed JSON returns parse error
		ParseErrorCode int

		// Connection failures trigger reconnection
		AutoReconnect bool
	}

	// Performance defines expected performance characteristics
	Performance struct {
		// Minimum requests per second
		MinRequestsPerSecond int

		// Maximum latency for local communication (stdio)
		MaxLocalLatencyMs int

		// Maximum latency for network communication (HTTP)
		MaxNetworkLatencyMs int
	}
}

// TestScenarios defines common test scenarios for all transports
var TestScenarios = struct {
	// SimpleEcho tests basic request/response
	SimpleEcho func(transport Transport) error

	// ConcurrentLoad tests high concurrent load
	ConcurrentLoad func(transport Transport, numRequests int) error

	// StreamingData tests streaming/progressive responses (if supported)
	StreamingData func(transport Transport) error

	// ErrorRecovery tests recovery from various error conditions
	ErrorRecovery func(transport Transport) error

	// TypeScriptInterop tests compatibility with TypeScript SDK
	TypeScriptInterop func(transport Transport) error
}{}

// CommonTransportBehaviors defines behaviors all transports must implement
type CommonTransportBehaviors struct {
	// MustSupportJSONRPC20 verifies JSON-RPC 2.0 compliance
	MustSupportJSONRPC20 bool

	// MustCorrelateResponses verifies request/response correlation by ID
	MustCorrelateResponses bool

	// MustHandleNotifications verifies one-way notification support
	MustHandleNotifications bool

	// MustSupportConcurrency verifies concurrent request handling
	MustSupportConcurrency bool

	// MustHandleContextCancellation verifies context cancellation
	MustHandleContextCancellation bool

	// MustProvideStatistics verifies statistics/metrics availability
	MustProvideStatistics bool

	// MustHandleBackpressure verifies graceful handling of overload
	MustHandleBackpressure bool

	// MustCleanupResources verifies proper resource cleanup
	MustCleanupResources bool
}

// BenchmarkRequirements defines performance benchmarks transports should meet
type BenchmarkRequirements struct {
	// StdioTransport performance requirements
	Stdio struct {
		MinRequestsPerSecond      int // Expected: 100,000+
		MinNotificationsPerSecond int // Expected: 200,000+
		MaxLatencyMicroseconds    int // Expected: < 100μs
	}

	// StreamableHTTP performance requirements
	StreamableHTTP struct {
		MinRequestsPerSecond      int // Expected: 10,000+
		MinConcurrentConnections  int // Expected: 1000+
		MaxLatencyMilliseconds    int // Expected: < 10ms
		MinSSEEventsPerSecond     int // Expected: 50,000+
	}
}

// RaceConditionTests defines tests that must pass with -race flag
type RaceConditionTests struct {
	// Concurrent reads and writes
	ConcurrentReadWrite func(t *testing.T, transport Transport)

	// Concurrent request correlation
	ConcurrentCorrelation func(t *testing.T, transport Transport)

	// Concurrent statistics updates
	ConcurrentStats func(t *testing.T, transport Transport)

	// Concurrent shutdown during operations
	ConcurrentShutdown func(t *testing.T, transport Transport)
}

// ProtocolComplianceTests verifies MCP protocol compliance
type ProtocolComplianceTests struct {
	// Initialize handshake
	TestInitialize func(t *testing.T, transport Transport)

	// Capabilities negotiation
	TestCapabilities func(t *testing.T, transport Transport)

	// Resource operations
	TestResourceOperations func(t *testing.T, transport Transport)

	// Tool operations
	TestToolOperations func(t *testing.T, transport Transport)

	// Prompt operations
	TestPromptOperations func(t *testing.T, transport Transport)

	// Logging operations
	TestLoggingOperations func(t *testing.T, transport Transport)

	// Completion operations
	TestCompletionOperations func(t *testing.T, transport Transport)
}

// SecurityTests defines security-related test cases
type SecurityTests struct {
	// Input validation
	TestInputValidation func(t *testing.T, transport Transport)

	// Message size limits
	TestMessageSizeLimits func(t *testing.T, transport Transport)

	// Rate limiting
	TestRateLimiting func(t *testing.T, transport Transport)

	// Authentication (for HTTP)
	TestAuthentication func(t *testing.T, transport Transport)

	// TLS/SSL (for HTTP)
	TestTLSSupport func(t *testing.T, transport Transport)
}

// InteroperabilityTests defines tests for TypeScript SDK compatibility
type InteroperabilityTests struct {
	// Test with actual TypeScript SDK client
	TestWithTypeScriptClient func(t *testing.T, transport Transport)

	// Test with actual TypeScript SDK server
	TestWithTypeScriptServer func(t *testing.T, transport Transport)

	// Test parameter format translation
	TestParameterTranslation func(t *testing.T, transport Transport)

	// Test response format compatibility
	TestResponseFormatting func(t *testing.T, transport Transport)
}

// TestHelpers provides utility functions for transport testing
type TestHelpers struct {
	// CreateMockTransport creates a mock transport for testing
	CreateMockTransport func(config interface{}) (Transport, error)

	// GenerateLargeMessage creates a large JSON-RPC message for testing
	GenerateLargeMessage func(sizeMB int) *shared.JSONRPCMessage

	// SimulateNetworkFailure simulates network issues
	SimulateNetworkFailure func(transport Transport) error

	// MeasureLatency measures round-trip latency
	MeasureLatency func(transport Transport) time.Duration

	// MeasureThroughput measures messages per second
	MeasureThroughput func(transport Transport, duration time.Duration) float64
}
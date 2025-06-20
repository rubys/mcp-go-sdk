package shared

import (
	"encoding/json"
	"fmt"
)

// Protocol version constants
const (
	ProtocolVersion = "2024-11-05"
)

// Message types for JSON-RPC
type MessageType string

const (
	MessageTypeRequest      MessageType = "request"
	MessageTypeNotification MessageType = "notification"
	MessageTypeResponse     MessageType = "response"
	MessageTypeError        MessageType = "error"
)

// JSONRPCMessage represents the base JSON-RPC message structure
type JSONRPCMessage struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Method  string      `json:"method,omitempty"`
	Params  interface{} `json:"params,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// Request represents a JSON-RPC request
type Request struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// Notification represents a JSON-RPC notification (no ID)
type Notification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// Response represents a JSON-RPC response
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// Implementation represents implementation metadata
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ClientCapabilities represents client capabilities
type ClientCapabilities struct {
	Experimental map[string]interface{} `json:"experimental,omitempty"`
	Sampling     *SamplingCapability    `json:"sampling,omitempty"`
}

// ServerCapabilities represents server capabilities
type ServerCapabilities struct {
	Experimental map[string]interface{} `json:"experimental,omitempty"`
	Logging      *LoggingCapability     `json:"logging,omitempty"`
	Prompts      *PromptsCapability     `json:"prompts,omitempty"`
	Resources    *ResourcesCapability   `json:"resources,omitempty"`
	Tools        *ToolsCapability       `json:"tools,omitempty"`
}

// SamplingCapability represents sampling capabilities
type SamplingCapability struct{}

// LoggingCapability represents logging capabilities
type LoggingCapability struct{}

// PromptsCapability represents prompts capabilities
type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCapability represents resources capabilities
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// ToolsCapability represents tools capabilities
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// Content types
type ContentType string

const (
	ContentTypeText  ContentType = "text"
	ContentTypeImage ContentType = "image"
	ContentTypeAudio ContentType = "audio"
)

// TextContent represents text content
type TextContent struct {
	Type ContentType `json:"type"`
	Text string      `json:"text"`
	URI  string      `json:"uri,omitempty"`
}

// ImageContent represents image content
type ImageContent struct {
	Type     ContentType `json:"type"`
	Data     string      `json:"data"`
	MimeType string      `json:"mimeType"`
}

// AudioContent represents audio content
type AudioContent struct {
	Type     ContentType `json:"type"`
	Data     string      `json:"data"`
	MimeType string      `json:"mimeType"`
}

// Content represents any content block
type Content interface {
	GetType() ContentType
}

func (t TextContent) GetType() ContentType  { return ContentTypeText }
func (i ImageContent) GetType() ContentType { return ContentTypeImage }
func (a AudioContent) GetType() ContentType { return ContentTypeAudio }

// Resource represents a resource
type Resource struct {
	URI         string      `json:"uri"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	MimeType    string      `json:"mimeType,omitempty"`
	Annotations interface{} `json:"annotations,omitempty"`
}

// Tool represents a tool
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// Prompt represents a prompt
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument represents a prompt argument
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// LogLevel represents logging levels
type LogLevel string

const (
	LogLevelDebug   LogLevel = "debug"
	LogLevelInfo    LogLevel = "info"
	LogLevelNotice  LogLevel = "notice"
	LogLevelWarning LogLevel = "warning"
	LogLevelError   LogLevel = "error"
	LogLevelCrit    LogLevel = "crit"
	LogLevelAlert   LogLevel = "alert"
	LogLevelEmerg   LogLevel = "emerg"
)

// LoggingMessage represents a logging message
type LoggingMessage struct {
	Level  LogLevel `json:"level"`
	Data   string   `json:"data"`
	Logger string   `json:"logger,omitempty"`
}

// InitializeRequest represents the initialize request
type InitializeRequest struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      Implementation     `json:"clientInfo"`
}

// InitializeResponse represents the initialize response
type InitializeResponse struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      Implementation     `json:"serverInfo"`
	Instructions    string             `json:"instructions,omitempty"`
}

// Utility functions for message parsing
func ParseMessage(data []byte) (*JSONRPCMessage, error) {
	var msg JSONRPCMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (msg *JSONRPCMessage) IsRequest() bool {
	return msg.Method != "" && msg.ID != nil
}

func (msg *JSONRPCMessage) IsNotification() bool {
	return msg.Method != "" && msg.ID == nil
}

func (msg *JSONRPCMessage) IsResponse() bool {
	return msg.Method == "" && msg.ID != nil
}

func (msg *JSONRPCMessage) ToRequest() *Request {
	if !msg.IsRequest() {
		return nil
	}
	return &Request{
		JSONRPC: msg.JSONRPC,
		ID:      msg.ID,
		Method:  msg.Method,
		Params:  msg.Params,
	}
}

func (msg *JSONRPCMessage) ToNotification() *Notification {
	if !msg.IsNotification() {
		return nil
	}
	return &Notification{
		JSONRPC: msg.JSONRPC,
		Method:  msg.Method,
		Params:  msg.Params,
	}
}

func (msg *JSONRPCMessage) ToResponse() *Response {
	if !msg.IsResponse() {
		return nil
	}
	return &Response{
		JSONRPC: msg.JSONRPC,
		ID:      msg.ID,
		Result:  msg.Result,
		Error:   msg.Error,
	}
}

// ParseMessageResult unmarshals a JSON-RPC result into a destination struct
func ParseMessageResult(result interface{}, dest interface{}) error {
	if result == nil {
		return nil
	}

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return nil
}

// Additional types for client compatibility

// ClientInfo represents client implementation information
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerInfo represents server implementation information  
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Result types for MCP operations

// ListResourcesResult represents the result of listing resources
type ListResourcesResult struct {
	Resources []Resource `json:"resources"`
}

// ReadResourceParams represents parameters for reading a resource
type ReadResourceParams struct {
	URI string `json:"uri"`
}

// ReadResourceResult represents the result of reading a resource
type ReadResourceResult struct {
	Contents []ResourceContent `json:"contents"`
}

// ResourceContent represents the content of a resource
type ResourceContent struct {
	URI      string      `json:"uri"`
	MimeType string      `json:"mimeType"`
	Text     string      `json:"text,omitempty"`
	Blob     string      `json:"blob,omitempty"`
}

// ListToolsResult represents the result of listing tools
type ListToolsResult struct {
	Tools []Tool `json:"tools"`
}

// CallToolRequest represents a tool call request
type CallToolRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// CallToolResult represents the result of calling a tool
type CallToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// ListPromptsResult represents the result of listing prompts
type ListPromptsResult struct {
	Prompts []Prompt `json:"prompts"`
}

// GetPromptRequest represents a prompt request
type GetPromptRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// GetPromptResult represents the result of getting a prompt
type GetPromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// PromptMessage represents a message in a prompt
type PromptMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // Can be Content or []Content
}

// LoggingLevel represents logging level for compatibility
type LoggingLevel = LogLevel

// Progress notification support
// ProgressNotification represents a progress notification following MCP spec
type ProgressNotification struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  ProgressParams `json:"params"`
}

// ProgressParams represents the parameters of a progress notification per MCP spec
type ProgressParams struct {
	ProgressToken interface{} `json:"progressToken"`
	Progress      int         `json:"progress"`
	Total         *int        `json:"total,omitempty"`
	Message       string      `json:"message,omitempty"`
}

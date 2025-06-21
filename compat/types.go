package compat

// Content types for mark3labs/mcp-go compatibility

// ContentType represents the type of content
type ContentType string

const (
	// ContentTypeText represents text content
	ContentTypeText ContentType = "text"
	// ContentTypeImage represents image content  
	ContentTypeImage ContentType = "image"
	// ContentTypeResource represents resource content
	ContentTypeResource ContentType = "resource"
)

// Content represents MCP content
type Content interface {
	isContent()
}

// TextContent represents text content
type TextContent struct {
	Type ContentType `json:"type"`
	Text string      `json:"text"`
}

func (TextContent) isContent() {}

// ImageContent represents image content
type ImageContent struct {
	Type     ContentType `json:"type"`
	Data     string      `json:"data"`
	MIMEType string      `json:"mimeType"`
}

func (ImageContent) isContent() {}

// ResourceContent represents resource content
type ResourceContent struct {
	Type     ContentType `json:"type"`
	Resource Resource    `json:"resource"`
}

func (ResourceContent) isContent() {}

// ProgressNotification represents a progress notification
type ProgressNotification struct {
	Method string         `json:"method"`
	Params ProgressParams `json:"params"`
}

// ProgressParams represents progress notification parameters
type ProgressParams struct {
	ProgressToken interface{} `json:"progressToken"`
	Progress      int         `json:"progress"`
	Total         *int        `json:"total,omitempty"`
	Message       string      `json:"message,omitempty"`
}

// PromptMessage represents a message in a prompt
type PromptMessage struct {
	Role    string    `json:"role"`
	Content []Content `json:"content"`
}
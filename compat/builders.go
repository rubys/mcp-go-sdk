package compat

import (
	"github.com/rubys/mcp-go-sdk/shared"
)

// Resource builder for fluent configuration
type Resource struct {
	uri         string
	name        string
	description string
	mimeType    string
	annotations map[string]interface{}
}

// NewResource creates a new resource builder
func NewResource(uri, name string, options ...ResourceOption) *Resource {
	r := &Resource{
		uri:         uri,
		name:        name,
		annotations: make(map[string]interface{}),
	}
	
	for _, opt := range options {
		opt(r)
	}
	
	return r
}

// ResourceOption configures a resource
type ResourceOption func(*Resource)

// WithResourceDescription sets the resource description
func WithResourceDescription(description string) ResourceOption {
	return func(r *Resource) {
		r.description = description
	}
}

// WithMIMEType sets the resource MIME type
func WithMIMEType(mimeType string) ResourceOption {
	return func(r *Resource) {
		r.mimeType = mimeType
	}
}

// WithResourceAnnotation adds an annotation to the resource
func WithResourceAnnotation(key string, value interface{}) ResourceOption {
	return func(r *Resource) {
		r.annotations[key] = value
	}
}

// Tool builder for fluent configuration
type Tool struct {
	name        string
	description string
	inputSchema map[string]interface{}
}

// NewTool creates a new tool builder
func NewTool(name string, options ...ToolOption) *Tool {
	t := &Tool{
		name: name,
		inputSchema: map[string]interface{}{
			"type":       "object",
			"properties": make(map[string]interface{}),
			"required":   []string{},
		},
	}
	
	for _, opt := range options {
		opt(t)
	}
	
	return t
}

// ToolOption configures a tool
type ToolOption func(*Tool)

// WithDescription sets the tool description
func WithDescription(description string) ToolOption {
	return func(t *Tool) {
		t.description = description
	}
}

// WithString adds a string parameter to the tool
func WithString(name string, options ...ArgumentOption) ToolOption {
	return func(t *Tool) {
		arg := &Argument{
			name:     name,
			argType:  "string",
			required: false,
		}
		
		for _, opt := range options {
			opt(arg)
		}
		
		addArgumentToSchema(t.inputSchema, arg)
	}
}

// WithNumber adds a number parameter to the tool
func WithNumber(name string, options ...ArgumentOption) ToolOption {
	return func(t *Tool) {
		arg := &Argument{
			name:     name,
			argType:  "number",
			required: false,
		}
		
		for _, opt := range options {
			opt(arg)
		}
		
		addArgumentToSchema(t.inputSchema, arg)
	}
}

// WithBoolean adds a boolean parameter to the tool
func WithBoolean(name string, options ...ArgumentOption) ToolOption {
	return func(t *Tool) {
		arg := &Argument{
			name:     name,
			argType:  "boolean",
			required: false,
		}
		
		for _, opt := range options {
			opt(arg)
		}
		
		addArgumentToSchema(t.inputSchema, arg)
	}
}

// Argument represents a tool argument
type Argument struct {
	name        string
	description string
	argType     string
	required    bool
	enum        []string
}

// ArgumentOption configures an argument
type ArgumentOption func(*Argument)

// Required marks an argument as required
func Required() ArgumentOption {
	return func(a *Argument) {
		a.required = true
	}
}

// Description sets the argument description
func Description(description string) ArgumentOption {
	return func(a *Argument) {
		a.description = description
	}
}

// Enum sets the allowed values for the argument
func Enum(values ...string) ArgumentOption {
	return func(a *Argument) {
		a.enum = values
	}
}

// Prompt builder for fluent configuration
type Prompt struct {
	name        string
	description string
	arguments   []shared.PromptArgument
}

// NewPrompt creates a new prompt builder
func NewPrompt(name string, options ...PromptOption) *Prompt {
	p := &Prompt{
		name:      name,
		arguments: []shared.PromptArgument{},
	}
	
	for _, opt := range options {
		opt(p)
	}
	
	return p
}

// PromptOption configures a prompt
type PromptOption func(*Prompt)

// WithPromptDescription sets the prompt description
func WithPromptDescription(description string) PromptOption {
	return func(p *Prompt) {
		p.description = description
	}
}

// WithArgument adds an argument to the prompt
func WithArgument(name string, options ...PromptArgumentOption) PromptOption {
	return func(p *Prompt) {
		arg := shared.PromptArgument{
			Name:     name,
			Required: false,
		}
		
		for _, opt := range options {
			opt(&arg)
		}
		
		p.arguments = append(p.arguments, arg)
	}
}

// PromptArgumentOption configures a prompt argument
type PromptArgumentOption func(*shared.PromptArgument)

// ArgumentDescription sets the prompt argument description
func ArgumentDescription(description string) PromptArgumentOption {
	return func(a *shared.PromptArgument) {
		a.Description = description
	}
}

// RequiredArgument marks a prompt argument as required
func RequiredArgument() PromptArgumentOption {
	return func(a *shared.PromptArgument) {
		a.Required = true
	}
}

// Helper function to add argument to tool schema
func addArgumentToSchema(schema map[string]interface{}, arg *Argument) {
	properties := schema["properties"].(map[string]interface{})
	required := schema["required"].([]string)
	
	prop := map[string]interface{}{
		"type": arg.argType,
	}
	
	if arg.description != "" {
		prop["description"] = arg.description
	}
	
	if len(arg.enum) > 0 {
		prop["enum"] = arg.enum
	}
	
	properties[arg.name] = prop
	
	if arg.required {
		required = append(required, arg.name)
		schema["required"] = required
	}
}

// Convert builders to shared types
func (r *Resource) toShared() shared.Resource {
	return shared.Resource{
		URI:         r.uri,
		Name:        r.name,
		Description: r.description,
		MimeType:    r.mimeType,
		Annotations: r.annotations,
	}
}

func (t *Tool) toShared() shared.Tool {
	return shared.Tool{
		Name:        t.name,
		Description: t.description,
		InputSchema: t.inputSchema,
	}
}

func (p *Prompt) toShared() shared.Prompt {
	return shared.Prompt{
		Name:        p.name,
		Description: p.description,
		Arguments:   p.arguments,
	}
}
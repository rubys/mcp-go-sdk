package server

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/rubys/mcp-go-sdk/shared"
)

// ResourceTemplate represents a template for dynamic resource generation
type ResourceTemplate struct {
	// URITemplate is the RFC 6570 URI template for generating resource URIs
	URITemplate string `json:"uriTemplate"`

	// Description describes what this template is for
	Description string `json:"description,omitempty"`

	// MimeType is the MIME type for all resources matching this template
	MimeType string `json:"mimeType,omitempty"`

	// Name is the identifier for this template
	Name string `json:"name"`

	// List callback for listing resources matching this template
	ListCallback ResourceTemplateListCallback `json:"-"`

	// Complete callbacks for parameter completion
	CompleteCallbacks map[string]ResourceTemplateCompleteCallback `json:"-"`
}

// ResourceTemplateListCallback generates a list of resources for this template
type ResourceTemplateListCallback func(ctx context.Context) (*ResourceTemplateListResult, error)

// ResourceTemplateCompleteCallback provides completion suggestions for template parameters
type ResourceTemplateCompleteCallback func(value string, context *CompletionContext) ([]string, error)

// ResourceTemplateListResult contains the result of listing template resources
type ResourceTemplateListResult struct {
	Resources []shared.Resource `json:"resources"`
}

// CompletionContext provides context for parameter completion
type CompletionContext struct {
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// ResourceTemplateReadCallback handles reading from template-generated URIs
type ResourceTemplateReadCallback func(ctx context.Context, uri string) (*shared.ReadResourceResult, error)

// RegisteredResourceTemplate represents a registered resource template
type RegisteredResourceTemplate struct {
	Template     *ResourceTemplate
	ReadCallback ResourceTemplateReadCallback
	Enabled      bool
	mu           sync.RWMutex

	// Update methods
	updateFn func(*ResourceTemplateUpdate)
	removeFn func()
}

// ResourceTemplateUpdate contains fields that can be updated
type ResourceTemplateUpdate struct {
	Description       *string
	MimeType          *string
	Enabled           *bool
	ReadCallback      ResourceTemplateReadCallback
	ListCallback      ResourceTemplateListCallback
	CompleteCallbacks map[string]ResourceTemplateCompleteCallback
}

// Update modifies the resource template
func (rt *RegisteredResourceTemplate) Update(update *ResourceTemplateUpdate) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if update.Description != nil {
		rt.Template.Description = *update.Description
	}
	if update.MimeType != nil {
		rt.Template.MimeType = *update.MimeType
	}
	if update.Enabled != nil {
		rt.Enabled = *update.Enabled
	}
	if update.ReadCallback != nil {
		rt.ReadCallback = update.ReadCallback
	}
	if update.ListCallback != nil {
		rt.Template.ListCallback = update.ListCallback
	}
	if update.CompleteCallbacks != nil {
		rt.Template.CompleteCallbacks = update.CompleteCallbacks
	}

	if rt.updateFn != nil {
		rt.updateFn(update)
	}
}

// Remove removes the resource template
func (rt *RegisteredResourceTemplate) Remove() {
	if rt.removeFn != nil {
		rt.removeFn()
	}
}

// Enable enables the resource template
func (rt *RegisteredResourceTemplate) Enable() {
	enabled := true
	rt.Update(&ResourceTemplateUpdate{Enabled: &enabled})
}

// Disable disables the resource template
func (rt *RegisteredResourceTemplate) Disable() {
	enabled := false
	rt.Update(&ResourceTemplateUpdate{Enabled: &enabled})
}

// IsEnabled returns whether the template is enabled
func (rt *RegisteredResourceTemplate) IsEnabled() bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.Enabled
}

// ResourceTemplateRegistry manages resource templates
type ResourceTemplateRegistry struct {
	templates map[string]*RegisteredResourceTemplate
	mu        sync.RWMutex

	// Callbacks for notifications
	onResourceListChanged func()
}

// NewResourceTemplateRegistry creates a new resource template registry
func NewResourceTemplateRegistry() *ResourceTemplateRegistry {
	return &ResourceTemplateRegistry{
		templates: make(map[string]*RegisteredResourceTemplate),
	}
}

// SetResourceListChangedCallback sets the callback for resource list changes
func (r *ResourceTemplateRegistry) SetResourceListChangedCallback(callback func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onResourceListChanged = callback
}

// RegisterTemplate registers a new resource template
func (r *ResourceTemplateRegistry) RegisterTemplate(
	name string,
	template *ResourceTemplate,
	readCallback ResourceTemplateReadCallback,
) (*RegisteredResourceTemplate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.templates[name]; exists {
		return nil, fmt.Errorf("resource template %s is already registered", name)
	}

	template.Name = name

	registered := &RegisteredResourceTemplate{
		Template:     template,
		ReadCallback: readCallback,
		Enabled:      true,
	}

	// Set up update and remove callbacks
	registered.updateFn = func(update *ResourceTemplateUpdate) {
		if r.onResourceListChanged != nil {
			r.onResourceListChanged()
		}
	}

	registered.removeFn = func() {
		r.UnregisterTemplate(name)
	}

	r.templates[name] = registered

	if r.onResourceListChanged != nil {
		r.onResourceListChanged()
	}

	return registered, nil
}

// UnregisterTemplate removes a resource template
func (r *ResourceTemplateRegistry) UnregisterTemplate(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.templates, name)

	if r.onResourceListChanged != nil {
		r.onResourceListChanged()
	}
}

// GetTemplate retrieves a resource template by name
func (r *ResourceTemplateRegistry) GetTemplate(name string) (*RegisteredResourceTemplate, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	template, exists := r.templates[name]
	return template, exists
}

// ListTemplates returns all registered resource templates
func (r *ResourceTemplateRegistry) ListTemplates() []*ResourceTemplate {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var templates []*ResourceTemplate
	for _, registered := range r.templates {
		if registered.IsEnabled() {
			templates = append(templates, registered.Template)
		}
	}

	return templates
}

// FindTemplateForURI finds a template that matches the given URI
func (r *ResourceTemplateRegistry) FindTemplateForURI(uri string) (*RegisteredResourceTemplate, map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, registered := range r.templates {
		if !registered.IsEnabled() {
			continue
		}

		variables, matches := r.matchesTemplate(uri, registered.Template.URITemplate)
		if matches {
			return registered, variables, nil
		}
	}

	return nil, nil, fmt.Errorf("no template found for URI: %s", uri)
}

// matchesTemplate checks if a URI matches a template and extracts variables
func (r *ResourceTemplateRegistry) matchesTemplate(uri, template string) (map[string]string, bool) {
	// Convert URI template to regex pattern
	pattern := template
	variables := make(map[string]string)

	// Find all variable expressions like {var} or {+var} or {var*}
	varRegex := regexp.MustCompile(`\{([+#./;?&]?)([^}]+)(\*?)\}`)
	matches := varRegex.FindAllStringSubmatch(pattern, -1)

	if len(matches) == 0 {
		// No variables, check exact match
		return variables, uri == template
	}

	// Build regex pattern and capture variable names
	varNames := make([]string, 0, len(matches))
	regexPattern := regexp.QuoteMeta(pattern)

	for _, match := range matches {
		operator := match[1]
		varName := match[2]
		modifier := match[3]

		varNames = append(varNames, varName)

		// Replace the escaped template variable with a capture group
		quotedVar := regexp.QuoteMeta(match[0])
		
		// Different operators have different matching patterns
		var replacement string
		switch operator {
		case "+": // Reserved string expansion
			replacement = "(.+)"
		case "#": // Fragment expansion
			replacement = "#([^?]*)"
		case ".": // Label expansion
			replacement = `\.([^./]+)`
		case "/": // Path expansion
			if modifier == "*" {
				replacement = "(/.*)"
			} else {
				replacement = "/([^/]+)"
			}
		case ";": // Parameter expansion
			replacement = ";([^;?&#]+)"
		case "?", "&": // Query expansion
			replacement = "[?&]([^&#]+)"
		default: // Simple string expansion
			replacement = "([^/]+)"
		}

		regexPattern = strings.Replace(regexPattern, quotedVar, replacement, 1)
	}

	// Compile and test the regex
	regex, err := regexp.Compile("^" + regexPattern + "$")
	if err != nil {
		return variables, false
	}

	submatches := regex.FindStringSubmatch(uri)
	if submatches == nil {
		return variables, false
	}

	// Extract variable values  
	for i, varName := range varNames {
		if i+1 < len(submatches) {
			value := submatches[i+1]
			
			// Get the original match to determine the operator
			varMatch := matches[i]
			operator := varMatch[1]
			
			// Clean up operator-specific prefixes
			switch operator {
			case "/": // Path expansion
				if strings.HasPrefix(value, "/") {
					value = value[1:]
				}
			case ".": // Label expansion
				if strings.HasPrefix(value, ".") {
					value = value[1:]
				}
			case "#": // Fragment expansion
				if strings.HasPrefix(value, "#") {
					value = value[1:]
				}
			}
			
			variables[varName] = value
		}
	}

	return variables, true
}

// ListResources lists all resources from all templates
func (r *ResourceTemplateRegistry) ListResources(ctx context.Context) ([]*shared.Resource, error) {
	r.mu.RLock()
	templates := make([]*RegisteredResourceTemplate, 0, len(r.templates))
	for _, template := range r.templates {
		if template.IsEnabled() {
			templates = append(templates, template)
		}
	}
	r.mu.RUnlock()

	var allResources []*shared.Resource

	for _, template := range templates {
		if template.Template.ListCallback != nil {
			result, err := template.Template.ListCallback(ctx)
			if err != nil {
				return nil, fmt.Errorf("error listing resources for template %s: %w", template.Template.Name, err)
			}

			// Apply template metadata to resources that don't override it
			for _, resource := range result.Resources {
				// Make a copy to avoid modifying the original
				resourceCopy := resource

				// Apply template defaults if not overridden
				if resourceCopy.Description == "" && template.Template.Description != "" {
					resourceCopy.Description = template.Template.Description
				}
				if resourceCopy.MimeType == "" && template.Template.MimeType != "" {
					resourceCopy.MimeType = template.Template.MimeType
				}

				allResources = append(allResources, &resourceCopy)
			}
		}
	}

	return allResources, nil
}

// CompleteParameter provides completion suggestions for template parameters
func (r *ResourceTemplateRegistry) CompleteParameter(
	templateName string,
	parameterName string,
	value string,
	context *CompletionContext,
) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	template, exists := r.templates[templateName]
	if !exists {
		return nil, fmt.Errorf("template not found: %s", templateName)
	}

	if !template.IsEnabled() {
		return nil, fmt.Errorf("template is disabled: %s", templateName)
	}

	if template.Template.CompleteCallbacks == nil {
		return []string{}, nil
	}

	callback, exists := template.Template.CompleteCallbacks[parameterName]
	if !exists {
		return []string{}, nil
	}

	return callback(value, context)
}

// ExtractTemplateVariables extracts variable names from a URI template
func ExtractTemplateVariables(uriTemplate string) []string {
	varRegex := regexp.MustCompile(`\{([+#./;?&]?)([^}]+)(\*?)\}`)
	matches := varRegex.FindAllStringSubmatch(uriTemplate, -1)

	variables := []string{} // Initialize as empty slice, not nil
	seen := make(map[string]bool)

	for _, match := range matches {
		varName := match[2]
		if !seen[varName] {
			variables = append(variables, varName)
			seen[varName] = true
		}
	}

	return variables
}

// ValidateURITemplate validates that a URI template is well-formed
func ValidateURITemplate(uriTemplate string) error {
	if uriTemplate == "" {
		return fmt.Errorf("URI template cannot be empty")
	}

	// Check for unmatched braces and nested braces in one pass
	var braceDepth int
	for _, char := range uriTemplate {
		switch char {
		case '{':
			braceDepth++
			if braceDepth > 1 {
				return fmt.Errorf("nested braces not allowed in URI template: %s", uriTemplate)
			}
		case '}':
			braceDepth--
			if braceDepth < 0 {
				return fmt.Errorf("unmatched closing brace in URI template: %s", uriTemplate)
			}
		}
	}

	if braceDepth > 0 {
		return fmt.Errorf("unmatched opening brace in URI template: %s", uriTemplate)
	}

	// Validate variable syntax - use different regex to catch empty variables
	varRegex := regexp.MustCompile(`\{([+#./;?&]?)([^}]*)(\*?)\}`)
	matches := varRegex.FindAllStringSubmatch(uriTemplate, -1)

	for _, match := range matches {
		varName := match[2]
		if varName == "" {
			return fmt.Errorf("empty variable name in URI template: %s", uriTemplate)
		}

		// Check for valid variable name characters
		if !regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`).MatchString(varName) {
			return fmt.Errorf("invalid variable name '%s' in URI template: %s", varName, uriTemplate)
		}
	}

	return nil
}

// ExpandURITemplate expands a URI template with the given variables
func ExpandURITemplate(uriTemplate string, variables map[string]string) (string, error) {
	if err := ValidateURITemplate(uriTemplate); err != nil {
		return "", err
	}

	result := uriTemplate

	// Find all variable expressions
	varRegex := regexp.MustCompile(`\{([+#./;?&]?)([^}*]+)(\*?)\}`)
	matches := varRegex.FindAllStringSubmatch(uriTemplate, -1)

	for _, match := range matches {
		fullMatch := match[0]
		operator := match[1]
		varName := match[2]

		value, exists := variables[varName]
		if !exists {
			return "", fmt.Errorf("variable %s not provided for template %s", varName, uriTemplate)
		}

		// Apply operator-specific formatting
		var replacement string
		switch operator {
		case "+": // Reserved string expansion
			replacement = value
		case "#": // Fragment expansion
			replacement = "#" + value
		case ".": // Label expansion
			replacement = "." + value
		case "/": // Path expansion
			replacement = "/" + value
		case ";": // Parameter expansion
			replacement = ";" + varName + "=" + value
		case "?": // Query expansion
			replacement = "?" + varName + "=" + value
		case "&": // Query continuation
			replacement = "&" + varName + "=" + value
		default: // Simple string expansion
			replacement = value
		}

		result = strings.Replace(result, fullMatch, replacement, 1)
	}

	return result, nil
}
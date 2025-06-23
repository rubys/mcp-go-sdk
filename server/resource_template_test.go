package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/rubys/mcp-go-sdk/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceTemplate_Basic(t *testing.T) {
	template := &ResourceTemplate{
		URITemplate: "test://resource/{id}",
		Description: "Test template",
		MimeType:    "text/plain",
		Name:        "test-template",
	}

	assert.Equal(t, "test://resource/{id}", template.URITemplate)
	assert.Equal(t, "Test template", template.Description)
	assert.Equal(t, "text/plain", template.MimeType)
	assert.Equal(t, "test-template", template.Name)
}

func TestResourceTemplateRegistry_RegisterTemplate(t *testing.T) {
	registry := NewResourceTemplateRegistry()

	template := &ResourceTemplate{
		URITemplate: "test://resource/{id}",
		Description: "Test template",
	}

	readCallback := func(ctx context.Context, uri string) (*shared.ReadResourceResult, error) {
		return &shared.ReadResourceResult{
			Contents: []shared.ResourceContent{
				{
					URI:  uri,
					Text: "Test content",
				},
			},
		}, nil
	}

	registered, err := registry.RegisterTemplate("test", template, readCallback)
	require.NoError(t, err)
	assert.NotNil(t, registered)
	assert.Equal(t, "test", registered.Template.Name)
	assert.True(t, registered.IsEnabled())
}

func TestResourceTemplateRegistry_DuplicateRegistration(t *testing.T) {
	registry := NewResourceTemplateRegistry()

	template := &ResourceTemplate{
		URITemplate: "test://resource/{id}",
	}

	readCallback := func(ctx context.Context, uri string) (*shared.ReadResourceResult, error) {
		return nil, nil
	}

	// First registration should succeed
	_, err := registry.RegisterTemplate("test", template, readCallback)
	require.NoError(t, err)

	// Second registration with same name should fail
	_, err = registry.RegisterTemplate("test", template, readCallback)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestResourceTemplateRegistry_UnregisterTemplate(t *testing.T) {
	registry := NewResourceTemplateRegistry()

	template := &ResourceTemplate{
		URITemplate: "test://resource/{id}",
	}

	readCallback := func(ctx context.Context, uri string) (*shared.ReadResourceResult, error) {
		return nil, nil
	}

	registered, err := registry.RegisterTemplate("test", template, readCallback)
	require.NoError(t, err)

	// Template should exist
	retrieved, exists := registry.GetTemplate("test")
	assert.True(t, exists)
	assert.Equal(t, registered, retrieved)

	// Unregister template
	registry.UnregisterTemplate("test")

	// Template should no longer exist
	_, exists = registry.GetTemplate("test")
	assert.False(t, exists)
}

func TestResourceTemplateRegistry_ListTemplates(t *testing.T) {
	registry := NewResourceTemplateRegistry()

	template1 := &ResourceTemplate{
		URITemplate: "test://resource/{id}",
		Description: "Template 1",
	}

	template2 := &ResourceTemplate{
		URITemplate: "test://other/{name}",
		Description: "Template 2",
	}

	readCallback := func(ctx context.Context, uri string) (*shared.ReadResourceResult, error) {
		return nil, nil
	}

	_, err := registry.RegisterTemplate("test1", template1, readCallback)
	require.NoError(t, err)

	registered2, err := registry.RegisterTemplate("test2", template2, readCallback)
	require.NoError(t, err)

	// Disable one template
	registered2.Disable()

	templates := registry.ListTemplates()
	assert.Len(t, templates, 1)
	assert.Equal(t, "Template 1", templates[0].Description)
}

func TestResourceTemplateRegistry_FindTemplateForURI(t *testing.T) {
	registry := NewResourceTemplateRegistry()

	template := &ResourceTemplate{
		URITemplate: "test://resource/{id}",
	}

	readCallback := func(ctx context.Context, uri string) (*shared.ReadResourceResult, error) {
		return nil, nil
	}

	_, err := registry.RegisterTemplate("test", template, readCallback)
	require.NoError(t, err)

	// Test matching URI
	found, variables, err := registry.FindTemplateForURI("test://resource/123")
	require.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "123", variables["id"])

	// Test non-matching URI
	_, _, err = registry.FindTemplateForURI("test://other/123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no template found")
}

func TestResourceTemplateRegistry_ListResources(t *testing.T) {
	registry := NewResourceTemplateRegistry()

	template := &ResourceTemplate{
		URITemplate: "test://resource/{id}",
		Description: "Test template",
		MimeType:    "text/plain",
		ListCallback: func(ctx context.Context) (*ResourceTemplateListResult, error) {
			return &ResourceTemplateListResult{
				Resources: []shared.Resource{
					{
						URI:  "test://resource/1",
						Name: "Resource 1",
					},
					{
						URI:         "test://resource/2",
						Name:        "Resource 2",
						Description: "Custom description", // Should override template
						MimeType:    "text/markdown",     // Should override template
					},
				},
			}, nil
		},
	}

	readCallback := func(ctx context.Context, uri string) (*shared.ReadResourceResult, error) {
		return nil, nil
	}

	_, err := registry.RegisterTemplate("test", template, readCallback)
	require.NoError(t, err)

	resources, err := registry.ListResources(context.Background())
	require.NoError(t, err)
	assert.Len(t, resources, 2)

	// First resource should inherit template metadata
	assert.Equal(t, "Resource 1", resources[0].Name)
	assert.Equal(t, "Test template", resources[0].Description)
	assert.Equal(t, "text/plain", resources[0].MimeType)

	// Second resource should keep its own metadata
	assert.Equal(t, "Resource 2", resources[1].Name)
	assert.Equal(t, "Custom description", resources[1].Description)
	assert.Equal(t, "text/markdown", resources[1].MimeType)
}

func TestResourceTemplateRegistry_CompleteParameter(t *testing.T) {
	registry := NewResourceTemplateRegistry()

	template := &ResourceTemplate{
		URITemplate: "test://resource/{category}/{id}",
		CompleteCallbacks: map[string]ResourceTemplateCompleteCallback{
			"category": func(value string, context *CompletionContext) ([]string, error) {
				categories := []string{"books", "movies", "music"}
				var matches []string
				for _, cat := range categories {
					if value == "" || cat[:len(value)] == value {
						matches = append(matches, cat)
					}
				}
				return matches, nil
			},
			"id": func(value string, context *CompletionContext) ([]string, error) {
				if context != nil && context.Arguments != nil {
					if category, ok := context.Arguments["category"].(string); ok && category == "books" {
						return []string{"book1", "book2", "book3"}, nil
					}
				}
				return []string{"id1", "id2", "id3"}, nil
			},
		},
	}

	readCallback := func(ctx context.Context, uri string) (*shared.ReadResourceResult, error) {
		return nil, nil
	}

	_, err := registry.RegisterTemplate("test", template, readCallback)
	require.NoError(t, err)

	// Test category completion
	completions, err := registry.CompleteParameter("test", "category", "m", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"movies", "music"}, completions)

	// Test id completion with context
	context := &CompletionContext{
		Arguments: map[string]interface{}{
			"category": "books",
		},
	}
	completions, err = registry.CompleteParameter("test", "id", "", context)
	require.NoError(t, err)
	assert.Equal(t, []string{"book1", "book2", "book3"}, completions)

	// Test non-existent parameter
	completions, err = registry.CompleteParameter("test", "nonexistent", "", nil)
	require.NoError(t, err)
	assert.Empty(t, completions)

	// Test non-existent template
	_, err = registry.CompleteParameter("nonexistent", "category", "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "template not found")
}

func TestRegisteredResourceTemplate_Update(t *testing.T) {
	registry := NewResourceTemplateRegistry()

	template := &ResourceTemplate{
		URITemplate: "test://resource/{id}",
		Description: "Original description",
		MimeType:    "text/plain",
	}

	readCallback := func(ctx context.Context, uri string) (*shared.ReadResourceResult, error) {
		return nil, nil
	}

	registered, err := registry.RegisterTemplate("test", template, readCallback)
	require.NoError(t, err)

	// Update template
	newDescription := "Updated description"
	newMimeType := "text/markdown"
	enabled := false

	registered.Update(&ResourceTemplateUpdate{
		Description: &newDescription,
		MimeType:    &newMimeType,
		Enabled:     &enabled,
	})

	assert.Equal(t, "Updated description", registered.Template.Description)
	assert.Equal(t, "text/markdown", registered.Template.MimeType)
	assert.False(t, registered.IsEnabled())
}

func TestRegisteredResourceTemplate_EnableDisable(t *testing.T) {
	registry := NewResourceTemplateRegistry()

	template := &ResourceTemplate{
		URITemplate: "test://resource/{id}",
	}

	readCallback := func(ctx context.Context, uri string) (*shared.ReadResourceResult, error) {
		return nil, nil
	}

	registered, err := registry.RegisterTemplate("test", template, readCallback)
	require.NoError(t, err)

	// Initially enabled
	assert.True(t, registered.IsEnabled())

	// Disable
	registered.Disable()
	assert.False(t, registered.IsEnabled())

	// Enable
	registered.Enable()
	assert.True(t, registered.IsEnabled())
}

func TestRegisteredResourceTemplate_Remove(t *testing.T) {
	registry := NewResourceTemplateRegistry()

	template := &ResourceTemplate{
		URITemplate: "test://resource/{id}",
	}

	readCallback := func(ctx context.Context, uri string) (*shared.ReadResourceResult, error) {
		return nil, nil
	}

	registered, err := registry.RegisterTemplate("test", template, readCallback)
	require.NoError(t, err)

	// Template should exist
	_, exists := registry.GetTemplate("test")
	assert.True(t, exists)

	// Remove template
	registered.Remove()

	// Template should no longer exist
	_, exists = registry.GetTemplate("test")
	assert.False(t, exists)
}

func TestMatchesTemplate_SimpleVariable(t *testing.T) {
	registry := NewResourceTemplateRegistry()

	tests := []struct {
		template string
		uri      string
		expected map[string]string
		matches  bool
	}{
		{
			template: "test://resource/{id}",
			uri:      "test://resource/123",
			expected: map[string]string{"id": "123"},
			matches:  true,
		},
		{
			template: "test://resource/{id}",
			uri:      "test://other/123",
			expected: map[string]string{},
			matches:  false,
		},
		{
			template: "test://resource/{category}/{id}",
			uri:      "test://resource/books/123",
			expected: map[string]string{"category": "books", "id": "123"},
			matches:  true,
		},
	}

	for _, test := range tests {
		variables, matches := registry.matchesTemplate(test.uri, test.template)
		assert.Equal(t, test.matches, matches, "Template: %s, URI: %s", test.template, test.uri)
		if matches {
			assert.Equal(t, test.expected, variables, "Template: %s, URI: %s", test.template, test.uri)
		}
	}
}

func TestMatchesTemplate_OperatorExpansion(t *testing.T) {
	registry := NewResourceTemplateRegistry()

	tests := []struct {
		template string
		uri      string
		expected map[string]string
		matches  bool
	}{
		{
			// Reserved string expansion
			template: "test://resource{+path}",
			uri:      "test://resource/path/to/resource",
			expected: map[string]string{"path": "/path/to/resource"},
			matches:  true,
		},
		{
			// Fragment expansion
			template: "test://resource{#fragment}",
			uri:      "test://resource#section1",
			expected: map[string]string{"fragment": "section1"},
			matches:  true,
		},
		{
			// Label expansion
			template: "test://resource{.extension}",
			uri:      "test://resource.json",
			expected: map[string]string{"extension": "json"},
			matches:  true,
		},
		{
			// Path expansion
			template: "test://resource{/path}",
			uri:      "test://resource/subpath",
			expected: map[string]string{"path": "subpath"},
			matches:  true,
		},
	}

	for _, test := range tests {
		variables, matches := registry.matchesTemplate(test.uri, test.template)
		assert.Equal(t, test.matches, matches, "Template: %s, URI: %s", test.template, test.uri)
		if matches {
			assert.Equal(t, test.expected, variables, "Template: %s, URI: %s", test.template, test.uri)
		}
	}
}

func TestExtractTemplateVariables(t *testing.T) {
	tests := []struct {
		template string
		expected []string
	}{
		{
			template: "test://resource/{id}",
			expected: []string{"id"},
		},
		{
			template: "test://resource/{category}/{id}",
			expected: []string{"category", "id"},
		},
		{
			template: "test://resource{+path}{.ext}{#fragment}",
			expected: []string{"path", "ext", "fragment"},
		},
		{
			template: "test://resource/{id}/{id}", // Duplicate variable
			expected: []string{"id"},
		},
		{
			template: "test://resource/static",
			expected: []string{},
		},
	}

	for _, test := range tests {
		variables := ExtractTemplateVariables(test.template)
		assert.Equal(t, test.expected, variables, "Template: %s", test.template)
	}
}

func TestValidateURITemplate(t *testing.T) {
	tests := []struct {
		template string
		valid    bool
		error    string
	}{
		{
			template: "test://resource/{id}",
			valid:    true,
		},
		{
			template: "test://resource/{category}/{id}",
			valid:    true,
		},
		{
			template: "test://resource{+path}",
			valid:    true,
		},
		{
			template: "",
			valid:    false,
			error:    "cannot be empty",
		},
		{
			template: "test://resource/{id",
			valid:    false,
			error:    "unmatched opening brace",
		},
		{
			template: "test://resource/id}",
			valid:    false,
			error:    "unmatched closing brace",
		},
		{
			template: "test://resource/{{id}}",
			valid:    false,
			error:    "nested braces",
		},
		{
			template: "test://resource/{}",
			valid:    false,
			error:    "empty variable name",
		},
		{
			template: "test://resource/{123invalid}",
			valid:    false,
			error:    "invalid variable name",
		},
	}

	for _, test := range tests {
		err := ValidateURITemplate(test.template)
		if test.valid {
			assert.NoError(t, err, "Template should be valid: %s", test.template)
		} else {
			assert.Error(t, err, "Template should be invalid: %s", test.template)
			assert.Contains(t, err.Error(), test.error, "Template: %s", test.template)
		}
	}
}

func TestExpandURITemplate(t *testing.T) {
	tests := []struct {
		template  string
		variables map[string]string
		expected  string
		hasError  bool
	}{
		{
			template:  "test://resource/{id}",
			variables: map[string]string{"id": "123"},
			expected:  "test://resource/123",
		},
		{
			template:  "test://resource/{category}/{id}",
			variables: map[string]string{"category": "books", "id": "123"},
			expected:  "test://resource/books/123",
		},
		{
			template:  "test://resource{+path}",
			variables: map[string]string{"path": "/path/to/resource"},
			expected:  "test://resource/path/to/resource",
		},
		{
			template:  "test://resource{#fragment}",
			variables: map[string]string{"fragment": "section1"},
			expected:  "test://resource#section1",
		},
		{
			template:  "test://resource{.ext}",
			variables: map[string]string{"ext": "json"},
			expected:  "test://resource.json",
		},
		{
			template:  "test://resource{/path}",
			variables: map[string]string{"path": "subpath"},
			expected:  "test://resource/subpath",
		},
		{
			template:  "test://resource{;param}",
			variables: map[string]string{"param": "value"},
			expected:  "test://resource;param=value",
		},
		{
			template:  "test://resource{?query}",
			variables: map[string]string{"query": "value"},
			expected:  "test://resource?query=value",
		},
		{
			template:  "test://resource{&param}",
			variables: map[string]string{"param": "value"},
			expected:  "test://resource&param=value",
		},
		{
			template:  "test://resource/{id}",
			variables: map[string]string{}, // Missing variable
			hasError:  true,
		},
	}

	for _, test := range tests {
		result, err := ExpandURITemplate(test.template, test.variables)
		if test.hasError {
			assert.Error(t, err, "Should have error for template: %s", test.template)
		} else {
			assert.NoError(t, err, "Should not have error for template: %s", test.template)
			assert.Equal(t, test.expected, result, "Template: %s", test.template)
		}
	}
}

func TestResourceTemplateRegistry_ResourceListChangedCallback(t *testing.T) {
	registry := NewResourceTemplateRegistry()

	var callbackCalled bool
	registry.SetResourceListChangedCallback(func() {
		callbackCalled = true
	})

	template := &ResourceTemplate{
		URITemplate: "test://resource/{id}",
	}

	readCallback := func(ctx context.Context, uri string) (*shared.ReadResourceResult, error) {
		return nil, nil
	}

	// Register template should trigger callback
	callbackCalled = false
	registered, err := registry.RegisterTemplate("test", template, readCallback)
	require.NoError(t, err)
	assert.True(t, callbackCalled)

	// Update template should trigger callback
	callbackCalled = false
	newDescription := "Updated"
	registered.Update(&ResourceTemplateUpdate{Description: &newDescription})
	assert.True(t, callbackCalled)

	// Remove template should trigger callback
	callbackCalled = false
	registered.Remove()
	assert.True(t, callbackCalled)
}

func TestResourceTemplateRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewResourceTemplateRegistry()

	template := &ResourceTemplate{
		URITemplate: "test://resource/{id}",
	}

	readCallback := func(ctx context.Context, uri string) (*shared.ReadResourceResult, error) {
		return nil, nil
	}

	// Test concurrent registrations
	const numRoutines = 100
	results := make(chan error, numRoutines)

	for i := 0; i < numRoutines; i++ {
		go func(index int) {
			templateName := fmt.Sprintf("test-%d", index)
			_, err := registry.RegisterTemplate(templateName, template, readCallback)
			results <- err
		}(i)
	}

	// Collect results
	for i := 0; i < numRoutines; i++ {
		err := <-results
		assert.NoError(t, err)
	}

	// Verify all templates were registered
	templates := registry.ListTemplates()
	assert.Len(t, templates, numRoutines)
}
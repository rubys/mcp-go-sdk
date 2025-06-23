package shared

import (
	"fmt"
	"strings"
	"testing"
)

func TestUriTemplate_IsTemplate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "simple template",
			input:    "{foo}",
			expected: true,
		},
		{
			name:     "path with template",
			input:    "/users/{id}",
			expected: true,
		},
		{
			name:     "complex template",
			input:    "http://example.com/{path}/{file}",
			expected: true,
		},
		{
			name:     "query template",
			input:    "/search{?q,limit}",
			expected: true,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "plain string",
			input:    "plain string",
			expected: false,
		},
		{
			name:     "url without template",
			input:    "http://example.com/foo/bar",
			expected: false,
		},
		{
			name:     "empty braces",
			input:    "{}",
			expected: false,
		},
		{
			name:     "whitespace braces",
			input:    "{ }",
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			template, _ := NewUriTemplate("")
			result := template.IsTemplate(test.input)
			if result != test.expected {
				t.Errorf("IsTemplate(%q) = %v, want %v", test.input, result, test.expected)
			}
		})
	}
}

func TestUriTemplate_SimpleStringExpansion(t *testing.T) {
	t.Run("simple variable expansion", func(t *testing.T) {
		template, err := NewUriTemplate("http://example.com/users/{username}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"username": "fred"})
		if err != nil {
			t.Fatal(err)
		}

		expected := "http://example.com/users/fred"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}

		names := template.VariableNames()
		if len(names) != 1 || names[0] != "username" {
			t.Errorf("VariableNames() = %v, want [username]", names)
		}
	})

	t.Run("multiple variables", func(t *testing.T) {
		template, err := NewUriTemplate("{x,y}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"x": "1024", "y": "768"})
		if err != nil {
			t.Fatal(err)
		}

		expected := "1024,768"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}

		names := template.VariableNames()
		if len(names) != 2 || names[0] != "x" || names[1] != "y" {
			t.Errorf("VariableNames() = %v, want [x y]", names)
		}
	})

	t.Run("encode reserved characters", func(t *testing.T) {
		template, err := NewUriTemplate("{var}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"var": "value with spaces"})
		if err != nil {
			t.Fatal(err)
		}

		expected := "value+with+spaces"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})

	t.Run("undefined variables", func(t *testing.T) {
		template, err := NewUriTemplate("{x,y}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"x": "1024"})
		if err != nil {
			t.Fatal(err)
		}

		expected := "1024"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})
}

func TestUriTemplate_ReservedExpansion(t *testing.T) {
	t.Run("reserved expansion", func(t *testing.T) {
		template, err := NewUriTemplate("{+var}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"var": "value"})
		if err != nil {
			t.Fatal(err)
		}

		expected := "value"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})

	t.Run("reserved with special chars", func(t *testing.T) {
		template, err := NewUriTemplate("{+path}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"path": "/foo/bar"})
		if err != nil {
			t.Fatal(err)
		}

		expected := "%2Ffoo%2Fbar"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})
}

func TestUriTemplate_FragmentExpansion(t *testing.T) {
	t.Run("fragment expansion", func(t *testing.T) {
		template, err := NewUriTemplate("{#var}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"var": "value"})
		if err != nil {
			t.Fatal(err)
		}

		expected := "#value"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})

	t.Run("fragment with multiple values", func(t *testing.T) {
		template, err := NewUriTemplate("{#var,x}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"var": "value", "x": "1024"})
		if err != nil {
			t.Fatal(err)
		}

		expected := "#value,1024"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})
}

func TestUriTemplate_DotExpansion(t *testing.T) {
	t.Run("dot expansion", func(t *testing.T) {
		template, err := NewUriTemplate("{.var}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"var": "value"})
		if err != nil {
			t.Fatal(err)
		}

		expected := ".value"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})

	t.Run("dot with multiple values", func(t *testing.T) {
		template, err := NewUriTemplate("{.var,x}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"var": "value", "x": "1024"})
		if err != nil {
			t.Fatal(err)
		}

		expected := ".value.1024"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})
}

func TestUriTemplate_PathExpansion(t *testing.T) {
	t.Run("path expansion", func(t *testing.T) {
		template, err := NewUriTemplate("{/var}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"var": "value"})
		if err != nil {
			t.Fatal(err)
		}

		expected := "/value"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})

	t.Run("path with multiple values", func(t *testing.T) {
		template, err := NewUriTemplate("{/var,x}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"var": "value", "x": "1024"})
		if err != nil {
			t.Fatal(err)
		}

		expected := "/value/1024"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})
}

func TestUriTemplate_QueryExpansion(t *testing.T) {
	t.Run("query expansion", func(t *testing.T) {
		template, err := NewUriTemplate("{?var}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"var": "value"})
		if err != nil {
			t.Fatal(err)
		}

		expected := "?var=value"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})

	t.Run("query with multiple values", func(t *testing.T) {
		template, err := NewUriTemplate("{?var,x}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"var": "value", "x": "1024"})
		if err != nil {
			t.Fatal(err)
		}

		expected := "?var=value&x=1024"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})

	t.Run("query continuation", func(t *testing.T) {
		template, err := NewUriTemplate("{?var}{&x}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"var": "value", "x": "1024"})
		if err != nil {
			t.Fatal(err)
		}

		expected := "?var=value&x=1024"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})

	t.Run("multiple query params with continuation", func(t *testing.T) {
		template, err := NewUriTemplate("/search{?q,limit}{&offset}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"q": "search", "limit": "10", "offset": "20"})
		if err != nil {
			t.Fatal(err)
		}

		expected := "/search?q=search&limit=10&offset=20"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})
}

func TestUriTemplate_ArrayValues(t *testing.T) {
	t.Run("array values simple", func(t *testing.T) {
		template, err := NewUriTemplate("{var}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"var": []string{"one", "two", "three"}})
		if err != nil {
			t.Fatal(err)
		}

		expected := "one,two,three"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})

	t.Run("array values in query", func(t *testing.T) {
		template, err := NewUriTemplate("{?var}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"var": []string{"one", "two", "three"}})
		if err != nil {
			t.Fatal(err)
		}

		expected := "?var=one,two,three"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})
}

func TestUriTemplate_EdgeCases(t *testing.T) {
	t.Run("empty template", func(t *testing.T) {
		template, err := NewUriTemplate("")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{})
		if err != nil {
			t.Fatal(err)
		}

		if result != "" {
			t.Errorf("Expand() = %q, want empty string", result)
		}
	})

	t.Run("no variables", func(t *testing.T) {
		template, err := NewUriTemplate("http://example.com/path")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{})
		if err != nil {
			t.Fatal(err)
		}

		expected := "http://example.com/path"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})

	t.Run("empty variable value", func(t *testing.T) {
		template, err := NewUriTemplate("/users/{id}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{"id": ""})
		if err != nil {
			t.Fatal(err)
		}

		expected := "/users/"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})

	t.Run("missing variable", func(t *testing.T) {
		template, err := NewUriTemplate("/users/{id}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Expand(Variables{})
		if err != nil {
			t.Fatal(err)
		}

		expected := "/users/"
		if result != expected {
			t.Errorf("Expand() = %q, want %q", result, expected)
		}
	})
}

func TestUriTemplate_Match(t *testing.T) {
	t.Run("simple variable match", func(t *testing.T) {
		template, err := NewUriTemplate("/users/{id}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Match("/users/123")
		if err != nil {
			t.Fatal(err)
		}

		if result == nil {
			t.Fatal("Match() returned nil")
		}

		if result["id"] != "123" {
			t.Errorf("Match() = %v, want id=123", result)
		}
	})

	t.Run("multiple variables match", func(t *testing.T) {
		template, err := NewUriTemplate("/users/{id}/posts/{postId}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Match("/users/123/posts/456")
		if err != nil {
			t.Fatal(err)
		}

		if result == nil {
			t.Fatal("Match() returned nil")
		}

		if result["id"] != "123" || result["postId"] != "456" {
			t.Errorf("Match() = %v, want id=123, postId=456", result)
		}
	})

	t.Run("no match", func(t *testing.T) {
		template, err := NewUriTemplate("/users/{id}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Match("/posts/123")
		if err != nil {
			t.Fatal(err)
		}

		if result != nil {
			t.Errorf("Match() = %v, want nil", result)
		}
	})

	t.Run("query parameter match", func(t *testing.T) {
		template, err := NewUriTemplate("/search{?q}")
		if err != nil {
			t.Fatal(err)
		}

		result, err := template.Match("/search?q=golang")
		if err != nil {
			t.Fatal(err)
		}

		if result == nil {
			t.Fatal("Match() returned nil")
		}

		if result["q"] != "golang" {
			t.Errorf("Match() = %v, want q=golang", result)
		}
	})
}

func TestUriTemplate_ValidationLimits(t *testing.T) {
	t.Run("template too long", func(t *testing.T) {
		longTemplate := strings.Repeat("a", MaxTemplateLength+1)
		_, err := NewUriTemplate(longTemplate)
		if err == nil {
			t.Error("Expected error for template too long")
		}
	})

	t.Run("too many expressions", func(t *testing.T) {
		var parts []string
		for i := 0; i < MaxTemplateExpressions+1; i++ {
			parts = append(parts, "{var"+fmt.Sprintf("%d", i)+"}")
		}
		template := strings.Join(parts, "/")
		
		_, err := NewUriTemplate(template)
		if err == nil {
			t.Error("Expected error for too many expressions")
		}
	})

	t.Run("unclosed expression", func(t *testing.T) {
		_, err := NewUriTemplate("/users/{id")
		if err == nil {
			t.Error("Expected error for unclosed expression")
		}
	})
}

func TestUriTemplate_String(t *testing.T) {
	template, err := NewUriTemplate("/users/{id}")
	if err != nil {
		t.Fatal(err)
	}

	if template.String() != "/users/{id}" {
		t.Errorf("String() = %q, want /users/{id}", template.String())
	}
}
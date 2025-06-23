package shared

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Variables represents a map of variable names to values for URI template expansion
type Variables map[string]interface{}

// Constants for validation limits
const (
	MaxTemplateLength   = 1000000 // 1MB
	MaxVariableLength   = 1000000 // 1MB
	MaxTemplateExpressions = 10000
	MaxRegexLength      = 1000000 // 1MB
)

// UriTemplate represents a URI template following RFC 6570
type UriTemplate struct {
	template string
	parts    []templatePart
}

// templatePart represents either a literal string or a variable expression
type templatePart interface {
	isTemplatePart()
}

// literalPart represents a literal string in the template
type literalPart string

func (l literalPart) isTemplatePart() {}

// expressionPart represents a variable expression in the template
type expressionPart struct {
	name     string
	operator string
	names    []string
	exploded bool
}

func (e expressionPart) isTemplatePart() {}

// IsTemplate returns true if the given string contains any URI template expressions
func (ut *UriTemplate) IsTemplate(str string) bool {
	// Look for any sequence of characters between curly braces that isn't just whitespace
	matched, _ := regexp.MatchString(`\{[^}\s]+\}`, str)
	return matched
}

// validateLength validates that a string doesn't exceed the maximum length
func validateLength(str string, max int, context string) error {
	if len(str) > max {
		return fmt.Errorf("%s exceeds maximum length of %d characters (got %d)", context, max, len(str))
	}
	return nil
}

// NewUriTemplate creates a new URI template from the given template string
func NewUriTemplate(template string) (*UriTemplate, error) {
	if err := validateLength(template, MaxTemplateLength, "Template"); err != nil {
		return nil, err
	}
	
	ut := &UriTemplate{
		template: template,
	}
	
	var err error
	ut.parts, err = ut.parse(template)
	if err != nil {
		return nil, err
	}
	
	return ut, nil
}

// String returns the original template string
func (ut *UriTemplate) String() string {
	return ut.template
}

// VariableNames returns all variable names found in the template
func (ut *UriTemplate) VariableNames() []string {
	var names []string
	for _, part := range ut.parts {
		if expr, ok := part.(expressionPart); ok {
			names = append(names, expr.names...)
		}
	}
	return names
}

// parse parses the template string into parts
func (ut *UriTemplate) parse(template string) ([]templatePart, error) {
	var parts []templatePart
	var currentText strings.Builder
	expressionCount := 0
	
	for i := 0; i < len(template); i++ {
		if template[i] == '{' {
			// Save any accumulated literal text
			if currentText.Len() > 0 {
				parts = append(parts, literalPart(currentText.String()))
				currentText.Reset()
			}
			
			// Find the closing brace
			end := strings.Index(template[i:], "}")
			if end == -1 {
				return nil, errors.New("unclosed template expression")
			}
			end += i
			
			expressionCount++
			if expressionCount > MaxTemplateExpressions {
				return nil, fmt.Errorf("template contains too many expressions (max %d)", MaxTemplateExpressions)
			}
			
			// Parse the expression
			expr := template[i+1 : end]
			operator := ut.getOperator(expr)
			exploded := strings.Contains(expr, "*")
			names := ut.getNames(expr)
			
			// Validate variable name lengths
			for _, name := range names {
				if err := validateLength(name, MaxVariableLength, "Variable name"); err != nil {
					return nil, err
				}
			}
			
			name := ""
			if len(names) > 0 {
				name = names[0]
			}
			
			parts = append(parts, expressionPart{
				name:     name,
				operator: operator,
				names:    names,
				exploded: exploded,
			})
			
			i = end
		} else {
			currentText.WriteByte(template[i])
		}
	}
	
	// Add any remaining literal text
	if currentText.Len() > 0 {
		parts = append(parts, literalPart(currentText.String()))
	}
	
	return parts, nil
}

// getOperator extracts the operator from an expression
func (ut *UriTemplate) getOperator(expr string) string {
	operators := []string{"+", "#", ".", "/", "?", "&"}
	for _, op := range operators {
		if strings.HasPrefix(expr, op) {
			return op
		}
	}
	return ""
}

// getNames extracts variable names from an expression
func (ut *UriTemplate) getNames(expr string) []string {
	operator := ut.getOperator(expr)
	namesPart := strings.TrimPrefix(expr, operator)
	
	var names []string
	for _, name := range strings.Split(namesPart, ",") {
		name = strings.TrimSpace(strings.Replace(name, "*", "", -1))
		if name != "" {
			names = append(names, name)
		}
	}
	
	return names
}

// encodeValue encodes a value according to the operator
func (ut *UriTemplate) encodeValue(value string, operator string) (string, error) {
	if err := validateLength(value, MaxVariableLength, "Variable value"); err != nil {
		return "", err
	}
	
	switch operator {
	case "+", "#":
		// Reserved character encoding
		return url.QueryEscape(value), nil
	default:
		// Percent encoding
		return url.QueryEscape(value), nil
	}
}

// expandPart expands a single expression part
func (ut *UriTemplate) expandPart(part expressionPart, variables Variables) (string, error) {
	switch part.operator {
	case "?", "&":
		var pairs []string
		for _, name := range part.names {
			value, exists := variables[name]
			if !exists {
				continue
			}
			
			var encoded string
			if values, ok := value.([]string); ok {
				var encodedValues []string
				for _, v := range values {
					enc, err := ut.encodeValue(v, part.operator)
					if err != nil {
						return "", err
					}
					encodedValues = append(encodedValues, enc)
				}
				encoded = strings.Join(encodedValues, ",")
			} else {
				var err error
				encoded, err = ut.encodeValue(fmt.Sprintf("%v", value), part.operator)
				if err != nil {
					return "", err
				}
			}
			
			pairs = append(pairs, fmt.Sprintf("%s=%s", name, encoded))
		}
		
		if len(pairs) == 0 {
			return "", nil
		}
		
		separator := "?"
		if part.operator == "&" {
			separator = "&"
		}
		return separator + strings.Join(pairs, "&"), nil
		
	default:
		if len(part.names) > 1 {
			var values []string
			for _, name := range part.names {
				if value, exists := variables[name]; exists {
					if valueSlice, ok := value.([]string); ok && len(valueSlice) > 0 {
						values = append(values, valueSlice[0])
					} else {
						values = append(values, fmt.Sprintf("%v", value))
					}
				}
			}
			if len(values) == 0 {
				return "", nil
			}
			
			// Apply operator to multiple values
			switch part.operator {
			case "":
				return strings.Join(values, ","), nil
			case "+":
				return strings.Join(values, ","), nil
			case "#":
				return "#" + strings.Join(values, ","), nil
			case ".":
				return "." + strings.Join(values, "."), nil
			case "/":
				return "/" + strings.Join(values, "/"), nil
			default:
				return strings.Join(values, ","), nil
			}
		}
		
		value, exists := variables[part.name]
		if !exists {
			return "", nil
		}
		
		var values []string
		if valueSlice, ok := value.([]string); ok {
			values = valueSlice
		} else {
			values = []string{fmt.Sprintf("%v", value)}
		}
		
		var encoded []string
		for _, v := range values {
			enc, err := ut.encodeValue(v, part.operator)
			if err != nil {
				return "", err
			}
			encoded = append(encoded, enc)
		}
		
		switch part.operator {
		case "":
			return strings.Join(encoded, ","), nil
		case "+":
			return strings.Join(encoded, ","), nil
		case "#":
			return "#" + strings.Join(encoded, ","), nil
		case ".":
			return "." + strings.Join(encoded, "."), nil
		case "/":
			return "/" + strings.Join(encoded, "/"), nil
		default:
			return strings.Join(encoded, ","), nil
		}
	}
}

// Expand expands the template with the given variables
func (ut *UriTemplate) Expand(variables Variables) (string, error) {
	var result strings.Builder
	hasQueryParam := false
	
	for _, part := range ut.parts {
		switch p := part.(type) {
		case literalPart:
			result.WriteString(string(p))
		case expressionPart:
			expanded, err := ut.expandPart(p, variables)
			if err != nil {
				return "", err
			}
			if expanded == "" {
				continue
			}
			
			// Convert ? to & if we already have a query parameter
			if (p.operator == "?" || p.operator == "&") && hasQueryParam {
				expanded = strings.Replace(expanded, "?", "&", 1)
			}
			
			result.WriteString(expanded)
			
			if p.operator == "?" || p.operator == "&" {
				hasQueryParam = true
			}
		}
	}
	
	return result.String(), nil
}

// escapeRegExp escapes special regex characters
func (ut *UriTemplate) escapeRegExp(str string) string {
	return regexp.QuoteMeta(str)
}

// Match matches a URI against the template and extracts variables
func (ut *UriTemplate) Match(uri string) (Variables, error) {
	if err := validateLength(uri, MaxTemplateLength, "URI"); err != nil {
		return nil, err
	}
	
	var pattern strings.Builder
	pattern.WriteString("^")
	
	var names []struct {
		name     string
		exploded bool
	}
	
	for _, part := range ut.parts {
		switch p := part.(type) {
		case literalPart:
			pattern.WriteString(ut.escapeRegExp(string(p)))
		case expressionPart:
			patterns := ut.partToRegExp(p)
			for _, patternInfo := range patterns {
				pattern.WriteString(patternInfo.pattern)
				names = append(names, struct {
					name     string
					exploded bool
				}{
					name:     patternInfo.name,
					exploded: p.exploded,
				})
			}
		}
	}
	
	pattern.WriteString("$")
	
	if err := validateLength(pattern.String(), MaxRegexLength, "Generated regex pattern"); err != nil {
		return nil, err
	}
	
	regex, err := regexp.Compile(pattern.String())
	if err != nil {
		return nil, fmt.Errorf("failed to compile regex: %w", err)
	}
	
	match := regex.FindStringSubmatch(uri)
	if match == nil {
		return nil, nil
	}
	
	result := make(Variables)
	for i, nameInfo := range names {
		if i+1 >= len(match) {
			continue
		}
		
		value := match[i+1]
		cleanName := strings.Replace(nameInfo.name, "*", "", -1)
		
		if nameInfo.exploded && strings.Contains(value, ",") {
			result[cleanName] = strings.Split(value, ",")
		} else {
			result[cleanName] = value
		}
	}
	
	return result, nil
}

// partToRegExp converts an expression part to regex patterns
func (ut *UriTemplate) partToRegExp(part expressionPart) []struct {
	pattern string
	name    string
} {
	var patterns []struct {
		pattern string
		name    string
	}
	
	// Validate variable name length for matching
	for _, name := range part.names {
		if err := validateLength(name, MaxVariableLength, "Variable name"); err != nil {
			// In a real implementation, we might want to handle this error differently
			continue
		}
	}
	
	if part.operator == "?" || part.operator == "&" {
		for i, name := range part.names {
			prefix := "\\" + part.operator
			if i > 0 {
				prefix = "&"
			}
			patterns = append(patterns, struct {
				pattern string
				name    string
			}{
				pattern: prefix + ut.escapeRegExp(name) + "=([^&]+)",
				name:    name,
			})
		}
		return patterns
	}
	
	var pattern string
	name := part.name
	
	switch part.operator {
	case "":
		if part.exploded {
			pattern = "([^/]+(?:,[^/]+)*)"
		} else {
			pattern = "([^/,]+)"
		}
	case "+", "#":
		pattern = "(.+)"
	case ".":
		pattern = "\\.([^/,]+)"
	case "/":
		if part.exploded {
			pattern = "/([^/]+(?:,[^/]+)*)"
		} else {
			pattern = "/([^/,]+)"
		}
	default:
		pattern = "([^/]+)"
	}
	
	patterns = append(patterns, struct {
		pattern string
		name    string
	}{
		pattern: pattern,
		name:    name,
	})
	
	return patterns
}
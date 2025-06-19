package shared

import (
	"fmt"
	"reflect"
	"strings"
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
	Value   interface{}
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error for field '%s': %s", e.Field, e.Message)
}

// ValidationErrors represents multiple validation errors
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "no validation errors"
	}

	var messages []string
	for _, err := range e {
		messages = append(messages, err.Error())
	}
	return strings.Join(messages, "; ")
}

// Validator interface for custom validation
type Validator interface {
	Validate() error
}

// ValidateStruct validates a struct using tags and interface methods
func ValidateStruct(v interface{}) error {
	if v == nil {
		return ValidationError{Field: "root", Message: "value cannot be nil"}
	}

	// Check if the struct implements Validator interface
	if validator, ok := v.(Validator); ok {
		if err := validator.Validate(); err != nil {
			return err
		}
	}

	value := reflect.ValueOf(v)
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return ValidationError{Field: "root", Message: "pointer cannot be nil"}
		}
		value = value.Elem()
	}

	if value.Kind() != reflect.Struct {
		return ValidationError{Field: "root", Message: "value must be a struct"}
	}

	return validateStructFields(value, "")
}

// validateStructFields validates individual struct fields
func validateStructFields(value reflect.Value, prefix string) error {
	var errors ValidationErrors
	typ := value.Type()

	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		fieldType := typ.Field(i)
		fieldName := fieldType.Name

		if prefix != "" {
			fieldName = prefix + "." + fieldName
		}

		// Skip unexported fields
		if !fieldType.IsExported() {
			continue
		}

		// Get validation tags
		tag := fieldType.Tag.Get("validate")
		if tag == "" {
			continue
		}

		// Parse and apply validation rules
		if err := validateField(field, fieldName, tag); err != nil {
			if validationErrs, ok := err.(ValidationErrors); ok {
				errors = append(errors, validationErrs...)
			} else {
				errors = append(errors, ValidationError{
					Field:   fieldName,
					Message: err.Error(),
					Value:   field.Interface(),
				})
			}
		}

		// Recursively validate nested structs
		if field.Kind() == reflect.Struct {
			if err := validateStructFields(field, fieldName); err != nil {
				if validationErrs, ok := err.(ValidationErrors); ok {
					errors = append(errors, validationErrs...)
				}
			}
		} else if field.Kind() == reflect.Ptr && !field.IsNil() && field.Elem().Kind() == reflect.Struct {
			if err := validateStructFields(field.Elem(), fieldName); err != nil {
				if validationErrs, ok := err.(ValidationErrors); ok {
					errors = append(errors, validationErrs...)
				}
			}
		}
	}

	if len(errors) > 0 {
		return errors
	}
	return nil
}

// validateField validates a single field based on tag rules
func validateField(field reflect.Value, fieldName, tag string) error {
	rules := strings.Split(tag, ",")
	var errors ValidationErrors

	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}

		switch {
		case rule == "required":
			if err := validateRequired(field, fieldName); err != nil {
				errors = append(errors, err.(ValidationError))
			}
		case strings.HasPrefix(rule, "min="):
			if err := validateMin(field, fieldName, rule[4:]); err != nil {
				errors = append(errors, err.(ValidationError))
			}
		case strings.HasPrefix(rule, "max="):
			if err := validateMax(field, fieldName, rule[4:]); err != nil {
				errors = append(errors, err.(ValidationError))
			}
		case rule == "email":
			if err := validateEmail(field, fieldName); err != nil {
				errors = append(errors, err.(ValidationError))
			}
		case rule == "url":
			if err := validateURL(field, fieldName); err != nil {
				errors = append(errors, err.(ValidationError))
			}
		}
	}

	if len(errors) > 0 {
		return errors
	}
	return nil
}

// validateRequired checks if a field has a non-zero value
func validateRequired(field reflect.Value, fieldName string) error {
	if field.Kind() == reflect.Ptr && field.IsNil() {
		return ValidationError{
			Field:   fieldName,
			Message: "field is required but is nil",
			Value:   nil,
		}
	}

	if field.Kind() == reflect.String && field.String() == "" {
		return ValidationError{
			Field:   fieldName,
			Message: "field is required but is empty",
			Value:   "",
		}
	}

	if field.Kind() == reflect.Slice && field.Len() == 0 {
		return ValidationError{
			Field:   fieldName,
			Message: "field is required but is empty slice",
			Value:   field.Interface(),
		}
	}

	return nil
}

// validateMin validates minimum length/value constraints
func validateMin(field reflect.Value, fieldName, minStr string) error {
	// This is a simplified implementation
	// In a full implementation, you'd parse minStr and apply appropriate validation
	return nil
}

// validateMax validates maximum length/value constraints
func validateMax(field reflect.Value, fieldName, maxStr string) error {
	// This is a simplified implementation
	// In a full implementation, you'd parse maxStr and apply appropriate validation
	return nil
}

// validateEmail validates email format
func validateEmail(field reflect.Value, fieldName string) error {
	if field.Kind() != reflect.String {
		return ValidationError{
			Field:   fieldName,
			Message: "email validation can only be applied to strings",
			Value:   field.Interface(),
		}
	}

	email := field.String()
	if email == "" {
		return nil // Allow empty strings unless required
	}

	// Simple email validation (in production, use a proper regex or library)
	if !strings.Contains(email, "@") {
		return ValidationError{
			Field:   fieldName,
			Message: "invalid email format",
			Value:   email,
		}
	}

	return nil
}

// validateURL validates URL format
func validateURL(field reflect.Value, fieldName string) error {
	if field.Kind() != reflect.String {
		return ValidationError{
			Field:   fieldName,
			Message: "url validation can only be applied to strings",
			Value:   field.Interface(),
		}
	}

	url := field.String()
	if url == "" {
		return nil // Allow empty strings unless required
	}

	// Simple URL validation (in production, use net/url package)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return ValidationError{
			Field:   fieldName,
			Message: "invalid URL format",
			Value:   url,
		}
	}

	return nil
}

// Validation helpers for common MCP types
func (r *Request) Validate() error {
	var errors ValidationErrors

	if r.JSONRPC != "2.0" {
		errors = append(errors, ValidationError{
			Field:   "jsonrpc",
			Message: "must be '2.0'",
			Value:   r.JSONRPC,
		})
	}

	if r.Method == "" {
		errors = append(errors, ValidationError{
			Field:   "method",
			Message: "method is required",
			Value:   r.Method,
		})
	}

	if r.ID == nil {
		errors = append(errors, ValidationError{
			Field:   "id",
			Message: "id is required for requests",
			Value:   r.ID,
		})
	}

	if len(errors) > 0 {
		return errors
	}
	return nil
}

func (n *Notification) Validate() error {
	var errors ValidationErrors

	if n.JSONRPC != "2.0" {
		errors = append(errors, ValidationError{
			Field:   "jsonrpc",
			Message: "must be '2.0'",
			Value:   n.JSONRPC,
		})
	}

	if n.Method == "" {
		errors = append(errors, ValidationError{
			Field:   "method",
			Message: "method is required",
			Value:   n.Method,
		})
	}

	if len(errors) > 0 {
		return errors
	}
	return nil
}

func (r *Response) Validate() error {
	var errors ValidationErrors

	if r.JSONRPC != "2.0" {
		errors = append(errors, ValidationError{
			Field:   "jsonrpc",
			Message: "must be '2.0'",
			Value:   r.JSONRPC,
		})
	}

	if r.ID == nil {
		errors = append(errors, ValidationError{
			Field:   "id",
			Message: "id is required for responses",
			Value:   r.ID,
		})
	}

	// Either result or error must be present, but not both
	if r.Result != nil && r.Error != nil {
		errors = append(errors, ValidationError{
			Field:   "result/error",
			Message: "response cannot have both result and error",
		})
	}

	if len(errors) > 0 {
		return errors
	}
	return nil
}

package server

import (
	"context"
	"fmt"
	"reflect"
)

// CompletionCallback provides completions for a given value and context
type CompletionCallback[T any] func(value T, context *CompletableContext) ([]T, error)

// CompletableContext provides additional context for completion
type CompletableContext struct {
	// Arguments from the request context (e.g. tool arguments, prompt arguments)
	Arguments map[string]interface{} `json:"arguments,omitempty"`
	
	// Additional context that might be useful for completion
	Context context.Context `json:"-"`
}

// Validator interface for value validation (similar to Zod's validation)
type Validator[T any] interface {
	// Validate checks if the value is valid and returns the validated value
	Validate(value interface{}) (T, error)
	
	// Description returns a description of what this validator accepts
	Description() string
}

// Completable wraps a validator with completion capabilities
type Completable[T any] struct {
	validator Validator[T]
	complete  CompletionCallback[T]
}

// NewCompletable creates a new completable validator
func NewCompletable[T any](validator Validator[T], complete CompletionCallback[T]) *Completable[T] {
	return &Completable[T]{
		validator: validator,
		complete:  complete,
	}
}

// Validate validates the value using the underlying validator
func (c *Completable[T]) Validate(value interface{}) (T, error) {
	return c.validator.Validate(value)
}

// Description returns the description from the underlying validator
func (c *Completable[T]) Description() string {
	return c.validator.Description()
}

// Complete provides completion suggestions for the given value
func (c *Completable[T]) Complete(value T, context *CompletableContext) ([]T, error) {
	return c.complete(value, context)
}

// Built-in validators

// StringValidator validates string values
type StringValidator struct {
	description string
	minLength   *int
	maxLength   *int
	pattern     *string // regex pattern
}

// NewStringValidator creates a new string validator
func NewStringValidator() *StringValidator {
	return &StringValidator{
		description: "string",
	}
}

// WithDescription sets the description
func (v *StringValidator) WithDescription(desc string) *StringValidator {
	v.description = desc
	return v
}

// WithMinLength sets minimum length requirement
func (v *StringValidator) WithMinLength(min int) *StringValidator {
	v.minLength = &min
	return v
}

// WithMaxLength sets maximum length requirement
func (v *StringValidator) WithMaxLength(max int) *StringValidator {
	v.maxLength = &max
	return v
}

// WithPattern sets regex pattern requirement
func (v *StringValidator) WithPattern(pattern string) *StringValidator {
	v.pattern = &pattern
	return v
}

// Validate validates a string value
func (v *StringValidator) Validate(value interface{}) (string, error) {
	str, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("expected string, got %T", value)
	}
	
	if v.minLength != nil && len(str) < *v.minLength {
		return "", fmt.Errorf("string too short, minimum length is %d", *v.minLength)
	}
	
	if v.maxLength != nil && len(str) > *v.maxLength {
		return "", fmt.Errorf("string too long, maximum length is %d", *v.maxLength)
	}
	
	// Pattern validation would go here if needed
	
	return str, nil
}

// Description returns the validator description
func (v *StringValidator) Description() string {
	return v.description
}

// NumberValidator validates numeric values
type NumberValidator struct {
	description string
	minimum     *float64
	maximum     *float64
}

// NewNumberValidator creates a new number validator
func NewNumberValidator() *NumberValidator {
	return &NumberValidator{
		description: "number",
	}
}

// WithDescription sets the description
func (v *NumberValidator) WithDescription(desc string) *NumberValidator {
	v.description = desc
	return v
}

// WithMinimum sets minimum value requirement
func (v *NumberValidator) WithMinimum(min float64) *NumberValidator {
	v.minimum = &min
	return v
}

// WithMaximum sets maximum value requirement
func (v *NumberValidator) WithMaximum(max float64) *NumberValidator {
	v.maximum = &max
	return v
}

// Validate validates a numeric value
func (v *NumberValidator) Validate(value interface{}) (float64, error) {
	var num float64
	
	switch val := value.(type) {
	case int:
		num = float64(val)
	case int32:
		num = float64(val)
	case int64:
		num = float64(val)
	case float32:
		num = float64(val)
	case float64:
		num = val
	default:
		return 0, fmt.Errorf("expected number, got %T", value)
	}
	
	if v.minimum != nil && num < *v.minimum {
		return 0, fmt.Errorf("number too small, minimum is %f", *v.minimum)
	}
	
	if v.maximum != nil && num > *v.maximum {
		return 0, fmt.Errorf("number too large, maximum is %f", *v.maximum)
	}
	
	return num, nil
}

// Description returns the validator description
func (v *NumberValidator) Description() string {
	return v.description
}

// EnumValidator validates enum values
type EnumValidator[T comparable] struct {
	description string
	values      []T
}

// NewEnumValidator creates a new enum validator
func NewEnumValidator[T comparable](values []T) *EnumValidator[T] {
	return &EnumValidator[T]{
		description: "enum",
		values:      values,
	}
}

// WithDescription sets the description
func (v *EnumValidator[T]) WithDescription(desc string) *EnumValidator[T] {
	v.description = desc
	return v
}

// Validate validates an enum value
func (v *EnumValidator[T]) Validate(value interface{}) (T, error) {
	var zero T
	
	// Try to convert the value to type T
	val, ok := value.(T)
	if !ok {
		return zero, fmt.Errorf("expected %T, got %T", zero, value)
	}
	
	// Check if the value is in the allowed set
	for _, allowed := range v.values {
		if val == allowed {
			return val, nil
		}
	}
	
	return zero, fmt.Errorf("value %v not in allowed enum values", val)
}

// Description returns the validator description
func (v *EnumValidator[T]) Description() string {
	return v.description
}

// Values returns the allowed enum values
func (v *EnumValidator[T]) Values() []T {
	return v.values
}

// Convenience functions for creating completable validators

// CompletableString creates a completable string validator
func CompletableString(complete CompletionCallback[string]) *Completable[string] {
	return NewCompletable(NewStringValidator(), complete)
}

// CompletableNumber creates a completable number validator
func CompletableNumber(complete CompletionCallback[float64]) *Completable[float64] {
	return NewCompletable(NewNumberValidator(), complete)
}

// CompletableEnum creates a completable enum validator
func CompletableEnum[T comparable](values []T, complete CompletionCallback[T]) *Completable[T] {
	return NewCompletable(NewEnumValidator(values), complete)
}

// CompletableFromValidator creates a completable from any validator
func CompletableFromValidator[T any](validator Validator[T], complete CompletionCallback[T]) *Completable[T] {
	return NewCompletable(validator, complete)
}

// Helper function to get the zero value of a type
func getZeroValue[T any]() T {
	var zero T
	return zero
}

// ValidateType checks if a value matches the expected type T
func ValidateType[T any](value interface{}) (T, error) {
	var zero T
	
	if val, ok := value.(T); ok {
		return val, nil
	}
	
	// Try to convert using reflection if direct type assertion fails
	valueType := reflect.TypeOf(value)
	targetType := reflect.TypeOf(zero)
	
	if valueType.ConvertibleTo(targetType) {
		converted := reflect.ValueOf(value).Convert(targetType)
		return converted.Interface().(T), nil
	}
	
	return zero, fmt.Errorf("expected %T, got %T", zero, value)
}
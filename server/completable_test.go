package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompletable_PreservesTypesAndValues(t *testing.T) {
	baseValidator := NewStringValidator()
	completable := NewCompletable(baseValidator, func(value string, context *CompletableContext) ([]string, error) {
		return []string{}, nil
	})

	// Test valid string
	result, err := completable.Validate("test")
	require.NoError(t, err)
	assert.Equal(t, "test", result)

	// Test invalid type
	_, err = completable.Validate(123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected string")
}

func TestCompletable_ProvidesAccessToCompletionFunction(t *testing.T) {
	completions := []string{"foo", "bar", "baz"}
	completable := CompletableString(func(value string, context *CompletableContext) ([]string, error) {
		return completions, nil
	})

	result, err := completable.Complete("", &CompletableContext{})
	require.NoError(t, err)
	assert.Equal(t, completions, result)
}

func TestCompletable_AllowsAsyncCompletionFunctions(t *testing.T) {
	completions := []string{"foo", "bar", "baz"}
	completable := CompletableString(func(value string, context *CompletableContext) ([]string, error) {
		// Simulate async operation by checking context
		if context != nil && context.Context != nil {
			select {
			case <-context.Context.Done():
				return nil, context.Context.Err()
			default:
				// Continue with completion
			}
		}
		return completions, nil
	})

	ctx := context.Background()
	result, err := completable.Complete("", &CompletableContext{
		Context: ctx,
	})
	require.NoError(t, err)
	assert.Equal(t, completions, result)
}

func TestCompletable_PassesCurrentValueToCompletionFunction(t *testing.T) {
	completable := CompletableString(func(value string, context *CompletableContext) ([]string, error) {
		return []string{value + "!"}, nil
	})

	result, err := completable.Complete("test", &CompletableContext{})
	require.NoError(t, err)
	assert.Equal(t, []string{"test!"}, result)
}

func TestCompletable_WorksWithNumberValidator(t *testing.T) {
	completable := CompletableNumber(func(value float64, context *CompletableContext) ([]float64, error) {
		return []float64{1.0, 2.0, 3.0}, nil
	})

	// Test validation
	result, err := completable.Validate(1.0)
	require.NoError(t, err)
	assert.Equal(t, 1.0, result)

	// Test completion
	completions, err := completable.Complete(0.0, &CompletableContext{})
	require.NoError(t, err)
	assert.Equal(t, []float64{1.0, 2.0, 3.0}, completions)
}

func TestCompletable_PreservesValidatorDescription(t *testing.T) {
	desc := "test description"
	validator := NewStringValidator().WithDescription(desc)
	completable := NewCompletable(validator, func(value string, context *CompletableContext) ([]string, error) {
		return []string{}, nil
	})

	assert.Equal(t, desc, completable.Description())
}

func TestStringValidator_BasicValidation(t *testing.T) {
	validator := NewStringValidator()

	// Valid string
	result, err := validator.Validate("hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", result)

	// Invalid type
	_, err = validator.Validate(123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected string")
}

func TestStringValidator_LengthConstraints(t *testing.T) {
	validator := NewStringValidator().
		WithMinLength(2).
		WithMaxLength(10)

	// Valid length
	result, err := validator.Validate("hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", result)

	// Too short
	_, err = validator.Validate("a")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too short")

	// Too long
	_, err = validator.Validate("this is way too long")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too long")
}

func TestStringValidator_Description(t *testing.T) {
	validator := NewStringValidator().WithDescription("custom description")
	assert.Equal(t, "custom description", validator.Description())
}

func TestNumberValidator_BasicValidation(t *testing.T) {
	validator := NewNumberValidator()

	tests := []struct {
		name     string
		input    interface{}
		expected float64
		hasError bool
	}{
		{"int", 42, 42.0, false},
		{"int32", int32(42), 42.0, false},
		{"int64", int64(42), 42.0, false},
		{"float32", float32(42.5), 42.5, false},
		{"float64", 42.5, 42.5, false},
		{"string", "42", 0, true},
		{"bool", true, 0, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := validator.Validate(test.input)
			if test.hasError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "expected number")
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expected, result)
			}
		})
	}
}

func TestNumberValidator_RangeConstraints(t *testing.T) {
	validator := NewNumberValidator().
		WithMinimum(1.0).
		WithMaximum(10.0)

	// Valid range
	result, err := validator.Validate(5.0)
	require.NoError(t, err)
	assert.Equal(t, 5.0, result)

	// Too small
	_, err = validator.Validate(0.5)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too small")

	// Too large
	_, err = validator.Validate(15.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

func TestEnumValidator_BasicValidation(t *testing.T) {
	values := []string{"red", "green", "blue"}
	validator := NewEnumValidator(values)

	// Valid value
	result, err := validator.Validate("red")
	require.NoError(t, err)
	assert.Equal(t, "red", result)

	// Invalid value
	_, err = validator.Validate("yellow")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not in allowed enum values")

	// Wrong type
	_, err = validator.Validate(123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected string")
}

func TestEnumValidator_IntEnum(t *testing.T) {
	values := []int{1, 2, 3}
	validator := NewEnumValidator(values)

	// Valid value
	result, err := validator.Validate(2)
	require.NoError(t, err)
	assert.Equal(t, 2, result)

	// Invalid value
	_, err = validator.Validate(5)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not in allowed enum values")
}

func TestEnumValidator_Values(t *testing.T) {
	values := []string{"a", "b", "c"}
	validator := NewEnumValidator(values)
	assert.Equal(t, values, validator.Values())
}

func TestCompletableEnum_Integration(t *testing.T) {
	colors := []string{"red", "green", "blue"}
	completable := CompletableEnum(colors, func(value string, context *CompletableContext) ([]string, error) {
		// Return completions that start with the current value
		var completions []string
		for _, color := range colors {
			if len(value) == 0 || color[:1] == value[:1] {
				completions = append(completions, color)
			}
		}
		return completions, nil
	})

	// Test validation
	result, err := completable.Validate("red")
	require.NoError(t, err)
	assert.Equal(t, "red", result)

	// Test completion with empty value
	completions, err := completable.Complete("", &CompletableContext{})
	require.NoError(t, err)
	assert.ElementsMatch(t, colors, completions)

	// Test completion with partial value
	completions, err = completable.Complete("g", &CompletableContext{})
	require.NoError(t, err)
	assert.Equal(t, []string{"green"}, completions)
}

func TestCompletableContext_Arguments(t *testing.T) {
	completable := CompletableString(func(value string, context *CompletableContext) ([]string, error) {
		if context != nil && context.Arguments != nil {
			if prefix, ok := context.Arguments["prefix"].(string); ok {
				return []string{prefix + value}, nil
			}
		}
		return []string{value}, nil
	})

	ctx := &CompletableContext{
		Arguments: map[string]interface{}{
			"prefix": "hello_",
		},
	}

	result, err := completable.Complete("world", ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"hello_world"}, result)
}

func TestCompletableContext_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	completable := CompletableString(func(value string, context *CompletableContext) ([]string, error) {
		if context != nil && context.Context != nil {
			select {
			case <-context.Context.Done():
				return nil, context.Context.Err()
			default:
				return []string{"result"}, nil
			}
		}
		return []string{"result"}, nil
	})

	_, err := completable.Complete("test", &CompletableContext{
		Context: ctx,
	})
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestCompletionCallback_ErrorHandling(t *testing.T) {
	expectedErr := fmt.Errorf("completion error")
	completable := CompletableString(func(value string, context *CompletableContext) ([]string, error) {
		return nil, expectedErr
	})

	_, err := completable.Complete("test", &CompletableContext{})
	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestConvenienceFunctions(t *testing.T) {
	t.Run("CompletableString", func(t *testing.T) {
		completable := CompletableString(func(value string, context *CompletableContext) ([]string, error) {
			return []string{value + "_completed"}, nil
		})

		result, err := completable.Validate("test")
		require.NoError(t, err)
		assert.Equal(t, "test", result)

		completions, err := completable.Complete("test", &CompletableContext{})
		require.NoError(t, err)
		assert.Equal(t, []string{"test_completed"}, completions)
	})

	t.Run("CompletableNumber", func(t *testing.T) {
		completable := CompletableNumber(func(value float64, context *CompletableContext) ([]float64, error) {
			return []float64{value + 1}, nil
		})

		result, err := completable.Validate(5.0)
		require.NoError(t, err)
		assert.Equal(t, 5.0, result)

		completions, err := completable.Complete(5.0, &CompletableContext{})
		require.NoError(t, err)
		assert.Equal(t, []float64{6.0}, completions)
	})

	t.Run("CompletableEnum", func(t *testing.T) {
		values := []int{1, 2, 3}
		completable := CompletableEnum(values, func(value int, context *CompletableContext) ([]int, error) {
			return values, nil
		})

		result, err := completable.Validate(2)
		require.NoError(t, err)
		assert.Equal(t, 2, result)

		completions, err := completable.Complete(1, &CompletableContext{})
		require.NoError(t, err)
		assert.Equal(t, values, completions)
	})
}

func TestValidateType_TypeConversion(t *testing.T) {
	tests := []struct {
		name      string
		input     interface{}
		hasError  bool
		expected  interface{}
	}{
		{"string to string", "hello", false, "hello"},
		{"int to float64", 42, false, 42.0}, // Reflection can convert int to float64
		{"int32 to int", int32(42), false, 42}, // Reflection can convert int32 to int
		{"wrong type", "hello", true, 0}, // string to int should fail
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s_%T", test.name, test.expected), func(t *testing.T) {
			switch test.expected.(type) {
			case string:
				result, err := ValidateType[string](test.input)
				if test.hasError {
					assert.Error(t, err)
				} else {
					require.NoError(t, err)
					assert.Equal(t, test.expected, result)
				}
			case int:
				result, err := ValidateType[int](test.input)
				if test.hasError {
					assert.Error(t, err)
				} else {
					require.NoError(t, err)
					assert.Equal(t, test.expected, result)
				}
			case float64:
				result, err := ValidateType[float64](test.input)
				if test.hasError {
					assert.Error(t, err)
				} else {
					require.NoError(t, err)
					assert.Equal(t, test.expected, result)
				}
			}
		})
	}
}

// Benchmark tests for performance validation
func BenchmarkStringValidator_Validate(b *testing.B) {
	validator := NewStringValidator()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = validator.Validate("test string")
	}
}

func BenchmarkCompletable_Complete(b *testing.B) {
	completions := []string{"option1", "option2", "option3"}
	completable := CompletableString(func(value string, context *CompletableContext) ([]string, error) {
		return completions, nil
	})
	
	ctx := &CompletableContext{}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = completable.Complete("test", ctx)
	}
}

// Race condition tests
func TestCompletable_ConcurrentAccess(t *testing.T) {
	completable := CompletableString(func(value string, context *CompletableContext) ([]string, error) {
		return []string{value + "_completed"}, nil
	})

	const numRoutines = 100
	results := make(chan bool, numRoutines)

	for i := 0; i < numRoutines; i++ {
		go func(id int) {
			// Test validation
			_, err1 := completable.Validate(fmt.Sprintf("test_%d", id))
			
			// Test completion
			_, err2 := completable.Complete(fmt.Sprintf("test_%d", id), &CompletableContext{})
			
			results <- (err1 == nil && err2 == nil)
		}(i)
	}

	// Collect all results
	for i := 0; i < numRoutines; i++ {
		result := <-results
		assert.True(t, result)
	}
}

func TestCompletableFromValidator_CustomValidator(t *testing.T) {
	// Create a custom validator
	customValidator := &customTestValidator{}
	
	completable := CompletableFromValidator(customValidator, func(value string, context *CompletableContext) ([]string, error) {
		return []string{"custom_" + value}, nil
	})

	// Test validation
	result, err := completable.Validate("test")
	require.NoError(t, err)
	assert.Equal(t, "custom_test", result)

	// Test completion
	completions, err := completable.Complete("test", &CompletableContext{})
	require.NoError(t, err)
	assert.Equal(t, []string{"custom_test"}, completions)

	// Test description
	assert.Equal(t, "custom validator", completable.Description())
}

// Custom validator for testing CompletableFromValidator
type customTestValidator struct{}

func (v *customTestValidator) Validate(value interface{}) (string, error) {
	str, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("expected string")
	}
	return "custom_" + str, nil
}

func (v *customTestValidator) Description() string {
	return "custom validator"
}
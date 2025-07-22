package cargo

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// Validator provides validation functionality for protobuf messages
type Validator struct {
	rules map[string]ValidationRule
}

// ValidationRule defines a validation constraint
type ValidationRule interface {
	Validate(value interface{}, fieldName string) error
}

// Required validation rule
type RequiredRule struct{}

func (r RequiredRule) Validate(value interface{}, fieldName string) error {
	if value == nil {
		return NewValidationError(fmt.Sprintf("%s is required", fieldName))
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.String:
		if v.String() == "" {
			return NewValidationError(fmt.Sprintf("%s cannot be empty", fieldName))
		}
	case reflect.Slice, reflect.Array, reflect.Map:
		if v.Len() == 0 {
			return NewValidationError(fmt.Sprintf("%s cannot be empty", fieldName))
		}
	case reflect.Ptr:
		if v.IsNil() {
			return NewValidationError(fmt.Sprintf("%s is required", fieldName))
		}
	}

	return nil
}

// Email validation rule
type EmailRule struct{}

func (r EmailRule) Validate(value interface{}, fieldName string) error {
	str, ok := value.(string)
	if !ok {
		return NewValidationError(fmt.Sprintf("%s must be a string", fieldName))
	}

	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(str) {
		return NewValidationError(fmt.Sprintf("%s must be a valid email address", fieldName))
	}

	return nil
}

// MinLength validation rule
type MinLengthRule struct {
	MinLength int
}

func (r MinLengthRule) Validate(value interface{}, fieldName string) error {
	str, ok := value.(string)
	if !ok {
		return NewValidationError(fmt.Sprintf("%s must be a string", fieldName))
	}

	if len(str) < r.MinLength {
		return NewValidationError(fmt.Sprintf("%s must be at least %d characters long", fieldName, r.MinLength))
	}

	return nil
}

// MaxLength validation rule
type MaxLengthRule struct {
	MaxLength int
}

func (r MaxLengthRule) Validate(value interface{}, fieldName string) error {
	str, ok := value.(string)
	if !ok {
		return NewValidationError(fmt.Sprintf("%s must be a string", fieldName))
	}

	if len(str) > r.MaxLength {
		return NewValidationError(fmt.Sprintf("%s must be at most %d characters long", fieldName, r.MaxLength))
	}

	return nil
}

// NewValidator creates a new validator instance
func NewValidator() *Validator {
	return &Validator{
		rules: make(map[string]ValidationRule),
	}
}

// AddRule adds a validation rule for a specific field
func (v *Validator) AddRule(fieldName string, rule ValidationRule) {
	v.rules[fieldName] = rule
}

// Validate validates a struct using reflection and validation tags
func (v *Validator) Validate(obj interface{}) error {
	if obj == nil {
		return NewValidationError("object cannot be nil")
	}

	value := reflect.ValueOf(obj)
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return NewValidationError("object cannot be nil")
		}
		value = value.Elem()
	}

	if value.Kind() != reflect.Struct {
		return NewValidationError("validation target must be a struct")
	}

	structType := value.Type()

	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		fieldType := structType.Field(i)
		fieldName := fieldType.Name

		// Check if field has validation rules defined
		if rule, exists := v.rules[fieldName]; exists {
			if err := rule.Validate(field.Interface(), fieldName); err != nil {
				return err
			}
		}

		// Parse validation tags
		if err := v.validateFieldByTags(field, fieldType); err != nil {
			return err
		}
	}

	return nil
}

// validateFieldByTags validates a field based on its struct tags
func (v *Validator) validateFieldByTags(field reflect.Value, fieldType reflect.StructField) error {
	fieldName := fieldType.Name

	// Get validation tag
	validationTag := fieldType.Tag.Get("validate")
	if validationTag == "" {
		return nil
	}

	// Parse validation rules from tag
	rules := strings.Split(validationTag, ",")

	for _, rule := range rules {
		rule = strings.TrimSpace(rule)

		if err := v.applyTagRule(field.Interface(), fieldName, rule); err != nil {
			return err
		}
	}

	return nil
}

// applyTagRule applies a specific validation rule from a tag
func (v *Validator) applyTagRule(value interface{}, fieldName, rule string) error {
	parts := strings.Split(rule, "=")
	ruleName := parts[0]

	switch ruleName {
	case "required":
		return RequiredRule{}.Validate(value, fieldName)

	case "email":
		return EmailRule{}.Validate(value, fieldName)

	case "min":
		if len(parts) != 2 {
			return NewValidationError("min rule requires a value")
		}
		minVal, err := strconv.Atoi(parts[1])
		if err != nil {
			return NewValidationError("min rule value must be a number")
		}
		return MinLengthRule{MinLength: minVal}.Validate(value, fieldName)

	case "max":
		if len(parts) != 2 {
			return NewValidationError("max rule requires a value")
		}
		maxVal, err := strconv.Atoi(parts[1])
		if err != nil {
			return NewValidationError("max rule value must be a number")
		}
		return MaxLengthRule{MaxLength: maxVal}.Validate(value, fieldName)

	default:
		return NewValidationError(fmt.Sprintf("unknown validation rule: %s", ruleName))
	}
}

// Built-in validation functions for common use cases

// ValidateEmail validates an email string
func ValidateEmail(email string) error {
	return EmailRule{}.Validate(email, "email")
}

// ValidateRequired validates that a value is not empty
func ValidateRequired(value interface{}, fieldName string) error {
	return RequiredRule{}.Validate(value, fieldName)
}

// ValidateStringLength validates string length constraints
func ValidateStringLength(value string, fieldName string, min, max int) error {
	minRule := MinLengthRule{MinLength: min}
	if err := minRule.Validate(value, fieldName); err != nil {
		return err
	}
	maxRule := MaxLengthRule{MaxLength: max}
	return maxRule.Validate(value, fieldName)
}

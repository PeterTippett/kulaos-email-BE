package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-playground/validator/v10"
)

// Validator wraps the validator library for request validation
type Validator struct {
	validate *validator.Validate
}

// NewValidator creates a new validator instance
func NewValidator() *Validator {
	return &Validator{
		validate: validator.New(),
	}
}

// ValidateStruct validates a struct and returns validation errors
func (v *Validator) ValidateStruct(s interface{}) error {
	return v.validate.Struct(s)
}

// ParseAndValidate parses JSON request body and validates it
func (v *Validator) ParseAndValidate(r *http.Request, dest interface{}) error {
	if err := json.NewDecoder(r.Body).Decode(dest); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if err := v.ValidateStruct(dest); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	return nil
}

// ValidationErrorResponse formats validation errors for HTTP response
func ValidationErrorResponse(err error) map[string]interface{} {
	validationErrors := make([]string, 0)

	if ve, ok := err.(validator.ValidationErrors); ok {
		for _, fe := range ve {
			validationErrors = append(validationErrors, fmt.Sprintf("field '%s' failed validation '%s'", fe.Field(), fe.Tag()))
		}
	} else {
		validationErrors = append(validationErrors, err.Error())
	}

	return map[string]interface{}{
		"error":  "validation failed",
		"fields": validationErrors,
	}
}

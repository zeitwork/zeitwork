package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ErrorType represents the type of error
type ErrorType string

const (
	ErrorTypeValidation   ErrorType = "validation"
	ErrorTypeNotFound     ErrorType = "not_found"
	ErrorTypeUnauthorized ErrorType = "unauthorized"
	ErrorTypeForbidden    ErrorType = "forbidden"
	ErrorTypeConflict     ErrorType = "conflict"
	ErrorTypeInternal     ErrorType = "internal"
	ErrorTypeUnavailable  ErrorType = "unavailable"
	ErrorTypeRateLimit    ErrorType = "rate_limit"
)

// Error represents a structured API error
type Error struct {
	Type    ErrorType              `json:"type"`
	Message string                 `json:"message"`
	Code    string                 `json:"code,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// Error implements the error interface
func (e *Error) Error() string {
	return e.Message
}

// StatusCode returns the HTTP status code for the error type
func (e *Error) StatusCode() int {
	switch e.Type {
	case ErrorTypeValidation:
		return http.StatusBadRequest
	case ErrorTypeNotFound:
		return http.StatusNotFound
	case ErrorTypeUnauthorized:
		return http.StatusUnauthorized
	case ErrorTypeForbidden:
		return http.StatusForbidden
	case ErrorTypeConflict:
		return http.StatusConflict
	case ErrorTypeUnavailable:
		return http.StatusServiceUnavailable
	case ErrorTypeRateLimit:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

// WriteJSON writes the error as JSON to the response writer
func (e *Error) WriteJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.StatusCode())
	json.NewEncoder(w).Encode(e)
}

// Common error constructors

// NewValidationError creates a validation error
func NewValidationError(message string, details map[string]interface{}) *Error {
	return &Error{
		Type:    ErrorTypeValidation,
		Message: message,
		Code:    "VALIDATION_ERROR",
		Details: details,
	}
}

// NewNotFoundError creates a not found error
func NewNotFoundError(resource string) *Error {
	return &Error{
		Type:    ErrorTypeNotFound,
		Message: fmt.Sprintf("%s not found", resource),
		Code:    "RESOURCE_NOT_FOUND",
		Details: map[string]interface{}{"resource": resource},
	}
}

// NewUnauthorizedError creates an unauthorized error
func NewUnauthorizedError(message string) *Error {
	if message == "" {
		message = "Authentication required"
	}
	return &Error{
		Type:    ErrorTypeUnauthorized,
		Message: message,
		Code:    "UNAUTHORIZED",
	}
}

// NewForbiddenError creates a forbidden error
func NewForbiddenError(message string) *Error {
	if message == "" {
		message = "Access denied"
	}
	return &Error{
		Type:    ErrorTypeForbidden,
		Message: message,
		Code:    "FORBIDDEN",
	}
}

// NewConflictError creates a conflict error
func NewConflictError(message string, details map[string]interface{}) *Error {
	return &Error{
		Type:    ErrorTypeConflict,
		Message: message,
		Code:    "CONFLICT",
		Details: details,
	}
}

// NewInternalError creates an internal server error
func NewInternalError(message string) *Error {
	if message == "" {
		message = "An internal error occurred"
	}
	return &Error{
		Type:    ErrorTypeInternal,
		Message: message,
		Code:    "INTERNAL_ERROR",
	}
}

// NewUnavailableError creates a service unavailable error
func NewUnavailableError(message string) *Error {
	if message == "" {
		message = "Service temporarily unavailable"
	}
	return &Error{
		Type:    ErrorTypeUnavailable,
		Message: message,
		Code:    "SERVICE_UNAVAILABLE",
	}
}

// NewRateLimitError creates a rate limit error
func NewRateLimitError(limit int) *Error {
	return &Error{
		Type:    ErrorTypeRateLimit,
		Message: "Rate limit exceeded",
		Code:    "RATE_LIMIT_EXCEEDED",
		Details: map[string]interface{}{"limit": limit},
	}
}

// HandleError is a helper function to handle errors in HTTP handlers
func HandleError(w http.ResponseWriter, err error) {
	if apiErr, ok := err.(*Error); ok {
		apiErr.WriteJSON(w)
	} else {
		// For non-API errors, return a generic internal error
		internalErr := NewInternalError("")
		internalErr.WriteJSON(w)
	}
}

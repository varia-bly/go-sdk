package variably

import (
	"fmt"
	"net/http"
)

// VariablyError is the base error type for the SDK
type VariablyError struct {
	Message string
	Cause   error
}

func (e *VariablyError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *VariablyError) Unwrap() error {
	return e.Cause
}

// NewVariablyError creates a new VariablyError
func NewVariablyError(message string, cause error) *VariablyError {
	return &VariablyError{
		Message: message,
		Cause:   cause,
	}
}

// NetworkError represents network-related errors
type NetworkError struct {
	*VariablyError
	StatusCode int
	URL        string
}

// NewNetworkError creates a new NetworkError
func NewNetworkError(message string, statusCode int, url string, cause error) *NetworkError {
	return &NetworkError{
		VariablyError: NewVariablyError(message, cause),
		StatusCode:    statusCode,
		URL:           url,
	}
}

// IsRetryable returns true if the network error is retryable
func (e *NetworkError) IsRetryable() bool {
	return e.StatusCode >= 500 || e.StatusCode == http.StatusRequestTimeout || e.StatusCode == http.StatusTooManyRequests
}

// AuthenticationError represents authentication-related errors
type AuthenticationError struct {
	*VariablyError
}

// NewAuthenticationError creates a new AuthenticationError
func NewAuthenticationError(message string, cause error) *AuthenticationError {
	return &AuthenticationError{
		VariablyError: NewVariablyError(message, cause),
	}
}

// ValidationError represents validation-related errors
type ValidationError struct {
	*VariablyError
	Field string
}

// NewValidationError creates a new ValidationError
func NewValidationError(message, field string, cause error) *ValidationError {
	return &ValidationError{
		VariablyError: NewVariablyError(message, cause),
		Field:         field,
	}
}

// ConfigurationError represents configuration-related errors
type ConfigurationError struct {
	*VariablyError
	Field string
}

// NewConfigurationError creates a new ConfigurationError
func NewConfigurationError(message, field string, cause error) *ConfigurationError {
	return &ConfigurationError{
		VariablyError: NewVariablyError(message, cause),
		Field:         field,
	}
}

// ConnectionError represents WebSocket connection errors
type ConnectionError struct {
	*VariablyError
	Temporary bool
}

// NewConnectionError creates a new ConnectionError
func NewConnectionError(message string, temporary bool, cause error) *ConnectionError {
	return &ConnectionError{
		VariablyError: NewVariablyError(message, cause),
		Temporary:     temporary,
	}
}

// TimeoutError represents timeout errors
type TimeoutError struct {
	*VariablyError
	Duration string
}

// NewTimeoutError creates a new TimeoutError
func NewTimeoutError(message, duration string, cause error) *TimeoutError {
	return &TimeoutError{
		VariablyError: NewVariablyError(message, cause),
		Duration:      duration,
	}
}
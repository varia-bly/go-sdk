package variably

import (
	"fmt"
	"net/http"
)

// SDKError represents a Variably SDK error
type SDKError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
	Cause   error  `json:"-"`
}

func (e *SDKError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

func (e *SDKError) Unwrap() error {
	return e.Cause
}

// NetworkError represents network-related errors
type NetworkError struct {
	*SDKError
	StatusCode int    `json:"status_code,omitempty"`
	URL        string `json:"url,omitempty"`
}

// AuthenticationError represents authentication failures
type AuthenticationError struct {
	*SDKError
}

// ValidationError represents request validation failures
type ValidationError struct {
	*SDKError
	Field string `json:"field,omitempty"`
}

// RateLimitError represents rate limiting errors
type RateLimitError struct {
	*SDKError
	RetryAfter int `json:"retry_after,omitempty"`
}

// TimeoutError represents timeout errors
type TimeoutError struct {
	*SDKError
	Duration string `json:"duration,omitempty"`
}

// CacheError represents cache operation errors
type CacheError struct {
	*SDKError
	Operation string `json:"operation,omitempty"`
}

// ConfigError represents configuration errors
type ConfigError struct {
	*SDKError
	Field string `json:"field,omitempty"`
}

// NewNetworkError creates a new network error
func NewNetworkError(message string, statusCode int, url string, cause error) *NetworkError {
	return &NetworkError{
		SDKError: &SDKError{
			Code:    "NETWORK_ERROR",
			Message: message,
			Type:    "NetworkError",
			Cause:   cause,
		},
		StatusCode: statusCode,
		URL:        url,
	}
}

// NewAuthenticationError creates a new authentication error
func NewAuthenticationError(message string, cause error) *AuthenticationError {
	return &AuthenticationError{
		SDKError: &SDKError{
			Code:    "AUTHENTICATION_ERROR",
			Message: message,
			Type:    "AuthenticationError",
			Cause:   cause,
		},
	}
}

// NewValidationError creates a new validation error
func NewValidationError(message, field string, cause error) *ValidationError {
	return &ValidationError{
		SDKError: &SDKError{
			Code:    "VALIDATION_ERROR",
			Message: message,
			Type:    "ValidationError",
			Cause:   cause,
		},
		Field: field,
	}
}

// NewRateLimitError creates a new rate limit error
func NewRateLimitError(message string, retryAfter int, cause error) *RateLimitError {
	return &RateLimitError{
		SDKError: &SDKError{
			Code:    "RATE_LIMIT_ERROR",
			Message: message,
			Type:    "RateLimitError",
			Cause:   cause,
		},
		RetryAfter: retryAfter,
	}
}

// NewTimeoutError creates a new timeout error
func NewTimeoutError(message, duration string, cause error) *TimeoutError {
	return &TimeoutError{
		SDKError: &SDKError{
			Code:    "TIMEOUT_ERROR",
			Message: message,
			Type:    "TimeoutError",
			Cause:   cause,
		},
		Duration: duration,
	}
}

// NewCacheError creates a new cache error
func NewCacheError(message, operation string, cause error) *CacheError {
	return &CacheError{
		SDKError: &SDKError{
			Code:    "CACHE_ERROR",
			Message: message,
			Type:    "CacheError",
			Cause:   cause,
		},
		Operation: operation,
	}
}

// NewConfigError creates a new configuration error
func NewConfigError(message, field string, cause error) *ConfigError {
	return &ConfigError{
		SDKError: &SDKError{
			Code:    "CONFIG_ERROR",
			Message: message,
			Type:    "ConfigError",
			Cause:   cause,
		},
		Field: field,
	}
}

// IsRetryable determines if an error is retryable
func IsRetryable(err error) bool {
	switch e := err.(type) {
	case *NetworkError:
		// Retry on server errors and some client errors
		return e.StatusCode >= 500 || e.StatusCode == http.StatusTooManyRequests || e.StatusCode == http.StatusRequestTimeout
	case *TimeoutError:
		return true
	case *RateLimitError:
		return true
	default:
		return false
	}
}

// IsTemporary determines if an error is temporary
func IsTemporary(err error) bool {
	switch err.(type) {
	case *NetworkError:
		return true
	case *TimeoutError:
		return true
	case *RateLimitError:
		return true
	default:
		return false
	}
}

// GetRetryDelay returns the recommended retry delay for an error
func GetRetryDelay(err error) int {
	if rateLimitErr, ok := err.(*RateLimitError); ok && rateLimitErr.RetryAfter > 0 {
		return rateLimitErr.RetryAfter
	}
	return 0
}
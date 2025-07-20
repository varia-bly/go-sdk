package variably

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// HTTPClient handles all HTTP communication with the Variably API
type HTTPClient struct {
	client        *http.Client
	baseURL       string
	apiKey        string
	retryAttempts int
	logger        Logger
	metrics       *MetricsCollector
}

// NewHTTPClient creates a new HTTP client with retry logic and circuit breaker
func NewHTTPClient(config *Config, logger Logger, metrics *MetricsCollector) *HTTPClient {
	return &HTTPClient{
		client: &http.Client{
			Timeout: config.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		baseURL:       config.BaseURL,
		apiKey:        config.APIKey,
		retryAttempts: config.RetryAttempts,
		logger:        logger,
		metrics:       metrics,
	}
}

// EvaluateFlagRequest represents a single flag evaluation request
type EvaluateFlagRequest struct {
	FlagKey string      `json:"flag_key"`
	Context UserContext `json:"context"`
}

// EvaluateFlagResponse represents a single flag evaluation response
type EvaluateFlagResponse struct {
	Enabled   bool   `json:"enabled"`
	FlagKey   string `json:"flag_key"`
	UserID    string `json:"user_id"`
	Timestamp string `json:"timestamp"`
}

// BatchEvaluateFlagsRequest represents a batch flag evaluation request
type BatchEvaluateFlagsRequest struct {
	FlagKeys    []string    `json:"flag_keys"`
	Context     UserContext `json:"context"`
	Environment string      `json:"environment,omitempty"`
}

// BatchEvaluateFlagsResponse represents a batch flag evaluation response
type BatchEvaluateFlagsResponse struct {
	Results map[string]EvaluateFlagResponse `json:"results"`
}

// EvaluateGateRequest represents a feature gate evaluation request
type EvaluateGateRequest struct {
	GateKey string      `json:"gate_key"`
	Context UserContext `json:"context"`
}

// EvaluateGateResponse represents a feature gate evaluation response
type EvaluateGateResponse struct {
	Enabled       bool   `json:"enabled"`
	GateKey       string `json:"gate_key"`
	UserID        string `json:"user_id"`
	AccessGranted bool   `json:"access_granted"`
	Timestamp     string `json:"timestamp"`
}

// BatchEvaluateGatesRequest represents a batch gate evaluation request
type BatchEvaluateGatesRequest struct {
	GateKeys    []string    `json:"gate_keys"`
	UserID      string      `json:"user_id"`
	Environment string      `json:"environment,omitempty"`
	UserContext UserContext `json:"user_context"`
}

// BatchEvaluateGatesResponse represents a batch gate evaluation response
type BatchEvaluateGatesResponse struct {
	Results map[string]EvaluateGateResponse `json:"results"`
}

// TrackEventRequest represents an event tracking request
type TrackEventRequest struct {
	Name       string                 `json:"name"`
	UserID     string                 `json:"user_id"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
}

// BatchTrackEventsRequest represents a batch event tracking request
type BatchTrackEventsRequest struct {
	Events []TrackEventRequest `json:"events"`
}

// APIResponse represents a generic API response
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
}

// APIError represents an API error response
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// EvaluateFlag evaluates a single feature flag
func (c *HTTPClient) EvaluateFlag(ctx context.Context, flagKey string, userContext UserContext, environment string) (*EvaluateFlagResponse, error) {
	req := EvaluateFlagRequest{
		FlagKey: flagKey,
		Context: userContext,
	}

	var resp EvaluateFlagResponse
	err := c.makeRequest(ctx, "POST", "/api/v1/sdk/evaluate", req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// EvaluateFlags evaluates multiple feature flags in batch
func (c *HTTPClient) EvaluateFlags(ctx context.Context, flagKeys []string, userContext UserContext, environment string) (*BatchEvaluateFlagsResponse, error) {
	req := BatchEvaluateFlagsRequest{
		FlagKeys:    flagKeys,
		Context:     userContext,
		Environment: environment,
	}

	var resp BatchEvaluateFlagsResponse
	err := c.makeRequest(ctx, "POST", "/api/v1/sdk/evaluate/batch", req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// EvaluateGate evaluates a single feature gate
func (c *HTTPClient) EvaluateGate(ctx context.Context, gateKey string, userContext UserContext, environment string) (*EvaluateGateResponse, error) {
	req := EvaluateGateRequest{
		GateKey: gateKey,
		Context: userContext,
	}

	var resp EvaluateGateResponse
	err := c.makeRequest(ctx, "POST", "/api/v1/sdk/feature-gates/evaluate", req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// EvaluateGates evaluates multiple feature gates in batch
func (c *HTTPClient) EvaluateGates(ctx context.Context, gateKeys []string, userContext UserContext, environment string) (*BatchEvaluateGatesResponse, error) {
	req := BatchEvaluateGatesRequest{
		GateKeys:    gateKeys,
		UserID:      userContext.UserID,
		Environment: environment,
		UserContext: userContext,
	}

	var resp BatchEvaluateGatesResponse
	err := c.makeRequest(ctx, "POST", "/api/v1/sdk/feature-gates/evaluate/batch", req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// TrackEvent tracks a single analytics event
func (c *HTTPClient) TrackEvent(ctx context.Context, event Event) error {
	req := TrackEventRequest{
		Name:       event.Name,
		UserID:     event.UserID,
		Properties: event.Properties,
		Timestamp:  event.Timestamp,
	}

	return c.makeRequest(ctx, "POST", "/api/v1/sdk/events", req, nil)
}

// TrackEvents tracks multiple analytics events in batch
func (c *HTTPClient) TrackEvents(ctx context.Context, events []Event) error {
	reqs := make([]TrackEventRequest, len(events))
	for i, event := range events {
		reqs[i] = TrackEventRequest{
			Name:       event.Name,
			UserID:     event.UserID,
			Properties: event.Properties,
			Timestamp:  event.Timestamp,
		}
	}

	req := BatchTrackEventsRequest{Events: reqs}
	return c.makeRequest(ctx, "POST", "/api/v1/sdk/events/batch", req, nil)
}

// makeRequest makes an HTTP request with retry logic and error handling
func (c *HTTPClient) makeRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	var lastErr error

	for attempt := 0; attempt <= c.retryAttempts; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff with jitter
			backoff := c.calculateBackoff(attempt)
			c.logger.Debug("Retrying request", "attempt", attempt, "backoff", backoff)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		startTime := time.Now()
		err := c.doRequest(ctx, method, path, body, result)
		latency := time.Since(startTime)

		// Update metrics
		c.metrics.RecordAPICall(latency, err == nil)

		if err == nil {
			return nil
		}

		lastErr = err

		// Don't retry non-retryable errors
		if !IsRetryable(err) {
			c.logger.Debug("Non-retryable error, not retrying", "error", err)
			break
		}

		c.logger.Debug("Retryable error occurred", "error", err, "attempt", attempt)
	}

	return lastErr
}

// doRequest performs the actual HTTP request
func (c *HTTPClient) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return NewValidationError("Failed to marshal request body", "", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return NewNetworkError("Failed to create request", 0, url, err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("User-Agent", "Variably-Go-SDK/1.0.0")

	c.logger.Debug("Making HTTP request", "method", method, "url", url)

	resp, err := c.client.Do(req)
	if err != nil {
		return NewNetworkError("Request failed", 0, url, err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return NewNetworkError("Failed to read response body", resp.StatusCode, url, err)
	}

	// Check for HTTP errors
	if resp.StatusCode >= 400 {
		return c.handleHTTPError(resp.StatusCode, respBody, url)
	}

	// Parse successful response
	if result != nil {
		// Try to parse as APIResponse first
		var apiResp APIResponse
		if err := json.Unmarshal(respBody, &apiResp); err == nil && apiResp.Error != nil {
			return NewNetworkError(apiResp.Error.Message, resp.StatusCode, url, nil)
		}

		// Parse as direct result
		if err := json.Unmarshal(respBody, result); err != nil {
			return NewNetworkError("Failed to parse response", resp.StatusCode, url, err)
		}
	}

	c.logger.Debug("HTTP request successful", "status", resp.StatusCode, "url", url)
	return nil
}

// handleHTTPError converts HTTP error responses to appropriate SDK errors
func (c *HTTPClient) handleHTTPError(statusCode int, body []byte, url string) error {
	var apiErr APIError
	if err := json.Unmarshal(body, &apiErr); err != nil {
		// If we can't parse the error, create a generic one
		return NewNetworkError(fmt.Sprintf("HTTP %d", statusCode), statusCode, url, nil)
	}

	switch statusCode {
	case http.StatusUnauthorized:
		return NewAuthenticationError(apiErr.Message, nil)
	case http.StatusBadRequest:
		return NewValidationError(apiErr.Message, "", nil)
	case http.StatusTooManyRequests:
		retryAfter := 0
		if apiErr.Details != "" {
			if parsed, err := strconv.Atoi(apiErr.Details); err == nil {
				retryAfter = parsed
			}
		}
		return NewRateLimitError(apiErr.Message, retryAfter, nil)
	case http.StatusRequestTimeout:
		return NewTimeoutError(apiErr.Message, "", nil)
	default:
		return NewNetworkError(apiErr.Message, statusCode, url, nil)
	}
}

// calculateBackoff calculates exponential backoff with jitter
func (c *HTTPClient) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff: 2^attempt * 100ms, max 30 seconds
	base := time.Duration(100) * time.Millisecond
	backoff := base * time.Duration(math.Pow(2, float64(attempt)))

	// Cap at 30 seconds
	maxBackoff := 30 * time.Second
	if backoff > maxBackoff {
		backoff = maxBackoff
	}

	// Add jitter (±25% random variation)
	jitter := time.Duration(rand.Float64()*0.5-0.25) * backoff
	return backoff + jitter
}
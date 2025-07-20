package variably

import (
	"context"
	"sync"
	"time"
)

// VariablyClient implements the Client interface
type VariablyClient struct {
	config       *Config
	httpClient   *HTTPClient
	evaluator    *Evaluator
	cacheManager *CacheManager
	metrics      *MetricsCollector
	logger       Logger

	// Real-time updates
	subscriptions map[string][]UpdateCallback
	subMutex      sync.RWMutex

	// Lifecycle
	closed   bool
	stopCh   chan struct{}
	closeMux sync.Mutex
}

// NewClient creates a new Variably client instance
func NewClient(config *Config) (Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, NewConfigError("Invalid configuration", "", err)
	}

	// Set up logger
	logger := config.Logger
	if logger == nil {
		logger = NewDefaultLogger(config.LogConfig)
	}

	// Create metrics collector
	metrics := NewMetricsCollector()

	// Create cache manager
	cacheManager := NewCacheManager(config.CacheConfig, logger)

	// Create HTTP client
	httpClient := NewHTTPClient(config, logger, metrics)

	// Create evaluator
	evaluator := NewEvaluator(httpClient, cacheManager, metrics, logger, config)

	client := &VariablyClient{
		config:        config,
		httpClient:    httpClient,
		evaluator:     evaluator,
		cacheManager:  cacheManager,
		metrics:       metrics,
		logger:        logger,
		subscriptions: make(map[string][]UpdateCallback),
		stopCh:        make(chan struct{}),
	}

	// Start background tasks
	client.startBackgroundTasks()

	logger.Info("Variably client initialized", "environment", config.Environment, "base_url", config.BaseURL)

	return client, nil
}

// NewClientFromEnv creates a new client using environment variables
func NewClientFromEnv() (Client, error) {
	config := LoadConfigFromEnv()
	return NewClient(config)
}

// NewClientFromFile creates a new client using a configuration file
func NewClientFromFile(filename string) (Client, error) {
	config, err := LoadConfigFromFile(filename)
	if err != nil {
		return nil, err
	}
	return NewClient(config)
}

// Feature Flag Operations

// EvaluateFlag evaluates a feature flag and returns the full result
func (c *VariablyClient) EvaluateFlag(ctx context.Context, flagKey string, defaultValue interface{}, userContext UserContext) FlagResult {
	c.ensureNotClosed()
	return c.evaluator.EvaluateFlag(ctx, flagKey, defaultValue, userContext)
}

// EvaluateFlagBool evaluates a boolean feature flag
func (c *VariablyClient) EvaluateFlagBool(ctx context.Context, flagKey string, defaultValue bool, userContext UserContext) bool {
	result := c.EvaluateFlag(ctx, flagKey, defaultValue, userContext)
	if result.Error != nil {
		c.logger.Debug("Flag evaluation error, using default", "flag_key", flagKey, "default", defaultValue, "error", result.Error)
		return defaultValue
	}
	
	if value, ok := result.Value.(bool); ok {
		return value
	}
	
	c.logger.Warn("Flag value is not boolean, using default", "flag_key", flagKey, "value", result.Value, "default", defaultValue)
	return defaultValue
}

// EvaluateFlagString evaluates a string feature flag
func (c *VariablyClient) EvaluateFlagString(ctx context.Context, flagKey string, defaultValue string, userContext UserContext) string {
	result := c.EvaluateFlag(ctx, flagKey, defaultValue, userContext)
	if result.Error != nil {
		c.logger.Debug("Flag evaluation error, using default", "flag_key", flagKey, "default", defaultValue, "error", result.Error)
		return defaultValue
	}
	
	if value, ok := result.Value.(string); ok {
		return value
	}
	
	c.logger.Warn("Flag value is not string, using default", "flag_key", flagKey, "value", result.Value, "default", defaultValue)
	return defaultValue
}

// EvaluateFlagInt evaluates an integer feature flag
func (c *VariablyClient) EvaluateFlagInt(ctx context.Context, flagKey string, defaultValue int, userContext UserContext) int {
	result := c.EvaluateFlag(ctx, flagKey, defaultValue, userContext)
	if result.Error != nil {
		c.logger.Debug("Flag evaluation error, using default", "flag_key", flagKey, "default", defaultValue, "error", result.Error)
		return defaultValue
	}
	
	// Handle both int and float64 (JSON numbers)
	switch v := result.Value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		c.logger.Warn("Flag value is not numeric, using default", "flag_key", flagKey, "value", result.Value, "default", defaultValue)
		return defaultValue
	}
}

// EvaluateFlagFloat evaluates a float feature flag
func (c *VariablyClient) EvaluateFlagFloat(ctx context.Context, flagKey string, defaultValue float64, userContext UserContext) float64 {
	result := c.EvaluateFlag(ctx, flagKey, defaultValue, userContext)
	if result.Error != nil {
		c.logger.Debug("Flag evaluation error, using default", "flag_key", flagKey, "default", defaultValue, "error", result.Error)
		return defaultValue
	}
	
	// Handle both float64 and int (JSON numbers)
	switch v := result.Value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		c.logger.Warn("Flag value is not numeric, using default", "flag_key", flagKey, "value", result.Value, "default", defaultValue)
		return defaultValue
	}
}

// EvaluateFlagJSON evaluates a JSON feature flag (returns interface{})
func (c *VariablyClient) EvaluateFlagJSON(ctx context.Context, flagKey string, defaultValue interface{}, userContext UserContext) interface{} {
	result := c.EvaluateFlag(ctx, flagKey, defaultValue, userContext)
	if result.Error != nil {
		c.logger.Debug("Flag evaluation error, using default", "flag_key", flagKey, "error", result.Error)
		return defaultValue
	}
	
	return result.Value
}

// Feature Gate Operations

// EvaluateGate evaluates a feature gate
func (c *VariablyClient) EvaluateGate(ctx context.Context, gateKey string, userContext UserContext) bool {
	c.ensureNotClosed()
	return c.evaluator.EvaluateGate(ctx, gateKey, userContext)
}

// Batch Operations

// EvaluateFlags evaluates multiple feature flags
func (c *VariablyClient) EvaluateFlags(ctx context.Context, flagKeys []string, userContext UserContext) map[string]FlagResult {
	c.ensureNotClosed()
	return c.evaluator.EvaluateFlags(ctx, flagKeys, userContext)
}

// EvaluateGates evaluates multiple feature gates
func (c *VariablyClient) EvaluateGates(ctx context.Context, gateKeys []string, userContext UserContext) map[string]bool {
	c.ensureNotClosed()
	return c.evaluator.EvaluateGates(ctx, gateKeys, userContext)
}

// Event Tracking

// Track tracks a single analytics event
func (c *VariablyClient) Track(ctx context.Context, event Event) error {
	c.ensureNotClosed()
	
	if !c.config.EnableAnalytics {
		c.logger.Debug("Analytics disabled, skipping event tracking")
		return nil
	}
	
	// Ensure event has timestamp
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	
	c.metrics.RecordEventTracked()
	
	err := c.httpClient.TrackEvent(ctx, event)
	if err != nil {
		c.logger.Error("Failed to track event", "event_name", event.Name, "error", err)
		return err
	}
	
	c.logger.Debug("Event tracked successfully", "event_name", event.Name, "user_id", event.UserID)
	return nil
}

// TrackBatch tracks multiple analytics events
func (c *VariablyClient) TrackBatch(ctx context.Context, events []Event) error {
	c.ensureNotClosed()
	
	if !c.config.EnableAnalytics {
		c.logger.Debug("Analytics disabled, skipping batch event tracking")
		return nil
	}
	
	// Ensure all events have timestamps
	for i := range events {
		if events[i].Timestamp.IsZero() {
			events[i].Timestamp = time.Now()
		}
	}
	
	for range events {
		c.metrics.RecordEventTracked()
	}
	
	err := c.httpClient.TrackEvents(ctx, events)
	if err != nil {
		c.logger.Error("Failed to track batch events", "count", len(events), "error", err)
		return err
	}
	
	c.logger.Debug("Batch events tracked successfully", "count", len(events))
	return nil
}

// Real-time Updates

// Subscribe subscribes to real-time flag updates
func (c *VariablyClient) Subscribe(ctx context.Context, flagKeys []string, callback UpdateCallback) error {
	c.ensureNotClosed()
	
	if !c.config.EnableRealTimeSync {
		return NewConfigError("Real-time sync is disabled", "EnableRealTimeSync", nil)
	}
	
	c.subMutex.Lock()
	defer c.subMutex.Unlock()
	
	for _, flagKey := range flagKeys {
		c.subscriptions[flagKey] = append(c.subscriptions[flagKey], callback)
	}
	
	c.logger.Info("Subscribed to flag updates", "flags", flagKeys)
	return nil
}

// Unsubscribe unsubscribes from real-time flag updates
func (c *VariablyClient) Unsubscribe(flagKeys []string) error {
	c.ensureNotClosed()
	
	c.subMutex.Lock()
	defer c.subMutex.Unlock()
	
	for _, flagKey := range flagKeys {
		delete(c.subscriptions, flagKey)
	}
	
	c.logger.Info("Unsubscribed from flag updates", "flags", flagKeys)
	return nil
}

// Cache Management

// RefreshCache clears the cache to force fresh evaluation
func (c *VariablyClient) RefreshCache(ctx context.Context) error {
	c.ensureNotClosed()
	return c.evaluator.RefreshCache(ctx)
}

// ClearCache clears the cache
func (c *VariablyClient) ClearCache() error {
	c.ensureNotClosed()
	return c.evaluator.ClearCache()
}

// Metrics

// GetMetrics returns current SDK metrics
func (c *VariablyClient) GetMetrics() Metrics {
	return c.metrics.GetMetrics()
}

// Lifecycle

// Close closes the client and cleans up resources
func (c *VariablyClient) Close() error {
	c.closeMux.Lock()
	defer c.closeMux.Unlock()
	
	if c.closed {
		return nil
	}
	
	c.closed = true
	close(c.stopCh)
	
	c.logger.Info("Variably client closed")
	return nil
}

// Private methods

// ensureNotClosed panics if the client has been closed
func (c *VariablyClient) ensureNotClosed() {
	c.closeMux.Lock()
	defer c.closeMux.Unlock()
	
	if c.closed {
		panic("Variably client has been closed")
	}
}

// startBackgroundTasks starts background goroutines for maintenance tasks
func (c *VariablyClient) startBackgroundTasks() {
	// Start cache cleanup
	go c.cacheManager.StartCleanup(c.stopCh)
	
	// Start polling for updates if enabled
	if c.config.PollingConfig.Enabled {
		go c.startPolling()
	}
}

// startPolling starts polling for flag updates
func (c *VariablyClient) startPolling() {
	ticker := time.NewTicker(c.config.PollingConfig.Interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			c.pollForUpdates()
		case <-c.stopCh:
			return
		}
	}
}

// pollForUpdates polls the API for flag updates
func (c *VariablyClient) pollForUpdates() {
	// This is a simplified implementation
	// In a real implementation, you would call an API endpoint
	// that returns flag changes since the last poll
	c.logger.Debug("Polling for flag updates")
	
	// For now, just clear cache to force refresh
	// This ensures flags are re-evaluated on next access
	if c.config.PollingConfig.Enabled {
		// Only refresh a subset of cache based on subscriptions
		c.subMutex.RLock()
		hasSubscriptions := len(c.subscriptions) > 0
		c.subMutex.RUnlock()
		
		if hasSubscriptions {
			// In production, you would selectively refresh only subscribed flags
			c.logger.Debug("Refreshing cache due to polling")
		}
	}
}
package variably

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// VariablyClient provides both dynamic configuration and traditional feature flag functionality
type VariablyClient struct {
	config ClientConfig
	logger Logger
	cache  *CacheManager
	wsClient *WebSocketClient
	httpClient *http.Client
	metrics *MetricsCollector
	flagSnapshots *flagSnapshotStore
	
	// State management
	configValues    map[string]*DynamicConfigResult
	configCallbacks map[string][]ConfigChangeCallback
	stateMutex      sync.RWMutex
	
	// Traditional feature flag subscriptions
	flagSubscriptions map[string][]UpdateCallback
	flagSubMutex      sync.RWMutex
	
	// Connection state
	isRealTimeMode bool
	fallbackMode   bool
	pollingTimer   *time.Timer
	
	// Control
	ctx    context.Context
	cancel context.CancelFunc
}

// NewVariablyClient creates a new Variably client supporting both dynamic configs and feature flags
func NewVariablyClient(config ClientConfig, logger Logger) (*VariablyClient, error) {
	// Validate required fields
	if config.APIKey == "" {
		return nil, NewConfigurationError("API key is required", "api_key", nil)
	}
	if config.ProjectID == "" {
		return nil, NewConfigurationError("Project ID is required", "project_id", nil)
	}
	if config.BaseURL == "" {
		// Fail closed, matching the Python SDK. Defaulting to localhost means a
		// service that forgets to configure this silently reaches nothing, every
		// evaluation falls back to its default, and the result is indistinguishable
		// from a real rollout that turned everything off.
		return nil, NewConfigurationError(
			"Base URL is required — set it to your Variably API endpoint. No default, "+
				"so evaluation data is never sent to an unintended host.",
			"base_url", nil,
		)
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	client := &VariablyClient{
		config:            config,
		logger:            logger,
		cache:             NewCacheManager(config.Cache, logger),
		httpClient:        &http.Client{Timeout: 30 * time.Second},
		flagSnapshots:     newFlagSnapshotStore(config.Cache),
		metrics:           NewMetricsCollector(),
		configValues:      make(map[string]*DynamicConfigResult),
		configCallbacks:   make(map[string][]ConfigChangeCallback),
		flagSubscriptions: make(map[string][]UpdateCallback),
		ctx:               ctx,
		cancel:            cancel,
	}
	
	// Initialize WebSocket client if real-time is enabled
	if config.EnableRealtime && config.JWTToken != "" {
		client.initializeWebSocket()
	} else {
		client.logger.Info("Real-time updates disabled, using polling mode only", nil)
		client.startPolling()
	}
	
	client.logger.Info("DynamicConfigClient initialized", map[string]interface{}{
		"project_id":       config.ProjectID,
		"enable_realtime":  config.EnableRealtime,
		"polling_interval": config.PollingInterval,
	})
	
	return client, nil
}

// ========== DYNAMIC CONFIGURATION API ==========

// GetConfig retrieves a dynamic configuration value
func (c *VariablyClient) GetConfig(ctx context.Context, configKey string, defaultValue interface{}, userContext UserContext) (interface{}, error) {
	result, err := c.EvaluateConfig(ctx, configKey, defaultValue, userContext)
	if err != nil {
		return defaultValue, err
	}
	
	if result.Error != nil {
		return defaultValue, result.Error
	}
	
	return result.Value, nil
}

// GetConfigBool retrieves a boolean dynamic configuration
func (c *VariablyClient) GetConfigBool(ctx context.Context, configKey string, defaultValue bool, userContext UserContext) (bool, error) {
	result, err := c.EvaluateConfig(ctx, configKey, defaultValue, userContext)
	if err != nil {
		return defaultValue, err
	}
	
	if result.Error != nil {
		return defaultValue, result.Error
	}
	
	if val, ok := result.Value.(bool); ok {
		return val, nil
	}
	
	return defaultValue, nil
}

// GetConfigString retrieves a string dynamic configuration
func (c *VariablyClient) GetConfigString(ctx context.Context, configKey string, defaultValue string, userContext UserContext) (string, error) {
	result, err := c.EvaluateConfig(ctx, configKey, defaultValue, userContext)
	if err != nil {
		return defaultValue, err
	}
	
	if result.Error != nil {
		return defaultValue, result.Error
	}
	
	if val, ok := result.Value.(string); ok {
		return val, nil
	}
	
	return defaultValue, nil
}

// GetConfigNumber retrieves a numeric dynamic configuration
func (c *VariablyClient) GetConfigNumber(ctx context.Context, configKey string, defaultValue float64, userContext UserContext) (float64, error) {
	result, err := c.EvaluateConfig(ctx, configKey, defaultValue, userContext)
	if err != nil {
		return defaultValue, err
	}
	
	if result.Error != nil {
		return defaultValue, result.Error
	}
	
	if val, ok := result.Value.(float64); ok {
		return val, nil
	}
	
	// Try to convert from other numeric types
	if val, ok := result.Value.(int); ok {
		return float64(val), nil
	}
	if val, ok := result.Value.(int64); ok {
		return float64(val), nil
	}
	
	return defaultValue, nil
}

// GetConfigJSON retrieves a JSON dynamic configuration
func (c *VariablyClient) GetConfigJSON(ctx context.Context, configKey string, defaultValue interface{}, userContext UserContext) (interface{}, error) {
	result, err := c.EvaluateConfig(ctx, configKey, defaultValue, userContext)
	if err != nil {
		return defaultValue, err
	}
	
	if result.Error != nil {
		return defaultValue, result.Error
	}
	
	return result.Value, nil
}

// EvaluateConfig evaluates a dynamic configuration with full details
func (c *VariablyClient) EvaluateConfig(ctx context.Context, configKey string, defaultValue interface{}, userContext UserContext) (*DynamicConfigResult, error) {
	// Validate inputs
	if configKey == "" {
		return nil, NewValidationError("Config key must be a non-empty string", "configKey", nil)
	}
	if userContext.UserID == "" {
		return nil, NewValidationError("User context must include UserID", "userContext.UserID", nil)
	}
	
	// Check cache first
	cacheKey := c.generateCacheKey(configKey, userContext)
	if cached, found := c.cache.Get(ctx, cacheKey); found {
		if result, ok := cached.(*DynamicConfigResult); ok {
			c.logger.Debug("Config evaluation cache hit", map[string]interface{}{
				"config_key": configKey,
				"user_id":    userContext.UserID,
			})
			
			// Update cache hit flag
			result.CacheHit = true
			result.RealTimeUpdate = false
			return result, nil
		}
	}
	
	// Evaluate via API
	response, err := c.evaluateConfigViaAPI(ctx, configKey, userContext, "")
	if err != nil {
		c.logger.Error("Config evaluation failed", map[string]interface{}{
			"config_key": configKey,
			"error":      err.Error(),
			"user_id":    userContext.UserID,
		})
		
		// Return default value with error
		return &DynamicConfigResult{
			Key:            configKey,
			Value:          defaultValue,
			Reason:         "error_fallback",
			CacheHit:       false,
			RealTimeUpdate: false,
			RetrievedAt:    time.Now(),
			Error:          err,
		}, nil
	}
	
	// Parse the value if it's JSON
	var value interface{} = defaultValue
	if response != nil && response.Value != nil {
		var parsed interface{}
		if err := json.Unmarshal(response.Value, &parsed); err == nil {
			value = parsed
		} else {
			// If JSON parsing fails, use the raw value
			value = string(response.Value)
		}
	}
	
	result := &DynamicConfigResult{
		Key:            configKey,
		Value:          value,
		Reason:         "api_evaluation",
		CacheHit:       false,
		RealTimeUpdate: false,
		RetrievedAt:    time.Now(),
	}
	
	if response != nil {
		result.RuleID = response.RuleID
		result.ETag = response.ETag
		result.Version = response.Version
		
		if response.UpdatedAt != nil && *response.UpdatedAt != "" {
			if updatedAt, err := time.Parse(time.RFC3339, *response.UpdatedAt); err == nil {
				result.UpdatedAt = &updatedAt
			}
		}
	}
	
	// Cache the result
	c.cache.Set(ctx, cacheKey, result, c.config.Cache.TTL)
	
	// Store in local state
	c.stateMutex.Lock()
	c.configValues[configKey] = result
	c.stateMutex.Unlock()
	
	c.logger.Debug("Config evaluation successful", map[string]interface{}{
		"config_key": configKey,
		"value":      value,
		"version":    result.Version,
		"user_id":    userContext.UserID,
	})
	
	return result, nil
}

// OnConfigChange subscribes to real-time updates for a specific configuration
func (c *VariablyClient) OnConfigChange(configKey string, callback ConfigChangeCallback) func() {
	c.stateMutex.Lock()
	defer c.stateMutex.Unlock()
	
	if _, exists := c.configCallbacks[configKey]; !exists {
		c.configCallbacks[configKey] = make([]ConfigChangeCallback, 0)
	}
	
	c.configCallbacks[configKey] = append(c.configCallbacks[configKey], callback)
	
	// Subscribe to WebSocket updates if available
	if c.wsClient != nil && c.isRealTimeMode {
		c.wsClient.Subscribe(c.config.ProjectID, configKey)
	}
	
	c.logger.Debug("Config change subscription added", map[string]interface{}{
		"config_key": configKey,
	})
	
	// Return unsubscribe function
	return func() {
		c.stateMutex.Lock()
		defer c.stateMutex.Unlock()
		
		callbacks := c.configCallbacks[configKey]
		for i, cb := range callbacks {
			// Compare function pointers (this is a limitation in Go)
			if fmt.Sprintf("%p", cb) == fmt.Sprintf("%p", callback) {
				// Remove callback from slice
				c.configCallbacks[configKey] = append(callbacks[:i], callbacks[i+1:]...)
				break
			}
		}
		
		// Remove empty callback slice
		if len(c.configCallbacks[configKey]) == 0 {
			delete(c.configCallbacks, configKey)
			
			// Unsubscribe from WebSocket if no more callbacks
			if c.wsClient != nil {
				c.wsClient.Unsubscribe(c.config.ProjectID, configKey)
			}
		}
	}
}

// OnAnyConfigChange subscribes to all configuration changes in the project
func (c *VariablyClient) OnAnyConfigChange(callback ConfigChangeCallback) func() {
	return c.OnConfigChange("*", callback)
}

// RefreshConfigs refreshes all cached configurations
func (c *VariablyClient) RefreshConfigs(ctx context.Context, userContext UserContext) error {
	c.stateMutex.RLock()
	configKeys := make([]string, 0, len(c.configValues))
	for key := range c.configValues {
		configKeys = append(configKeys, key)
	}
	c.stateMutex.RUnlock()
	
	if len(configKeys) == 0 {
		c.logger.Debug("No configs to refresh", nil)
		return nil
	}
	
	c.logger.Info("Refreshing cached configurations", map[string]interface{}{
		"count": len(configKeys),
	})
	
	// Clear cache
	c.cache.Clear(ctx)
	
	// Refresh each config
	for _, configKey := range configKeys {
		c.stateMutex.RLock()
		cached := c.configValues[configKey]
		c.stateMutex.RUnlock()
		
		if cached != nil {
			_, err := c.EvaluateConfig(ctx, configKey, cached.Value, userContext)
			if err != nil {
				c.logger.Error("Failed to refresh config", map[string]interface{}{
					"config_key": configKey,
					"error":      err.Error(),
				})
			}
		}
	}
	
	return nil
}

// GetConnectionStatus returns current connection status
func (c *VariablyClient) GetConnectionStatus() ConnectionStatus {
	if c.wsClient != nil && c.isRealTimeMode {
		return ConnectionStatus{
			Mode:           "realtime",
			Connected:      c.wsClient.IsConnected(),
			FallbackActive: c.fallbackMode,
		}
	}
	
	return ConnectionStatus{
		Mode:           "polling",
		Connected:      true,
		FallbackActive: false,
	}
}

// Close disconnects and cleans up the client
func (c *VariablyClient) Close() error {
	c.logger.Info("Closing DynamicConfigClient", nil)
	
	// Cancel context
	c.cancel()
	
	// Close WebSocket client
	if c.wsClient != nil {
		c.wsClient.Disconnect()
	}
	
	// Stop polling timer
	if c.pollingTimer != nil {
		c.pollingTimer.Stop()
	}
	
	// Clear state
	c.stateMutex.Lock()
	c.configCallbacks = make(map[string][]ConfigChangeCallback)
	c.configValues = make(map[string]*DynamicConfigResult)
	c.stateMutex.Unlock()
	
	// Close cache
	c.cache.Close()
	
	return nil
}

// initializeWebSocket initializes the WebSocket client for real-time updates
func (c *VariablyClient) initializeWebSocket() {
	if c.config.JWTToken == "" {
		c.logger.Warn("JWT token not provided, falling back to polling mode", nil)
		c.startPolling()
		return
	}
	
	c.wsClient = NewWebSocketClient(c.config.BaseURL, c.config.JWTToken, c.config.WebSocket, c.logger)
	
	// Set up event handlers
	c.wsClient.OnConnectionState(func(state ConnectionState) {
		c.logger.Debug("WebSocket connection state changed", map[string]interface{}{
			"state": string(state),
		})
		
		switch state {
		case Connected:
			c.isRealTimeMode = true
			c.fallbackMode = false
			c.stopPolling()
			
			// Subscribe to project-wide updates
			c.wsClient.Subscribe(c.config.ProjectID)
			
			// Subscribe to specific configs that have callbacks
			c.stateMutex.RLock()
			for configKey := range c.configCallbacks {
				if configKey != "*" {
					c.wsClient.Subscribe(c.config.ProjectID, configKey)
				}
			}
			c.stateMutex.RUnlock()
			
		case Disconnected, Error:
			c.isRealTimeMode = false
			
			if !c.fallbackMode {
				c.logger.Info("WebSocket disconnected, falling back to polling mode", nil)
				c.fallbackMode = true
				c.startPolling()
			}
		}
	})
	
	c.wsClient.OnConfigUpdate("*", func(result *DynamicConfigResult) {
		c.handleRealtimeConfigUpdate(result)
	})
	
	c.wsClient.OnError(func(err error) {
		c.logger.Error("WebSocket error", map[string]interface{}{
			"error": err.Error(),
		})
	})
	
	// Connect to WebSocket
	go func() {
		if err := c.wsClient.Connect(); err != nil {
			c.logger.Error("Failed to connect to WebSocket", map[string]interface{}{
				"error": err.Error(),
			})
			c.fallbackMode = true
			c.startPolling()
		}
	}()
}

// handleRealtimeConfigUpdate handles real-time configuration updates
func (c *VariablyClient) handleRealtimeConfigUpdate(result *DynamicConfigResult) {
	c.logger.Info("Real-time config update received", map[string]interface{}{
		"config_key": result.Key,
		"version":    result.Version,
	})
	
	// Update local state
	c.stateMutex.Lock()
	c.configValues[result.Key] = result
	c.stateMutex.Unlock()
	
	// Clear related cache entries
	c.cache.ClearByPattern(context.Background(), fmt.Sprintf("config:%s:*", result.Key))
	
	// Notify callbacks
	c.notifyConfigCallbacks(result.Key, result)
}

// notifyConfigCallbacks notifies configuration change callbacks
func (c *VariablyClient) notifyConfigCallbacks(configKey string, result *DynamicConfigResult) {
	c.stateMutex.RLock()
	
	// Get specific callbacks
	specificCallbacks := make([]ConfigChangeCallback, len(c.configCallbacks[configKey]))
	copy(specificCallbacks, c.configCallbacks[configKey])
	
	// Get wildcard callbacks
	wildcardCallbacks := make([]ConfigChangeCallback, len(c.configCallbacks["*"]))
	copy(wildcardCallbacks, c.configCallbacks["*"])
	
	c.stateMutex.RUnlock()
	
	// Notify specific callbacks
	for _, callback := range specificCallbacks {
		go func(cb ConfigChangeCallback) {
			defer func() {
				if r := recover(); r != nil {
					c.logger.Error("Config callback panic", map[string]interface{}{
						"config_key": configKey,
						"error":      r,
					})
				}
			}()
			cb(result)
		}(callback)
	}
	
	// Notify wildcard callbacks
	for _, callback := range wildcardCallbacks {
		go func(cb ConfigChangeCallback) {
			defer func() {
				if r := recover(); r != nil {
					c.logger.Error("Wildcard config callback panic", map[string]interface{}{
						"config_key": configKey,
						"error":      r,
					})
				}
			}()
			cb(result)
		}(callback)
	}
}

// startPolling starts polling mode for fallback
func (c *VariablyClient) startPolling() {
	if c.pollingTimer != nil {
		return
	}
	
	c.logger.Info("Starting polling mode", map[string]interface{}{
		"interval": c.config.PollingInterval,
	})
	
	c.pollingTimer = time.AfterFunc(c.config.PollingInterval, func() {
		c.pollConfigurations()
		c.startPolling() // Schedule next poll
	})
}

// stopPolling stops polling mode
func (c *VariablyClient) stopPolling() {
	if c.pollingTimer != nil {
		c.pollingTimer.Stop()
		c.pollingTimer = nil
		c.logger.Debug("Polling mode stopped", nil)
	}
}

// pollConfigurations polls for configuration changes (fallback mode)
func (c *VariablyClient) pollConfigurations() {
	c.stateMutex.RLock()
	configKeys := make([]string, 0, len(c.configValues))
	cachedResults := make(map[string]*DynamicConfigResult)
	for key, result := range c.configValues {
		configKeys = append(configKeys, key)
		cachedResults[key] = result
	}
	c.stateMutex.RUnlock()
	
	if len(configKeys) == 0 {
		return
	}
	
	c.logger.Debug("Polling configurations for changes", map[string]interface{}{
		"count": len(configKeys),
	})
	
	for _, configKey := range configKeys {
		cachedResult := cachedResults[configKey]
		
		// Use a generic user context for polling
		userContext := UserContext{UserID: "polling-user"}
		
		response, err := c.evaluateConfigViaAPI(context.Background(), configKey, userContext, cachedResult.ETag)
		if err != nil {
			c.logger.Error("Polling failed for config", map[string]interface{}{
				"config_key": configKey,
				"error":      err.Error(),
			})
			continue
		}
		
		if response != nil {
			// Configuration has changed
			var value interface{}
			if response.Value != nil {
				var parsed interface{}
				if err := json.Unmarshal(response.Value, &parsed); err == nil {
					value = parsed
				} else {
					value = string(response.Value)
				}
			}
			
			result := &DynamicConfigResult{
				Key:            configKey,
				Value:          value,
				Reason:         "polling_update",
				RuleID:         response.RuleID,
				ETag:           response.ETag,
				Version:        response.Version,
				CacheHit:       false,
				RealTimeUpdate: false,
				RetrievedAt:    time.Now(),
			}
			
			if response.UpdatedAt != nil && *response.UpdatedAt != "" {
				if updatedAt, err := time.Parse(time.RFC3339, *response.UpdatedAt); err == nil {
					result.UpdatedAt = &updatedAt
				}
			}
			
			c.stateMutex.Lock()
			c.configValues[configKey] = result
			c.stateMutex.Unlock()
			
			c.notifyConfigCallbacks(configKey, result)
		}
	}
}

// evaluateConfigViaAPI evaluates configuration via HTTP API with ETag support
func (c *VariablyClient) evaluateConfigViaAPI(ctx context.Context, configKey string, userContext UserContext, ifNoneMatch string) (*DynamicConfigEvaluationResponse, error) {
	request := DynamicConfigEvaluationRequest{
		ConfigKey: configKey,
		Context:   userContext,
	}
	
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, NewNetworkError("Failed to marshal request", 0, "", err)
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/api/v1/sdk/dynamic-configs/evaluate", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, NewNetworkError("Failed to create request", 0, "", err)
	}
	
	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.config.APIKey)
	
	// Add If-None-Match header if ETag is provided
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, NewNetworkError("HTTP request failed", 0, req.URL.String(), err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusNotModified {
		// Not Modified - return nil to indicate no change
		return nil, nil
	}
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, NewNetworkError(
			fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)),
			resp.StatusCode,
			req.URL.String(),
			nil,
		)
	}
	
	var response DynamicConfigEvaluationResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, NewNetworkError("Failed to decode response", resp.StatusCode, req.URL.String(), err)
	}
	
	return &response, nil
}

// generateCacheKey generates a cache key for configuration
func (c *VariablyClient) generateCacheKey(configKey string, userContext UserContext) string {
	contextHash := c.hashUserContext(userContext)
	return fmt.Sprintf("config:%s:%s", configKey, contextHash)
}

// hashUserContext generates a simple hash of user context for caching
func (c *VariablyClient) hashUserContext(userContext UserContext) string {
	// Create a simple hash based on user context
	contextData := fmt.Sprintf("%s:%s", userContext.UserID, c.serializeAttributes(userContext.Attributes))
	
	// Simple hash function
	hash := 0
	for _, char := range contextData {
		hash = ((hash << 5) - hash) + int(char)
		hash = hash & hash // Convert to 32-bit integer
	}
	
	return fmt.Sprintf("%x", hash)
}

// serializeAttributes serializes user attributes for hashing
func (c *VariablyClient) serializeAttributes(attributes map[string]interface{}) string {
	if attributes == nil || len(attributes) == 0 {
		return ""
	}
	
	data, err := json.Marshal(attributes)
	if err != nil {
		return ""
	}
	
	return string(data)
}

// ========== TRADITIONAL FEATURE FLAG API ==========

// EvaluateFlag evaluates a feature flag with full details
func (c *VariablyClient) EvaluateFlag(ctx context.Context, flagKey string, defaultValue interface{}, userContext UserContext) FlagResult {
	startTime := time.Now()
	c.metrics.RecordFlagEvaluation()

	// Serve from the project snapshot when we already hold one for this context: a
	// flag read on a request path should not cost a round trip.
	if c.flagSnapshots.enabled {
		if entry, ok := c.flagSnapshots.get(contextKey(userContext)); ok {
			if result, found := entry.flags[flagKey]; found {
				c.metrics.RecordCacheHit()
				result.CacheHit = true
				return result
			}
		}
	}

	result, err := c.evaluateFlagRemote(ctx, flagKey, userContext)
	c.metrics.RecordAPICall(time.Since(startTime), err)

	if err != nil {
		// Value is deliberately left nil. Returning defaultValue beside a non-nil
		// Error gives a caller who forgets to check Error a plausible answer that
		// Variably never served.
		return FlagResult{
			Key:         flagKey,
			Reason:      "evaluation_error",
			EvaluatedAt: time.Now(),
			Error:       err,
		}
	}

	// A flag the project does not define is a configuration error, surfaced on the
	// result so typed accessors can return it rather than a plausible default.
	if result.Reason == FlagNotFoundReason {
		result.Value = defaultValue
		result.Error = &FlagNotFoundError{FlagKey: flagKey}
		return result
	}
	if result.Value == nil {
		result.Value = defaultValue
	}
	return result
}

// EvaluateFlagBool evaluates a boolean feature flag
func (c *VariablyClient) EvaluateFlagBool(ctx context.Context, flagKey string, defaultValue bool, userContext UserContext) (bool, error) {
	result := c.EvaluateFlag(ctx, flagKey, defaultValue, userContext)
	if result.Error != nil {
		// Includes FlagNotFoundError: a missing flag must reach the caller rather
		// than arriving as an indistinguishable default.
		return defaultValue, result.Error
	}

	if val, ok := result.Value.(bool); ok {
		return val, nil
	}
	return defaultValue, nil
}

// EvaluateFlagString evaluates a string feature flag
func (c *VariablyClient) EvaluateFlagString(ctx context.Context, flagKey string, defaultValue string, userContext UserContext) (string, error) {
	result := c.EvaluateFlag(ctx, flagKey, defaultValue, userContext)
	if result.Error != nil {
		// Includes FlagNotFoundError: a missing flag must reach the caller rather
		// than arriving as an indistinguishable default.
		return defaultValue, result.Error
	}

	if val, ok := result.Value.(string); ok {
		return val, nil
	}
	return defaultValue, nil
}

// EvaluateFlagInt evaluates an integer feature flag
func (c *VariablyClient) EvaluateFlagInt(ctx context.Context, flagKey string, defaultValue int, userContext UserContext) (int, error) {
	result := c.EvaluateFlag(ctx, flagKey, defaultValue, userContext)
	if result.Error != nil {
		// Includes FlagNotFoundError: a missing flag must reach the caller rather
		// than arriving as an indistinguishable default.
		return defaultValue, result.Error
	}

	if val, ok := result.Value.(int); ok {
		return val, nil
	}
	return defaultValue, nil
}

// EvaluateFlagFloat evaluates a float feature flag
func (c *VariablyClient) EvaluateFlagFloat(ctx context.Context, flagKey string, defaultValue float64, userContext UserContext) (float64, error) {
	result := c.EvaluateFlag(ctx, flagKey, defaultValue, userContext)
	if result.Error != nil {
		// Includes FlagNotFoundError: a missing flag must reach the caller rather
		// than arriving as an indistinguishable default.
		return defaultValue, result.Error
	}

	if val, ok := result.Value.(float64); ok {
		return val, nil
	}
	return defaultValue, nil
}

// EvaluateFlagJSON evaluates a JSON feature flag
func (c *VariablyClient) EvaluateFlagJSON(ctx context.Context, flagKey string, defaultValue interface{}, userContext UserContext) (interface{}, error) {
	result := c.EvaluateFlag(ctx, flagKey, defaultValue, userContext)
	if result.Error != nil {
		return defaultValue, result.Error
	}
	return result.Value, nil
}

// EvaluateGate evaluates a feature gate (boolean flag)
func (c *VariablyClient) EvaluateGate(ctx context.Context, gateKey string, userContext UserContext) (bool, error) {
	c.metrics.RecordGateEvaluation()
	// Denying on a transport failure would be indistinguishable from the gate
	// actually being closed, so the error reaches the caller instead.
	return c.EvaluateFlagBool(ctx, gateKey, false, userContext)
}

// ========== BATCH OPERATIONS ==========

// EvaluateFlags evaluates multiple feature flags in a single call
func (c *VariablyClient) EvaluateFlags(ctx context.Context, flagKeys []string, userContext UserContext) map[string]FlagResult {
	results, err := c.evaluateFlagsRemote(ctx, flagKeys, userContext)
	if err != nil {
		// Every key carries the error and no value, so a caller who checks none of
		// them cannot mistake a transport failure for "every flag is off".
		results = make(map[string]FlagResult, len(flagKeys))
		for _, flagKey := range flagKeys {
			results[flagKey] = FlagResult{
				Key:         flagKey,
				Reason:      "evaluation_error",
				EvaluatedAt: time.Now(),
				Error:       err,
			}
		}
	}
	return results
}

// EvaluateGates evaluates multiple feature gates in a single call
func (c *VariablyClient) EvaluateGates(ctx context.Context, gateKeys []string, userContext UserContext) map[string]bool {
	results := make(map[string]bool)
	
	for _, gateKey := range gateKeys {
		value, err := c.EvaluateGate(ctx, gateKey, userContext)
		if err != nil {
			// Recorded rather than silently denied; the caller sees which gate failed.
			c.logger.Error("gate evaluation failed", map[string]interface{}{"gate_key": gateKey, "error": err.Error()})
		}
		results[gateKey] = value
	}
	
	return results
}

// ========== EVENT TRACKING ==========

// Track sends a single event for analytics
func (c *VariablyClient) Track(ctx context.Context, event Event) error {
	c.metrics.RecordEvent()
	
	// Convert event to API format
	eventData := map[string]interface{}{
		"event_name":  event.Name,
		"user_id":     event.UserID,
		"session_id":  event.SessionID,
		"properties":  event.Properties,
		"timestamp":   event.Timestamp.Format(time.RFC3339),
		"context":     event.Context,
	}
	
	requestBody, err := json.Marshal(eventData)
	if err != nil {
		return NewValidationError("Failed to marshal event", "event", err)
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/api/v1/sdk/events", bytes.NewBuffer(requestBody))
	if err != nil {
		return NewNetworkError("Failed to create request", 0, "", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.config.APIKey)
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return NewNetworkError("HTTP request failed", 0, req.URL.String(), err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return NewNetworkError(
			fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)),
			resp.StatusCode,
			req.URL.String(),
			nil,
		)
	}
	
	return nil
}

// TrackBatch sends multiple events in a single call
func (c *VariablyClient) TrackBatch(ctx context.Context, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	
	for range events {
		c.metrics.RecordEvent()
	}
	
	// Convert events to API format
	eventList := make([]map[string]interface{}, len(events))
	for i, event := range events {
		eventList[i] = map[string]interface{}{
			"event_name":  event.Name,
			"user_id":     event.UserID,
			"session_id":  event.SessionID,
			"properties":  event.Properties,
			"timestamp":   event.Timestamp.Format(time.RFC3339),
			"context":     event.Context,
		}
	}
	
	batchData := map[string]interface{}{
		"events": eventList,
	}
	
	requestBody, err := json.Marshal(batchData)
	if err != nil {
		return NewValidationError("Failed to marshal events", "events", err)
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/api/v1/sdk/events/batch", bytes.NewBuffer(requestBody))
	if err != nil {
		return NewNetworkError("Failed to create request", 0, "", err)
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.config.APIKey)
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return NewNetworkError("HTTP request failed", 0, req.URL.String(), err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return NewNetworkError(
			fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)),
			resp.StatusCode,
			req.URL.String(),
			nil,
		)
	}
	
	return nil
}

// ========== TRADITIONAL FLAG SUBSCRIPTIONS ==========

// Subscribe subscribes to real-time updates for feature flags
func (c *VariablyClient) Subscribe(ctx context.Context, flagKeys []string, callback UpdateCallback) error {
	c.flagSubMutex.Lock()
	defer c.flagSubMutex.Unlock()
	
	for _, flagKey := range flagKeys {
		if _, exists := c.flagSubscriptions[flagKey]; !exists {
			c.flagSubscriptions[flagKey] = make([]UpdateCallback, 0)
		}
		
		c.flagSubscriptions[flagKey] = append(c.flagSubscriptions[flagKey], callback)
		
		// Subscribe to WebSocket updates if available
		if c.wsClient != nil && c.isRealTimeMode {
			c.wsClient.Subscribe(c.config.ProjectID, flagKey)
		}
	}
	
	c.logger.Debug("Subscribed to flag updates", map[string]interface{}{
		"flag_keys": flagKeys,
	})
	
	return nil
}

// Unsubscribe removes subscriptions for feature flags
func (c *VariablyClient) Unsubscribe(flagKeys []string) error {
	c.flagSubMutex.Lock()
	defer c.flagSubMutex.Unlock()
	
	for _, flagKey := range flagKeys {
		delete(c.flagSubscriptions, flagKey)
		
		// Unsubscribe from WebSocket if no more callbacks
		if c.wsClient != nil {
			c.wsClient.Unsubscribe(c.config.ProjectID, flagKey)
		}
	}
	
	c.logger.Debug("Unsubscribed from flag updates", map[string]interface{}{
		"flag_keys": flagKeys,
	})
	
	return nil
}

// ========== CACHE MANAGEMENT ==========

// RefreshCache refreshes all cached values
func (c *VariablyClient) RefreshCache(ctx context.Context) error {
	c.cache.Clear(ctx)
	c.logger.Info("Cache refreshed", nil)
	return nil
}

// ClearCache clears all cached values
func (c *VariablyClient) ClearCache() error {
	c.cache.Clear(context.Background())
	c.logger.Info("Cache cleared", nil)
	return nil
}

// ========== METRICS ==========

// GetMetrics returns current SDK metrics
func (c *VariablyClient) GetMetrics() Metrics {
	return c.metrics.GetMetrics()
}
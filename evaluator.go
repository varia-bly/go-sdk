package variably

import (
	"context"
	"fmt"
	"time"
)

// Evaluator handles flag and gate evaluation with caching and fallback logic
type Evaluator struct {
	httpClient   *HTTPClient
	cacheManager *CacheManager
	metrics      *MetricsCollector
	logger       Logger
	config       *Config
}

// NewEvaluator creates a new evaluator instance
func NewEvaluator(httpClient *HTTPClient, cacheManager *CacheManager, metrics *MetricsCollector, logger Logger, config *Config) *Evaluator {
	return &Evaluator{
		httpClient:   httpClient,
		cacheManager: cacheManager,
		metrics:      metrics,
		logger:       logger,
		config:       config,
	}
}

// EvaluateFlag evaluates a single feature flag with caching and fallback
func (e *Evaluator) EvaluateFlag(ctx context.Context, flagKey string, defaultValue interface{}, userContext UserContext) FlagResult {
	e.metrics.RecordFlagEvaluation()
	
	// Ensure user context has timestamp
	if userContext.Timestamp.IsZero() {
		userContext.Timestamp = time.Now()
	}

	// Generate cache key
	cacheKey := e.generateCacheKey(flagKey, userContext)

	// Try cache first
	if cachedResult, found := e.cacheManager.Get(cacheKey); found {
		e.metrics.RecordCacheHit()
		e.logger.Debug("Flag evaluation cache hit", "flag_key", flagKey, "user_id", userContext.UserID)
		cachedResult.CacheHit = true
		return cachedResult
	}

	e.metrics.RecordCacheMiss()

	// Cache miss, evaluate via API
	result := e.evaluateFlagFromAPI(ctx, flagKey, defaultValue, userContext)

	// Cache the result if successful
	if result.Error == nil {
		e.cacheManager.Set(cacheKey, result, 0) // Use default TTL
	}

	return result
}

// EvaluateFlags evaluates multiple feature flags in batch
func (e *Evaluator) EvaluateFlags(ctx context.Context, flagKeys []string, userContext UserContext) map[string]FlagResult {
	results := make(map[string]FlagResult)

	// Ensure user context has timestamp
	if userContext.Timestamp.IsZero() {
		userContext.Timestamp = time.Now()
	}

	// Check cache for each flag
	var uncachedFlags []string
	for _, flagKey := range flagKeys {
		e.metrics.RecordFlagEvaluation()
		cacheKey := e.generateCacheKey(flagKey, userContext)
		
		if cachedResult, found := e.cacheManager.Get(cacheKey); found {
			e.metrics.RecordCacheHit()
			e.logger.Debug("Flag evaluation cache hit", "flag_key", flagKey, "user_id", userContext.UserID)
			cachedResult.CacheHit = true
			results[flagKey] = cachedResult
		} else {
			e.metrics.RecordCacheMiss()
			uncachedFlags = append(uncachedFlags, flagKey)
		}
	}

	// If all flags were cached, return results
	if len(uncachedFlags) == 0 {
		return results
	}

	// Evaluate uncached flags via batch API
	batchResults := e.evaluateFlagsFromAPI(ctx, uncachedFlags, userContext)

	// Merge batch results and cache them
	for flagKey, result := range batchResults {
		results[flagKey] = result
		if result.Error == nil {
			cacheKey := e.generateCacheKey(flagKey, userContext)
			e.cacheManager.Set(cacheKey, result, 0)
		}
	}

	return results
}

// EvaluateGate evaluates a single feature gate
func (e *Evaluator) EvaluateGate(ctx context.Context, gateKey string, userContext UserContext) bool {
	e.metrics.RecordGateEvaluation()

	// Ensure user context has timestamp
	if userContext.Timestamp.IsZero() {
		userContext.Timestamp = time.Now()
	}

	// Generate cache key for gate
	cacheKey := e.generateGateCacheKey(gateKey, userContext)

	// Try cache first
	if cachedResult, found := e.cacheManager.Get(cacheKey); found {
		e.metrics.RecordCacheHit()
		e.logger.Debug("Gate evaluation cache hit", "gate_key", gateKey, "user_id", userContext.UserID)
		if value, ok := cachedResult.Value.(bool); ok {
			return value
		}
	}

	e.metrics.RecordCacheMiss()

	// Cache miss, evaluate via API
	response, err := e.httpClient.EvaluateGate(ctx, gateKey, userContext, e.config.Environment)
	if err != nil {
		e.logger.Error("Failed to evaluate gate", "gate_key", gateKey, "error", err)
		return false // Default to false for gates
	}

	// Cache the result
	result := FlagResult{
		Key:         gateKey,
		Value:       response.Enabled,
		Reason:      "api_evaluation",
		EvaluatedAt: time.Now(),
		CacheHit:    false,
	}
	e.cacheManager.Set(cacheKey, result, 0)

	e.logger.Debug("Gate evaluation successful", "gate_key", gateKey, "enabled", response.Enabled)
	return response.Enabled
}

// EvaluateGates evaluates multiple feature gates in batch
func (e *Evaluator) EvaluateGates(ctx context.Context, gateKeys []string, userContext UserContext) map[string]bool {
	results := make(map[string]bool)

	// Ensure user context has timestamp
	if userContext.Timestamp.IsZero() {
		userContext.Timestamp = time.Now()
	}

	// Check cache for each gate
	var uncachedGates []string
	for _, gateKey := range gateKeys {
		e.metrics.RecordGateEvaluation()
		cacheKey := e.generateGateCacheKey(gateKey, userContext)
		
		if cachedResult, found := e.cacheManager.Get(cacheKey); found {
			e.metrics.RecordCacheHit()
			e.logger.Debug("Gate evaluation cache hit", "gate_key", gateKey, "user_id", userContext.UserID)
			if value, ok := cachedResult.Value.(bool); ok {
				results[gateKey] = value
			} else {
				results[gateKey] = false
			}
		} else {
			e.metrics.RecordCacheMiss()
			uncachedGates = append(uncachedGates, gateKey)
		}
	}

	// If all gates were cached, return results
	if len(uncachedGates) == 0 {
		return results
	}

	// Evaluate uncached gates via batch API
	response, err := e.httpClient.EvaluateGates(ctx, uncachedGates, userContext, e.config.Environment)
	if err != nil {
		e.logger.Error("Failed to evaluate gates batch", "error", err)
		// Set defaults for uncached gates
		for _, gateKey := range uncachedGates {
			results[gateKey] = false
		}
		return results
	}

	// Process batch results and cache them
	for gateKey, gateResult := range response.Results {
		results[gateKey] = gateResult.Enabled
		
		// Cache the result
		result := FlagResult{
			Key:         gateKey,
			Value:       gateResult.Enabled,
			Reason:      "api_evaluation",
			EvaluatedAt: time.Now(),
			CacheHit:    false,
		}
		cacheKey := e.generateGateCacheKey(gateKey, userContext)
		e.cacheManager.Set(cacheKey, result, 0)
	}

	return results
}

// evaluateFlagFromAPI evaluates a single flag via API with fallback handling
func (e *Evaluator) evaluateFlagFromAPI(ctx context.Context, flagKey string, defaultValue interface{}, userContext UserContext) FlagResult {
	response, err := e.httpClient.EvaluateFlag(ctx, flagKey, userContext, e.config.Environment)
	if err != nil {
		e.logger.Error("Failed to evaluate flag", "flag_key", flagKey, "error", err)
		
		// Return default value with error
		return FlagResult{
			Key:         flagKey,
			Value:       defaultValue,
			Reason:      "error_fallback",
			Error:       err,
			EvaluatedAt: time.Now(),
			CacheHit:    false,
		}
	}

	e.logger.Debug("Flag evaluation successful", "flag_key", flagKey, "enabled", response.Enabled)

	return FlagResult{
		Key:         flagKey,
		Value:       response.Enabled,
		Reason:      "api_evaluation",
		EvaluatedAt: time.Now(),
		CacheHit:    false,
	}
}

// evaluateFlagsFromAPI evaluates multiple flags via batch API with fallback handling
func (e *Evaluator) evaluateFlagsFromAPI(ctx context.Context, flagKeys []string, userContext UserContext) map[string]FlagResult {
	results := make(map[string]FlagResult)

	response, err := e.httpClient.EvaluateFlags(ctx, flagKeys, userContext, e.config.Environment)
	if err != nil {
		e.logger.Error("Failed to evaluate flags batch", "error", err)
		
		// Return defaults for all flags with error
		for _, flagKey := range flagKeys {
			results[flagKey] = FlagResult{
				Key:         flagKey,
				Value:       nil, // No default value available in batch
				Reason:      "error_fallback",
				Error:       err,
				EvaluatedAt: time.Now(),
				CacheHit:    false,
			}
		}
		return results
	}

	// Process successful results
	for flagKey, flagResult := range response.Results {
		results[flagKey] = FlagResult{
			Key:         flagKey,
			Value:       flagResult.Enabled,
			Reason:      "api_evaluation",
			RuleID:      "",
			Variation:   "",
			EvaluatedAt: time.Now(),
			CacheHit:    false,
		}
	}

	// Add missing flags as errors (shouldn't happen with a good API)
	for _, flagKey := range flagKeys {
		if _, exists := results[flagKey]; !exists {
			results[flagKey] = FlagResult{
				Key:         flagKey,
				Value:       nil,
				Reason:      "not_found",
				Error:       fmt.Errorf("flag not found in response"),
				EvaluatedAt: time.Now(),
				CacheHit:    false,
			}
		}
	}

	return results
}

// generateCacheKey creates a cache key for flag evaluation
func (e *Evaluator) generateCacheKey(flagKey string, userContext UserContext) string {
	// Create a simple cache key based on flag and user ID
	// In production, you might want to include more context attributes
	// that affect targeting rules
	return fmt.Sprintf("flag:%s:user:%s:env:%s", flagKey, userContext.UserID, e.config.Environment)
}

// generateGateCacheKey creates a cache key for gate evaluation
func (e *Evaluator) generateGateCacheKey(gateKey string, userContext UserContext) string {
	return fmt.Sprintf("gate:%s:user:%s:env:%s", gateKey, userContext.UserID, e.config.Environment)
}

// RefreshCache clears all cached values to force fresh evaluation
func (e *Evaluator) RefreshCache(ctx context.Context) error {
	e.cacheManager.Clear()
	e.logger.Info("Cache refreshed - all cached values cleared")
	return nil
}

// ClearCache alias for RefreshCache for backward compatibility
func (e *Evaluator) ClearCache() error {
	return e.RefreshCache(context.Background())
}
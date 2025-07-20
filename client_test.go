package variably

import (
	"context"
	"testing"
	"time"
)

func TestMockClient(t *testing.T) {
	client := NewMockClient()
	defer client.Close()

	// Test user context
	user := UserContext{
		UserID:  "test_user",
		Email:   "test@example.com",
		Country: "US",
	}

	// Test boolean flag evaluation
	t.Run("Boolean Flag", func(t *testing.T) {
		client.SetFlagValue("test_flag", true)
		
		result := client.EvaluateFlagBool(context.Background(), "test_flag", false, user)
		if !result {
			t.Errorf("Expected true, got %v", result)
		}
		
		// Test default value for non-existent flag
		result = client.EvaluateFlagBool(context.Background(), "non_existent", false, user)
		if result {
			t.Errorf("Expected false (default), got %v", result)
		}
	})

	// Test string flag evaluation
	t.Run("String Flag", func(t *testing.T) {
		client.SetFlagValue("theme", "dark")
		
		result := client.EvaluateFlagString(context.Background(), "theme", "light", user)
		if result != "dark" {
			t.Errorf("Expected 'dark', got %v", result)
		}
	})

	// Test integer flag evaluation
	t.Run("Integer Flag", func(t *testing.T) {
		client.SetFlagValue("max_items", 20)
		
		result := client.EvaluateFlagInt(context.Background(), "max_items", 10, user)
		if result != 20 {
			t.Errorf("Expected 20, got %v", result)
		}
	})

	// Test feature gate evaluation
	t.Run("Feature Gate", func(t *testing.T) {
		client.SetGateValue("premium_features", true)
		
		result := client.EvaluateGate(context.Background(), "premium_features", user)
		if !result {
			t.Errorf("Expected true, got %v", result)
		}
		
		// Test default value for non-existent gate
		result = client.EvaluateGate(context.Background(), "non_existent_gate", user)
		if result {
			t.Errorf("Expected false (default), got %v", result)
		}
	})

	// Test batch operations
	t.Run("Batch Evaluation", func(t *testing.T) {
		client.SetFlagValue("flag_a", "value_a")
		client.SetFlagValue("flag_b", true)
		
		results := client.EvaluateFlags(context.Background(), []string{"flag_a", "flag_b", "flag_c"}, user)
		
		if len(results) != 3 {
			t.Errorf("Expected 3 results, got %d", len(results))
		}
		
		if results["flag_a"].Value != "value_a" {
			t.Errorf("Expected 'value_a', got %v", results["flag_a"].Value)
		}
		
		if results["flag_b"].Value != true {
			t.Errorf("Expected true, got %v", results["flag_b"].Value)
		}
		
		// flag_c should return nil as default since it's not set
		if results["flag_c"].Value != nil {
			t.Errorf("Expected nil (default), got %v", results["flag_c"].Value)
		}
	})

	// Test event tracking
	t.Run("Event Tracking", func(t *testing.T) {
		event := Event{
			Name:   "test_event",
			UserID: "test_user",
			Properties: map[string]interface{}{
				"action": "click",
				"value":  42,
			},
		}
		
		err := client.Track(context.Background(), event)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		
		events := client.GetTrackedEvents()
		if len(events) != 1 {
			t.Errorf("Expected 1 tracked event, got %d", len(events))
		}
		
		if events[0].Name != "test_event" {
			t.Errorf("Expected event name 'test_event', got %v", events[0].Name)
		}
		
		// Test batch tracking
		batchEvents := []Event{
			{Name: "event1", UserID: "user1"},
			{Name: "event2", UserID: "user2"},
		}
		
		err = client.TrackBatch(context.Background(), batchEvents)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		
		allEvents := client.GetTrackedEvents()
		if len(allEvents) != 3 {
			t.Errorf("Expected 3 total tracked events, got %d", len(allEvents))
		}
	})

	// Test metrics
	t.Run("Metrics", func(t *testing.T) {
		metrics := client.GetMetrics()
		
		if metrics.FlagsEvaluated == 0 {
			t.Error("Expected some flag evaluations to be recorded")
		}
		
		if metrics.EventsTracked == 0 {
			t.Error("Expected some events to be recorded")
		}
	})
}

func TestConfig(t *testing.T) {
	t.Run("Default Config", func(t *testing.T) {
		config := DefaultConfig()
		
		if config.BaseURL == "" {
			t.Error("Expected default base URL to be set")
		}
		
		if config.Environment == "" {
			t.Error("Expected default environment to be set")
		}
		
		if config.Timeout == 0 {
			t.Error("Expected default timeout to be set")
		}
		
		if config.CacheConfig.TTL == 0 {
			t.Error("Expected default cache TTL to be set")
		}
	})

	t.Run("Config Validation", func(t *testing.T) {
		// Valid config
		config := &Config{
			APIKey:      "test-key",
			BaseURL:     "https://api.example.com",
			Environment: "test",
			Timeout:     5 * time.Second,
		}
		
		err := config.Validate()
		if err != nil {
			t.Errorf("Expected valid config to pass validation, got error: %v", err)
		}
		
		// Invalid config - missing API key
		invalidConfig := &Config{
			BaseURL:     "https://api.example.com",
			Environment: "test",
			Timeout:     5 * time.Second,
		}
		
		err = invalidConfig.Validate()
		if err == nil {
			t.Error("Expected invalid config to fail validation")
		}
	})
}

func TestCache(t *testing.T) {
	cache := NewMemoryCache(100, 5*time.Minute)
	
	// Test basic operations
	t.Run("Basic Operations", func(t *testing.T) {
		// Set and get
		result := FlagResult{
			Key:   "test_flag",
			Value: true,
		}
		
		cache.Set("test_key", result, time.Minute)
		
		retrieved, found := cache.Get("test_key")
		if !found {
			t.Error("Expected to find cached item")
		}
		
		if retrieved.Key != "test_flag" {
			t.Errorf("Expected key 'test_flag', got %v", retrieved.Key)
		}
		
		if retrieved.Value != true {
			t.Errorf("Expected value true, got %v", retrieved.Value)
		}
		
		// Test non-existent key
		_, found = cache.Get("non_existent")
		if found {
			t.Error("Expected not to find non-existent key")
		}
	})

	t.Run("Size Management", func(t *testing.T) {
		if cache.Size() == 0 {
			t.Error("Expected cache to have items from previous test")
		}
		
		cache.Clear()
		if cache.Size() != 0 {
			t.Error("Expected cache to be empty after clear")
		}
	})
}

func TestMetrics(t *testing.T) {
	metrics := NewMetricsCollector()
	
	// Record some operations
	metrics.RecordAPICall(100*time.Millisecond, true)
	metrics.RecordAPICall(200*time.Millisecond, false)
	metrics.RecordCacheHit()
	metrics.RecordCacheMiss()
	metrics.RecordFlagEvaluation()
	metrics.RecordEventTracked()
	
	summary := metrics.GetMetrics()
	
	if summary.APICalls != 2 {
		t.Errorf("Expected 2 API calls, got %d", summary.APICalls)
	}
	
	if summary.ErrorCount != 1 {
		t.Errorf("Expected 1 error, got %d", summary.ErrorCount)
	}
	
	if summary.CacheHits != 1 {
		t.Errorf("Expected 1 cache hit, got %d", summary.CacheHits)
	}
	
	if summary.CacheMisses != 1 {
		t.Errorf("Expected 1 cache miss, got %d", summary.CacheMisses)
	}
	
	if summary.FlagsEvaluated != 1 {
		t.Errorf("Expected 1 flag evaluation, got %d", summary.FlagsEvaluated)
	}
	
	if summary.EventsTracked != 1 {
		t.Errorf("Expected 1 event tracked, got %d", summary.EventsTracked)
	}
	
	if summary.ErrorRate != 50.0 {
		t.Errorf("Expected 50%% error rate, got %.2f%%", summary.ErrorRate)
	}
	
	if summary.CacheHitRate != 50.0 {
		t.Errorf("Expected 50%% cache hit rate, got %.2f%%", summary.CacheHitRate)
	}
}
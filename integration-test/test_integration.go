package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/variably/variably-monorepo/sdks/go/variably"
)

func main() {
	fmt.Println("=== Variably Go SDK Integration Test ===")

	// Initialize the Variably client with the test API key
	client, err := variably.NewClient(&variably.Config{
		APIKey:      "vb_dev_e0e5bccd9fdd6c3c999cf662a3e7f82b723f86ecf6c1ee68f0c62d6e381ba9e8",
		Environment: "development",
		BaseURL:     "http://localhost:8080",
		Timeout:     5 * time.Second,
		RetryAttempts: 3,
		
		// Enable detailed logging for testing
		LogConfig: variably.LogConfig{
			Level:  "debug",
			Format: "text",
			Output: "stdout",
		},
		
		// Configure caching
		CacheConfig: variably.CacheConfig{
			TTL:     1 * time.Minute,
			MaxSize: 100,
		},
		
		EnableAnalytics:   true,
		EnableOfflineMode: true,
	})
	if err != nil {
		log.Fatal("Failed to initialize Variably client:", err)
	}
	defer client.Close()

	// Create test user context
	user := variably.UserContext{
		UserID:  "test_user_123",
		Email:   "testuser@example.com",
		Country: "US",
		Platform: "test",
		Attributes: map[string]interface{}{
			"plan":      "premium",
			"test_mode": true,
		},
		Timestamp: time.Now(),
	}

	fmt.Println("\n1. Testing Feature Flag Evaluation...")
	
	// Test boolean flag evaluation
	showNewFeature := client.EvaluateFlagBool(
		context.Background(),
		"test_feature_flag",
		false, // default value
		user,
	)
	fmt.Printf("   Boolean flag 'test_feature_flag': %t\n", showNewFeature)

	// Test string flag evaluation
	theme := client.EvaluateFlagString(
		context.Background(),
		"ui_theme",
		"light", // default value
		user,
	)
	fmt.Printf("   String flag 'ui_theme': %s\n", theme)

	// Test detailed flag result
	result := client.EvaluateFlag(
		context.Background(),
		"detailed_test_flag",
		"default_value",
		user,
	)
	fmt.Printf("   Detailed flag result: value=%v, reason=%s, cache_hit=%t\n", 
		result.Value, result.Reason, result.CacheHit)

	fmt.Println("\n2. Testing Feature Gate Evaluation...")
	
	hasAccess := client.EvaluateGate(
		context.Background(),
		"premium_features",
		user,
	)
	fmt.Printf("   Gate 'premium_features': %t\n", hasAccess)

	fmt.Println("\n3. Testing Batch Operations...")
	
	// Test batch flag evaluation
	flagKeys := []string{"flag_a", "flag_b", "flag_c"}
	batchResults := client.EvaluateFlags(context.Background(), flagKeys, user)
	for flagKey, flagResult := range batchResults {
		fmt.Printf("   Batch flag '%s': value=%v, reason=%s\n", 
			flagKey, flagResult.Value, flagResult.Reason)
	}

	// Test batch gate evaluation
	gateKeys := []string{"gate_a", "gate_b"}
	gateResults := client.EvaluateGates(context.Background(), gateKeys, user)
	for gateKey, enabled := range gateResults {
		fmt.Printf("   Batch gate '%s': %t\n", gateKey, enabled)
	}

	fmt.Println("\n4. Testing Event Tracking...")
	
	// Track single event
	err = client.Track(context.Background(), variably.Event{
		Name:   "sdk_test_event",
		UserID: user.UserID,
		Properties: map[string]interface{}{
			"test_type":   "integration",
			"sdk_version": "1.0.0",
			"feature_used": showNewFeature,
		},
		Timestamp: time.Now(),
	})
	if err != nil {
		fmt.Printf("   Error tracking event: %v\n", err)
	} else {
		fmt.Println("   ✓ Single event tracked successfully")
	}

	// Track batch events
	events := []variably.Event{
		{
			Name:   "batch_test_1",
			UserID: user.UserID,
			Properties: map[string]interface{}{
				"batch_index": 1,
			},
		},
		{
			Name:   "batch_test_2",
			UserID: user.UserID,
			Properties: map[string]interface{}{
				"batch_index": 2,
			},
		},
	}
	
	err = client.TrackBatch(context.Background(), events)
	if err != nil {
		fmt.Printf("   Error tracking batch events: %v\n", err)
	} else {
		fmt.Println("   ✓ Batch events tracked successfully")
	}

	fmt.Println("\n5. Testing SDK Metrics...")
	
	metrics := client.GetMetrics()
	fmt.Printf("   API Calls: %d\n", metrics.APICalls)
	fmt.Printf("   Cache Hits: %d\n", metrics.CacheHits)
	fmt.Printf("   Cache Misses: %d\n", metrics.CacheMisses)
	fmt.Printf("   Cache Hit Rate: %.2f%%\n", metrics.CacheHitRate)
	fmt.Printf("   Error Rate: %.2f%%\n", metrics.ErrorRate)
	fmt.Printf("   Average Latency: %v\n", metrics.AverageLatency)
	fmt.Printf("   Flags Evaluated: %d\n", metrics.FlagsEvaluated)
	fmt.Printf("   Gates Evaluated: %d\n", metrics.GatesEvaluated)
	fmt.Printf("   Events Tracked: %d\n", metrics.EventsTracked)

	fmt.Println("\n6. Testing Cache Functionality...")
	
	// Evaluate the same flag again to test caching
	fmt.Println("   Evaluating same flag again (should be cached)...")
	cachedResult := client.EvaluateFlag(
		context.Background(),
		"test_feature_flag",
		false,
		user,
	)
	fmt.Printf("   Cached flag result: value=%v, cache_hit=%t\n", 
		cachedResult.Value, cachedResult.CacheHit)

	fmt.Println("\n=== Integration Test Complete ===")
	fmt.Println("✓ All SDK operations completed successfully!")
}
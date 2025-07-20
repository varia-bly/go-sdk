package main

import (
	"context"
	"log"
	"time"

	"github.com/variably/variably-monorepo/sdks/go/variably"
)

func main() {
	// Initialize the Variably client
	client, err := variably.NewClient(&variably.Config{
		APIKey:      "your-api-key-here",
		Environment: "production",
		BaseURL:     "http://localhost:8080", // Point to local development server
		
		// Enable caching for better performance
		CacheConfig: variably.CacheConfig{
			TTL:               5 * time.Minute,
			MaxSize:           1000,
			EnablePersistence: true,
			PersistencePath:   "/tmp/variably-cache",
		},
		
		// Enable analytics
		EnableAnalytics:   true,
		EnableOfflineMode: true,
		
		// Configure logging
		LogConfig: variably.LogConfig{
			Level:  "info",
			Format: "text",
			Output: "stdout",
		},
	})
	if err != nil {
		log.Fatal("Failed to initialize Variably client:", err)
	}
	defer client.Close()

	// Create user context for targeting
	user := variably.UserContext{
		UserID:  "user_12345",
		Email:   "user@example.com",
		Country: "US",
		Platform: "web",
		Attributes: map[string]interface{}{
			"plan":        "premium",
			"signup_date": "2023-01-15",
			"beta_user":   true,
		},
		Timestamp: time.Now(),
	}

	// Example 1: Evaluate a boolean feature flag
	log.Println("=== Boolean Feature Flag ===")
	showNewUI := client.EvaluateFlagBool(
		context.Background(),
		"new_dashboard_ui",
		false, // default value
		user,
	)
	log.Printf("Show new dashboard UI: %t", showNewUI)

	// Example 2: Evaluate a string feature flag
	log.Println("\n=== String Feature Flag ===")
	theme := client.EvaluateFlagString(
		context.Background(),
		"ui_theme",
		"light", // default value
		user,
	)
	log.Printf("UI Theme: %s", theme)

	// Example 3: Evaluate a numeric feature flag
	log.Println("\n=== Numeric Feature Flag ===")
	maxItems := client.EvaluateFlagInt(
		context.Background(),
		"max_items_per_page",
		10, // default value
		user,
	)
	log.Printf("Max items per page: %d", maxItems)

	// Example 4: Evaluate a feature gate
	log.Println("\n=== Feature Gate ===")
	hasAccess := client.EvaluateGate(
		context.Background(),
		"premium_features",
		user,
	)
	log.Printf("Has access to premium features: %t", hasAccess)

	// Example 5: Batch evaluation for performance
	log.Println("\n=== Batch Flag Evaluation ===")
	flagKeys := []string{"feature_a", "feature_b", "feature_c"}
	results := client.EvaluateFlags(context.Background(), flagKeys, user)
	
	for flagKey, result := range results {
		if result.Error != nil {
			log.Printf("Error evaluating %s: %v", flagKey, result.Error)
			continue
		}
		log.Printf("Flag %s = %v (reason: %s, cache_hit: %t)", 
			flagKey, result.Value, result.Reason, result.CacheHit)
	}

	// Example 6: Batch gate evaluation
	log.Println("\n=== Batch Gate Evaluation ===")
	gateKeys := []string{"beta_features", "admin_panel", "experimental_ai"}
	gateResults := client.EvaluateGates(context.Background(), gateKeys, user)
	
	for gateKey, enabled := range gateResults {
		log.Printf("Gate %s = %t", gateKey, enabled)
	}

	// Example 7: Track user events for analytics
	log.Println("\n=== Event Tracking ===")
	err = client.Track(context.Background(), variably.Event{
		Name:   "dashboard_viewed",
		UserID: user.UserID,
		Properties: map[string]interface{}{
			"ui_version":     map[bool]string{true: "new", false: "legacy"}[showNewUI],
			"theme":          theme,
			"max_items":      maxItems,
			"has_premium":    hasAccess,
			"view_duration":  "30s",
		},
		Timestamp: time.Now(),
	})
	if err != nil {
		log.Printf("Failed to track event: %v", err)
	} else {
		log.Println("Event tracked successfully")
	}

	// Example 8: Get detailed flag result with metadata
	log.Println("\n=== Detailed Flag Result ===")
	result := client.EvaluateFlag(
		context.Background(),
		"experiment_variant",
		"control",
		user,
	)
	
	if result.Error != nil {
		log.Printf("Error: %v", result.Error)
	} else {
		log.Printf("Flag: %s", result.Key)
		log.Printf("Value: %v", result.Value)
		log.Printf("Reason: %s", result.Reason)
		log.Printf("Rule ID: %s", result.RuleID)
		log.Printf("Variation: %s", result.Variation)
		log.Printf("Cache Hit: %t", result.CacheHit)
		log.Printf("Evaluated At: %s", result.EvaluatedAt.Format(time.RFC3339))
	}

	// Example 9: Check SDK metrics
	log.Println("\n=== SDK Metrics ===")
	metrics := client.GetMetrics()
	log.Printf("API Calls: %d", metrics.APICalls)
	log.Printf("Cache Hits: %d", metrics.CacheHits)
	log.Printf("Cache Misses: %d", metrics.CacheMisses)
	log.Printf("Cache Hit Rate: %.2f%%", metrics.CacheHitRate)
	log.Printf("Error Rate: %.2f%%", metrics.ErrorRate)
	log.Printf("Average Latency: %v", metrics.AverageLatency)
	log.Printf("Flags Evaluated: %d", metrics.FlagsEvaluated)
	log.Printf("Gates Evaluated: %d", metrics.GatesEvaluated)
	log.Printf("Events Tracked: %d", metrics.EventsTracked)

	log.Println("\n=== Example Complete ===")
}
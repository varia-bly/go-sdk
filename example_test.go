package variably_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/varia-bly/go-sdk"
)

// Example demonstrates basic usage of the Variably Go SDK
func Example() {
	// Create a new client
	client, err := variably.NewClient(variably.ClientConfig{
		APIKey:         "your-api-key",
		JWTToken:       "your-jwt-token", // Optional, required for real-time updates
		BaseURL:        "https://api.variably.com",
		ProjectID:      "your-project-id",
		EnableRealtime: true,
		Debug:          false,
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create user context
	userContext := variably.UserContext{
		UserID:   "user123",
		Email:    "user@example.com",
		Country:  "US",
		Platform: "web",
		Attributes: map[string]interface{}{
			"subscription_tier": "premium",
			"signup_date":       "2023-01-15",
		},
	}

	ctx := context.Background()

	// Get different types of configurations
	featureEnabled, err := client.GetConfigBool(ctx, "new_feature_enabled", false, userContext)
	if err != nil {
		log.Printf("Error getting boolean config: %v", err)
	}
	fmt.Printf("Feature enabled: %v\n", featureEnabled)

	maxRetries, err := client.GetConfigNumber(ctx, "max_retries", 3, userContext)
	if err != nil {
		log.Printf("Error getting number config: %v", err)
	}
	fmt.Printf("Max retries: %.0f\n", maxRetries)

	welcomeMessage, err := client.GetConfigString(ctx, "welcome_message", "Welcome!", userContext)
	if err != nil {
		log.Printf("Error getting string config: %v", err)
	}
	fmt.Printf("Welcome message: %s\n", welcomeMessage)

	// Get JSON configuration
	uiConfig, err := client.GetConfigJSON(ctx, "ui_config", map[string]interface{}{
		"theme": "light",
		"layout": "default",
	}, userContext)
	if err != nil {
		log.Printf("Error getting JSON config: %v", err)
	}
	fmt.Printf("UI config: %+v\n", uiConfig)

	// Subscribe to real-time updates
	unsubscribe := client.OnConfigChange("new_feature_enabled", func(result *variably.DynamicConfigResult) {
		fmt.Printf("🔄 Config '%s' updated to: %v (reason: %s)\n", 
			result.Key, result.Value, result.Reason)
		
		if result.RealTimeUpdate {
			fmt.Println("✅ Received via real-time WebSocket")
		} else {
			fmt.Println("📊 Received via polling fallback")
		}
	})
	defer unsubscribe()

	// Subscribe to all configuration changes
	unsubscribeAll := client.OnAnyConfigChange(func(result *variably.DynamicConfigResult) {
		fmt.Printf("🌟 Any config updated: %s = %v\n", result.Key, result.Value)
	})
	defer unsubscribeAll()

	// Check connection status
	status := client.GetConnectionStatus()
	fmt.Printf("Connection mode: %s, connected: %v, fallback: %v\n", 
		status.Mode, status.Connected, status.FallbackActive)

	// Keep the example running to receive updates
	time.Sleep(5 * time.Second)

	// Output:
	// Feature enabled: true
	// Max retries: 5
	// Welcome message: Hello Premium User!
	// UI config: map[layout:premium theme:dark]
	// Connection mode: realtime, connected: true, fallback: false
}

// ExampleVariablyClient_EvaluateConfig demonstrates detailed configuration evaluation
func ExampleVariablyClient_EvaluateConfig() {
	client, err := variably.NewClient(variably.ClientConfig{
		APIKey:    "your-api-key",
		ProjectID: "your-project-id",
		BaseURL:   "https://api.variably.com",
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	userContext := variably.UserContext{
		UserID: "user456",
		Attributes: map[string]interface{}{
			"beta_tester": true,
		},
	}

	// Get detailed evaluation information
	result, err := client.EvaluateConfig(context.Background(), "beta_features", false, userContext)
	if err != nil {
		log.Fatalf("Failed to evaluate config: %v", err)
	}

	fmt.Printf("Config: %s\n", result.Key)
	fmt.Printf("Value: %v\n", result.Value)
	fmt.Printf("Reason: %s\n", result.Reason)
	fmt.Printf("Cache Hit: %v\n", result.CacheHit)
	fmt.Printf("Real-time Update: %v\n", result.RealTimeUpdate)
	fmt.Printf("Retrieved At: %v\n", result.RetrievedAt)

	if result.RuleID != nil {
		fmt.Printf("Rule ID: %s\n", *result.RuleID)
	}
	if result.Version != nil {
		fmt.Printf("Version: %d\n", *result.Version)
	}
	if result.UpdatedAt != nil {
		fmt.Printf("Updated At: %v\n", *result.UpdatedAt)
	}

	// Output:
	// Config: beta_features
	// Value: true
	// Reason: rule_match
	// Cache Hit: false
	// Real-time Update: false
	// Retrieved At: 2023-12-07 10:30:45 +0000 UTC
	// Rule ID: rule_beta_testers
	// Version: 42
	// Updated At: 2023-12-07 09:15:30 +0000 UTC
}

// ExampleVariablyClient_RefreshConfigs demonstrates cache refresh
func ExampleVariablyClient_RefreshConfigs() {
	client, err := variably.NewClient(variably.ClientConfig{
		APIKey:    "your-api-key",
		ProjectID: "your-project-id",
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	userContext := variably.UserContext{UserID: "user789"}
	ctx := context.Background()

	// Get some configurations (they will be cached)
	_, _ = client.GetConfigBool(ctx, "feature_a", false, userContext)
	_, _ = client.GetConfigString(ctx, "feature_b", "default", userContext)

	// Later, refresh all cached configurations
	err = client.RefreshConfigs(ctx, userContext)
	if err != nil {
		log.Printf("Failed to refresh configs: %v", err)
	}

	fmt.Println("All configurations refreshed")

	// Output:
	// All configurations refreshed
}

// ExampleNewClientWithLogger demonstrates using a custom logger
func ExampleNewClientWithLogger() {
	// Create a custom logger
	logger := variably.NewDefaultLogger(variably.LogLevelWarn)

	client, err := variably.NewClientWithLogger(variably.ClientConfig{
		APIKey:    "your-api-key",
		ProjectID: "your-project-id",
	}, logger)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	fmt.Println("Client created with custom logger")

	// Output:
	// Client created with custom logger
}

// ExampleDefaultClientConfig demonstrates using default configuration
func ExampleDefaultClientConfig() {
	// Get default configuration
	config := variably.DefaultClientConfig()
	
	// Customize as needed
	config.APIKey = "your-api-key"
	config.ProjectID = "your-project-id"
	config.Debug = true

	client, err := variably.NewClient(config)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	fmt.Printf("Client created with polling interval: %v\n", config.PollingInterval)
	fmt.Printf("Cache TTL: %v\n", config.Cache.TTL)
	fmt.Printf("Max cache size: %d\n", config.Cache.MaxSize)

	// Output:
	// Client created with polling interval: 30s
	// Cache TTL: 5m0s
	// Max cache size: 1000
}
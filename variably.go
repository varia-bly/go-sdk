// Package variably provides the official Go SDK for Variably's dynamic configuration and feature flag platform.
//
// The SDK provides real-time configuration updates via WebSocket connections with intelligent fallback
// to polling mode, comprehensive caching, and type-safe configuration access methods.
//
// Basic usage:
//
//	client, err := variably.NewClient(variably.ClientConfig{
//		APIKey:    "your-api-key",
//		ProjectID: "your-project-id",
//		BaseURL:   "https://api.variably.com",
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer client.Close()
//
//	// Get a configuration value
//	userContext := variably.UserContext{UserID: "user123"}
//	value, err := client.GetConfigBool(ctx, "feature_enabled", false, userContext)
//
// Real-time updates:
//
//	// Subscribe to configuration changes
//	unsubscribe := client.OnConfigChange("feature_enabled", func(result *variably.DynamicConfigResult) {
//		fmt.Printf("Config updated: %s = %v\n", result.Key, result.Value)
//	})
//	defer unsubscribe()
//
package variably

import (
	"time"
)

// Version of the SDK
// Version is asserted against the release tag by .github/workflows/go-sdk-publish.yml.
//
// 1.1.0 rather than 1.0.0: the constant was never released (the module has no
// published versions), and the surface has since gained flag evaluation, AllFlags,
// ValidateFlags and the prompt config store. The typed flag accessors also changed
// to return (value, error) — strictly a breaking change, but one that cannot break
// a caller, because flag evaluation did not work before.
const Version = "1.1.0"

// NewClient creates a new Variably client with default settings (supports both dynamic configs and feature flags)
func NewClient(config ClientConfig) (Client, error) {
	// Apply defaults
	if config.BaseURL == "" {
		config.BaseURL = "https://api.variably.com"
	}
	if config.PollingInterval == 0 {
		config.PollingInterval = 30 * time.Second
	}
	if config.Cache.TTL == 0 {
		config.Cache.TTL = 5 * time.Minute
	}
	if config.Cache.MaxSize == 0 {
		config.Cache.MaxSize = 1000
	}
	if config.WebSocket.ReconnectInterval == 0 {
		config.WebSocket.ReconnectInterval = 5 * time.Second
	}
	if config.WebSocket.MaxReconnectAttempts == 0 {
		config.WebSocket.MaxReconnectAttempts = 10
	}
	if config.WebSocket.ConnectionTimeout == 0 {
		config.WebSocket.ConnectionTimeout = 10 * time.Second
	}
	
	// Set cache enabled by default
	config.Cache.Enabled = true
	
	// Set auto-reconnect by default
	config.WebSocket.AutoReconnect = true
	
	// Set real-time enabled by default if JWT token is provided
	if config.JWTToken != "" {
		config.EnableRealtime = true
	}
	
	// Create logger
	var logger Logger
	if config.Debug {
		logger = NewDefaultLogger(LogLevelDebug)
	} else {
		logger = NewDefaultLogger(LogLevelInfo)
	}
	
	return NewVariablyClient(config, logger)
}

// NewClientWithLogger creates a new Variably client with a custom logger
func NewClientWithLogger(config ClientConfig, logger Logger) (Client, error) {
	return NewVariablyClient(config, logger)
}

// MustNewClient creates a new Variably client and panics if there's an error
func MustNewClient(config ClientConfig) Client {
	client, err := NewClient(config)
	if err != nil {
		panic("Failed to create Variably client: " + err.Error())
	}
	return client
}
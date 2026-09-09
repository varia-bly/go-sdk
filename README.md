# Variably Go SDK

The official Go SDK for [Variably](https://variably.com)'s dynamic configuration and feature flag platform. Get real-time configuration updates with intelligent fallback, comprehensive caching, and type-safe access methods.

## Features

- 🚀 **Real-time Updates**: WebSocket-based configuration updates with automatic fallback to polling
- 🔒 **Type Safety**: Strongly typed configuration access methods
- ⚡ **High Performance**: Multi-layer caching with intelligent invalidation
- 🛡️ **Resilient**: Automatic reconnection and error handling
- 📊 **Observable**: Comprehensive logging and metrics
- 🔧 **Flexible**: Extensive configuration options

## Installation

```bash
go get github.com/varia-bly/go-sdk
```

## Quick Start

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/varia-bly/go-sdk"
)

func main() {
    // Create client
    client, err := variably.NewClient(variably.ClientConfig{
        APIKey:    "your-api-key",
        ProjectID: "your-project-id",
        BaseURL:   "https://api.variably.com",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Create user context
    userContext := variably.UserContext{
        UserID: "user123",
        Email:  "user@example.com",
        Attributes: map[string]interface{}{
            "subscription_tier": "premium",
        },
    }

    ctx := context.Background()

    // Get configurations
    featureEnabled, _ := client.GetConfigBool(ctx, "new_feature", false, userContext)
    maxRetries, _ := client.GetConfigNumber(ctx, "max_retries", 3, userContext)
    theme, _ := client.GetConfigString(ctx, "theme", "light", userContext)

    fmt.Printf("Feature: %v, Retries: %.0f, Theme: %s\n", 
        featureEnabled, maxRetries, theme)
}
```

### Real-time Updates

```go
// Enable real-time updates
client, err := variably.NewClient(variably.ClientConfig{
    APIKey:         "your-api-key",
    JWTToken:       "your-jwt-token", // Required for WebSocket auth
    ProjectID:      "your-project-id",
    EnableRealtime: true,
})
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Subscribe to specific config changes
unsubscribe := client.OnConfigChange("feature_flag", func(result *variably.DynamicConfigResult) {
    fmt.Printf("Config updated: %s = %v\n", result.Key, result.Value)
    
    if result.RealTimeUpdate {
        fmt.Println("✅ Received via WebSocket")
    } else {
        fmt.Println("📊 Received via polling")
    }
})
defer unsubscribe()

// Subscribe to all config changes
unsubscribeAll := client.OnAnyConfigChange(func(result *variably.DynamicConfigResult) {
    fmt.Printf("Any config changed: %s\n", result.Key)
})
defer unsubscribeAll()
```

## Configuration

### Client Configuration

```go
config := variably.ClientConfig{
    // Required
    APIKey:    "your-api-key",
    ProjectID: "your-project-id",
    
    // Optional
    JWTToken:       "jwt-token",              // For WebSocket authentication
    BaseURL:        "https://api.variably.com", // API base URL
    EnableRealtime: true,                     // Enable real-time updates
    PollingInterval: 30 * time.Second,       // Fallback polling interval
    Debug:          false,                    // Enable debug logging
    
    // Cache configuration
    Cache: variably.CacheConfig{
        TTL:     5 * time.Minute, // Cache time-to-live
        MaxSize: 1000,            // Maximum cached entries
        Enabled: true,            // Enable caching
    },
    
    // WebSocket configuration
    WebSocket: variably.WSConfig{
        ReconnectInterval:    5 * time.Second, // Time between reconnect attempts
        MaxReconnectAttempts: 10,              // Maximum reconnection attempts
        ConnectionTimeout:    10 * time.Second,// WebSocket connection timeout
        AutoReconnect:        true,            // Enable automatic reconnection
    },
}

client, err := variably.NewClient(config)
```

### Default Configuration

```go
// Use default configuration with minimal setup
config := variably.DefaultClientConfig()
config.APIKey = "your-api-key"
config.ProjectID = "your-project-id"

client, err := variably.NewClient(config)
```

## API Reference

### Configuration Access Methods

#### GetConfig
```go
value, err := client.GetConfig(ctx, "config_key", defaultValue, userContext)
```

#### GetConfigBool
```go
enabled, err := client.GetConfigBool(ctx, "feature_enabled", false, userContext)
```

#### GetConfigString
```go
message, err := client.GetConfigString(ctx, "welcome_message", "Hello", userContext)
```

#### GetConfigNumber
```go
limit, err := client.GetConfigNumber(ctx, "rate_limit", 100.0, userContext)
```

#### GetConfigJSON
```go
config, err := client.GetConfigJSON(ctx, "ui_config", defaultConfig, userContext)
```

#### EvaluateConfig
```go
result, err := client.EvaluateConfig(ctx, "config_key", defaultValue, userContext)
// Result contains detailed information:
// - Key, Value, Reason
// - RuleID, ETag, Version
// - CacheHit, RealTimeUpdate
// - RetrievedAt, UpdatedAt
```

### Subscription Methods

#### OnConfigChange
```go
unsubscribe := client.OnConfigChange("config_key", func(result *variably.DynamicConfigResult) {
    // Handle configuration change
})
defer unsubscribe()
```

#### OnAnyConfigChange
```go
unsubscribe := client.OnAnyConfigChange(func(result *variably.DynamicConfigResult) {
    // Handle any configuration change in the project
})
defer unsubscribe()
```

### Utility Methods

#### RefreshConfigs
```go
err := client.RefreshConfigs(ctx, userContext)
```

#### GetConnectionStatus
```go
status := client.GetConnectionStatus()
fmt.Printf("Mode: %s, Connected: %v\n", status.Mode, status.Connected)
```

#### Close
```go
client.Close() // Clean shutdown
```

## User Context

The user context provides information for configuration evaluation:

```go
userContext := variably.UserContext{
    UserID:     "user123",           // Required
    Email:      "user@example.com",  // Optional
    Country:    "US",                // Optional
    Language:   "en",                // Optional
    Platform:   "web",               // Optional
    Version:    "1.2.3",             // Optional
    IPAddress:  "192.168.1.1",       // Optional
    UserAgent:  "Mozilla/5.0...",    // Optional
    SessionID:  "session123",        // Optional
    Attributes: map[string]interface{}{ // Optional custom attributes
        "subscription_tier": "premium",
        "signup_date":      "2023-01-15",
        "experiments":      []string{"exp1", "exp2"},
    },
}
```

## Connection Modes

The SDK operates in three modes:

1. **Real-time Mode**: WebSocket connection for instant updates
2. **Polling Mode**: HTTP polling fallback when WebSocket unavailable
3. **Offline Mode**: Uses cached values only

The SDK automatically handles fallback between modes based on connectivity.

## Caching

The SDK implements intelligent caching:

- **L1 Cache**: In-memory cache with TTL
- **Context-aware**: Separate cache entries per user context
- **Smart Invalidation**: Real-time cache invalidation on updates
- **Pattern Matching**: Bulk cache clearing by pattern

## Error Handling

The SDK provides specific error types:

```go
import "errors"

if err != nil {
    var netErr *variably.NetworkError
    var authErr *variably.AuthenticationError
    var configErr *variably.ConfigurationError
    
    switch {
    case errors.As(err, &netErr):
        if netErr.IsRetryable() {
            // Retry the operation
        }
    case errors.As(err, &authErr):
        // Handle authentication error
    case errors.As(err, &configErr):
        // Handle configuration error
    }
}
```

## Logging

### Default Logger

```go
// Debug logging
client, err := variably.NewClient(variably.ClientConfig{
    Debug: true, // Enables debug logging
    // ... other config
})
```

### Custom Logger

```go
// Implement the Logger interface
type CustomLogger struct{}

func (l *CustomLogger) Debug(message string, fields map[string]interface{}) {
    // Your debug logging implementation
}

func (l *CustomLogger) Info(message string, fields map[string]interface{}) {
    // Your info logging implementation
}

func (l *CustomLogger) Warn(message string, fields map[string]interface{}) {
    // Your warning logging implementation
}

func (l *CustomLogger) Error(message string, fields map[string]interface{}) {
    // Your error logging implementation
}

func (l *CustomLogger) SetLevel(level variably.LogLevel) {
    // Set logging level
}

// Use custom logger
logger := &CustomLogger{}
client, err := variably.NewClientWithLogger(config, logger)
```

### No-op Logger

```go
// Disable all logging
logger := variably.NewNoOpLogger()
client, err := variably.NewClientWithLogger(config, logger)
```

## Best Practices

### 1. Resource Management
Always close the client when done:
```go
defer client.Close()
```

### 2. Context Usage
Use appropriate contexts for cancellation:
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

value, err := client.GetConfig(ctx, "key", default, userContext)
```

### 3. Error Handling
Handle errors appropriately:
```go
value, err := client.GetConfigBool(ctx, "feature", false, userContext)
if err != nil {
    log.Printf("Failed to get config: %v", err)
    // Use default value or handle error
}
```

### 4. Subscription Management
Always unsubscribe to prevent memory leaks:
```go
unsubscribe := client.OnConfigChange("key", callback)
defer unsubscribe()
```

### 5. User Context Reuse
Reuse user context objects when possible:
```go
userContext := variably.UserContext{UserID: "user123"}

// Reuse for multiple calls
value1, _ := client.GetConfig(ctx, "key1", default1, userContext)
value2, _ := client.GetConfig(ctx, "key2", default2, userContext)
```

## Examples

See the [example_test.go](example_test.go) file for comprehensive usage examples.

## Performance Considerations

- **Caching**: Enable caching (default) for better performance
- **Connection Pooling**: The SDK reuses HTTP connections
- **Batching**: Group multiple config requests when possible
- **Context Reuse**: Reuse user context objects to improve cache hit rates

## Troubleshooting

### WebSocket Connection Issues
- Ensure JWT token is provided and valid
- Check firewall settings for WebSocket connections
- Verify base URL uses correct protocol (https for wss)

### Cache Issues
- Monitor cache hit rates via logging
- Adjust TTL based on your use case
- Clear cache manually if needed: `client.RefreshConfigs()`

### Performance Issues  
- Enable debug logging to identify bottlenecks
- Monitor connection status: `client.GetConnectionStatus()`
- Consider increasing cache size for high-traffic applications

## License

This SDK is licensed under the MIT License. See LICENSE file for details.

## Support

For support and questions:
- 📧 Email: support@variably.com
- 📖 Documentation: https://docs.variably.com
- 🐛 Issues: https://github.com/varia-bly/go-sdk/issues
## Prompt config store (local assignment)

Assigning a prompt variant used to mean a backend call per event. `PromptConfigStore`
bootstraps the project's prompt-experiment config once, refreshes it in the background
with `ETag`/`If-None-Match` (a 304 is a no-op), and assigns variants **in-process**, so a
per-event assignment costs no network call.

```go
store := variably.NewPromptConfigStore(
    variably.HTTPPromptConfigFetcher(baseURL, apiKey, nil),
)
if err := store.Refresh(ctx); err != nil {
    // a failed bootstrap leaves the store empty; Assign returns nil and the
    // caller should fall back to the server
}
store.StartPolling(variably.DefaultPromptPollInterval) // 45s, matching the Python SDK
defer store.Close()

variant := store.Assign("greeting-experiment", userID, sessionID)
if variant == nil {
    // Not in the snapshot — the experiment may have been created between polls.
    // Fall back to the server rather than treating it as absent.
}
```

Assignment is deterministic and sticky: the same `(experiment, user)` always maps to the
same variant, using the exact bucketing the server uses.

**`assignment.go` is a port of the server's `internal/assignment` and must stay
byte-for-byte equivalent to it.** Both sides compute assignments, so any divergence would
flip a user between variants depending on which side answered — corrupting an experiment
rather than merely disagreeing. `assignment_test.go` pins the shared golden vectors;
a failure there means drift, and the drift is what needs fixing.

Verified identical across the server, this SDK, and the Python SDK:

| salt \| key | bucket |
| --- | --- |
| `exp-1` \| `user-1` | 60.33 |
| `exp-1` \| `user-2` | 31.76 |
| `exp-2` \| `user-1` | 9.74 |
| `greeting` \| `abc123` | 94.44 |

# Variably Go SDK

The official Go SDK for the Variably experimentation platform. This SDK provides a simple, type-safe, and performant way to integrate feature flags, A/B testing, and user targeting into your Go applications.

## Features

- ✅ **Type-Safe API**: Strongly typed with generic methods for different flag types
- ✅ **High Performance**: Intelligent caching with configurable TTL and persistence
- ✅ **Batch Operations**: Evaluate multiple flags in a single API call
- ✅ **Real-time Updates**: WebSocket support for live flag updates
- ✅ **Offline Support**: Cached fallbacks when API is unavailable
- ✅ **Context Aware**: Full `context.Context` support for cancellation and timeouts
- ✅ **Production Ready**: Circuit breakers, retries, and comprehensive error handling
- ✅ **Observability**: Structured logging and built-in metrics

## Quick Start

### Installation

```bash
go get github.com/varia-bly/go-sdk@latest
```

> **Note**: Always use the latest version tag for best compatibility. The current stable version is `v1.0.2`.

### API Key Setup

Before using the SDK, you need to generate an API key:

1. **Register/Login** to get a JWT token:
   ```bash
   curl -X POST https://api.variably.com/api/v1/auth/register \
     -H "Content-Type: application/json" \
     -d '{
       "email": "you@company.com",
       "password": "secure_password",
       "first_name": "Your",
       "last_name": "Name",
       "organization_name": "Your Company"
     }'
   ```

2. **Create an API Key**:
   ```bash
   curl -X POST https://api.variably.com/api/v1/api-keys \
     -H "Authorization: Bearer YOUR_JWT_TOKEN" \
     -d '{
       "name": "Production App Key",
       "environment": "production",
       "scopes": ["flags.read", "gates.read", "events.write"]
     }'
   ```

3. **Save the key** - it's only shown once!
   ```
   vb_live_a1b2c3d4e5f6...your_key_here
   ```

### Basic Usage

```go
package main

import (
    "context"
    "log"
    
    "github.com/varia-bly/go-sdk"
)

func main() {
    // Initialize the client
    client, err := variably.NewClient(&variably.Config{
        APIKey:      "vb_live_your_api_key_here",
        Environment: "production",
    })
    if err != nil {
        log.Fatal("Failed to initialize Variably client:", err)
    }
    defer client.Close()
    
    // Create user context
    user := variably.UserContext{
        UserID:  "user_123",
        Email:   "user@example.com",
        Country: "US",
        Attributes: map[string]interface{}{
            "plan": "premium",
            "signup_date": "2023-01-15",
        },
    }
    
    // Evaluate a boolean feature flag
    showNewFeature := client.EvaluateFlagBool(
        context.Background(),
        "new_dashboard_ui",
        false, // default value
        user,
    )
    
    if showNewFeature {
        // Show new UI
        log.Println("Showing new dashboard UI")
    } else {
        // Show old UI
        log.Println("Showing legacy dashboard UI")
    }
    
    // Evaluate a feature gate
    canAccessPremium := client.EvaluateGate(
        context.Background(),
        "premium_features",
        user,
    )
    
    if canAccessPremium {
        // Enable premium features
        log.Println("User has premium access")
    }
    
    // Track user interaction
    client.Track(context.Background(), variably.Event{
        Name:   "dashboard_viewed",
        UserID: user.UserID,
        Properties: map[string]interface{}{
            "ui_version": map[bool]string{true: "new", false: "legacy"}[showNewFeature],
            "has_premium": canAccessPremium,
        },
    })
}
```

## API Key Management

### Key Format

API keys follow this format: `{environment_prefix}_{secret}`

- **Production**: `vb_live_` + 64-character secret
- **Staging**: `vb_test_` + 64-character secret  
- **Development**: `vb_dev_` + 64-character secret

### Key Scopes

When creating API keys, specify the required scopes:

| Scope | Description | SDK Methods Enabled |
|-------|-------------|-------------------|
| `flags.read` | Read feature flags | `EvaluateFlag*` methods |
| `gates.read` | Read feature gates | `EvaluateGate*` methods |
| `experiments.read` | Read experiments | Experiment assignment |
| `events.write` | Send analytics events | `Track*` methods |

### Environment Variables

Store your API key securely using environment variables:

```bash
export VARIABLY_API_KEY="vb_live_your_key_here"
export VARIABLY_ENVIRONMENT="production"
```

```go
// Load configuration from environment
client, err := variably.NewClientFromEnv()
```

### Configuration Files

Create a `variably.yaml` file:

```yaml
api_key: "vb_live_your_key_here"
environment: "production"
timeout: "5s"
enable_analytics: true

cache_config:
  ttl: "5m"
  max_size: 1000
  enable_persistence: true

log_config:
  level: "info"
  format: "json"
```

```go
client, err := variably.NewClientFromFile("variably.yaml")
```

## Advanced Usage

### Type-Safe Flag Evaluation

The SDK provides type-safe methods for different flag types:

```go
// Boolean flags
enabled := client.EvaluateFlagBool(ctx, "feature_enabled", false, user)

// String flags  
theme := client.EvaluateFlagString(ctx, "ui_theme", "light", user)

// Integer flags
maxItems := client.EvaluateFlagInt(ctx, "max_items_per_page", 10, user)

// Float flags
conversionRate := client.EvaluateFlagFloat(ctx, "conversion_rate", 0.05, user)

// JSON flags (complex objects)
config := client.EvaluateFlagJSON(ctx, "advanced_config", map[string]interface{}{
    "timeout": 30,
    "retries": 3,
}, user)
```

### Batch Operations

For high-performance scenarios, use batch operations:

```go
// Evaluate multiple flags in one API call
flagKeys := []string{"feature_a", "feature_b", "feature_c"}
results := client.EvaluateFlags(context.Background(), flagKeys, user)

for flagKey, result := range results {
    if result.Error != nil {
        log.Printf("Error evaluating %s: %v", flagKey, result.Error)
        continue
    }
    log.Printf("Flag %s = %v (reason: %s)", flagKey, result.Value, result.Reason)
}

// Evaluate multiple gates
gateKeys := []string{"premium_features", "beta_access", "admin_panel"}
gateResults := client.EvaluateGates(context.Background(), gateKeys, user)
```

### Error Handling

The SDK provides comprehensive error handling:

```go
result := client.EvaluateFlag(ctx, "my_flag", "default", user)
if result.Error != nil {
    switch err := result.Error.(type) {
    case *variably.NetworkError:
        log.Printf("Network error: %v", err)
        // Handle network issues
    case *variably.AuthenticationError:
        log.Printf("Authentication error: %v", err)
        // Handle invalid API key
    case *variably.RateLimitError:
        log.Printf("Rate limit exceeded: %v", err)
        // Handle rate limiting
    default:
        log.Printf("Unknown error: %v", err)
    }
}
```

### Custom Configuration

```go
client, err := variably.NewClient(&variably.Config{
    APIKey:      "vb_live_your_key_here",
    Environment: "production",
    BaseURL:     "https://api.yourcompany.com", // Custom endpoint
    
    // Performance tuning
    Timeout:       10 * time.Second,
    RetryAttempts: 5,
    MaxCacheSize:  2000,
    
    // Caching configuration
    CacheConfig: variably.CacheConfig{
        TTL:               10 * time.Minute,
        MaxSize:           2000,
        EnablePersistence: true,
        PersistencePath:   "/var/cache/variably",
        EvictionPolicy:    "LRU",
    },
    
    // Real-time updates
    PollingConfig: variably.PollingConfig{
        Enabled:  true,
        Interval: 30 * time.Second,
        Jitter:   5 * time.Second,
    },
    
    // Features
    EnableAnalytics:    true,
    EnableOfflineMode:  true,
    EnableRealTimeSync: true,
    
    // Custom logging
    LogConfig: variably.LogConfig{
        Level:  "debug",
        Format: "json",
        Output: "stdout",
    },
})
```

### Real-time Updates

Subscribe to real-time flag updates:

```go
// Subscribe to flag changes
flagKeys := []string{"feature_enabled", "ui_theme"}
err := client.Subscribe(context.Background(), flagKeys, func(flagKey string, newResult variably.FlagResult) {
    log.Printf("Flag %s updated: %v", flagKey, newResult.Value)
    
    // Update your application state
    switch flagKey {
    case "feature_enabled":
        updateFeatureState(newResult.Value.(bool))
    case "ui_theme":
        updateUITheme(newResult.Value.(string))
    }
})

// Later, unsubscribe when no longer needed
client.Unsubscribe(flagKeys)
```

### Analytics and Event Tracking

Track user interactions and custom events:

```go
// Track single event
client.Track(context.Background(), variably.Event{
    Name:   "button_clicked",
    UserID: user.UserID,
    Properties: map[string]interface{}{
        "button_id": "signup_cta",
        "page":      "landing",
        "variant":   "blue_button",
    },
})

// Track multiple events efficiently
events := []variably.Event{
    {
        Name:   "page_viewed",
        UserID: user.UserID,
        Properties: map[string]interface{}{
            "page": "dashboard",
            "load_time": 1.23,
        },
    },
    {
        Name:   "feature_used",
        UserID: user.UserID,
        Properties: map[string]interface{}{
            "feature": "export_data",
            "format":  "csv",
        },
    },
}

client.TrackBatch(context.Background(), events)
```

### Performance Monitoring

Monitor SDK performance and usage:

```go
metrics := client.GetMetrics()

log.Printf("API Calls: %d", metrics.APICalls)
log.Printf("Cache Hit Rate: %.2f%%", metrics.CacheHitRate)
log.Printf("Error Rate: %.2f%%", metrics.ErrorRate)
log.Printf("Average Latency: %v", metrics.AverageLatency)
log.Printf("Flags Evaluated: %d", metrics.FlagsEvaluated)
log.Printf("Events Tracked: %d", metrics.EventsTracked)
```

## Testing

### Unit Testing with Mock Client

The SDK provides a mock client for unit testing:

```go
func TestMyFeature(t *testing.T) {
    // Create mock client
    mockClient := variably.NewMockClient()
    
    // Set up mock responses
    mockClient.SetFlagValue("feature_enabled", true)
    mockClient.SetFlagValue("ui_theme", "dark")
    mockClient.SetGateValue("premium_features", false)
    
    // Use in your service
    service := NewMyService(mockClient)
    
    // Test your logic
    result := service.ProcessUser(user)
    
    // Verify behavior
    assert.True(t, result.FeatureEnabled)
    assert.Equal(t, "dark", result.Theme)
    
    // Verify tracked events
    events := mockClient.GetTrackedEvents()
    assert.Len(t, events, 1)
    assert.Equal(t, "feature_used", events[0].Name)
}
```

### Integration Testing

Test against a real Variably instance:

```go
func TestIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }
    
    client, err := variably.NewClient(&variably.Config{
        APIKey:      os.Getenv("VARIABLY_TEST_API_KEY"),
        Environment: "test",
        BaseURL:     "http://localhost:8080",
    })
    require.NoError(t, err)
    defer client.Close()
    
    // Test flag evaluation
    result := client.EvaluateFlagBool(
        context.Background(),
        "test_flag",
        false,
        variably.UserContext{UserID: "test_user"},
    )
    
    // Verify result
    assert.NotNil(t, result)
}
```

## Performance Tips

1. **Use Batch Operations**: Evaluate multiple flags in a single call
2. **Enable Caching**: Configure appropriate TTL for your use case
3. **Use Persistence**: Enable disk cache for faster startup
4. **Optimize User Context**: Only include necessary attributes
5. **Monitor Metrics**: Use built-in metrics to optimize performance

## Migration Guide

### From LaunchDarkly

```go
// LaunchDarkly
variation := ldClient.BoolVariation("feature-key", user, false)

// Variably
enabled := client.EvaluateFlagBool(ctx, "feature-key", false, userContext)
```

### From Split

```go
// Split
treatment := splitClient.GetTreatment("user-key", "feature-name")

// Variably  
value := client.EvaluateFlagString(ctx, "feature-name", "control", userContext)
```

### From Optimizely

```go
// Optimizely
enabled := optimizelyClient.IsFeatureEnabled("feature-key", "user-id", userAttributes)

// Variably
enabled := client.EvaluateFlagBool(ctx, "feature-key", false, userContext)
```

## Examples

See the [`examples/`](./examples/) directory for complete examples:

- [Basic Usage](./examples/basic_usage.go) - Simple flag evaluation
- [Web Application](./examples/web_server.go) - HTTP server integration
- [Background Worker](./examples/worker.go) - Long-running service
- [Testing](./examples/testing_example_test.go) - Unit and integration tests

## API Reference

### Client Interface

```go
type Client interface {
    // Feature Flag Operations
    EvaluateFlag(ctx context.Context, flagKey string, defaultValue interface{}, userContext UserContext) FlagResult
    EvaluateFlagBool(ctx context.Context, flagKey string, defaultValue bool, userContext UserContext) bool
    EvaluateFlagString(ctx context.Context, flagKey string, defaultValue string, userContext UserContext) string
    EvaluateFlagInt(ctx context.Context, flagKey string, defaultValue int, userContext UserContext) int
    EvaluateFlagFloat(ctx context.Context, flagKey string, defaultValue float64, userContext UserContext) float64
    EvaluateFlagJSON(ctx context.Context, flagKey string, defaultValue interface{}, userContext UserContext) interface{}
    
    // Feature Gate Operations
    EvaluateGate(ctx context.Context, gateKey string, userContext UserContext) bool
    
    // Batch Operations
    EvaluateFlags(ctx context.Context, flagKeys []string, userContext UserContext) map[string]FlagResult
    EvaluateGates(ctx context.Context, gateKeys []string, userContext UserContext) map[string]bool
    
    // Event Tracking
    Track(ctx context.Context, event Event) error
    TrackBatch(ctx context.Context, events []Event) error
    
    // Real-time Updates
    Subscribe(ctx context.Context, flagKeys []string, callback UpdateCallback) error
    Unsubscribe(flagKeys []string) error
    
    // Cache Management
    RefreshCache(ctx context.Context) error
    ClearCache() error
    
    // Metrics
    GetMetrics() Metrics
    
    // Lifecycle
    Close() error
}
```

## Requirements

- Go 1.19 or later
- Network access to Variably API (or cached flags for offline mode)

## Support

- 📧 Email: support@variably.com
- 💬 Discord: [Variably Community](https://discord.gg/variably)
- 🐛 Issues: [GitHub Issues](https://github.com/varia-bly/go-sdk/issues)
- 📚 Docs: [Documentation Site](https://docs.variably.com)

## License

MIT License. See [LICENSE](../../LICENSE) for details.
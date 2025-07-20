package variably

import (
	"context"
	"time"
)

// Client provides the main interface for feature flag evaluation and analytics tracking
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

// UserContext provides user information for targeting rules
type UserContext struct {
	UserID     string                 `json:"user_id"`
	SessionID  string                 `json:"session_id,omitempty"`
	Email      string                 `json:"email,omitempty"`
	Country    string                 `json:"country,omitempty"`
	Language   string                 `json:"language,omitempty"`
	Platform   string                 `json:"platform,omitempty"`
	Version    string                 `json:"version,omitempty"`
	IPAddress  string                 `json:"ip_address,omitempty"`
	UserAgent  string                 `json:"user_agent,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
}

// FlagResult contains the evaluation result with metadata
type FlagResult struct {
	Key         string      `json:"key"`
	Value       interface{} `json:"value"`
	Reason      string      `json:"reason"`
	RuleID      string      `json:"rule_id,omitempty"`
	Variation   string      `json:"variation,omitempty"`
	Error       error       `json:"-"`
	EvaluatedAt time.Time   `json:"evaluated_at"`
	CacheHit    bool        `json:"cache_hit"`
}

// Event represents a tracking event for analytics
type Event struct {
	Name       string                 `json:"event_name"`
	UserID     string                 `json:"user_id"`
	SessionID  string                 `json:"session_id,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	Context    UserContext            `json:"context,omitempty"`
}

// UpdateCallback is called when a flag value changes in real-time
type UpdateCallback func(flagKey string, newValue FlagResult)

// Metrics provides SDK performance and usage statistics
type Metrics struct {
	APICalls        int64         `json:"api_calls"`
	CacheHits       int64         `json:"cache_hits"`
	CacheMisses     int64         `json:"cache_misses"`
	ErrorCount      int64         `json:"error_count"`
	AverageLatency  time.Duration `json:"average_latency"`
	TotalLatency    time.Duration `json:"total_latency"`
	ErrorRate       float64       `json:"error_rate"`
	CacheHitRate    float64       `json:"cache_hit_rate"`
	LastUpdated     time.Time     `json:"last_updated"`
	StartTime       time.Time     `json:"start_time"`
	FlagsEvaluated  int64         `json:"flags_evaluated"`
	GatesEvaluated  int64         `json:"gates_evaluated"`
	EventsTracked   int64         `json:"events_tracked"`
}

// Logger interface for custom logging implementations
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

// Cache interface for different caching implementations
type Cache interface {
	Get(key string) (FlagResult, bool)
	Set(key string, result FlagResult, ttl time.Duration)
	Delete(key string)
	Clear()
	Size() int
	Keys() []string
}
package variably

import (
	"context"
	"sync"
	"time"
)

// Client provides the main interface for feature flag evaluation and dynamic configuration
type Client interface {
	// Dynamic Configuration Operations (Real-time)
	GetConfig(ctx context.Context, configKey string, defaultValue interface{}, userContext UserContext) (interface{}, error)
	GetConfigBool(ctx context.Context, configKey string, defaultValue bool, userContext UserContext) (bool, error)
	GetConfigString(ctx context.Context, configKey string, defaultValue string, userContext UserContext) (string, error)
	GetConfigNumber(ctx context.Context, configKey string, defaultValue float64, userContext UserContext) (float64, error)
	GetConfigJSON(ctx context.Context, configKey string, defaultValue interface{}, userContext UserContext) (interface{}, error)
	EvaluateConfig(ctx context.Context, configKey string, defaultValue interface{}, userContext UserContext) (*DynamicConfigResult, error)

	// Traditional Feature Flag Operations
	EvaluateFlag(ctx context.Context, flagKey string, defaultValue interface{}, userContext UserContext) FlagResult
	EvaluateFlagBool(ctx context.Context, flagKey string, defaultValue bool, userContext UserContext) (bool, error)
	EvaluateFlagString(ctx context.Context, flagKey string, defaultValue string, userContext UserContext) (string, error)
	EvaluateFlagInt(ctx context.Context, flagKey string, defaultValue int, userContext UserContext) (int, error)
	EvaluateFlagFloat(ctx context.Context, flagKey string, defaultValue float64, userContext UserContext) (float64, error)
	EvaluateFlagJSON(ctx context.Context, flagKey string, defaultValue interface{}, userContext UserContext) (interface{}, error)

	// Feature Gate Operations
	EvaluateGate(ctx context.Context, gateKey string, userContext UserContext) (bool, error)

	// ValidateFlags reports which of expectedKeys the project does not define.
	ValidateFlags(ctx context.Context, expectedKeys []string, userContext UserContext) ([]string, error)
	// AllFlags evaluates every flag in the project for one context.
	AllFlags(ctx context.Context, userContext UserContext) (map[string]FlagResult, error)

	// Batch Operations
	EvaluateFlags(ctx context.Context, flagKeys []string, userContext UserContext) map[string]FlagResult
	EvaluateGates(ctx context.Context, gateKeys []string, userContext UserContext) map[string]bool

	// Event Tracking
	Track(ctx context.Context, event Event) error
	TrackBatch(ctx context.Context, events []Event) error

	// Real-time Updates (Dynamic Config)
	OnConfigChange(configKey string, callback ConfigChangeCallback) func()
	OnAnyConfigChange(callback ConfigChangeCallback) func()
	RefreshConfigs(ctx context.Context, userContext UserContext) error

	// Real-time Updates (Feature Flags)
	Subscribe(ctx context.Context, flagKeys []string, callback UpdateCallback) error
	Unsubscribe(flagKeys []string) error

	// Cache Management
	RefreshCache(ctx context.Context) error
	ClearCache() error

	// Connection Status
	GetConnectionStatus() ConnectionStatus

	// Metrics
	GetMetrics() Metrics

	// Lifecycle
	Close() error
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
	ConfigsEvaluated int64        `json:"configs_evaluated"`
}

// MetricsCollector collects and tracks SDK performance metrics
type MetricsCollector struct {
	metrics     Metrics
	mutex       sync.RWMutex
	startTime   time.Time
	latencySum  time.Duration
	requestID   int64
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		metrics: Metrics{
			StartTime:   time.Now(),
			LastUpdated: time.Now(),
		},
		startTime: time.Now(),
	}
}

// RecordAPICall records an API call with latency
func (m *MetricsCollector) RecordAPICall(latency time.Duration, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.metrics.APICalls++
	m.latencySum += latency
	m.metrics.TotalLatency = m.latencySum
	m.metrics.AverageLatency = m.latencySum / time.Duration(m.metrics.APICalls)

	if err != nil {
		m.metrics.ErrorCount++
	}

	m.updateRates()
	m.metrics.LastUpdated = time.Now()
}

// RecordCacheHit records a cache hit
func (m *MetricsCollector) RecordCacheHit() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.metrics.CacheHits++
	m.updateRates()
	m.metrics.LastUpdated = time.Now()
}

// RecordCacheMiss records a cache miss
func (m *MetricsCollector) RecordCacheMiss() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.metrics.CacheMisses++
	m.updateRates()
	m.metrics.LastUpdated = time.Now()
}

// RecordFlagEvaluation records a feature flag evaluation
func (m *MetricsCollector) RecordFlagEvaluation() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.metrics.FlagsEvaluated++
	m.metrics.LastUpdated = time.Now()
}

// RecordGateEvaluation records a feature gate evaluation
func (m *MetricsCollector) RecordGateEvaluation() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.metrics.GatesEvaluated++
	m.metrics.LastUpdated = time.Now()
}

// RecordConfigEvaluation records a dynamic config evaluation
func (m *MetricsCollector) RecordConfigEvaluation() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.metrics.ConfigsEvaluated++
	m.metrics.LastUpdated = time.Now()
}

// RecordEvent records an event tracking
func (m *MetricsCollector) RecordEvent() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.metrics.EventsTracked++
	m.metrics.LastUpdated = time.Now()
}

// GetMetrics returns a copy of current metrics
func (m *MetricsCollector) GetMetrics() Metrics {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Update rates before returning
	m.updateRates()

	// Return a copy
	return m.metrics
}

// updateRates calculates derived metrics (must be called with lock held)
func (m *MetricsCollector) updateRates() {
	totalRequests := m.metrics.CacheHits + m.metrics.CacheMisses
	if totalRequests > 0 {
		m.metrics.CacheHitRate = float64(m.metrics.CacheHits) / float64(totalRequests)
	}

	if m.metrics.APICalls > 0 {
		m.metrics.ErrorRate = float64(m.metrics.ErrorCount) / float64(m.metrics.APICalls) * 100
	}
}
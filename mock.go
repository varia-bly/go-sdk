package variably

import (
	"context"
	"sync"
	"time"
)

// MockClient implements the Client interface for testing purposes
type MockClient struct {
	flagValues    map[string]interface{}
	gateValues    map[string]bool
	trackedEvents []Event
	metrics       *MetricsCollector
	mutex         sync.RWMutex
}

// NewMockClient creates a new mock client for testing
func NewMockClient() *MockClient {
	return &MockClient{
		flagValues:    make(map[string]interface{}),
		gateValues:    make(map[string]bool),
		trackedEvents: make([]Event, 0),
		metrics:       NewMetricsCollector(),
	}
}

// SetFlagValue sets a mock flag value
func (m *MockClient) SetFlagValue(flagKey string, value interface{}) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.flagValues[flagKey] = value
}

// SetGateValue sets a mock gate value
func (m *MockClient) SetGateValue(gateKey string, value bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.gateValues[gateKey] = value
}

// GetTrackedEvents returns all tracked events
func (m *MockClient) GetTrackedEvents() []Event {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	events := make([]Event, len(m.trackedEvents))
	copy(events, m.trackedEvents)
	return events
}

// ClearTrackedEvents clears all tracked events
func (m *MockClient) ClearTrackedEvents() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.trackedEvents = make([]Event, 0)
}

// Reset resets all mock data
func (m *MockClient) Reset() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.flagValues = make(map[string]interface{})
	m.gateValues = make(map[string]bool)
	m.trackedEvents = make([]Event, 0)
	m.metrics.Reset()
}

// Client interface implementation

func (m *MockClient) EvaluateFlag(ctx context.Context, flagKey string, defaultValue interface{}, userContext UserContext) FlagResult {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	m.metrics.RecordFlagEvaluation()
	
	if value, exists := m.flagValues[flagKey]; exists {
		return FlagResult{
			Key:         flagKey,
			Value:       value,
			Reason:      "mock_rule",
			EvaluatedAt: time.Now(),
			CacheHit:    false,
		}
	}
	
	return FlagResult{
		Key:         flagKey,
		Value:       defaultValue,
		Reason:      "default",
		EvaluatedAt: time.Now(),
		CacheHit:    false,
	}
}

func (m *MockClient) EvaluateFlagBool(ctx context.Context, flagKey string, defaultValue bool, userContext UserContext) bool {
	result := m.EvaluateFlag(ctx, flagKey, defaultValue, userContext)
	if value, ok := result.Value.(bool); ok {
		return value
	}
	return defaultValue
}

func (m *MockClient) EvaluateFlagString(ctx context.Context, flagKey string, defaultValue string, userContext UserContext) string {
	result := m.EvaluateFlag(ctx, flagKey, defaultValue, userContext)
	if value, ok := result.Value.(string); ok {
		return value
	}
	return defaultValue
}

func (m *MockClient) EvaluateFlagInt(ctx context.Context, flagKey string, defaultValue int, userContext UserContext) int {
	result := m.EvaluateFlag(ctx, flagKey, defaultValue, userContext)
	switch v := result.Value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return defaultValue
	}
}

func (m *MockClient) EvaluateFlagFloat(ctx context.Context, flagKey string, defaultValue float64, userContext UserContext) float64 {
	result := m.EvaluateFlag(ctx, flagKey, defaultValue, userContext)
	switch v := result.Value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		return defaultValue
	}
}

func (m *MockClient) EvaluateFlagJSON(ctx context.Context, flagKey string, defaultValue interface{}, userContext UserContext) interface{} {
	result := m.EvaluateFlag(ctx, flagKey, defaultValue, userContext)
	return result.Value
}

func (m *MockClient) EvaluateGate(ctx context.Context, gateKey string, userContext UserContext) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	m.metrics.RecordGateEvaluation()
	
	if value, exists := m.gateValues[gateKey]; exists {
		return value
	}
	
	return false // Default for gates
}

func (m *MockClient) EvaluateFlags(ctx context.Context, flagKeys []string, userContext UserContext) map[string]FlagResult {
	results := make(map[string]FlagResult)
	for _, flagKey := range flagKeys {
		results[flagKey] = m.EvaluateFlag(ctx, flagKey, nil, userContext)
	}
	return results
}

func (m *MockClient) EvaluateGates(ctx context.Context, gateKeys []string, userContext UserContext) map[string]bool {
	results := make(map[string]bool)
	for _, gateKey := range gateKeys {
		results[gateKey] = m.EvaluateGate(ctx, gateKey, userContext)
	}
	return results
}

func (m *MockClient) Track(ctx context.Context, event Event) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	
	m.trackedEvents = append(m.trackedEvents, event)
	m.metrics.RecordEventTracked()
	return nil
}

func (m *MockClient) TrackBatch(ctx context.Context, events []Event) error {
	for _, event := range events {
		if err := m.Track(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockClient) Subscribe(ctx context.Context, flagKeys []string, callback UpdateCallback) error {
	// Mock implementation - no actual subscription
	return nil
}

func (m *MockClient) Unsubscribe(flagKeys []string) error {
	// Mock implementation - no actual unsubscription
	return nil
}

func (m *MockClient) RefreshCache(ctx context.Context) error {
	// Mock implementation - no actual cache
	return nil
}

func (m *MockClient) ClearCache() error {
	// Mock implementation - no actual cache
	return nil
}

func (m *MockClient) GetMetrics() Metrics {
	return m.metrics.GetMetrics()
}

func (m *MockClient) Close() error {
	// Mock implementation - nothing to close
	return nil
}
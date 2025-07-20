package variably

import (
	"sync"
	"sync/atomic"
	"time"
)

// MetricsCollector collects SDK performance and usage metrics
type MetricsCollector struct {
	startTime time.Time
	
	// Atomic counters for thread-safe updates
	apiCalls        int64
	cacheHits       int64
	cacheMisses     int64
	errorCount      int64
	flagsEvaluated  int64
	gatesEvaluated  int64
	eventsTracked   int64
	
	// Latency tracking
	totalLatency time.Duration
	latencyMutex sync.RWMutex
	
	// Rate tracking
	lastErrorRate    float64
	lastCacheHitRate float64
	rateMutex        sync.RWMutex
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		startTime: time.Now(),
	}
}

// RecordAPICall records an API call with latency and success status
func (m *MetricsCollector) RecordAPICall(latency time.Duration, success bool) {
	atomic.AddInt64(&m.apiCalls, 1)
	
	if !success {
		atomic.AddInt64(&m.errorCount, 1)
	}
	
	m.latencyMutex.Lock()
	m.totalLatency += latency
	m.latencyMutex.Unlock()
}

// RecordCacheHit records a cache hit
func (m *MetricsCollector) RecordCacheHit() {
	atomic.AddInt64(&m.cacheHits, 1)
}

// RecordCacheMiss records a cache miss
func (m *MetricsCollector) RecordCacheMiss() {
	atomic.AddInt64(&m.cacheMisses, 1)
}

// RecordFlagEvaluation records a flag evaluation
func (m *MetricsCollector) RecordFlagEvaluation() {
	atomic.AddInt64(&m.flagsEvaluated, 1)
}

// RecordGateEvaluation records a gate evaluation
func (m *MetricsCollector) RecordGateEvaluation() {
	atomic.AddInt64(&m.gatesEvaluated, 1)
}

// RecordEventTracked records an event tracking
func (m *MetricsCollector) RecordEventTracked() {
	atomic.AddInt64(&m.eventsTracked, 1)
}

// GetMetrics returns current metrics snapshot
func (m *MetricsCollector) GetMetrics() Metrics {
	apiCalls := atomic.LoadInt64(&m.apiCalls)
	cacheHits := atomic.LoadInt64(&m.cacheHits)
	cacheMisses := atomic.LoadInt64(&m.cacheMisses)
	errorCount := atomic.LoadInt64(&m.errorCount)
	flagsEvaluated := atomic.LoadInt64(&m.flagsEvaluated)
	gatesEvaluated := atomic.LoadInt64(&m.gatesEvaluated)
	eventsTracked := atomic.LoadInt64(&m.eventsTracked)
	
	m.latencyMutex.RLock()
	totalLatency := m.totalLatency
	m.latencyMutex.RUnlock()
	
	var averageLatency time.Duration
	if apiCalls > 0 {
		averageLatency = totalLatency / time.Duration(apiCalls)
	}
	
	var errorRate float64
	if apiCalls > 0 {
		errorRate = float64(errorCount) / float64(apiCalls) * 100
	}
	
	totalCacheOps := cacheHits + cacheMisses
	var cacheHitRate float64
	if totalCacheOps > 0 {
		cacheHitRate = float64(cacheHits) / float64(totalCacheOps) * 100
	}
	
	// Update cached rates
	m.rateMutex.Lock()
	m.lastErrorRate = errorRate
	m.lastCacheHitRate = cacheHitRate
	m.rateMutex.Unlock()
	
	return Metrics{
		APICalls:        apiCalls,
		CacheHits:       cacheHits,
		CacheMisses:     cacheMisses,
		ErrorCount:      errorCount,
		AverageLatency:  averageLatency,
		TotalLatency:    totalLatency,
		ErrorRate:       errorRate,
		CacheHitRate:    cacheHitRate,
		LastUpdated:     time.Now(),
		StartTime:       m.startTime,
		FlagsEvaluated:  flagsEvaluated,
		GatesEvaluated:  gatesEvaluated,
		EventsTracked:   eventsTracked,
	}
}

// Reset resets all metrics counters
func (m *MetricsCollector) Reset() {
	atomic.StoreInt64(&m.apiCalls, 0)
	atomic.StoreInt64(&m.cacheHits, 0)
	atomic.StoreInt64(&m.cacheMisses, 0)
	atomic.StoreInt64(&m.errorCount, 0)
	atomic.StoreInt64(&m.flagsEvaluated, 0)
	atomic.StoreInt64(&m.gatesEvaluated, 0)
	atomic.StoreInt64(&m.eventsTracked, 0)
	
	m.latencyMutex.Lock()
	m.totalLatency = 0
	m.latencyMutex.Unlock()
	
	m.startTime = time.Now()
}

// GetErrorRate returns the current error rate
func (m *MetricsCollector) GetErrorRate() float64 {
	m.rateMutex.RLock()
	defer m.rateMutex.RUnlock()
	return m.lastErrorRate
}

// GetCacheHitRate returns the current cache hit rate
func (m *MetricsCollector) GetCacheHitRate() float64 {
	m.rateMutex.RLock()
	defer m.rateMutex.RUnlock()
	return m.lastCacheHitRate
}

// GetUptime returns how long the metrics collector has been running
func (m *MetricsCollector) GetUptime() time.Duration {
	return time.Since(m.startTime)
}

// Summary returns a human-readable summary of metrics
func (m *MetricsCollector) Summary() map[string]interface{} {
	metrics := m.GetMetrics()
	
	return map[string]interface{}{
		"uptime":           m.GetUptime().String(),
		"api_calls":        metrics.APICalls,
		"flags_evaluated":  metrics.FlagsEvaluated,
		"gates_evaluated":  metrics.GatesEvaluated,
		"events_tracked":   metrics.EventsTracked,
		"cache_hits":       metrics.CacheHits,
		"cache_misses":     metrics.CacheMisses,
		"cache_hit_rate":   metrics.CacheHitRate,
		"error_count":      metrics.ErrorCount,
		"error_rate":       metrics.ErrorRate,
		"average_latency":  metrics.AverageLatency.String(),
		"total_latency":    metrics.TotalLatency.String(),
	}
}
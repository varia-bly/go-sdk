package variably

import (
	"context"
	"regexp"
	"sync"
	"time"
)

// CacheEntry represents a cached item
type CacheEntry struct {
	Value     interface{}
	Timestamp time.Time
	TTL       time.Duration
}

// IsExpired checks if the cache entry has expired
func (e *CacheEntry) IsExpired() bool {
	return time.Since(e.Timestamp) > e.TTL
}

// Cache provides an in-memory cache with TTL support
type Cache interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{}, ttl time.Duration)
	Delete(key string) bool
	Clear()
	ClearByPattern(pattern string) int
	Size() int
	GetStats() CacheStats
}

// CacheStats provides cache statistics
type CacheStats struct {
	Size    int     `json:"size"`
	MaxSize int     `json:"max_size"`
	HitRate float64 `json:"hit_rate"`
	Enabled bool    `json:"enabled"`
}

// MemoryCache is an in-memory implementation of Cache
type MemoryCache struct {
	config    CacheConfig
	entries   map[string]*CacheEntry
	mutex     sync.RWMutex
	stopCh    chan struct{}
	hits      int64
	misses    int64
	statsLock sync.RWMutex
}

// NewMemoryCache creates a new memory cache
func NewMemoryCache(config CacheConfig) *MemoryCache {
	cache := &MemoryCache{
		config:  config,
		entries: make(map[string]*CacheEntry),
		stopCh:  make(chan struct{}),
	}
	
	if config.Enabled {
		go cache.startCleanup()
	}
	
	return cache
}

// Get retrieves a value from the cache
func (c *MemoryCache) Get(key string) (interface{}, bool) {
	if !c.config.Enabled {
		c.recordMiss()
		return nil, false
	}

	c.mutex.RLock()
	entry, exists := c.entries[key]
	c.mutex.RUnlock()

	if !exists {
		c.recordMiss()
		return nil, false
	}

	if entry.IsExpired() {
		c.mutex.Lock()
		delete(c.entries, key)
		c.mutex.Unlock()
		c.recordMiss()
		return nil, false
	}

	c.recordHit()
	return entry.Value, true
}

// Set stores a value in the cache
func (c *MemoryCache) Set(key string, value interface{}, ttl time.Duration) {
	if !c.config.Enabled {
		return
	}

	if ttl == 0 {
		ttl = c.config.TTL
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Enforce max size by removing oldest entries
	if len(c.entries) >= c.config.MaxSize {
		c.evictOldest()
	}

	c.entries[key] = &CacheEntry{
		Value:     value,
		Timestamp: time.Now(),
		TTL:       ttl,
	}
}

// Delete removes a value from the cache
func (c *MemoryCache) Delete(key string) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	_, existed := c.entries[key]
	delete(c.entries, key)
	return existed
}

// Clear removes all entries from the cache
func (c *MemoryCache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.entries = make(map[string]*CacheEntry)
}

// ClearByPattern removes entries matching a pattern
func (c *MemoryCache) ClearByPattern(pattern string) int {
	if !c.config.Enabled {
		return 0
	}

	// Convert simple pattern to regex (supports * wildcard)
	regexPattern := "^" + regexp.QuoteMeta(pattern)
	regexPattern = regexp.MustCompile(`\\\*`).ReplaceAllString(regexPattern, ".*")
	regexPattern += "$"
	
	regex, err := regexp.Compile(regexPattern)
	if err != nil {
		return 0
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	keysToDelete := make([]string, 0)
	for key := range c.entries {
		if regex.MatchString(key) {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(c.entries, key)
	}

	return len(keysToDelete)
}

// Size returns the number of entries in the cache
func (c *MemoryCache) Size() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return len(c.entries)
}

// GetStats returns cache statistics
func (c *MemoryCache) GetStats() CacheStats {
	c.statsLock.RLock()
	hits := c.hits
	misses := c.misses
	c.statsLock.RUnlock()

	total := hits + misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return CacheStats{
		Size:    c.Size(),
		MaxSize: c.config.MaxSize,
		HitRate: hitRate,
		Enabled: c.config.Enabled,
	}
}

// Close stops the cleanup goroutine
func (c *MemoryCache) Close() {
	close(c.stopCh)
}

// evictOldest removes the oldest entry from the cache
func (c *MemoryCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	first := true

	for key, entry := range c.entries {
		if first || entry.Timestamp.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.Timestamp
			first = false
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

// cleanup removes expired entries
func (c *MemoryCache) cleanup() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	keysToDelete := make([]string, 0)
	for key, entry := range c.entries {
		if entry.IsExpired() {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(c.entries, key)
	}
}

// startCleanup starts the periodic cleanup goroutine
func (c *MemoryCache) startCleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCh:
			return
		}
	}
}

// recordHit increments the hit counter
func (c *MemoryCache) recordHit() {
	c.statsLock.Lock()
	c.hits++
	c.statsLock.Unlock()
}

// recordMiss increments the miss counter
func (c *MemoryCache) recordMiss() {
	c.statsLock.Lock()
	c.misses++
	c.statsLock.Unlock()
}

// CacheManager manages cache operations with a specific context
type CacheManager struct {
	cache  Cache
	logger Logger
}

// NewCacheManager creates a new cache manager
func NewCacheManager(config CacheConfig, logger Logger) *CacheManager {
	return &CacheManager{
		cache:  NewMemoryCache(config),
		logger: logger,
	}
}

// Get retrieves a value from the cache
func (cm *CacheManager) Get(ctx context.Context, key string) (interface{}, bool) {
	value, found := cm.cache.Get(key)
	if found {
		cm.logger.Debug("Cache hit", map[string]interface{}{"key": key})
	} else {
		cm.logger.Debug("Cache miss", map[string]interface{}{"key": key})
	}
	return value, found
}

// Set stores a value in the cache
func (cm *CacheManager) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) {
	cm.cache.Set(key, value, ttl)
	cm.logger.Debug("Cache entry set", map[string]interface{}{"key": key, "ttl": ttl.String()})
}

// Delete removes a value from the cache
func (cm *CacheManager) Delete(ctx context.Context, key string) bool {
	deleted := cm.cache.Delete(key)
	if deleted {
		cm.logger.Debug("Cache entry deleted", map[string]interface{}{"key": key})
	}
	return deleted
}

// Clear removes all entries from the cache
func (cm *CacheManager) Clear(ctx context.Context) {
	size := cm.cache.Size()
	cm.cache.Clear()
	cm.logger.Debug("Cache cleared", map[string]interface{}{"entries_removed": size})
}

// ClearByPattern removes entries matching a pattern
func (cm *CacheManager) ClearByPattern(ctx context.Context, pattern string) int {
	removed := cm.cache.ClearByPattern(pattern)
	if removed > 0 {
		cm.logger.Debug("Cache entries cleared by pattern", map[string]interface{}{
			"pattern":         pattern,
			"entries_removed": removed,
		})
	}
	return removed
}

// GetStats returns cache statistics
func (cm *CacheManager) GetStats() CacheStats {
	return cm.cache.GetStats()
}

// Close closes the cache manager
func (cm *CacheManager) Close() {
	if memCache, ok := cm.cache.(*MemoryCache); ok {
		memCache.Close()
	}
}
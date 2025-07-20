package variably

import (
	"container/list"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// MemoryCache implements an in-memory LRU cache with TTL support
type MemoryCache struct {
	maxSize    int
	items      map[string]*cacheItem
	lruList    *list.List
	mutex      sync.RWMutex
	defaultTTL time.Duration
}

type cacheItem struct {
	key        string
	value      FlagResult
	expiration time.Time
	element    *list.Element
}

// NewMemoryCache creates a new in-memory cache
func NewMemoryCache(maxSize int, defaultTTL time.Duration) *MemoryCache {
	return &MemoryCache{
		maxSize:    maxSize,
		items:      make(map[string]*cacheItem),
		lruList:    list.New(),
		defaultTTL: defaultTTL,
	}
}

// Get retrieves a value from the cache
func (c *MemoryCache) Get(key string) (FlagResult, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	item, exists := c.items[key]
	if !exists {
		return FlagResult{}, false
	}

	// Check if item has expired
	if time.Now().After(item.expiration) {
		// Item expired, remove it (but don't delete while holding read lock)
		go c.Delete(key)
		return FlagResult{}, false
	}

	// Move to front (most recently used)
	c.lruList.MoveToFront(item.element)
	return item.value, true
}

// Set stores a value in the cache
func (c *MemoryCache) Set(key string, result FlagResult, ttl time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if ttl == 0 {
		ttl = c.defaultTTL
	}

	expiration := time.Now().Add(ttl)

	// If item already exists, update it
	if existingItem, exists := c.items[key]; exists {
		existingItem.value = result
		existingItem.expiration = expiration
		c.lruList.MoveToFront(existingItem.element)
		return
	}

	// Create new item
	item := &cacheItem{
		key:        key,
		value:      result,
		expiration: expiration,
	}

	// Add to front of LRU list
	item.element = c.lruList.PushFront(item)
	c.items[key] = item

	// Evict if over capacity
	c.evictIfNeeded()
}

// Delete removes a value from the cache
func (c *MemoryCache) Delete(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if item, exists := c.items[key]; exists {
		c.lruList.Remove(item.element)
		delete(c.items, key)
	}
}

// Clear removes all items from the cache
func (c *MemoryCache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.items = make(map[string]*cacheItem)
	c.lruList.Init()
}

// Size returns the current number of items in the cache
func (c *MemoryCache) Size() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return len(c.items)
}

// Keys returns all keys in the cache
func (c *MemoryCache) Keys() []string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	keys := make([]string, 0, len(c.items))
	for key := range c.items {
		keys = append(keys, key)
	}
	return keys
}

// evictIfNeeded removes the least recently used items if over capacity
func (c *MemoryCache) evictIfNeeded() {
	for len(c.items) > c.maxSize {
		// Remove least recently used item (from back of list)
		oldest := c.lruList.Back()
		if oldest != nil {
			item := oldest.Value.(*cacheItem)
			c.lruList.Remove(oldest)
			delete(c.items, item.key)
		}
	}
}

// CleanupExpired removes all expired items from the cache
func (c *MemoryCache) CleanupExpired() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now()
	var expiredKeys []string

	for key, item := range c.items {
		if now.After(item.expiration) {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		if item, exists := c.items[key]; exists {
			c.lruList.Remove(item.element)
			delete(c.items, key)
		}
	}
}

// PersistentCache implements a file-based persistent cache
type PersistentCache struct {
	memoryCache *MemoryCache
	filePath    string
	mutex       sync.RWMutex
}

type persistentCacheData struct {
	Items map[string]persistentCacheItem `json:"items"`
}

type persistentCacheItem struct {
	Value      FlagResult `json:"value"`
	Expiration time.Time  `json:"expiration"`
}

// NewPersistentCache creates a new persistent cache
func NewPersistentCache(maxSize int, defaultTTL time.Duration, filePath string) *PersistentCache {
	cache := &PersistentCache{
		memoryCache: NewMemoryCache(maxSize, defaultTTL),
		filePath:    filePath,
	}

	// Load existing data from file
	cache.loadFromFile()

	return cache
}

// Get retrieves a value from the cache
func (c *PersistentCache) Get(key string) (FlagResult, bool) {
	return c.memoryCache.Get(key)
}

// Set stores a value in the cache and persists it
func (c *PersistentCache) Set(key string, result FlagResult, ttl time.Duration) {
	c.memoryCache.Set(key, result, ttl)
	c.saveToFile()
}

// Delete removes a value from the cache and updates persistence
func (c *PersistentCache) Delete(key string) {
	c.memoryCache.Delete(key)
	c.saveToFile()
}

// Clear removes all items from the cache and clears persistence
func (c *PersistentCache) Clear() {
	c.memoryCache.Clear()
	c.saveToFile()
}

// Size returns the current number of items in the cache
func (c *PersistentCache) Size() int {
	return c.memoryCache.Size()
}

// Keys returns all keys in the cache
func (c *PersistentCache) Keys() []string {
	return c.memoryCache.Keys()
}

// loadFromFile loads cache data from the persistent file
func (c *PersistentCache) loadFromFile() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	data, err := os.ReadFile(c.filePath)
	if err != nil {
		// File doesn't exist or can't be read, start with empty cache
		return
	}

	var cacheData persistentCacheData
	if err := json.Unmarshal(data, &cacheData); err != nil {
		// Invalid file format, start with empty cache
		return
	}

	// Load non-expired items into memory cache
	now := time.Now()
	for key, item := range cacheData.Items {
		if now.Before(item.Expiration) {
			ttl := time.Until(item.Expiration)
			c.memoryCache.Set(key, item.Value, ttl)
		}
	}
}

// saveToFile saves current cache data to the persistent file
func (c *PersistentCache) saveToFile() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Get all current items from memory cache
	items := make(map[string]persistentCacheItem)
	
	c.memoryCache.mutex.RLock()
	for key, item := range c.memoryCache.items {
		items[key] = persistentCacheItem{
			Value:      item.value,
			Expiration: item.expiration,
		}
	}
	c.memoryCache.mutex.RUnlock()

	cacheData := persistentCacheData{Items: items}

	data, err := json.Marshal(cacheData)
	if err != nil {
		// Failed to marshal, can't save
		return
	}

	// Write to temporary file first, then rename (atomic operation)
	tempFile := c.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return
	}

	os.Rename(tempFile, c.filePath)
}

// CacheManager manages different cache implementations
type CacheManager struct {
	cache  Cache
	config CacheConfig
	logger Logger
}

// NewCacheManager creates a new cache manager with the specified configuration
func NewCacheManager(config CacheConfig, logger Logger) *CacheManager {
	var cache Cache

	if config.EnablePersistence && config.PersistencePath != "" {
		cache = NewPersistentCache(config.MaxSize, config.TTL, config.PersistencePath)
	} else {
		cache = NewMemoryCache(config.MaxSize, config.TTL)
	}

	return &CacheManager{
		cache:  cache,
		config: config,
		logger: logger,
	}
}

// Get retrieves a value from the cache
func (cm *CacheManager) Get(key string) (FlagResult, bool) {
	return cm.cache.Get(key)
}

// Set stores a value in the cache
func (cm *CacheManager) Set(key string, result FlagResult, ttl time.Duration) {
	if ttl == 0 {
		ttl = cm.config.TTL
	}
	cm.cache.Set(key, result, ttl)
}

// Delete removes a value from the cache
func (cm *CacheManager) Delete(key string) {
	cm.cache.Delete(key)
}

// Clear removes all values from the cache
func (cm *CacheManager) Clear() {
	cm.cache.Clear()
	cm.logger.Info("Cache cleared")
}

// Size returns the current cache size
func (cm *CacheManager) Size() int {
	return cm.cache.Size()
}

// Keys returns all cache keys
func (cm *CacheManager) Keys() []string {
	return cm.cache.Keys()
}

// StartCleanup starts a background goroutine to clean up expired cache entries
func (cm *CacheManager) StartCleanup(stopCh <-chan struct{}) {
	ticker := time.NewTicker(time.Minute) // Clean up every minute
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if memCache, ok := cm.cache.(*MemoryCache); ok {
				memCache.CleanupExpired()
			} else if persCache, ok := cm.cache.(*PersistentCache); ok {
				persCache.memoryCache.CleanupExpired()
				persCache.saveToFile()
			}
		case <-stopCh:
			return
		}
	}
}

// GetStats returns cache statistics
func (cm *CacheManager) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"size":     cm.cache.Size(),
		"max_size": cm.config.MaxSize,
		"ttl":      cm.config.TTL.String(),
		"keys":     len(cm.cache.Keys()),
	}
}
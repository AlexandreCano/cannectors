// Package cache provides caching implementations for the Cannectors runtime.
// It includes an LRU cache with TTL support for use in enrichment modules.
package cache

import (
	"sync"
	"sync/atomic"
	"time"
)

// Cache defines the interface for cache implementations.
// All implementations must be thread-safe.
type Cache interface {
	// Get retrieves a value from the cache.
	// Returns the value and true if found and not expired, nil and false otherwise.
	Get(key string) (any, bool)

	// Set stores a value in the cache with the specified TTL.
	// If the cache is at capacity, the least recently used entry is evicted.
	Set(key string, value any, ttl time.Duration)

	// Delete removes a value from the cache.
	Delete(key string)

	// Clear removes all entries from the cache.
	Clear()

	// Size returns the current number of entries in the cache.
	Size() int

	// Stats returns cache statistics.
	Stats() Stats
}

// Stats contains cache performance statistics.
type Stats struct {
	Hits      int64 `json:"hits"`
	Misses    int64 `json:"misses"`
	Evictions int64 `json:"evictions"`
}

// cacheItem represents a single cache entry with expiration and access tracking.
type cacheItem struct {
	value     any
	expiresAt time.Time
	lastUsed  time.Time
}

// isExpired returns true if the cache item has expired.
func (item *cacheItem) isExpired(now time.Time) bool {
	return now.After(item.expiresAt)
}

// LRUCache implements an LRU cache with TTL support.
// Thread-safe with mutex protection for concurrent access.
type LRUCache struct {
	maxSize    int
	defaultTTL time.Duration
	mu         sync.RWMutex
	items      map[string]*cacheItem
	hits       int64
	misses     int64
	evictions  int64
}

// NewLRUCache creates a new LRU cache with the specified configuration.
// maxSize specifies the maximum number of entries (default 1000 if <= 0).
// defaultTTL specifies the default TTL for entries (default 5 minutes if <= 0).
func NewLRUCache(maxSize int, defaultTTL time.Duration) *LRUCache {
	if maxSize <= 0 {
		maxSize = 1000
	}
	if defaultTTL <= 0 {
		defaultTTL = 5 * time.Minute
	}

	return &LRUCache{
		maxSize:    maxSize,
		defaultTTL: defaultTTL,
		items:      make(map[string]*cacheItem),
	}
}

// Get retrieves a value from the cache.
// Returns the value and true if found and not expired.
// Updates the lastUsed time for LRU tracking on cache hit.
func (c *LRUCache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, exists := c.items[key]
	if !exists {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	now := time.Now()

	// Check TTL expiration
	if item.isExpired(now) {
		delete(c.items, key)
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	// Update last used time for LRU tracking
	item.lastUsed = now
	atomic.AddInt64(&c.hits, 1)
	return item.value, true
}

// Set stores a value in the cache with the specified TTL.
// If ttl is 0, uses the default TTL.
// If the cache is at capacity and the key doesn't exist, evicts the LRU entry.
func (c *LRUCache) Set(key string, value any, ttl time.Duration) {
	if ttl <= 0 {
		ttl = c.defaultTTL
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Evict if at capacity and this is a new key
	if len(c.items) >= c.maxSize {
		if _, exists := c.items[key]; !exists {
			c.evictLRU()
		}
	}

	c.items[key] = &cacheItem{
		value:     value,
		expiresAt: now.Add(ttl),
		lastUsed:  now,
	}
}

// Delete removes a value from the cache.
func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// Clear removes all entries from the cache.
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*cacheItem)
}

// Size returns the current number of entries in the cache.
func (c *LRUCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Stats returns cache performance statistics.
func (c *LRUCache) Stats() Stats {
	return Stats{
		Hits:      atomic.LoadInt64(&c.hits),
		Misses:    atomic.LoadInt64(&c.misses),
		Evictions: atomic.LoadInt64(&c.evictions),
	}
}

// evictLRU removes the least recently used entry from the cache.
// Must be called with lock held.
func (c *LRUCache) evictLRU() {
	if len(c.items) == 0 {
		return
	}

	var oldestKey string
	var oldestTime time.Time
	first := true

	for key, item := range c.items {
		if first || item.lastUsed.Before(oldestTime) {
			oldestKey = key
			oldestTime = item.lastUsed
			first = false
		}
	}

	if oldestKey != "" {
		delete(c.items, oldestKey)
		atomic.AddInt64(&c.evictions, 1)
	}
}

// CleanExpired removes all expired entries from the cache.
// This can be called periodically to prevent memory from being held by expired entries.
func (c *LRUCache) CleanExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	count := 0

	for key, item := range c.items {
		if item.isExpired(now) {
			delete(c.items, key)
			count++
		}
	}

	return count
}

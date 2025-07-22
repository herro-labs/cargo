package cargo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Cache interface defines the contract for all cache implementations
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	DeletePattern(ctx context.Context, pattern string) error
	Exists(ctx context.Context, key string) (bool, error)
	Clear(ctx context.Context) error
	TTL(ctx context.Context, key string) (time.Duration, error)
	Stats() CacheStats
}

// CacheStats provides statistics about cache performance
type CacheStats struct {
	Hits      int64     `json:"hits"`
	Misses    int64     `json:"misses"`
	Sets      int64     `json:"sets"`
	Deletes   int64     `json:"deletes"`
	Evictions int64     `json:"evictions"`
	Keys      int64     `json:"keys"`
	Memory    int64     `json:"memory_bytes"`
	HitRate   float64   `json:"hit_rate"`
	LastReset time.Time `json:"last_reset"`
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	Backend        string        `json:"backend" yaml:"backend"` // "memory", "redis"
	DefaultTTL     time.Duration `json:"default_ttl" yaml:"default_ttl"`
	MaxSize        int64         `json:"max_size" yaml:"max_size"`               // Max number of items
	MaxMemory      int64         `json:"max_memory" yaml:"max_memory"`           // Max memory in bytes
	EvictionPolicy string        `json:"eviction_policy" yaml:"eviction_policy"` // "lru", "lfu", "ttl"
	RedisAddr      string        `json:"redis_addr" yaml:"redis_addr"`
	RedisPassword  string        `json:"redis_password" yaml:"redis_password"`
	RedisDB        int           `json:"redis_db" yaml:"redis_db"`
	Prefix         string        `json:"prefix" yaml:"prefix"`
	Serialization  string        `json:"serialization" yaml:"serialization"` // "json", "gob"
}

// CacheItem represents a cached item with metadata
type CacheItem struct {
	Value       []byte    `json:"value"`
	ExpiresAt   time.Time `json:"expires_at"`
	AccessedAt  time.Time `json:"accessed_at"`
	CreatedAt   time.Time `json:"created_at"`
	AccessCount int64     `json:"access_count"`
	Size        int64     `json:"size"`
}

// MemoryCache implements an in-memory cache with TTL and eviction policies
type MemoryCache struct {
	items  map[string]*CacheItem
	mutex  sync.RWMutex
	config CacheConfig
	stats  CacheStats
	stopCh chan struct{}
}

// NewMemoryCache creates a new in-memory cache
func NewMemoryCache(config CacheConfig) *MemoryCache {
	if config.DefaultTTL == 0 {
		config.DefaultTTL = time.Hour
	}
	if config.MaxSize == 0 {
		config.MaxSize = 10000
	}
	if config.EvictionPolicy == "" {
		config.EvictionPolicy = "lru"
	}

	cache := &MemoryCache{
		items:  make(map[string]*CacheItem),
		config: config,
		stats: CacheStats{
			LastReset: time.Now(),
		},
		stopCh: make(chan struct{}),
	}

	// Start cleanup goroutine
	go cache.cleanup()

	return cache
}

// Get retrieves a value from the cache
func (mc *MemoryCache) Get(ctx context.Context, key string) ([]byte, error) {
	mc.mutex.RLock()
	item, exists := mc.items[key]
	mc.mutex.RUnlock()

	if !exists {
		mc.incrementMisses()
		return nil, NewNotFoundError("cache miss")
	}

	// Check if expired
	if time.Now().After(item.ExpiresAt) {
		mc.mutex.Lock()
		delete(mc.items, key)
		mc.mutex.Unlock()
		mc.incrementMisses()
		return nil, NewNotFoundError("cache expired")
	}

	// Update access metadata
	mc.mutex.Lock()
	item.AccessedAt = time.Now()
	item.AccessCount++
	mc.mutex.Unlock()

	mc.incrementHits()
	return item.Value, nil
}

// Set stores a value in the cache
func (mc *MemoryCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl == 0 {
		ttl = mc.config.DefaultTTL
	}

	now := time.Now()
	item := &CacheItem{
		Value:       value,
		ExpiresAt:   now.Add(ttl),
		AccessedAt:  now,
		CreatedAt:   now,
		AccessCount: 0,
		Size:        int64(len(value)),
	}

	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	// Check if we need to evict items
	if mc.shouldEvict() {
		mc.evictItems()
	}

	mc.items[key] = item
	mc.incrementSets()
	return nil
}

// Delete removes a value from the cache
func (mc *MemoryCache) Delete(ctx context.Context, key string) error {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	if _, exists := mc.items[key]; exists {
		delete(mc.items, key)
		mc.incrementDeletes()
	}

	return nil
}

// DeletePattern removes all keys matching a pattern
func (mc *MemoryCache) DeletePattern(ctx context.Context, pattern string) error {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	keysToDelete := make([]string, 0)
	for key := range mc.items {
		if mc.matchPattern(key, pattern) {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(mc.items, key)
		mc.incrementDeletes()
	}

	return nil
}

// Exists checks if a key exists in the cache
func (mc *MemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	mc.mutex.RLock()
	item, exists := mc.items[key]
	mc.mutex.RUnlock()

	if !exists {
		return false, nil
	}

	// Check if expired
	if time.Now().After(item.ExpiresAt) {
		mc.mutex.Lock()
		delete(mc.items, key)
		mc.mutex.Unlock()
		return false, nil
	}

	return true, nil
}

// Clear removes all items from the cache
func (mc *MemoryCache) Clear(ctx context.Context) error {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	mc.items = make(map[string]*CacheItem)
	mc.resetStats()
	return nil
}

// TTL returns the time-to-live for a key
func (mc *MemoryCache) TTL(ctx context.Context, key string) (time.Duration, error) {
	mc.mutex.RLock()
	item, exists := mc.items[key]
	mc.mutex.RUnlock()

	if !exists {
		return 0, NewNotFoundError("key not found")
	}

	remaining := time.Until(item.ExpiresAt)
	if remaining < 0 {
		return 0, nil
	}

	return remaining, nil
}

// Stats returns cache statistics
func (mc *MemoryCache) Stats() CacheStats {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	stats := mc.stats
	stats.Keys = int64(len(mc.items))

	// Calculate memory usage
	var memory int64
	for _, item := range mc.items {
		memory += item.Size + 64 // Approximate overhead
	}
	stats.Memory = memory

	// Calculate hit rate
	total := stats.Hits + stats.Misses
	if total > 0 {
		stats.HitRate = float64(stats.Hits) / float64(total)
	}

	return stats
}

// shouldEvict determines if items should be evicted
func (mc *MemoryCache) shouldEvict() bool {
	if mc.config.MaxSize > 0 && int64(len(mc.items)) >= mc.config.MaxSize {
		return true
	}

	if mc.config.MaxMemory > 0 {
		var memory int64
		for _, item := range mc.items {
			memory += item.Size
		}
		if memory >= mc.config.MaxMemory {
			return true
		}
	}

	return false
}

// evictItems removes items based on the eviction policy
func (mc *MemoryCache) evictItems() {
	evictCount := len(mc.items) / 10 // Evict 10% of items
	if evictCount < 1 {
		evictCount = 1
	}

	keysToEvict := make([]string, 0, evictCount)

	switch mc.config.EvictionPolicy {
	case "lru":
		keysToEvict = mc.getLRUKeys(evictCount)
	case "lfu":
		keysToEvict = mc.getLFUKeys(evictCount)
	case "ttl":
		keysToEvict = mc.getTTLKeys(evictCount)
	default:
		keysToEvict = mc.getLRUKeys(evictCount)
	}

	for _, key := range keysToEvict {
		delete(mc.items, key)
		mc.incrementEvictions()
	}
}

// getLRUKeys returns keys for LRU eviction
func (mc *MemoryCache) getLRUKeys(count int) []string {
	type keyTime struct {
		key  string
		time time.Time
	}

	keyTimes := make([]keyTime, 0, len(mc.items))
	for key, item := range mc.items {
		keyTimes = append(keyTimes, keyTime{key: key, time: item.AccessedAt})
	}

	// Sort by access time (oldest first)
	for i := 0; i < len(keyTimes)-1; i++ {
		for j := i + 1; j < len(keyTimes); j++ {
			if keyTimes[i].time.After(keyTimes[j].time) {
				keyTimes[i], keyTimes[j] = keyTimes[j], keyTimes[i]
			}
		}
	}

	keys := make([]string, 0, count)
	for i := 0; i < count && i < len(keyTimes); i++ {
		keys = append(keys, keyTimes[i].key)
	}

	return keys
}

// getLFUKeys returns keys for LFU eviction
func (mc *MemoryCache) getLFUKeys(count int) []string {
	type keyFreq struct {
		key  string
		freq int64
	}

	keyFreqs := make([]keyFreq, 0, len(mc.items))
	for key, item := range mc.items {
		keyFreqs = append(keyFreqs, keyFreq{key: key, freq: item.AccessCount})
	}

	// Sort by access count (lowest first)
	for i := 0; i < len(keyFreqs)-1; i++ {
		for j := i + 1; j < len(keyFreqs); j++ {
			if keyFreqs[i].freq > keyFreqs[j].freq {
				keyFreqs[i], keyFreqs[j] = keyFreqs[j], keyFreqs[i]
			}
		}
	}

	keys := make([]string, 0, count)
	for i := 0; i < count && i < len(keyFreqs); i++ {
		keys = append(keys, keyFreqs[i].key)
	}

	return keys
}

// getTTLKeys returns keys for TTL eviction (shortest TTL first)
func (mc *MemoryCache) getTTLKeys(count int) []string {
	type keyTTL struct {
		key string
		ttl time.Time
	}

	keyTTLs := make([]keyTTL, 0, len(mc.items))
	for key, item := range mc.items {
		keyTTLs = append(keyTTLs, keyTTL{key: key, ttl: item.ExpiresAt})
	}

	// Sort by expiration time (earliest first)
	for i := 0; i < len(keyTTLs)-1; i++ {
		for j := i + 1; j < len(keyTTLs); j++ {
			if keyTTLs[i].ttl.After(keyTTLs[j].ttl) {
				keyTTLs[i], keyTTLs[j] = keyTTLs[j], keyTTLs[i]
			}
		}
	}

	keys := make([]string, 0, count)
	for i := 0; i < count && i < len(keyTTLs); i++ {
		keys = append(keys, keyTTLs[i].key)
	}

	return keys
}

// cleanup removes expired items periodically
func (mc *MemoryCache) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-mc.stopCh:
			return
		case <-ticker.C:
			mc.cleanupExpired()
		}
	}
}

// cleanupExpired removes expired items
func (mc *MemoryCache) cleanupExpired() {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	now := time.Now()
	keysToDelete := make([]string, 0)

	for key, item := range mc.items {
		if now.After(item.ExpiresAt) {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(mc.items, key)
		mc.incrementEvictions()
	}
}

// matchPattern checks if a key matches a pattern (simple wildcard support)
func (mc *MemoryCache) matchPattern(key, pattern string) bool {
	if pattern == "*" {
		return true
	}

	if strings.Contains(pattern, "*") {
		// Simple wildcard matching
		parts := strings.Split(pattern, "*")
		if len(parts) == 2 {
			return strings.HasPrefix(key, parts[0]) && strings.HasSuffix(key, parts[1])
		}
	}

	return key == pattern
}

// Stats increment methods
func (mc *MemoryCache) incrementHits() {
	mc.stats.Hits++
}

func (mc *MemoryCache) incrementMisses() {
	mc.stats.Misses++
}

func (mc *MemoryCache) incrementSets() {
	mc.stats.Sets++
}

func (mc *MemoryCache) incrementDeletes() {
	mc.stats.Deletes++
}

func (mc *MemoryCache) incrementEvictions() {
	mc.stats.Evictions++
}

func (mc *MemoryCache) resetStats() {
	mc.stats = CacheStats{
		LastReset: time.Now(),
	}
}

// Stop stops the cache cleanup goroutine
func (mc *MemoryCache) Stop() {
	close(mc.stopCh)
}

// CacheManager manages multiple cache instances
type CacheManager struct {
	caches map[string]Cache
	mutex  sync.RWMutex
}

// NewCacheManager creates a new cache manager
func NewCacheManager() *CacheManager {
	return &CacheManager{
		caches: make(map[string]Cache),
	}
}

// AddCache adds a cache instance with a name
func (cm *CacheManager) AddCache(name string, cache Cache) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	cm.caches[name] = cache
}

// GetCache retrieves a cache by name
func (cm *CacheManager) GetCache(name string) (Cache, bool) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()
	cache, exists := cm.caches[name]
	return cache, exists
}

// GetDefault returns the default cache or creates one
func (cm *CacheManager) GetDefault() Cache {
	cache, exists := cm.GetCache("default")
	if !exists {
		cache = NewMemoryCache(CacheConfig{
			DefaultTTL:     time.Hour,
			MaxSize:        1000,
			EvictionPolicy: "lru",
		})
		cm.AddCache("default", cache)
	}
	return cache
}

// AllStats returns statistics for all caches
func (cm *CacheManager) AllStats() map[string]CacheStats {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	stats := make(map[string]CacheStats)
	for name, cache := range cm.caches {
		stats[name] = cache.Stats()
	}
	return stats
}

// Global cache manager
var globalCacheManager *CacheManager

func init() {
	globalCacheManager = NewCacheManager()
}

// SetGlobalCacheManager sets the global cache manager
func SetGlobalCacheManager(manager *CacheManager) {
	globalCacheManager = manager
}

// GetGlobalCacheManager returns the global cache manager
func GetGlobalCacheManager() *CacheManager {
	return globalCacheManager
}

// Convenience functions for global cache manager
func GetCache(name string) (Cache, bool) {
	return globalCacheManager.GetCache(name)
}

func GetDefaultCache() Cache {
	return globalCacheManager.GetDefault()
}

// Cache key helpers
func CacheKey(parts ...string) string {
	return strings.Join(parts, ":")
}

func ServiceCacheKey(service, method string, args ...string) string {
	key := fmt.Sprintf("service:%s:%s", service, method)
	if len(args) > 0 {
		key += ":" + strings.Join(args, ":")
	}
	return key
}

func UserCacheKey(userID string, parts ...string) string {
	key := fmt.Sprintf("user:%s", userID)
	if len(parts) > 0 {
		key += ":" + strings.Join(parts, ":")
	}
	return key
}

// Serialization helpers
func MarshalJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func UnmarshalJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

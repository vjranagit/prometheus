package storage

import (
	"container/list"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/vjranagit/prometheus/pkg/types"
)

// QueryCache implements an LRU cache for query results
type QueryCache struct {
	capacity int
	ttl      time.Duration
	mu       sync.RWMutex
	cache    map[string]*cacheEntry
	lru      *list.List
}

// cacheEntry represents a cached query result
type cacheEntry struct {
	key       string
	result    *types.QueryResult
	timestamp time.Time
	element   *list.Element
}

// NewQueryCache creates a new query cache
func NewQueryCache(capacity int, ttl time.Duration) *QueryCache {
	return &QueryCache{
		capacity: capacity,
		ttl:      ttl,
		cache:    make(map[string]*cacheEntry),
		lru:      list.New(),
	}
}

// Get retrieves a cached query result
func (qc *QueryCache) Get(req *types.QueryRequest) (*types.QueryResult, bool) {
	qc.mu.Lock()
	defer qc.mu.Unlock()

	key := qc.generateKey(req)
	entry, exists := qc.cache[key]
	
	if !exists {
		return nil, false
	}

	// Check if entry has expired
	if time.Since(entry.timestamp) > qc.ttl {
		qc.removeLocked(key)
		return nil, false
	}

	// Move to front of LRU list (most recently used)
	qc.lru.MoveToFront(entry.element)

	return entry.result, true
}

// Put stores a query result in the cache
func (qc *QueryCache) Put(req *types.QueryRequest, result *types.QueryResult) {
	qc.mu.Lock()
	defer qc.mu.Unlock()

	key := qc.generateKey(req)

	// Check if entry already exists
	if entry, exists := qc.cache[key]; exists {
		// Update existing entry
		entry.result = result
		entry.timestamp = time.Now()
		qc.lru.MoveToFront(entry.element)
		return
	}

	// Create new entry
	entry := &cacheEntry{
		key:       key,
		result:    result,
		timestamp: time.Now(),
	}

	// Add to cache and LRU list
	entry.element = qc.lru.PushFront(entry)
	qc.cache[key] = entry

	// Evict oldest entry if cache is full
	if qc.lru.Len() > qc.capacity {
		oldest := qc.lru.Back()
		if oldest != nil {
			oldestEntry := oldest.Value.(*cacheEntry)
			qc.removeLocked(oldestEntry.key)
		}
	}
}

// removeLocked removes an entry from the cache (must hold lock)
func (qc *QueryCache) removeLocked(key string) {
	if entry, exists := qc.cache[key]; exists {
		qc.lru.Remove(entry.element)
		delete(qc.cache, key)
	}
}

// Clear clears all cache entries
func (qc *QueryCache) Clear() {
	qc.mu.Lock()
	defer qc.mu.Unlock()

	qc.cache = make(map[string]*cacheEntry)
	qc.lru = list.New()
}

// Size returns the current cache size
func (qc *QueryCache) Size() int {
	qc.mu.RLock()
	defer qc.mu.RUnlock()
	return len(qc.cache)
}

// Stats returns cache statistics
func (qc *QueryCache) Stats() CacheStats {
	qc.mu.RLock()
	defer qc.mu.Unlock()

	expired := 0
	for _, entry := range qc.cache {
		if time.Since(entry.timestamp) > qc.ttl {
			expired++
		}
	}

	return CacheStats{
		Size:     len(qc.cache),
		Capacity: qc.capacity,
		Expired:  expired,
	}
}

// CacheStats contains cache statistics
type CacheStats struct {
	Size     int
	Capacity int
	Expired  int
}

// generateKey generates a cache key from a query request
func (qc *QueryCache) generateKey(req *types.QueryRequest) string {
	// Create deterministic key from request parameters
	data, _ := json.Marshal(map[string]interface{}{
		"tenant":    req.TenantID,
		"query":     req.Query,
		"start":     req.StartTime.Unix(),
		"end":       req.EndTime.Unix(),
	})

	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

// CachedStorage wraps a storage with query caching
type CachedStorage struct {
	storage Storage
	cache   *QueryCache
	hits    uint64
	misses  uint64
	mu      sync.RWMutex
}

// NewCachedStorage creates a cached storage wrapper
func NewCachedStorage(storage Storage, cacheCapacity int, cacheTTL time.Duration) *CachedStorage {
	return &CachedStorage{
		storage: storage,
		cache:   NewQueryCache(cacheCapacity, cacheTTL),
	}
}

// Write passes through to underlying storage
func (cs *CachedStorage) Write(ctx interface{}, req *types.WriteRequest) error {
	// Clear cache on write to invalidate stale results
	// In production, use more sophisticated invalidation
	cs.cache.Clear()
	
	if s, ok := cs.storage.(*badgerStorage); ok {
		return s.Write(ctx.(*interface{}), req)
	}
	return fmt.Errorf("unsupported storage type")
}

// Query checks cache before querying storage
func (cs *CachedStorage) Query(ctx interface{}, req *types.QueryRequest) (*types.QueryResult, error) {
	// Try cache first
	if result, ok := cs.cache.Get(req); ok {
		cs.mu.Lock()
		cs.hits++
		cs.mu.Unlock()
		return result, nil
	}

	// Cache miss - query storage
	cs.mu.Lock()
	cs.misses++
	cs.mu.Unlock()

	var result *types.QueryResult
	var err error
	
	if s, ok := cs.storage.(*badgerStorage); ok {
		result, err = s.Query(ctx.(*interface{}), req)
	} else {
		return nil, fmt.Errorf("unsupported storage type")
	}

	if err != nil {
		return nil, err
	}

	// Cache the result
	cs.cache.Put(req, result)

	return result, nil
}

// Close closes the underlying storage
func (cs *CachedStorage) Close() error {
	return cs.storage.Close()
}

// CacheStats returns cache statistics
func (cs *CachedStorage) CacheStats() (CacheStats, uint64, uint64) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cache.Stats(), cs.hits, cs.misses
}

// CacheHitRate returns the cache hit rate as a percentage
func (cs *CachedStorage) CacheHitRate() float64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	total := cs.hits + cs.misses
	if total == 0 {
		return 0.0
	}

	return float64(cs.hits) / float64(total) * 100.0
}

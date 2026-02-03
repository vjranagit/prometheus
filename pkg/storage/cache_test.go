package storage
package storage

import (
	"testing"
	"time"

	"github.com/vjranagit/prometheus/pkg/types"
)

func TestQueryCache(t *testing.T) {
	cache := NewQueryCache(100, 1*time.Minute)

	// Test cache miss
	req := &types.QueryRequest{
		TenantID:  "test",
		Query:     "test_metric",
		StartTime: time.Now().Add(-1 * time.Hour),
		EndTime:   time.Now(),
	}

	_, ok := cache.Get(req)
	if ok {
		t.Error("Expected cache miss, got hit")
	}

	// Test cache put and get
	result := &types.QueryResult{
		Series: []types.Series{
			{
				Metric: types.Metric{
					Name: "test_metric",
					Labels: map[string]string{
						"label": "value",
					},
				},
				Samples: []types.Sample{
					{Timestamp: time.Now(), Value: 42.0},
				},
			},
		},
	}

	cache.Put(req, result)

	cachedResult, ok := cache.Get(req)
	if !ok {
		t.Fatal("Expected cache hit, got miss")
	}

	if len(cachedResult.Series) != 1 {
		t.Errorf("Expected 1 series, got %d", len(cachedResult.Series))
	}

	if cachedResult.Series[0].Samples[0].Value != 42.0 {
		t.Errorf("Expected value 42.0, got %f", cachedResult.Series[0].Samples[0].Value)
	}
}

func TestQueryCacheTTL(t *testing.T) {
	// Short TTL for testing
	cache := NewQueryCache(100, 100*time.Millisecond)

	req := &types.QueryRequest{
		TenantID:  "test",
		Query:     "test_metric",
		StartTime: time.Now().Add(-1 * time.Hour),
		EndTime:   time.Now(),
	}

	result := &types.QueryResult{
		Series: []types.Series{},
	}

	cache.Put(req, result)

	// Should be in cache
	_, ok := cache.Get(req)
	if !ok {
		t.Error("Expected cache hit")
	}

	// Wait for expiry
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	_, ok = cache.Get(req)
	if ok {
		t.Error("Expected cache miss after TTL expiry")
	}
}

func TestQueryCacheLRUEviction(t *testing.T) {
	// Small cache for testing eviction
	cache := NewQueryCache(3, 1*time.Minute)

	result := &types.QueryResult{Series: []types.Series{}}

	// Fill cache
	for i := 0; i < 4; i++ {
		req := &types.QueryRequest{
			TenantID:  "test",
			Query:     fmt.Sprintf("metric_%d", i),
			StartTime: time.Now().Add(-1 * time.Hour),
			EndTime:   time.Now(),
		}
		cache.Put(req, result)
	}

	// Cache should have 3 entries (oldest evicted)
	if cache.Size() != 3 {
		t.Errorf("Expected cache size 3, got %d", cache.Size())
	}

	// First entry should be evicted
	req0 := &types.QueryRequest{
		TenantID:  "test",
		Query:     "metric_0",
		StartTime: time.Now().Add(-1 * time.Hour),
		EndTime:   time.Now(),
	}

	_, ok := cache.Get(req0)
	if ok {
		t.Error("Expected metric_0 to be evicted")
	}

	// Last entry should still be in cache
	req3 := &types.QueryRequest{
		TenantID:  "test",
		Query:     "metric_3",
		StartTime: time.Now().Add(-1 * time.Hour),
		EndTime:   time.Now(),
	}

	_, ok = cache.Get(req3)
	if !ok {
		t.Error("Expected metric_3 to be in cache")
	}
}

func TestCacheStats(t *testing.T) {
	cache := NewQueryCache(100, 1*time.Minute)

	stats := cache.Stats()
	if stats.Size != 0 {
		t.Errorf("Expected initial size 0, got %d", stats.Size)
	}

	// Add some entries
	result := &types.QueryResult{Series: []types.Series{}}
	for i := 0; i < 10; i++ {
		req := &types.QueryRequest{
			TenantID:  "test",
			Query:     fmt.Sprintf("metric_%d", i),
			StartTime: time.Now().Add(-1 * time.Hour),
			EndTime:   time.Now(),
		}
		cache.Put(req, result)
	}

	stats = cache.Stats()
	if stats.Size != 10 {
		t.Errorf("Expected size 10, got %d", stats.Size)
	}

	if stats.Capacity != 100 {
		t.Errorf("Expected capacity 100, got %d", stats.Capacity)
	}
}

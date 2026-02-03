package storage

import (
	"testing"

	"github.com/vjranagit/prometheus/pkg/types"
)

func TestIndexAddSeries(t *testing.T) {
	idx := NewIndex()

	metric := types.Metric{
		Name: "http_requests_total",
		Labels: map[string]string{
			"method": "GET",
			"status": "200",
		},
	}

	id, err := idx.AddSeries(&metric)
	if err != nil {
		t.Fatalf("Failed to add series: %v", err)
	}

	if id == 0 {
		t.Error("Expected non-zero series ID")
	}

	// Adding same series again should return same ID
	id2, err := idx.AddSeries(&metric)
	if err != nil {
		t.Fatalf("Failed to add series again: %v", err)
	}

	if id != id2 {
		t.Errorf("Expected same ID for duplicate series: %d != %d", id, id2)
	}

	if idx.SeriesCount() != 1 {
		t.Errorf("Expected 1 series, got %d", idx.SeriesCount())
	}
}

func TestIndexFindSeries(t *testing.T) {
	idx := NewIndex()

	// Add multiple series
	metrics := []types.Metric{
		{
			Name: "http_requests_total",
			Labels: map[string]string{
				"method": "GET",
				"status": "200",
			},
		},
		{
			Name: "http_requests_total",
			Labels: map[string]string{
				"method": "POST",
				"status": "200",
			},
		},
		{
			Name: "http_requests_total",
			Labels: map[string]string{
				"method": "GET",
				"status": "404",
			},
		},
	}

	for i := range metrics {
		_, err := idx.AddSeries(&metrics[i])
		if err != nil {
			t.Fatalf("Failed to add series %d: %v", i, err)
		}
	}

	// Test finding series by label
	found := idx.FindSeries(map[string]string{"method": "GET"})
	if len(found) != 2 {
		t.Errorf("Expected 2 series with method=GET, got %d", len(found))
	}

	// Test finding with multiple labels
	found = idx.FindSeries(map[string]string{
		"method": "GET",
		"status": "200",
	})
	if len(found) != 1 {
		t.Errorf("Expected 1 series with method=GET and status=200, got %d", len(found))
	}

	// Test finding non-existent
	found = idx.FindSeries(map[string]string{"method": "DELETE"})
	if len(found) != 0 {
		t.Errorf("Expected 0 series with method=DELETE, got %d", len(found))
	}
}

func TestIndexUpdateTimeRange(t *testing.T) {
	idx := NewIndex()

	metric := types.Metric{
		Name: "cpu_usage",
		Labels: map[string]string{
			"host": "server1",
		},
	}

	id, err := idx.AddSeries(&metric)
	if err != nil {
		t.Fatalf("Failed to add series: %v", err)
	}

	// Update time range
	err = idx.UpdateTimeRange(id, 1000, 2000)
	if err != nil {
		t.Fatalf("Failed to update time range: %v", err)
	}

	meta, ok := idx.GetSeries(id)
	if !ok {
		t.Fatal("Series not found")
	}

	if meta.MinTime != 1000 {
		t.Errorf("Expected MinTime=1000, got %d", meta.MinTime)
	}
	if meta.MaxTime != 2000 {
		t.Errorf("Expected MaxTime=2000, got %d", meta.MaxTime)
	}

	// Update with expanded range
	err = idx.UpdateTimeRange(id, 500, 2500)
	if err != nil {
		t.Fatalf("Failed to update time range: %v", err)
	}

	meta, _ = idx.GetSeries(id)
	if meta.MinTime != 500 {
		t.Errorf("Expected MinTime=500, got %d", meta.MinTime)
	}
	if meta.MaxTime != 2500 {
		t.Errorf("Expected MaxTime=2500, got %d", meta.MaxTime)
	}
}

func TestCalculateFingerprint(t *testing.T) {
	metric1 := types.Metric{
		Name: "test_metric",
		Labels: map[string]string{
			"a": "1",
			"b": "2",
		},
	}

	metric2 := types.Metric{
		Name: "test_metric",
		Labels: map[string]string{
			"b": "2", // Different order
			"a": "1",
		},
	}

	fp1 := calculateFingerprint(&metric1)
	fp2 := calculateFingerprint(&metric2)

	if fp1 != fp2 {
		t.Error("Fingerprints should be same regardless of label order")
	}

	metric3 := types.Metric{
		Name: "test_metric",
		Labels: map[string]string{
			"a": "1",
			"b": "3", // Different value
		},
	}

	fp3 := calculateFingerprint(&metric3)
	if fp1 == fp3 {
		t.Error("Different metrics should have different fingerprints")
	}
}

func TestIndexSerialize(t *testing.T) {
	idx := NewIndex()

	// Add some series
	metrics := []types.Metric{
		{
			Name: "metric1",
			Labels: map[string]string{
				"label1": "value1",
			},
		},
		{
			Name: "metric2",
			Labels: map[string]string{
				"label2": "value2",
			},
		},
	}

	for i := range metrics {
		_, err := idx.AddSeries(&metrics[i])
		if err != nil {
			t.Fatalf("Failed to add series: %v", err)
		}
	}

	// Serialize
	data, err := idx.Serialize()
	if err != nil {
		t.Fatalf("Failed to serialize index: %v", err)
	}

	if len(data) == 0 {
		t.Error("Expected non-empty serialized data")
	}
}

func BenchmarkIndexAddSeries(b *testing.B) {
	idx := NewIndex()

	metric := types.Metric{
		Name: "http_requests_total",
		Labels: map[string]string{
			"method": "GET",
			"status": "200",
			"path":   "/api/v1/query",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.AddSeries(&metric)
	}
}

func BenchmarkIndexFindSeries(b *testing.B) {
	idx := NewIndex()

	// Add 10000 series
	for i := 0; i < 10000; i++ {
		metric := types.Metric{
			Name: "http_requests_total",
			Labels: map[string]string{
				"method": "GET",
				"status": "200",
			},
		}
		idx.AddSeries(&metric)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.FindSeries(map[string]string{"method": "GET"})
	}
}

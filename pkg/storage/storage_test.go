package storage

import (
	"context"
	"testing"
	"time"

	"github.com/vjranagit/prometheus/pkg/types"
)

func TestBadgerStorageWriteAndQuery(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()

	// Create storage
	cfg := &Config{
		Path:             tmpDir,
		RetentionDays:    30,
		CompressionLevel: 3,
	}

	store, err := NewStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Write test data
	ctx := context.Background()
	now := time.Now()

	writeReq := &types.WriteRequest{
		TenantID: "test-tenant",
		Series: []types.Series{
			{
				Metric: types.Metric{
					Name: "http_requests_total",
					Labels: map[string]string{
						"method": "GET",
						"status": "200",
					},
				},
				Samples: []types.Sample{
					{Timestamp: now.Add(-2 * time.Hour), Value: 100.0},
					{Timestamp: now.Add(-1 * time.Hour), Value: 150.0},
					{Timestamp: now, Value: 200.0},
				},
			},
		},
	}

	if err := store.Write(ctx, writeReq); err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Query data
	queryReq := &types.QueryRequest{
		TenantID:  "test-tenant",
		Query:     "http_requests_total",
		StartTime: now.Add(-3 * time.Hour),
		EndTime:   now.Add(1 * time.Hour),
	}

	result, err := store.Query(ctx, queryReq)
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}

	// Verify results
	if len(result.Series) != 1 {
		t.Errorf("Expected 1 series, got %d", len(result.Series))
	}

	if len(result.Series) > 0 {
		if len(result.Series[0].Samples) != 3 {
			t.Errorf("Expected 3 samples, got %d", len(result.Series[0].Samples))
		}
	}
}

func TestBadgerStorageMultiTenant(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &Config{
		Path:             tmpDir,
		CompressionLevel: 3,
	}

	store, err := NewStorage(cfg)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now()

	// Write for tenant A
	writeReqA := &types.WriteRequest{
		TenantID: "tenant-a",
		Series: []types.Series{
			{
				Metric: types.Metric{
					Name: "cpu_usage",
					Labels: map[string]string{
						"host": "server1",
					},
				},
				Samples: []types.Sample{
					{Timestamp: now, Value: 50.0},
				},
			},
		},
	}

	// Write for tenant B
	writeReqB := &types.WriteRequest{
		TenantID: "tenant-b",
		Series: []types.Series{
			{
				Metric: types.Metric{
					Name: "cpu_usage",
					Labels: map[string]string{
						"host": "server2",
					},
				},
				Samples: []types.Sample{
					{Timestamp: now, Value: 75.0},
				},
			},
		},
	}

	if err := store.Write(ctx, writeReqA); err != nil {
		t.Fatalf("Failed to write tenant A: %v", err)
	}

	if err := store.Write(ctx, writeReqB); err != nil {
		t.Fatalf("Failed to write tenant B: %v", err)
	}

	// Query tenant A
	queryReqA := &types.QueryRequest{
		TenantID:  "tenant-a",
		Query:     "cpu_usage",
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(1 * time.Hour),
	}

	resultA, err := store.Query(ctx, queryReqA)
	if err != nil {
		t.Fatalf("Failed to query tenant A: %v", err)
	}

	if len(resultA.Series) != 1 {
		t.Errorf("Tenant A: expected 1 series, got %d", len(resultA.Series))
	}

	// Query tenant B
	queryReqB := &types.QueryRequest{
		TenantID:  "tenant-b",
		Query:     "cpu_usage",
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(1 * time.Hour),
	}

	resultB, err := store.Query(ctx, queryReqB)
	if err != nil {
		t.Fatalf("Failed to query tenant B: %v", err)
	}

	if len(resultB.Series) != 1 {
		t.Errorf("Tenant B: expected 1 series, got %d", len(resultB.Series))
	}
}

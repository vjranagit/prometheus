package storage

import (
	"testing"
	"time"

	"github.com/vjranagit/prometheus/pkg/types"
)

func TestWAL(t *testing.T) {
	tmpDir := t.TempDir()

	// Create WAL
	wal, err := NewWAL(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}
	defer wal.Close()

	// Write test entry
	req := &types.WriteRequest{
		TenantID: "test",
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

	if err := wal.Append(req); err != nil {
		t.Fatalf("Failed to append to WAL: %v", err)
	}

	if err := wal.Flush(); err != nil {
		t.Fatalf("Failed to flush WAL: %v", err)
	}

	wal.Close()

	// Replay WAL
	replayed := false
	err = ReplayWAL(tmpDir, func(r *types.WriteRequest) error {
		replayed = true
		if r.TenantID != "test" {
			t.Errorf("Expected tenant 'test', got %s", r.TenantID)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("WAL replay failed: %v", err)
	}

	if !replayed {
		t.Error("WAL was not replayed")
	}
}

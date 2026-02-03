package storage

import (
	"math"
	"testing"
	"time"
)

func TestCompressTimestamps(t *testing.T) {
	comp, err := NewCompressor(2)
	if err != nil {
		t.Fatalf("Failed to create compressor: %v", err)
	}
	defer comp.Close()

	// Create regular interval timestamps
	now := time.Now().Unix()
	timestamps := make([]int64, 100)
	for i := 0; i < 100; i++ {
		timestamps[i] = now + int64(i*60) // 1 minute intervals
	}

	// Compress
	compressed, err := comp.CompressTimestamps(timestamps)
	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}

	// Should achieve good compression on regular intervals
	originalSize := len(timestamps) * 8
	if len(compressed) >= originalSize {
		t.Errorf("Compression ineffective: original=%d, compressed=%d",
			originalSize, len(compressed))
	}

	// Decompress
	decompressed, err := comp.DecompressTimestamps(compressed, len(timestamps))
	if err != nil {
		t.Fatalf("Decompression failed: %v", err)
	}

	// Verify
	if len(decompressed) != len(timestamps) {
		t.Fatalf("Length mismatch: expected %d, got %d",
			len(timestamps), len(decompressed))
	}

	for i := range timestamps {
		if timestamps[i] != decompressed[i] {
			t.Errorf("Timestamp mismatch at %d: expected %d, got %d",
				i, timestamps[i], decompressed[i])
		}
	}
}

func TestCompressValues(t *testing.T) {
	comp, err := NewCompressor(2)
	if err != nil {
		t.Fatalf("Failed to create compressor: %v", err)
	}
	defer comp.Close()

	// Create values with small variations (common in metrics)
	values := make([]float64, 100)
	base := 100.0
	for i := 0; i < 100; i++ {
		values[i] = base + math.Sin(float64(i)*0.1)*10
	}

	// Compress
	compressed, err := comp.CompressValues(values)
	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}

	// Decompress
	decompressed, err := comp.DecompressValues(compressed, len(values))
	if err != nil {
		t.Fatalf("Decompression failed: %v", err)
	}

	// Verify
	if len(decompressed) != len(values) {
		t.Fatalf("Length mismatch: expected %d, got %d",
			len(values), len(decompressed))
	}

	for i := range values {
		if values[i] != decompressed[i] {
			t.Errorf("Value mismatch at %d: expected %f, got %f",
				i, values[i], decompressed[i])
		}
	}
}

func TestCompressionLevels(t *testing.T) {
	testCases := []struct {
		level       int
		description string
	}{
		{1, "fastest"},
		{2, "default"},
		{3, "better"},
		{4, "best"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			comp, err := NewCompressor(tc.level)
			if err != nil {
				t.Fatalf("Failed to create compressor at level %d: %v",
					tc.level, err)
			}
			defer comp.Close()

			// Test with sample data
			values := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
			compressed, err := comp.CompressValues(values)
			if err != nil {
				t.Fatalf("Compression failed: %v", err)
			}

			decompressed, err := comp.DecompressValues(compressed, len(values))
			if err != nil {
				t.Fatalf("Decompression failed: %v", err)
			}

			// Verify correctness
			for i := range values {
				if values[i] != decompressed[i] {
					t.Errorf("Mismatch at index %d", i)
				}
			}
		})
	}
}

func BenchmarkCompressTimestamps(b *testing.B) {
	comp, _ := NewCompressor(2)
	defer comp.Close()

	// Generate timestamps
	now := time.Now().Unix()
	timestamps := make([]int64, 1000)
	for i := 0; i < 1000; i++ {
		timestamps[i] = now + int64(i*60)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressTimestamps(timestamps)
	}
}

func BenchmarkCompressValues(b *testing.B) {
	comp, _ := NewCompressor(2)
	defer comp.Close()

	// Generate values
	values := make([]float64, 1000)
	for i := 0; i < 1000; i++ {
		values[i] = 100.0 + math.Sin(float64(i)*0.1)*10
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = comp.CompressValues(values)
	}
}

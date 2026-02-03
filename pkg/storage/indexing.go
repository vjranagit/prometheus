package storage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/vjranagit/prometheus/pkg/types"
)

// Index manages the time-series index
type Index struct {
	// Maps metric fingerprint to series metadata
	series map[uint64]*seriesMetadata
	// Inverted index: label name -> label value -> series IDs
	labelIndex map[string]map[string][]uint64
}

// seriesMetadata holds metadata about a single series
type seriesMetadata struct {
	ID      uint64
	Metric  types.Metric
	MinTime int64
	MaxTime int64
}

// NewIndex creates a new index
func NewIndex() *Index {
	return &Index{
		series:     make(map[uint64]*seriesMetadata),
		labelIndex: make(map[string]map[string][]uint64),
	}
}

// AddSeries adds a series to the index
func (idx *Index) AddSeries(metric *types.Metric) (uint64, error) {
	fingerprint := calculateFingerprint(metric)

	// Check if series already exists
	if meta, exists := idx.series[fingerprint]; exists {
		return meta.ID, nil
	}

	// Create new series metadata
	meta := &seriesMetadata{
		ID:      fingerprint,
		Metric:  *metric,
		MinTime: 0,
		MaxTime: 0,
	}

	idx.series[fingerprint] = meta

	// Update inverted index
	// Index metric name as __name__ label
	if idx.labelIndex["__name__"] == nil {
		idx.labelIndex["__name__"] = make(map[string][]uint64)
	}
	idx.labelIndex["__name__"][metric.Name] = append(idx.labelIndex["__name__"][metric.Name], fingerprint)
	
	// Index other labels
	for name, value := range metric.Labels {
		if idx.labelIndex[name] == nil {
			idx.labelIndex[name] = make(map[string][]uint64)
		}
		idx.labelIndex[name][value] = append(idx.labelIndex[name][value], fingerprint)
	}

	return fingerprint, nil
}

// GetSeries retrieves series metadata by ID
func (idx *Index) GetSeries(id uint64) (*seriesMetadata, bool) {
	meta, ok := idx.series[id]
	return meta, ok
}

// FindSeries finds series matching label selectors
func (idx *Index) FindSeries(labelSelectors map[string]string) []uint64 {
	if len(labelSelectors) == 0 {
		// Return all series
		result := make([]uint64, 0, len(idx.series))
		for id := range idx.series {
			result = append(result, id)
		}
		return result
	}

	// Find intersection of matching series across all selectors
	var result []uint64
	first := true

	for labelName, labelValue := range labelSelectors {
		valueMap, ok := idx.labelIndex[labelName]
		if !ok {
			return nil // Label name doesn't exist
		}

		seriesIDs, ok := valueMap[labelValue]
		if !ok {
			return nil // Label value doesn't exist
		}

		if first {
			result = append([]uint64(nil), seriesIDs...)
			first = false
		} else {
			result = intersect(result, seriesIDs)
		}

		if len(result) == 0 {
			return nil // No matches
		}
	}

	return result
}

// UpdateTimeRange updates the time range for a series
func (idx *Index) UpdateTimeRange(id uint64, minTime, maxTime int64) error {
	meta, ok := idx.series[id]
	if !ok {
		return fmt.Errorf("series %d not found", id)
	}

	if meta.MinTime == 0 || minTime < meta.MinTime {
		meta.MinTime = minTime
	}
	if meta.MaxTime == 0 || maxTime > meta.MaxTime {
		meta.MaxTime = maxTime
	}

	return nil
}

// SeriesCount returns the number of indexed series
func (idx *Index) SeriesCount() int {
	return len(idx.series)
}

// calculateFingerprint generates a unique fingerprint for a metric
func calculateFingerprint(metric *types.Metric) uint64 {
	// Sort label keys for consistent fingerprinting
	keys := make([]string, 0, len(metric.Labels))
	for k := range metric.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build fingerprint string
	buf := new(bytes.Buffer)
	buf.WriteString(metric.Name)

	for _, k := range keys {
		buf.WriteByte(0) // Separator
		buf.WriteString(k)
		buf.WriteByte(0)
		buf.WriteString(metric.Labels[k])
	}

	// Hash to uint64
	return hashBytes(buf.Bytes())
}

// hashBytes computes a simple hash of bytes
// In production, use a proper hash function like xxhash
func hashBytes(data []byte) uint64 {
	var hash uint64 = 14695981039346656037 // FNV-1a offset basis
	for _, b := range data {
		hash ^= uint64(b)
		hash *= 1099511628211 // FNV-1a prime
	}
	return hash
}

// intersect finds common elements in two sorted slices
func intersect(a, b []uint64) []uint64 {
	sort.Slice(a, func(i, j int) bool { return a[i] < a[j] })
	sort.Slice(b, func(i, j int) bool { return b[i] < b[j] })

	result := make([]uint64, 0)
	i, j := 0, 0

	for i < len(a) && j < len(b) {
		if a[i] < b[j] {
			i++
		} else if a[i] > b[j] {
			j++
		} else {
			result = append(result, a[i])
			i++
			j++
		}
	}

	return result
}

// Serialize serializes the index to bytes
func (idx *Index) Serialize() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write series count
	if err := binary.Write(buf, binary.LittleEndian, uint32(len(idx.series))); err != nil {
		return nil, err
	}

	// Write each series
	for _, meta := range idx.series {
		// Serialize series metadata
		// (Simplified - production version would use protobuf)
		if err := binary.Write(buf, binary.LittleEndian, meta.ID); err != nil {
			return nil, err
		}
		if err := binary.Write(buf, binary.LittleEndian, meta.MinTime); err != nil {
			return nil, err
		}
		if err := binary.Write(buf, binary.LittleEndian, meta.MaxTime); err != nil {
			return nil, err
		}

		// Write metric name
		nameBytes := []byte(meta.Metric.Name)
		if err := binary.Write(buf, binary.LittleEndian, uint16(len(nameBytes))); err != nil {
			return nil, err
		}
		if _, err := buf.Write(nameBytes); err != nil {
			return nil, err
		}

		// Write labels
		if err := binary.Write(buf, binary.LittleEndian, uint16(len(meta.Metric.Labels))); err != nil {
			return nil, err
		}
		for k, v := range meta.Metric.Labels {
			// Write key
			kBytes := []byte(k)
			if err := binary.Write(buf, binary.LittleEndian, uint16(len(kBytes))); err != nil {
				return nil, err
			}
			if _, err := buf.Write(kBytes); err != nil {
				return nil, err
			}
			// Write value
			vBytes := []byte(v)
			if err := binary.Write(buf, binary.LittleEndian, uint16(len(vBytes))); err != nil {
				return nil, err
			}
			if _, err := buf.Write(vBytes); err != nil {
				return nil, err
			}
		}
	}

	return buf.Bytes(), nil
}

// Clear clears the index
func (idx *Index) Clear() {
	idx.series = make(map[uint64]*seriesMetadata)
	idx.labelIndex = make(map[string]map[string][]uint64)
}

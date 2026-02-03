package storage

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/vjranagit/prometheus/pkg/types"
)

// Storage interface defines the contract for time-series storage
type Storage interface {
	// Write writes samples to storage
	Write(ctx context.Context, req *types.WriteRequest) error

	// Query executes a query and returns results
	Query(ctx context.Context, req *types.QueryRequest) (*types.QueryResult, error)

	// Close closes the storage
	Close() error
}

// Config holds storage configuration
type Config struct {
	Path              string
	RetentionDays     int
	CompressionLevel  int
	MaxOpenFiles      int
	EnableWAL         bool
}

// DefaultConfig returns default storage configuration
func DefaultConfig() *Config {
	return &Config{
		Path:              "./data",
		RetentionDays:     30,
		CompressionLevel:  3,
		MaxOpenFiles:      1000,
		EnableWAL:         true,
	}
}

// badgerStorage implements Storage using BadgerDB
type badgerStorage struct {
	cfg        *Config
	db         *badger.DB
	index      *Index
	compressor *Compressor
	mu         sync.RWMutex
}

// NewStorage creates a new storage instance
func NewStorage(cfg *Config) (Storage, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Initialize BadgerDB
	opts := badger.DefaultOptions(filepath.Join(cfg.Path, "badger"))
	opts.Logger = nil // Disable BadgerDB logging
	
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open BadgerDB: %w", err)
	}

	// Create compressor
	compressor, err := NewCompressor(cfg.CompressionLevel)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create compressor: %w", err)
	}

	s := &badgerStorage{
		cfg:        cfg,
		db:         db,
		index:      NewIndex(),
		compressor: compressor,
	}

	return s, nil
}

// Write implements Storage.Write
func (s *badgerStorage) Write(ctx context.Context, req *types.WriteRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Process each series
	for _, series := range req.Series {
		// Add series to index
		seriesID, err := s.index.AddSeries(&series.Metric)
		if err != nil {
			return fmt.Errorf("failed to index series: %w", err)
		}

		// Group samples by time block (1 hour blocks)
		blocks := s.groupSamplesByBlock(series.Samples)

		// Write each block
		for blockTime, samples := range blocks {
			if err := s.writeBlock(req.TenantID, seriesID, blockTime, samples); err != nil {
				return fmt.Errorf("failed to write block: %w", err)
			}
		}
	}

	return nil
}

// groupSamplesByBlock groups samples into 1-hour blocks
func (s *badgerStorage) groupSamplesByBlock(samples []types.Sample) map[int64][]types.Sample {
	blocks := make(map[int64][]types.Sample)
	
	for _, sample := range samples {
		// Round to hour
		blockTime := sample.Timestamp.Truncate(time.Hour).Unix()
		blocks[blockTime] = append(blocks[blockTime], sample)
	}
	
	return blocks
}

// writeBlock writes a block of samples to BadgerDB
func (s *badgerStorage) writeBlock(tenantID string, seriesID uint64, blockTime int64, samples []types.Sample) error {
	// Extract timestamps and values
	timestamps := make([]int64, len(samples))
	values := make([]float64, len(samples))
	
	for i, sample := range samples {
		timestamps[i] = sample.Timestamp.Unix()
		values[i] = sample.Value
	}

	// Compress data
	compressedTS, err := s.compressor.CompressTimestamps(timestamps)
	if err != nil {
		return fmt.Errorf("failed to compress timestamps: %w", err)
	}

	compressedVals, err := s.compressor.CompressValues(values)
	if err != nil {
		return fmt.Errorf("failed to compress values: %w", err)
	}

	// Create value payload
	payload := &blockPayload{
		Count:              len(samples),
		CompressedTS:       compressedTS,
		CompressedValues:   compressedVals,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Generate key
	key := generateKey(tenantID, seriesID, blockTime)

	// Write to BadgerDB
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, payloadBytes)
	})
}

type blockPayload struct {
	Count            int
	CompressedTS     []byte
	CompressedValues []byte
}

// Query implements Storage.Query
func (s *badgerStorage) Query(ctx context.Context, req *types.QueryRequest) (*types.QueryResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Simple query: just return all series matching the query string as a label selector
	// For production, this should parse PromQL
	labelSelectors := parseLabelSelectors(req.Query)
	
	// Find matching series
	seriesIDs := s.index.FindSeries(labelSelectors)
	
	result := &types.QueryResult{
		Series: make([]types.Series, 0, len(seriesIDs)),
	}

	// Read data for each series
	for _, seriesID := range seriesIDs {
		meta, ok := s.index.GetSeries(seriesID)
		if !ok {
			continue
		}

		series := types.Series{
			Metric:  meta.Metric,
			Samples: []types.Sample{},
		}

		// Read blocks covering the time range
		startBlock := req.StartTime.Truncate(time.Hour).Unix()
		endBlock := req.EndTime.Truncate(time.Hour).Unix()

		for blockTime := startBlock; blockTime <= endBlock; blockTime += 3600 {
			samples, err := s.readBlock(req.TenantID, seriesID, blockTime)
			if err != nil {
				continue // Block might not exist
			}

			// Filter samples within time range
			for _, sample := range samples {
				if sample.Timestamp.After(req.StartTime) && sample.Timestamp.Before(req.EndTime) {
					series.Samples = append(series.Samples, sample)
				}
			}
		}

		if len(series.Samples) > 0 {
			result.Series = append(result.Series, series)
		}
	}

	return result, nil
}

// readBlock reads a block of samples from BadgerDB
func (s *badgerStorage) readBlock(tenantID string, seriesID uint64, blockTime int64) ([]types.Sample, error) {
	key := generateKey(tenantID, seriesID, blockTime)

	var payloadBytes []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			payloadBytes = append([]byte{}, val...)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	// Unmarshal payload
	var payload blockPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	// Decompress data
	timestamps, err := s.compressor.DecompressTimestamps(payload.CompressedTS, payload.Count)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress timestamps: %w", err)
	}

	values, err := s.compressor.DecompressValues(payload.CompressedValues, payload.Count)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress values: %w", err)
	}

	// Build samples
	samples := make([]types.Sample, payload.Count)
	for i := 0; i < payload.Count; i++ {
		samples[i] = types.Sample{
			Timestamp: time.Unix(timestamps[i], 0),
			Value:     values[i],
		}
	}

	return samples, nil
}

// Close implements Storage.Close
func (s *badgerStorage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// generateKey generates a storage key for a time block
func generateKey(tenantID string, seriesID uint64, blockTime int64) []byte {
	buf := new(bytes.Buffer)
	
	// Write tenant ID
	buf.WriteString(tenantID)
	buf.WriteByte('/')
	
	// Write series ID
	binary.Write(buf, binary.BigEndian, seriesID)
	buf.WriteByte('/')
	
	// Write block time
	binary.Write(buf, binary.BigEndian, blockTime)
	
	return buf.Bytes()
}

// parseLabelSelectors parses a simple query string into label selectors
// Format: metric_name{label1="value1",label2="value2"}
// For simplicity, just parse the metric name for now
func parseLabelSelectors(query string) map[string]string {
	// Simple implementation: treat query as metric name
	if query == "" {
		return nil
	}
	
	return map[string]string{
		"__name__": query,
	}
}


package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vjranagit/prometheus/pkg/types"
)

// WAL implements a Write-Ahead Log for durability
type WAL struct {
	path       string
	file       *os.File
	writer     *bufio.Writer
	mu         sync.Mutex
	flushTimer *time.Timer
}

// WALEntry represents a single WAL entry
type WALEntry struct {
	Timestamp time.Time           `json:"timestamp"`
	TenantID  string              `json:"tenant_id"`
	Series    []types.Series      `json:"series"`
}

// NewWAL creates a new Write-Ahead Log
func NewWAL(dataPath string) (*WAL, error) {
	walPath := filepath.Join(dataPath, "wal")
	if err := os.MkdirAll(walPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create WAL directory: %w", err)
	}

	// Open or create WAL file
	filename := filepath.Join(walPath, fmt.Sprintf("wal-%d.log", time.Now().Unix()))
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open WAL file: %w", err)
	}

	wal := &WAL{
		path:   walPath,
		file:   file,
		writer: bufio.NewWriter(file),
	}

	// Start auto-flush timer (flush every 1 second)
	wal.flushTimer = time.AfterFunc(1*time.Second, wal.autoFlush)

	return wal, nil
}

// Append appends a write request to the WAL
func (w *WAL) Append(req *types.WriteRequest) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	entry := WALEntry{
		Timestamp: time.Now(),
		TenantID:  req.TenantID,
		Series:    req.Series,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal WAL entry: %w", err)
	}

	// Write entry with newline
	if _, err := w.writer.Write(data); err != nil {
		return fmt.Errorf("failed to write to WAL: %w", err)
	}
	if err := w.writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return nil
}

// Flush flushes the WAL to disk
func (w *WAL) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush WAL: %w", err)
	}

	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync WAL: %w", err)
	}

	return nil
}

// autoFlush periodically flushes the WAL
func (w *WAL) autoFlush() {
	w.Flush()
	w.mu.Lock()
	w.flushTimer.Reset(1 * time.Second)
	w.mu.Unlock()
}

// Close closes the WAL
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.flushTimer != nil {
		w.flushTimer.Stop()
	}

	if err := w.writer.Flush(); err != nil {
		return err
	}

	if err := w.file.Sync(); err != nil {
		return err
	}

	return w.file.Close()
}

// Replay replays WAL entries for recovery
func ReplayWAL(dataPath string, handler func(*types.WriteRequest) error) error {
	walPath := filepath.Join(dataPath, "wal")
	
	entries, err := os.ReadDir(walPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No WAL to replay
		}
		return fmt.Errorf("failed to read WAL directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := filepath.Join(walPath, entry.Name())
		if err := replayWALFile(filename, handler); err != nil {
			return fmt.Errorf("failed to replay %s: %w", filename, err)
		}

		// Remove replayed WAL file
		os.Remove(filename)
	}

	return nil
}

// replayWALFile replays a single WAL file
func replayWALFile(filename string, handler func(*types.WriteRequest) error) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry WALEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return fmt.Errorf("failed to unmarshal WAL entry: %w", err)
		}

		req := &types.WriteRequest{
			TenantID: entry.TenantID,
			Series:   entry.Series,
		}

		if err := handler(req); err != nil {
			return fmt.Errorf("failed to replay entry: %w", err)
		}
	}

	return scanner.Err()
}

// BatchWriter buffers writes for batch processing
type BatchWriter struct {
	storage    *badgerStorage
	wal        *WAL
	buffer     []*types.WriteRequest
	bufferSize int
	mu         sync.Mutex
	flushTimer *time.Timer
	ctx        chan struct{}
}

// NewBatchWriter creates a new batch writer
func NewBatchWriter(storage *badgerStorage, wal *WAL, bufferSize int) *BatchWriter {
	bw := &BatchWriter{
		storage:    storage,
		wal:        wal,
		buffer:     make([]*types.WriteRequest, 0, bufferSize),
		bufferSize: bufferSize,
		ctx:        make(chan struct{}),
	}

	// Start auto-flush timer (flush every 100ms or when buffer is full)
	bw.flushTimer = time.AfterFunc(100*time.Millisecond, bw.autoFlush)

	return bw
}

// Write buffers a write request
func (bw *BatchWriter) Write(req *types.WriteRequest) error {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	// Append to WAL first for durability
	if bw.wal != nil {
		if err := bw.wal.Append(req); err != nil {
			return fmt.Errorf("WAL append failed: %w", err)
		}
	}

	// Add to buffer
	bw.buffer = append(bw.buffer, req)

	// Flush if buffer is full
	if len(bw.buffer) >= bw.bufferSize {
		return bw.flushLocked()
	}

	return nil
}

// Flush flushes the buffer
func (bw *BatchWriter) Flush() error {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	return bw.flushLocked()
}

// flushLocked flushes the buffer (must hold lock)
func (bw *BatchWriter) flushLocked() error {
	if len(bw.buffer) == 0 {
		return nil
	}

	// Combine all write requests into batches per tenant
	tenantBatches := make(map[string][]types.Series)
	
	for _, req := range bw.buffer {
		tenantBatches[req.TenantID] = append(tenantBatches[req.TenantID], req.Series...)
	}

	// Write each tenant's batch
	for tenantID, series := range tenantBatches {
		batchReq := &types.WriteRequest{
			TenantID: tenantID,
			Series:   series,
		}

		if err := bw.storage.writeDirect(batchReq); err != nil {
			return fmt.Errorf("batch write failed: %w", err)
		}
	}

	// Clear buffer
	bw.buffer = bw.buffer[:0]

	return nil
}

// autoFlush periodically flushes the buffer
func (bw *BatchWriter) autoFlush() {
	bw.Flush()
	bw.mu.Lock()
	bw.flushTimer.Reset(100 * time.Millisecond)
	bw.mu.Unlock()
}

// Close closes the batch writer
func (bw *BatchWriter) Close() error {
	if bw.flushTimer != nil {
		bw.flushTimer.Stop()
	}

	close(bw.ctx)

	// Final flush
	return bw.Flush()
}

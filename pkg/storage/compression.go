package storage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/klauspost/compress/zstd"
)

// Compressor handles data compression for time-series data
type Compressor struct {
	encoder *zstd.Encoder
	decoder *zstd.Decoder
}

// NewCompressor creates a new compressor
func NewCompressor(level int) (*Compressor, error) {
	// Create encoder with specified compression level
	encLevel := zstd.SpeedDefault
	switch level {
	case 1:
		encLevel = zstd.SpeedFastest
	case 2:
		encLevel = zstd.SpeedDefault
	case 3:
		encLevel = zstd.SpeedBetterCompression
	case 4:
		encLevel = zstd.SpeedBestCompression
	}

	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(encLevel))
	if err != nil {
		return nil, fmt.Errorf("failed to create encoder: %w", err)
	}

	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create decoder: %w", err)
	}

	return &Compressor{
		encoder: encoder,
		decoder: decoder,
	}, nil
}

// CompressTimestamps compresses a series of timestamps using delta encoding + zstd
func (c *Compressor) CompressTimestamps(timestamps []int64) ([]byte, error) {
	if len(timestamps) == 0 {
		return nil, nil
	}

	buf := new(bytes.Buffer)

	// Write first timestamp as-is
	if err := binary.Write(buf, binary.LittleEndian, timestamps[0]); err != nil {
		return nil, err
	}

	// Write deltas (delta-of-delta encoding)
	var prevDelta int64 = 0
	for i := 1; i < len(timestamps); i++ {
		delta := timestamps[i] - timestamps[i-1]
		deltaOfDelta := delta - prevDelta

		// Variable-length encoding for delta-of-delta
		if err := binary.Write(buf, binary.LittleEndian, deltaOfDelta); err != nil {
			return nil, err
		}

		prevDelta = delta
	}

	// Compress the delta-encoded data
	compressed := c.encoder.EncodeAll(buf.Bytes(), make([]byte, 0, buf.Len()))
	return compressed, nil
}

// DecompressTimestamps decompresses timestamps
func (c *Compressor) DecompressTimestamps(data []byte, count int) ([]int64, error) {
	if len(data) == 0 {
		return nil, nil
	}

	// Decompress
	decompressed, err := c.decoder.DecodeAll(data, nil)
	if err != nil {
		return nil, fmt.Errorf("decompression failed: %w", err)
	}

	buf := bytes.NewReader(decompressed)
	timestamps := make([]int64, count)

	// Read first timestamp
	if err := binary.Read(buf, binary.LittleEndian, &timestamps[0]); err != nil {
		return nil, err
	}

	// Read deltas and reconstruct
	var prevDelta int64 = 0
	for i := 1; i < count; i++ {
		var deltaOfDelta int64
		if err := binary.Read(buf, binary.LittleEndian, &deltaOfDelta); err != nil {
			return nil, err
		}

		delta := deltaOfDelta + prevDelta
		timestamps[i] = timestamps[i-1] + delta
		prevDelta = delta
	}

	return timestamps, nil
}

// CompressValues compresses float64 values using XOR encoding + zstd
func (c *Compressor) CompressValues(values []float64) ([]byte, error) {
	if len(values) == 0 {
		return nil, nil
	}

	buf := new(bytes.Buffer)

	// Write first value as-is
	if err := binary.Write(buf, binary.LittleEndian, math.Float64bits(values[0])); err != nil {
		return nil, err
	}

	// XOR encoding for subsequent values
	prevBits := math.Float64bits(values[0])
	for i := 1; i < len(values); i++ {
		currentBits := math.Float64bits(values[i])
		xorBits := currentBits ^ prevBits

		if err := binary.Write(buf, binary.LittleEndian, xorBits); err != nil {
			return nil, err
		}

		prevBits = currentBits
	}

	// Compress the XOR-encoded data
	compressed := c.encoder.EncodeAll(buf.Bytes(), make([]byte, 0, buf.Len()))
	return compressed, nil
}

// DecompressValues decompresses float64 values
func (c *Compressor) DecompressValues(data []byte, count int) ([]float64, error) {
	if len(data) == 0 {
		return nil, nil
	}

	// Decompress
	decompressed, err := c.decoder.DecodeAll(data, nil)
	if err != nil {
		return nil, fmt.Errorf("decompression failed: %w", err)
	}

	buf := bytes.NewReader(decompressed)
	values := make([]float64, count)

	// Read first value
	var firstBits uint64
	if err := binary.Read(buf, binary.LittleEndian, &firstBits); err != nil {
		return nil, err
	}
	values[0] = math.Float64frombits(firstBits)

	// Decode XOR values
	prevBits := firstBits
	for i := 1; i < count; i++ {
		var xorBits uint64
		if err := binary.Read(buf, binary.LittleEndian, &xorBits); err != nil {
			return nil, err
		}

		currentBits := xorBits ^ prevBits
		values[i] = math.Float64frombits(currentBits)
		prevBits = currentBits
	}

	return values, nil
}

// Close closes the compressor resources
func (c *Compressor) Close() {
	if c.encoder != nil {
		c.encoder.Close()
	}
	if c.decoder != nil {
		c.decoder.Close()
	}
}

# Architecture Documentation

## Overview

This Prometheus fork is designed for high-performance time-series storage and querying with a focus on:
- Efficient compression and storage
- Multi-tenancy support
- Horizontal scalability
- Modern Go practices

## Components

### Storage Engine (`pkg/storage/`)

The storage engine is built on BadgerDB (LSM-tree) with custom compression.

**Key files:**
- `storage.go` - Main storage interface and BadgerDB integration
- `compression.go` - Delta encoding + Zstandard compression
- `indexing.go` - Inverted index for label-based queries

**Design decisions:**
1. **BadgerDB over custom storage:** Proven LSM-tree implementation reduces development time
2. **Zstandard compression:** Industry-standard, faster than custom algorithms
3. **In-memory inverted index:** Fast label lookups, persisted periodically

**Write path:**
```
HTTP POST → API Server → Storage.Write() → Compress → BadgerDB
                                         → Update Index
```

**Query path:**
```
HTTP GET → API Server → Storage.Query() → Index Lookup → BadgerDB Read → Decompress
```

### API Server (`pkg/api/`)

RESTful HTTP API compatible with Prometheus remote write protocol.

**Endpoints:**
- `POST /api/v1/write` - Ingest metrics
- `GET /api/v1/query` - Execute queries
- `GET /health` - Health check
- `GET /metrics` - Internal metrics

**Multi-tenancy:**
- Tenant ID via `X-Tenant-ID` header
- Separate storage namespaces per tenant
- Resource quotas (future)

### Configuration (`internal/config/`)

Environment-driven configuration with sensible defaults.

**Settings:**
- Server: listen address, timeouts
- Storage: path, retention, compression level
- All configurable via environment variables

## Data Model

### Metric

```go
type Metric struct {
    Name   string            // e.g., "http_requests_total"
    Labels map[string]string // e.g., {"method": "GET", "status": "200"}
}
```

### Sample

```go
type Sample struct {
    Timestamp time.Time
    Value     float64
}
```

### Series

```go
type Series struct {
    Metric  Metric
    Samples []Sample
}
```

## Storage Format

### Key Design

```
<tenant_id>/<series_fingerprint>/<timestamp_block>
```

Example:
```
default/8374982374982/20210101120000
```

### Value Format

```
[compressed_timestamps][compressed_values][metadata]
```

**Compression:**
1. Timestamps: Delta-of-delta encoding + Zstandard
2. Values: XOR encoding + Zstandard
3. Metadata: Series info, label counts

## Index Structure

### Inverted Index

```
Label Name → Label Value → [Series IDs]
```

Example:
```
"method" → {
    "GET":  [123, 456, 789]
    "POST": [234, 567]
}
```

### Series Metadata

```go
type seriesMetadata struct {
    ID      uint64       // Fingerprint
    Metric  Metric       // Full metric definition
    MinTime int64        // Earliest sample
    MaxTime int64        // Latest sample
}
```

## Performance Characteristics

### Compression Ratios

- Timestamps (regular intervals): ~80-90% compression
- Values (typical metrics): ~60-70% compression
- Overall: 5-7x compression vs raw storage

### Latency Targets

- Write: <1ms p99
- Point lookup: <10ms p99
- Range query: <100ms p99 (100K samples)

### Throughput

- Writes: 100K+ samples/sec (single node)
- Queries: 1K+ qps (simple queries)

## Comparison with Forks

### vs VictoriaMetrics

**Similarities:**
- High-performance storage focus
- Advanced compression
- Single-node simplicity

**Differences:**
- Uses BadgerDB vs custom storage engine
- Zstandard vs custom compression
- Simpler codebase, fewer features initially

### vs Cortex

**Similarities:**
- Multi-tenancy support
- Distributed architecture (planned)
- Cloud storage backends (planned)

**Differences:**
- Simpler microservices (fewer components)
- HTTP-first vs gRPC-first
- etcd vs memberlist for coordination

## Future Enhancements

### Phase 2 (2022)
- Multi-protocol ingestion (InfluxDB, OpenTelemetry)
- Query optimization
- Advanced caching

### Phase 3 (2023)
- Distributed components
- Cloud storage backends
- Kubernetes operator

### Phase 4 (2024)
- Production hardening
- Advanced monitoring
- Enterprise features

## Development Principles

1. **Simplicity over features:** Start minimal, add complexity only when needed
2. **Proven technologies:** Use established libraries (BadgerDB, Zstandard) over custom
3. **Test coverage:** Maintain >80% coverage with benchmarks
4. **Performance:** Profile and optimize hot paths
5. **Documentation:** Keep architecture docs updated

## References

- Prometheus: https://prometheus.io
- VictoriaMetrics: https://github.com/VictoriaMetrics/VictoriaMetrics
- Cortex: https://github.com/cortexproject/cortex
- BadgerDB: https://github.com/dgraph-io/badger
- Zstandard: https://github.com/klauspost/compress

package types

import "time"

// Sample represents a single time-series sample
type Sample struct {
	Timestamp time.Time
	Value     float64
}

// Metric represents a time-series metric with labels
type Metric struct {
	Name   string
	Labels map[string]string
}

// Series represents a complete time-series
type Series struct {
	Metric  Metric
	Samples []Sample
}

// WriteRequest represents a write request to the storage engine
type WriteRequest struct {
	TenantID string
	Series   []Series
}

// QueryRequest represents a query request
type QueryRequest struct {
	TenantID  string
	Query     string
	StartTime time.Time
	EndTime   time.Time
}

// QueryResult represents query results
type QueryResult struct {
	Series []Series
	Error  error
}

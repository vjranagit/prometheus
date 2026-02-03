package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/vjranagit/prometheus/pkg/storage"
	"github.com/vjranagit/prometheus/pkg/types"
)

// Server implements the HTTP API server
type Server struct {
	storage storage.Storage
	addr    string
	server  *http.Server
}

// NewServer creates a new API server
func NewServer(addr string, store storage.Storage) *Server {
	return &Server{
		storage: store,
		addr:    addr,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Register handlers
	mux.HandleFunc("/api/v1/write", s.handleWrite)
	mux.HandleFunc("/api/v1/query", s.handleQuery)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/metrics", s.handleMetrics)

	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	return s.server.ListenAndServe()
}

// Stop stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// handleWrite handles remote write requests
func (s *Server) handleWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req types.WriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Extract tenant ID from header (multi-tenancy support)
	tenantID := r.Header.Get("X-Tenant-ID")
	if tenantID == "" {
		tenantID = "default"
	}
	req.TenantID = tenantID

	// Write to storage
	ctx := r.Context()
	if err := s.storage.Write(ctx, &req); err != nil {
		http.Error(w, fmt.Sprintf("Write failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
	})
}

// handleQuery handles query requests
func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		http.Error(w, "Missing query parameter", http.StatusBadRequest)
		return
	}

	// Parse time range
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	var startTime, endTime time.Time
	var err error

	if startStr != "" {
		startTime, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			http.Error(w, "Invalid start time", http.StatusBadRequest)
			return
		}
	} else {
		startTime = time.Now().Add(-1 * time.Hour)
	}

	if endStr != "" {
		endTime, err = time.Parse(time.RFC3339, endStr)
		if err != nil {
			http.Error(w, "Invalid end time", http.StatusBadRequest)
			return
		}
	} else {
		endTime = time.Now()
	}

	// Extract tenant ID
	tenantID := r.Header.Get("X-Tenant-ID")
	if tenantID == "" {
		tenantID = "default"
	}

	req := &types.QueryRequest{
		TenantID:  tenantID,
		Query:     query,
		StartTime: startTime,
		EndTime:   endTime,
	}

	// Execute query
	ctx := r.Context()
	result, err := s.storage.Query(ctx, req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Query failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

// handleMetrics handles internal metrics requests
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// TODO: Export internal metrics in Prometheus format
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "# Prometheus Fork Internal Metrics\n")
	fmt.Fprintf(w, "# Coming soon...\n")
}

package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"
)

// Handler provides health check HTTP endpoints
type Handler struct {
	mu        sync.RWMutex
	checks    map[string]Check
	readiness []Check
	liveness  []Check
	startTime time.Time
	monitor   *Monitor
}

// Check represents a health check function
type Check func(context.Context) error

// Response represents a health check response
type Response struct {
	Status    string                 `json:"status"`
	Checks    map[string]CheckResult `json:"checks,omitempty"`
	Metrics   *SystemMetrics         `json:"metrics,omitempty"`
	Uptime    string                 `json:"uptime"`
	Timestamp time.Time              `json:"timestamp"`
}

// CheckResult represents a single check result
type CheckResult struct {
	Status   string        `json:"status"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"duration_ms"`
}

// SystemMetrics represents system metrics
type SystemMetrics struct {
	CPU         CPUMetrics    `json:"cpu"`
	Memory      MemoryMetrics `json:"memory"`
	Goroutines  int           `json:"goroutines"`
	RequestRate float64       `json:"request_rate"`
	ErrorRate   float64       `json:"error_rate"`
}

// CPUMetrics represents CPU metrics
type CPUMetrics struct {
	Usage   float64   `json:"usage_percent"`
	Cores   int       `json:"cores"`
	LoadAvg []float64 `json:"load_avg,omitempty"`
}

// MemoryMetrics represents memory metrics
type MemoryMetrics struct {
	Used      uint64  `json:"used_bytes"`
	Available uint64  `json:"available_bytes"`
	Total     uint64  `json:"total_bytes"`
	Usage     float64 `json:"usage_percent"`
}

// NewHandler creates a new health handler
func NewHandler() *Handler {
	return &Handler{
		checks:    make(map[string]Check),
		startTime: time.Now(),
	}
}

// AddCheck adds a health check
func (h *Handler) AddCheck(name string, check Check) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks[name] = check
}

// AddReadinessCheck adds a readiness check
func (h *Handler) AddReadinessCheck(check Check) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.readiness = append(h.readiness, check)
}

// AddLivenessCheck adds a liveness check
func (h *Handler) AddLivenessCheck(check Check) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.liveness = append(h.liveness, check)
}

// SetMonitor sets the health monitor
func (h *Handler) SetMonitor(monitor *Monitor) {
	h.monitor = monitor
}

// HandleHealth handles /health endpoint
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	response := h.performChecks(ctx, h.checks)

	// Add system metrics
	response.Metrics = h.getSystemMetrics()

	// Set status code based on health
	statusCode := http.StatusOK
	if response.Status != "healthy" {
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// HandleReady handles /ready endpoint (Kubernetes readiness)
func (h *Handler) HandleReady(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Run readiness checks
	for _, check := range h.readiness {
		if err := check(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "not_ready",
				"error":  err.Error(),
			})
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ready",
	})
}

// HandleLive handles /live endpoint (Kubernetes liveness)
func (h *Handler) HandleLive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Run liveness checks
	for _, check := range h.liveness {
		if err := check(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "dead",
				"error":  err.Error(),
			})
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "alive",
	})
}

// HandleMetrics handles /metrics endpoint
func (h *Handler) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := h.getSystemMetrics()

	// Add component health if monitor is available
	var components map[string]*ComponentHealth
	if h.monitor != nil {
		components = h.monitor.GetAllComponentHealth()
	}

	response := map[string]interface{}{
		"system":     metrics,
		"components": components,
		"timestamp":  time.Now(),
		"uptime":     time.Since(h.startTime).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleStatus handles /status endpoint
func (h *Handler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	status := "healthy"

	// Get overall system status from monitor
	if h.monitor != nil {
		systemHealth := h.monitor.GetSystemHealth()
		status = string(systemHealth)
	}

	response := map[string]interface{}{
		"status":     status,
		"timestamp":  time.Now(),
		"uptime":     time.Since(h.startTime).String(),
		"version":    getVersion(),
		"build_time": getBuildTime(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// performChecks runs health checks
func (h *Handler) performChecks(ctx context.Context, checks map[string]Check) Response {
	h.mu.RLock()
	defer h.mu.RUnlock()

	results := make(map[string]CheckResult)
	overallStatus := "healthy"

	for name, check := range checks {
		start := time.Now()
		err := check(ctx)
		duration := time.Since(start)

		result := CheckResult{
			Status:   "healthy",
			Duration: duration / time.Millisecond,
		}

		if err != nil {
			result.Status = "unhealthy"
			result.Error = err.Error()
			overallStatus = "unhealthy"
		}

		results[name] = result
	}

	return Response{
		Status:    overallStatus,
		Checks:    results,
		Uptime:    time.Since(h.startTime).String(),
		Timestamp: time.Now(),
	}
}

// getSystemMetrics collects system metrics
func (h *Handler) getSystemMetrics() *SystemMetrics {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return &SystemMetrics{
		CPU: CPUMetrics{
			Cores: runtime.NumCPU(),
			Usage: getCPUUsage(),
		},
		Memory: MemoryMetrics{
			Used:      m.Alloc,
			Available: m.Sys - m.Alloc,
			Total:     m.Sys,
			Usage:     float64(m.Alloc) / float64(m.Sys) * 100,
		},
		Goroutines: runtime.NumGoroutine(),
	}
}

// RegisterHandlers registers all health endpoints on a mux
func (h *Handler) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.HandleHealth)
	mux.HandleFunc("/ready", h.HandleReady)
	mux.HandleFunc("/live", h.HandleLive)
	mux.HandleFunc("/metrics", h.HandleMetrics)
	mux.HandleFunc("/status", h.HandleStatus)
}

// getCPUUsage gets current CPU usage (placeholder)
func getCPUUsage() float64 {
	// In production, this would use actual CPU metrics
	return 30.0
}

// getVersion returns the service version
func getVersion() string {
	// In production, this would be set during build
	return "1.0.0"
}

// getBuildTime returns the build time
func getBuildTime() string {
	// In production, this would be set during build
	return time.Now().Format(time.RFC3339)
}

// Common health checks

// DatabaseCheck checks database connectivity
func DatabaseCheck(db interface{ Ping(context.Context) error }) Check {
	return func(ctx context.Context) error {
		return db.Ping(ctx)
	}
}

// HTTPCheck checks HTTP endpoint availability
func HTTPCheck(url string, timeout time.Duration) Check {
	return func(ctx context.Context) error {
		client := &http.Client{Timeout: timeout}
		resp, err := client.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unhealthy status: %d", resp.StatusCode)
		}
		return nil
	}
}

// DiskSpaceCheck checks available disk space
func DiskSpaceCheck(minFreeGB int) Check {
	return func(ctx context.Context) error {
		// In production, this would check actual disk space
		// For now, always return healthy
		return nil
	}
}

// MemoryCheck checks available memory
func MemoryCheck(maxUsagePercent float64) Check {
	return func(ctx context.Context) error {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		usage := float64(m.Alloc) / float64(m.Sys) * 100
		if usage > maxUsagePercent {
			return fmt.Errorf("memory usage too high: %.1f%%", usage)
		}
		return nil
	}
}

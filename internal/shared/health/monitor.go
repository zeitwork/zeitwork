package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zeitwork/zeitwork/internal/database"
)

// Monitor tracks health of all platform components
type Monitor struct {
	logger *slog.Logger
	db     *database.DB
	client *http.Client
	mu     sync.RWMutex

	// Component health states
	components map[string]*ComponentHealth

	// Alerting configuration
	alertHandlers []AlertHandler

	// Metrics collectors
	collectors map[string]MetricsCollector
}

// ComponentHealth represents health state of a component
type ComponentHealth struct {
	ID               string
	Name             string
	Type             ComponentType
	Status           HealthStatus
	LastCheck        time.Time
	LastHealthy      time.Time
	Metrics          *Metrics
	ErrorMessage     string
	ConsecutiveFails int
}

// ComponentType represents the type of component
type ComponentType string

const (
	ComponentNode         ComponentType = "node"
	ComponentInstance     ComponentType = "instance"
	ComponentService      ComponentType = "service"
	ComponentDatabase     ComponentType = "database"
	ComponentLoadBalancer ComponentType = "loadbalancer"
	ComponentEdgeProxy    ComponentType = "edgeproxy"
)

// HealthStatus represents health status
type HealthStatus string

const (
	HealthHealthy   HealthStatus = "healthy"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnhealthy HealthStatus = "unhealthy"
	HealthUnknown   HealthStatus = "unknown"
)

// Metrics holds component metrics
type Metrics struct {
	CPUUsage       float64   `json:"cpu_usage"`
	MemoryUsage    float64   `json:"memory_usage"`
	DiskUsage      float64   `json:"disk_usage"`
	NetworkRxBytes int64     `json:"network_rx_bytes"`
	NetworkTxBytes int64     `json:"network_tx_bytes"`
	RequestRate    float64   `json:"request_rate"`
	ErrorRate      float64   `json:"error_rate"`
	ResponseTime   float64   `json:"response_time_ms"`
	Uptime         int64     `json:"uptime_seconds"`
	Timestamp      time.Time `json:"timestamp"`
}

// AlertHandler handles health alerts
type AlertHandler interface {
	HandleAlert(component *ComponentHealth, alert Alert)
}

// Alert represents a health alert
type Alert struct {
	Severity  AlertSeverity
	Component string
	Message   string
	Timestamp time.Time
	Metrics   *Metrics
}

// AlertSeverity represents alert severity
type AlertSeverity string

const (
	AlertInfo     AlertSeverity = "info"
	AlertWarning  AlertSeverity = "warning"
	AlertCritical AlertSeverity = "critical"
)

// MetricsCollector collects metrics from a component
type MetricsCollector interface {
	CollectMetrics(ctx context.Context, componentID string) (*Metrics, error)
}

// Config holds monitor configuration
type Config struct {
	DatabaseURL        string
	CheckInterval      time.Duration
	UnhealthyThreshold int // Consecutive failures before marking unhealthy
}

// NewMonitor creates a new health monitor
func NewMonitor(config *Config, logger *slog.Logger) (*Monitor, error) {
	db, err := database.NewDB(config.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &Monitor{
		logger:     logger,
		db:         db,
		client:     &http.Client{Timeout: 5 * time.Second},
		components: make(map[string]*ComponentHealth),
		collectors: make(map[string]MetricsCollector),
	}, nil
}

// Start starts the health monitor
func (m *Monitor) Start(ctx context.Context) {
	m.logger.Info("Starting health monitor")

	// Start monitoring loops
	go m.monitorNodes(ctx)
	go m.monitorInstances(ctx)
	go m.monitorServices(ctx)
	go m.processAlerts(ctx)
}

// monitorNodes monitors node health
func (m *Monitor) monitorNodes(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkNodeHealth(ctx)
		}
	}
}

// checkNodeHealth checks health of all nodes
func (m *Monitor) checkNodeHealth(ctx context.Context) {
	nodes, err := m.db.Queries().NodeFind(ctx)
	if err != nil {
		m.logger.Error("Failed to get nodes", "error", err)
		return
	}

	for _, node := range nodes {
		nodeID := uuid.UUID(node.ID.Bytes).String()

		// Check node health via HTTP
		health := m.checkHTTPHealth(fmt.Sprintf("http://%s:8081/health", node.IpAddress))

		// Get node metrics
		metrics := m.getNodeMetrics(ctx, node)

		// Update component health
		m.updateComponentHealth(&ComponentHealth{
			ID:        nodeID,
			Name:      node.Hostname,
			Type:      ComponentNode,
			Status:    health,
			Metrics:   metrics,
			LastCheck: time.Now(),
		})

		// Check for issues
		if health != HealthHealthy {
			m.sendAlert(Alert{
				Severity:  AlertWarning,
				Component: node.Hostname,
				Message:   fmt.Sprintf("Node %s is %s", node.Hostname, health),
				Timestamp: time.Now(),
				Metrics:   metrics,
			})
		}

		// Check resource usage
		if metrics != nil {
			if metrics.CPUUsage > 90 {
				m.sendAlert(Alert{
					Severity:  AlertCritical,
					Component: node.Hostname,
					Message:   fmt.Sprintf("High CPU usage on node %s: %.1f%%", node.Hostname, metrics.CPUUsage),
					Timestamp: time.Now(),
					Metrics:   metrics,
				})
			}

			if metrics.MemoryUsage > 90 {
				m.sendAlert(Alert{
					Severity:  AlertCritical,
					Component: node.Hostname,
					Message:   fmt.Sprintf("High memory usage on node %s: %.1f%%", node.Hostname, metrics.MemoryUsage),
					Timestamp: time.Now(),
					Metrics:   metrics,
				})
			}

			if metrics.DiskUsage > 85 {
				m.sendAlert(Alert{
					Severity:  AlertWarning,
					Component: node.Hostname,
					Message:   fmt.Sprintf("High disk usage on node %s: %.1f%%", node.Hostname, metrics.DiskUsage),
					Timestamp: time.Now(),
					Metrics:   metrics,
				})
			}
		}

		// Update node state in database
		if health != HealthHealthy && node.State == "ready" {
			m.updateNodeState(ctx, node.ID, "degraded")
		} else if health == HealthHealthy && node.State != "ready" {
			m.updateNodeState(ctx, node.ID, "ready")
		}
	}
}

// monitorInstances monitors instance health
func (m *Monitor) monitorInstances(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkInstanceHealth(ctx)
		}
	}
}

// checkInstanceHealth checks health of all instances
func (m *Monitor) checkInstanceHealth(ctx context.Context) {
	instances, err := m.db.Queries().InstanceFindByState(ctx, "running")
	if err != nil {
		m.logger.Error("Failed to get instances", "error", err)
		return
	}

	for _, instance := range instances {
		instanceID := uuid.UUID(instance.ID.Bytes).String()

		// Check instance health
		health := m.checkHTTPHealth(fmt.Sprintf("http://[%s]:8080/health", instance.IpAddress))

		// Get instance metrics
		metrics := m.getInstanceMetrics(ctx, instance)

		// Update component health
		component := m.updateComponentHealth(&ComponentHealth{
			ID:        instanceID,
			Name:      fmt.Sprintf("instance-%s", instanceID[:8]),
			Type:      ComponentInstance,
			Status:    health,
			Metrics:   metrics,
			LastCheck: time.Now(),
		})

		// Check if instance is failing
		if health != HealthHealthy {
			component.ConsecutiveFails++

			if component.ConsecutiveFails >= 3 {
				m.sendAlert(Alert{
					Severity:  AlertCritical,
					Component: instanceID,
					Message:   fmt.Sprintf("Instance %s is unhealthy (failed %d checks)", instanceID[:8], component.ConsecutiveFails),
					Timestamp: time.Now(),
					Metrics:   metrics,
				})

				// Mark instance as unhealthy in database
				m.updateInstanceState(ctx, instance.ID, "unhealthy")
			}
		} else {
			component.ConsecutiveFails = 0
			component.LastHealthy = time.Now()
		}
	}
}

// monitorServices monitors service health
func (m *Monitor) monitorServices(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	services := []struct {
		name string
		url  string
		typ  ComponentType
	}{
		{"operator", fmt.Sprintf("http://%s/health", getServiceURL("operator")), ComponentService},
		{"load-balancer", fmt.Sprintf("http://%s/health", getServiceURL("load-balancer")), ComponentLoadBalancer},
		{"edge-proxy", fmt.Sprintf("http://%s/health", getServiceURL("edge-proxy")), ComponentEdgeProxy},
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, svc := range services {
				health := m.checkHTTPHealth(svc.url)

				m.updateComponentHealth(&ComponentHealth{
					ID:        svc.name,
					Name:      svc.name,
					Type:      svc.typ,
					Status:    health,
					LastCheck: time.Now(),
				})

				if health != HealthHealthy {
					m.sendAlert(Alert{
						Severity:  AlertCritical,
						Component: svc.name,
						Message:   fmt.Sprintf("Service %s is %s", svc.name, health),
						Timestamp: time.Now(),
					})
				}
			}

			// Check database health
			m.checkDatabaseHealth(ctx)
		}
	}
}

// checkHTTPHealth checks health via HTTP endpoint
func (m *Monitor) checkHTTPHealth(url string) HealthStatus {
	resp, err := m.client.Get(url)
	if err != nil {
		return HealthUnhealthy
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return HealthHealthy
	} else if resp.StatusCode >= 500 {
		return HealthUnhealthy
	}
	return HealthDegraded
}

// checkDatabaseHealth checks database health
func (m *Monitor) checkDatabaseHealth(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var result int
	err := m.db.QueryRow(ctx, "SELECT 1").Scan(&result)

	status := HealthHealthy
	if err != nil {
		status = HealthUnhealthy
	}

	m.updateComponentHealth(&ComponentHealth{
		ID:        "database",
		Name:      "PostgreSQL Database",
		Type:      ComponentDatabase,
		Status:    status,
		LastCheck: time.Now(),
	})

	if status != HealthHealthy {
		m.sendAlert(Alert{
			Severity:  AlertCritical,
			Component: "database",
			Message:   "Database health check failed",
			Timestamp: time.Now(),
		})
	}
}

// updateComponentHealth updates component health state
func (m *Monitor) updateComponentHealth(health *ComponentHealth) *ComponentHealth {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.components[health.ID]; ok {
		// Preserve some fields
		if health.LastHealthy.IsZero() && existing.Status == HealthHealthy {
			health.LastHealthy = time.Now()
		} else if !existing.LastHealthy.IsZero() {
			health.LastHealthy = existing.LastHealthy
		}
		health.ConsecutiveFails = existing.ConsecutiveFails
	} else if health.Status == HealthHealthy {
		health.LastHealthy = time.Now()
	}

	m.components[health.ID] = health
	return health
}

// getNodeMetrics gets metrics for a node
func (m *Monitor) getNodeMetrics(ctx context.Context, node *database.Node) *Metrics {
	// Parse resources JSON to get metrics
	if node.Resources != nil {
		var resources map[string]interface{}
		if err := json.Unmarshal(node.Resources, &resources); err == nil {
			metrics := &Metrics{
				Timestamp: time.Now(),
			}

			// Extract CPU/memory usage if available
			if cpu, ok := resources["cpu_usage"].(float64); ok {
				metrics.CPUUsage = cpu
			}
			if mem, ok := resources["memory_usage"].(float64); ok {
				metrics.MemoryUsage = mem
			}
			if disk, ok := resources["disk_usage"].(float64); ok {
				metrics.DiskUsage = disk
			}

			return metrics
		}
	}

	// Default metrics
	return &Metrics{
		CPUUsage:    50.0, // Placeholder
		MemoryUsage: 60.0, // Placeholder
		DiskUsage:   40.0, // Placeholder
		Timestamp:   time.Now(),
	}
}

// getInstanceMetrics gets metrics for an instance
func (m *Monitor) getInstanceMetrics(ctx context.Context, instance *database.Instance) *Metrics {
	// In production, this would query actual metrics from monitoring system
	return &Metrics{
		CPUUsage:     45.0, // Placeholder
		MemoryUsage:  55.0, // Placeholder
		ResponseTime: 100,  // Placeholder
		RequestRate:  50,   // Placeholder
		ErrorRate:    0.1,  // Placeholder
		Timestamp:    time.Now(),
	}
}

// updateNodeState updates node state in database
func (m *Monitor) updateNodeState(ctx context.Context, nodeID pgtype.UUID, state string) {
	_, err := m.db.Queries().NodeUpdate(ctx, &database.NodeUpdateParams{
		ID:    nodeID,
		State: state,
	})
	if err != nil {
		m.logger.Error("Failed to update node state", "error", err)
	}
}

// updateInstanceState updates instance state in database
func (m *Monitor) updateInstanceState(ctx context.Context, instanceID pgtype.UUID, state string) {
	_, err := m.db.Queries().InstanceUpdateState(ctx, &database.InstanceUpdateStateParams{
		ID:    instanceID,
		State: state,
	})
	if err != nil {
		m.logger.Error("Failed to update instance state", "error", err)
	}
}

// processAlerts processes and sends alerts
func (m *Monitor) processAlerts(ctx context.Context) {
	// Alert processing would be handled here
	// For now, just log alerts
}

// sendAlert sends an alert to configured handlers
func (m *Monitor) sendAlert(alert Alert) {
	m.logger.Warn("Health alert",
		"severity", alert.Severity,
		"component", alert.Component,
		"message", alert.Message)

	// Send to alert handlers
	for _, handler := range m.alertHandlers {
		go handler.HandleAlert(nil, alert)
	}
}

// AddAlertHandler adds an alert handler
func (m *Monitor) AddAlertHandler(handler AlertHandler) {
	m.alertHandlers = append(m.alertHandlers, handler)
}

// GetComponentHealth returns current health of a component
func (m *Monitor) GetComponentHealth(componentID string) (*ComponentHealth, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	health, ok := m.components[componentID]
	return health, ok
}

// GetAllComponentHealth returns health of all components
func (m *Monitor) GetAllComponentHealth() map[string]*ComponentHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*ComponentHealth)
	for k, v := range m.components {
		result[k] = v
	}
	return result
}

// GetSystemHealth returns overall system health
func (m *Monitor) GetSystemHealth() HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.components) == 0 {
		return HealthUnknown
	}

	unhealthyCount := 0
	degradedCount := 0

	for _, component := range m.components {
		switch component.Status {
		case HealthUnhealthy:
			unhealthyCount++
		case HealthDegraded:
			degradedCount++
		}
	}

	// If any critical component is unhealthy, system is unhealthy
	if unhealthyCount > 0 {
		return HealthUnhealthy
	}

	// If multiple components are degraded, system is degraded
	if degradedCount > len(m.components)/4 {
		return HealthDegraded
	}

	return HealthHealthy
}

// getServiceURL gets the URL for a service
func getServiceURL(service string) string {
	// In production, this would come from service discovery
	switch service {
	case "operator":
		return "localhost:8080"
	case "load-balancer":
		return "localhost:8082"
	case "edge-proxy":
		return "localhost:8083"
	default:
		return "localhost:8080"
	}
}

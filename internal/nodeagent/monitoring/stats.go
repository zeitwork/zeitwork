package monitoring

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

// StatsCollector collects and aggregates resource usage statistics
type StatsCollector struct {
	logger  *slog.Logger
	runtime types.Runtime

	// Collection configuration
	collectInterval time.Duration
	retentionPeriod time.Duration

	// Statistics storage
	instanceStats map[string][]*types.InstanceStats
	statsMu       sync.RWMutex

	// Aggregated statistics
	nodeStats   *NodeStats
	nodeStatsMu sync.RWMutex
}

// NodeStats represents aggregated statistics for the entire node
type NodeStats struct {
	Timestamp        time.Time `json:"timestamp"`
	TotalInstances   int       `json:"total_instances"`
	RunningInstances int       `json:"running_instances"`

	// Aggregated resource usage
	TotalCPUPercent     float64 `json:"total_cpu_percent"`
	TotalMemoryUsed     uint64  `json:"total_memory_used"`
	TotalMemoryLimit    uint64  `json:"total_memory_limit"`
	TotalNetworkRxBytes uint64  `json:"total_network_rx_bytes"`
	TotalNetworkTxBytes uint64  `json:"total_network_tx_bytes"`
	TotalDiskReadBytes  uint64  `json:"total_disk_read_bytes"`
	TotalDiskWriteBytes uint64  `json:"total_disk_write_bytes"`

	// Averages
	AvgCPUPercent    float64 `json:"avg_cpu_percent"`
	AvgMemoryPercent float64 `json:"avg_memory_percent"`
}

// NewStatsCollector creates a new statistics collector
func NewStatsCollector(logger *slog.Logger, rt types.Runtime) *StatsCollector {
	return &StatsCollector{
		logger:          logger,
		runtime:         rt,
		collectInterval: 30 * time.Second,
		retentionPeriod: 24 * time.Hour,
		instanceStats:   make(map[string][]*types.InstanceStats),
		nodeStats:       &NodeStats{},
	}
}

// SetCollectInterval sets the statistics collection interval
func (s *StatsCollector) SetCollectInterval(interval time.Duration) {
	s.collectInterval = interval
}

// SetRetentionPeriod sets how long to retain historical statistics
func (s *StatsCollector) SetRetentionPeriod(period time.Duration) {
	s.retentionPeriod = period
}

// Start starts the statistics collection loop
func (s *StatsCollector) Start(ctx context.Context) {
	s.logger.Info("Starting statistics collection", "interval", s.collectInterval)

	ticker := time.NewTicker(s.collectInterval)
	defer ticker.Stop()

	// Initial collection
	s.collectStatistics(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Stopping statistics collection")
			return
		case <-ticker.C:
			s.collectStatistics(ctx)
		}
	}
}

// collectStatistics collects statistics from all instances
func (s *StatsCollector) collectStatistics(ctx context.Context) {
	s.logger.Debug("Collecting instance statistics")

	// Get current instances
	instances, err := s.runtime.ListInstances(ctx)
	if err != nil {
		s.logger.Error("Failed to list instances for stats collection", "error", err)
		return
	}

	// Collect stats for each instance
	var wg sync.WaitGroup
	statsChan := make(chan *types.InstanceStats, len(instances))

	for _, instance := range instances {
		wg.Add(1)
		go func(inst *types.Instance) {
			defer wg.Done()
			s.collectInstanceStats(ctx, inst, statsChan)
		}(instance)
	}

	// Close channel when all goroutines complete
	go func() {
		wg.Wait()
		close(statsChan)
	}()

	// Collect all stats
	var collectedStats []*types.InstanceStats
	for stats := range statsChan {
		if stats != nil {
			collectedStats = append(collectedStats, stats)
		}
	}

	// Store collected statistics
	s.storeStatistics(collectedStats)

	// Update node-level aggregated statistics
	s.updateNodeStats(instances, collectedStats)

	// Clean up old statistics
	s.cleanupOldStatistics()

	s.logger.Debug("Statistics collection completed",
		"instances", len(instances),
		"stats_collected", len(collectedStats))
}

// collectInstanceStats collects statistics for a single instance
func (s *StatsCollector) collectInstanceStats(ctx context.Context, instance *types.Instance, statsChan chan<- *types.InstanceStats) {
	// Only collect stats for running instances
	if instance.State != types.InstanceStateRunning {
		return
	}

	stats, err := s.runtime.GetStats(ctx, instance)
	if err != nil {
		s.logger.Debug("Failed to get instance stats",
			"instance_id", instance.ID,
			"error", err)
		return
	}

	statsChan <- stats
}

// storeStatistics stores collected statistics
func (s *StatsCollector) storeStatistics(stats []*types.InstanceStats) {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	for _, stat := range stats {
		// Initialize slice if needed
		if s.instanceStats[stat.InstanceID] == nil {
			s.instanceStats[stat.InstanceID] = make([]*types.InstanceStats, 0)
		}

		// Add new stats
		s.instanceStats[stat.InstanceID] = append(s.instanceStats[stat.InstanceID], stat)
	}
}

// updateNodeStats updates aggregated node-level statistics
func (s *StatsCollector) updateNodeStats(instances []*types.Instance, stats []*types.InstanceStats) {
	s.nodeStatsMu.Lock()
	defer s.nodeStatsMu.Unlock()

	nodeStats := &NodeStats{
		Timestamp:        time.Now(),
		TotalInstances:   len(instances),
		RunningInstances: len(stats),
	}

	// Aggregate statistics
	for _, stat := range stats {
		nodeStats.TotalCPUPercent += stat.CPUPercent
		nodeStats.TotalMemoryUsed += stat.MemoryUsed
		nodeStats.TotalMemoryLimit += stat.MemoryLimit
		nodeStats.TotalNetworkRxBytes += stat.NetworkRxBytes
		nodeStats.TotalNetworkTxBytes += stat.NetworkTxBytes
		nodeStats.TotalDiskReadBytes += stat.DiskReadBytes
		nodeStats.TotalDiskWriteBytes += stat.DiskWriteBytes
	}

	// Calculate averages
	if len(stats) > 0 {
		nodeStats.AvgCPUPercent = nodeStats.TotalCPUPercent / float64(len(stats))
		if nodeStats.TotalMemoryLimit > 0 {
			nodeStats.AvgMemoryPercent = float64(nodeStats.TotalMemoryUsed) / float64(nodeStats.TotalMemoryLimit) * 100
		}
	}

	s.nodeStats = nodeStats
}

// cleanupOldStatistics removes statistics older than the retention period
func (s *StatsCollector) cleanupOldStatistics() {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	cutoff := time.Now().Add(-s.retentionPeriod)

	for instanceID, statsList := range s.instanceStats {
		// Filter out old statistics
		var filteredStats []*types.InstanceStats
		for _, stat := range statsList {
			if stat.Timestamp.After(cutoff) {
				filteredStats = append(filteredStats, stat)
			}
		}

		if len(filteredStats) == 0 {
			delete(s.instanceStats, instanceID)
		} else {
			s.instanceStats[instanceID] = filteredStats
		}
	}
}

// GetInstanceStats returns recent statistics for a specific instance
func (s *StatsCollector) GetInstanceStats(instanceID string, limit int) []*types.InstanceStats {
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()

	stats, exists := s.instanceStats[instanceID]
	if !exists {
		return nil
	}

	// Return the most recent stats up to the limit
	if limit <= 0 || limit >= len(stats) {
		// Return a copy to avoid race conditions
		result := make([]*types.InstanceStats, len(stats))
		copy(result, stats)
		return result
	}

	// Return last 'limit' entries
	start := len(stats) - limit
	result := make([]*types.InstanceStats, limit)
	copy(result, stats[start:])
	return result
}

// GetLatestInstanceStats returns the most recent statistics for a specific instance
func (s *StatsCollector) GetLatestInstanceStats(instanceID string) *types.InstanceStats {
	stats := s.GetInstanceStats(instanceID, 1)
	if len(stats) == 0 {
		return nil
	}
	return stats[0]
}

// GetNodeStats returns the current aggregated node statistics
func (s *StatsCollector) GetNodeStats() *NodeStats {
	s.nodeStatsMu.RLock()
	defer s.nodeStatsMu.RUnlock()

	// Return a copy to avoid race conditions
	return &NodeStats{
		Timestamp:           s.nodeStats.Timestamp,
		TotalInstances:      s.nodeStats.TotalInstances,
		RunningInstances:    s.nodeStats.RunningInstances,
		TotalCPUPercent:     s.nodeStats.TotalCPUPercent,
		TotalMemoryUsed:     s.nodeStats.TotalMemoryUsed,
		TotalMemoryLimit:    s.nodeStats.TotalMemoryLimit,
		TotalNetworkRxBytes: s.nodeStats.TotalNetworkRxBytes,
		TotalNetworkTxBytes: s.nodeStats.TotalNetworkTxBytes,
		TotalDiskReadBytes:  s.nodeStats.TotalDiskReadBytes,
		TotalDiskWriteBytes: s.nodeStats.TotalDiskWriteBytes,
		AvgCPUPercent:       s.nodeStats.AvgCPUPercent,
		AvgMemoryPercent:    s.nodeStats.AvgMemoryPercent,
	}
}

// GetAllInstancesLatestStats returns the latest statistics for all instances
func (s *StatsCollector) GetAllInstancesLatestStats() map[string]*types.InstanceStats {
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()

	result := make(map[string]*types.InstanceStats)
	for instanceID, statsList := range s.instanceStats {
		if len(statsList) > 0 {
			// Get the most recent stats
			latest := statsList[len(statsList)-1]
			result[instanceID] = latest
		}
	}

	return result
}

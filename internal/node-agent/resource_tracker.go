package nodeagent

import (
	"fmt"
	"sync"
	"time"
)

// ResourceTracker tracks and manages VM resource allocation
type ResourceTracker struct {
	mu sync.RWMutex

	// Total resources
	totalVCPUs    int
	totalMemoryMB int

	// Allocated resources per VM
	allocations map[string]*ResourceAllocation

	// Usage history for trending
	history []ResourceSnapshot

	// Thresholds for alerts
	cpuThreshold    float64
	memoryThreshold float64
}

// ResourceAllocation represents resources allocated to a VM
type ResourceAllocation struct {
	VMID        string
	VCPUs       int
	MemoryMB    int
	AllocatedAt time.Time

	// Actual usage (updated periodically)
	ActualCPU    float64
	ActualMemory int64
	LastUpdated  time.Time
}

// ResourceSnapshot represents a point-in-time resource usage
type ResourceSnapshot struct {
	Timestamp       time.Time
	AllocatedVCPUs  int
	AllocatedMemory int
	UsedCPU         float64
	UsedMemory      int64
	VMCount         int
}

// ResourceStats provides resource statistics
type ResourceStats struct {
	TotalVCPUs        int     `json:"total_vcpus"`
	TotalMemoryMB     int     `json:"total_memory_mb"`
	AllocatedVCPUs    int     `json:"allocated_vcpus"`
	AllocatedMemory   int     `json:"allocated_memory_mb"`
	AvailableVCPUs    int     `json:"available_vcpus"`
	AvailableMemory   int     `json:"available_memory_mb"`
	CPUUtilization    float64 `json:"cpu_utilization"`
	MemoryUtilization float64 `json:"memory_utilization"`
	VMCount           int     `json:"vm_count"`
}

// NewResourceTracker creates a new resource tracker
func NewResourceTracker(totalVCPUs, totalMemoryMB int) *ResourceTracker {
	return &ResourceTracker{
		totalVCPUs:      totalVCPUs,
		totalMemoryMB:   totalMemoryMB,
		allocations:     make(map[string]*ResourceAllocation),
		history:         make([]ResourceSnapshot, 0, 1440), // 24 hours at 1 min intervals
		cpuThreshold:    0.8,                               // 80% threshold
		memoryThreshold: 0.9,                               // 90% threshold
	}
}

// CanAllocate checks if resources can be allocated
func (rt *ResourceTracker) CanAllocate(vcpus, memoryMB int) bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	allocatedVCPUs := 0
	allocatedMemory := 0

	for _, alloc := range rt.allocations {
		allocatedVCPUs += alloc.VCPUs
		allocatedMemory += alloc.MemoryMB
	}

	availableVCPUs := rt.totalVCPUs - allocatedVCPUs
	availableMemory := rt.totalMemoryMB - allocatedMemory

	return vcpus <= availableVCPUs && memoryMB <= availableMemory
}

// Allocate allocates resources for a VM
func (rt *ResourceTracker) Allocate(vmID string, vcpus, memoryMB int) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Check if already allocated
	if _, exists := rt.allocations[vmID]; exists {
		return fmt.Errorf("resources already allocated for VM: %s", vmID)
	}

	// Check availability
	allocatedVCPUs := 0
	allocatedMemory := 0

	for _, alloc := range rt.allocations {
		allocatedVCPUs += alloc.VCPUs
		allocatedMemory += alloc.MemoryMB
	}

	availableVCPUs := rt.totalVCPUs - allocatedVCPUs
	availableMemory := rt.totalMemoryMB - allocatedMemory

	if vcpus > availableVCPUs {
		return fmt.Errorf("insufficient vCPUs: requested %d, available %d", vcpus, availableVCPUs)
	}

	if memoryMB > availableMemory {
		return fmt.Errorf("insufficient memory: requested %d MB, available %d MB", memoryMB, availableMemory)
	}

	// Allocate resources
	rt.allocations[vmID] = &ResourceAllocation{
		VMID:        vmID,
		VCPUs:       vcpus,
		MemoryMB:    memoryMB,
		AllocatedAt: time.Now(),
	}

	// Record snapshot
	rt.recordSnapshot()

	return nil
}

// Release releases resources for a VM
func (rt *ResourceTracker) Release(vmID string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	delete(rt.allocations, vmID)
	rt.recordSnapshot()
}

// UpdateUsage updates actual resource usage for a VM
func (rt *ResourceTracker) UpdateUsage(vmID string, cpuUsage float64, memoryUsage int64) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if alloc, exists := rt.allocations[vmID]; exists {
		alloc.ActualCPU = cpuUsage
		alloc.ActualMemory = memoryUsage
		alloc.LastUpdated = time.Now()
	}
}

// GetStats returns current resource statistics
func (rt *ResourceTracker) GetStats() *ResourceStats {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	stats := &ResourceStats{
		TotalVCPUs:    rt.totalVCPUs,
		TotalMemoryMB: rt.totalMemoryMB,
		VMCount:       len(rt.allocations),
	}

	totalActualCPU := 0.0
	totalActualMemory := int64(0)

	for _, alloc := range rt.allocations {
		stats.AllocatedVCPUs += alloc.VCPUs
		stats.AllocatedMemory += alloc.MemoryMB
		totalActualCPU += alloc.ActualCPU
		totalActualMemory += alloc.ActualMemory
	}

	stats.AvailableVCPUs = stats.TotalVCPUs - stats.AllocatedVCPUs
	stats.AvailableMemory = stats.TotalMemoryMB - stats.AllocatedMemory

	if stats.TotalVCPUs > 0 {
		stats.CPUUtilization = float64(stats.AllocatedVCPUs) / float64(stats.TotalVCPUs)
	}

	if stats.TotalMemoryMB > 0 {
		stats.MemoryUtilization = float64(stats.AllocatedMemory) / float64(stats.TotalMemoryMB)
	}

	return stats
}

// GetUsage returns current allocated resources
func (rt *ResourceTracker) GetUsage() (vcpus int, memory int) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	for _, alloc := range rt.allocations {
		vcpus += alloc.VCPUs
		memory += alloc.MemoryMB
	}

	return vcpus, memory
}

// GetAvailable returns available resources
func (rt *ResourceTracker) GetAvailable() (vcpus int, memory int) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	allocatedVCPUs := 0
	allocatedMemory := 0

	for _, alloc := range rt.allocations {
		allocatedVCPUs += alloc.VCPUs
		allocatedMemory += alloc.MemoryMB
	}

	vcpus = rt.totalVCPUs - allocatedVCPUs
	memory = rt.totalMemoryMB - allocatedMemory

	return vcpus, memory
}

// GetAllocation returns allocation for a specific VM
func (rt *ResourceTracker) GetAllocation(vmID string) (*ResourceAllocation, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	alloc, exists := rt.allocations[vmID]
	return alloc, exists
}

// GetAllocations returns all current allocations
func (rt *ResourceTracker) GetAllocations() map[string]*ResourceAllocation {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	// Create a copy to avoid race conditions
	allocations := make(map[string]*ResourceAllocation)
	for id, alloc := range rt.allocations {
		allocCopy := *alloc
		allocations[id] = &allocCopy
	}

	return allocations
}

// IsOverThreshold checks if resource usage is over threshold
func (rt *ResourceTracker) IsOverThreshold() (bool, string) {
	stats := rt.GetStats()

	if stats.CPUUtilization > rt.cpuThreshold {
		return true, fmt.Sprintf("CPU utilization %.1f%% exceeds threshold %.1f%%",
			stats.CPUUtilization*100, rt.cpuThreshold*100)
	}

	if stats.MemoryUtilization > rt.memoryThreshold {
		return true, fmt.Sprintf("Memory utilization %.1f%% exceeds threshold %.1f%%",
			stats.MemoryUtilization*100, rt.memoryThreshold*100)
	}

	return false, ""
}

// GetHistory returns resource usage history
func (rt *ResourceTracker) GetHistory(duration time.Duration) []ResourceSnapshot {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	cutoff := time.Now().Add(-duration)
	var history []ResourceSnapshot

	for _, snapshot := range rt.history {
		if snapshot.Timestamp.After(cutoff) {
			history = append(history, snapshot)
		}
	}

	return history
}

// recordSnapshot records current resource state
func (rt *ResourceTracker) recordSnapshot() {
	snapshot := ResourceSnapshot{
		Timestamp: time.Now(),
		VMCount:   len(rt.allocations),
	}

	totalActualCPU := 0.0
	totalActualMemory := int64(0)

	for _, alloc := range rt.allocations {
		snapshot.AllocatedVCPUs += alloc.VCPUs
		snapshot.AllocatedMemory += alloc.MemoryMB
		totalActualCPU += alloc.ActualCPU
		totalActualMemory += alloc.ActualMemory
	}

	snapshot.UsedCPU = totalActualCPU
	snapshot.UsedMemory = totalActualMemory

	// Append to history, maintaining max size
	rt.history = append(rt.history, snapshot)
	if len(rt.history) > 1440 { // Keep last 24 hours at 1 min intervals
		rt.history = rt.history[1:]
	}
}

// PredictCapacity predicts when resources will be exhausted based on trends
func (rt *ResourceTracker) PredictCapacity() time.Duration {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	if len(rt.history) < 2 {
		return -1 // Not enough data
	}

	// Simple linear prediction based on last hour
	hourAgo := time.Now().Add(-time.Hour)
	var oldSnapshot, recentSnapshot *ResourceSnapshot

	for i := range rt.history {
		if rt.history[i].Timestamp.After(hourAgo) {
			if oldSnapshot == nil {
				oldSnapshot = &rt.history[i]
			}
			recentSnapshot = &rt.history[i]
		}
	}

	if oldSnapshot == nil || recentSnapshot == nil {
		return -1
	}

	// Calculate growth rate
	timeDiff := recentSnapshot.Timestamp.Sub(oldSnapshot.Timestamp).Hours()
	if timeDiff == 0 {
		return -1
	}

	vcpuGrowth := float64(recentSnapshot.AllocatedVCPUs-oldSnapshot.AllocatedVCPUs) / timeDiff
	memoryGrowth := float64(recentSnapshot.AllocatedMemory-oldSnapshot.AllocatedMemory) / timeDiff

	// Predict time to exhaustion
	availableVCPUs := rt.totalVCPUs - recentSnapshot.AllocatedVCPUs
	availableMemory := rt.totalMemoryMB - recentSnapshot.AllocatedMemory

	var timeToExhaustion float64 = -1

	if vcpuGrowth > 0 {
		vcpuTime := float64(availableVCPUs) / vcpuGrowth
		if timeToExhaustion < 0 || vcpuTime < timeToExhaustion {
			timeToExhaustion = vcpuTime
		}
	}

	if memoryGrowth > 0 {
		memoryTime := float64(availableMemory) / memoryGrowth
		if timeToExhaustion < 0 || memoryTime < timeToExhaustion {
			timeToExhaustion = memoryTime
		}
	}

	if timeToExhaustion < 0 {
		return -1
	}

	return time.Duration(timeToExhaustion * float64(time.Hour))
}

// SetThresholds sets alert thresholds
func (rt *ResourceTracker) SetThresholds(cpu, memory float64) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.cpuThreshold = cpu
	rt.memoryThreshold = memory
}

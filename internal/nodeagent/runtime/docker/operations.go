package docker

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	runtimeTypes "github.com/zeitwork/zeitwork/internal/nodeagent/types"
)

// GetStats retrieves resource usage statistics for a container
func (d *DockerRuntime) GetStats(ctx context.Context, instance *runtimeTypes.Instance) (*runtimeTypes.InstanceStats, error) {
	stats, err := d.client.ContainerStats(ctx, instance.RuntimeID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get container stats: %w", err)
	}
	defer stats.Body.Close()

	// Read stats data
	data, err := io.ReadAll(stats.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read stats data: %w", err)
	}

	// Parse Docker stats (simplified version)
	// In a real implementation, you'd parse the JSON response properly
	containerStats := &runtimeTypes.InstanceStats{
		InstanceID: instance.ID,
		// TODO: Parse actual Docker stats JSON response
		// This is a simplified placeholder
		CPUPercent:    0.0,
		MemoryUsed:    0,
		MemoryLimit:   0,
		MemoryPercent: 0.0,
	}

	d.logger.Debug("Retrieved container stats",
		"instance_id", instance.ID,
		"data_size", len(data))

	return containerStats, nil
}

// ExecuteCommand executes a command inside a running container
func (d *DockerRuntime) ExecuteCommand(ctx context.Context, instance *runtimeTypes.Instance, cmd []string) (string, error) {
	d.logger.Debug("Executing command in container",
		"instance_id", instance.ID,
		"command", strings.Join(cmd, " "))

	// Create exec configuration
	execConfig := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
	}

	// Create exec instance
	execResp, err := d.client.ContainerExecCreate(ctx, instance.RuntimeID, execConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create exec instance: %w", err)
	}

	// Attach to exec instance
	attachResp, err := d.client.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to attach to exec instance: %w", err)
	}
	defer attachResp.Close()

	// Start exec
	if err := d.client.ContainerExecStart(ctx, execResp.ID, container.ExecStartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start exec: %w", err)
	}

	// Read output
	output, err := io.ReadAll(attachResp.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to read exec output: %w", err)
	}

	// Check exec result
	execInspect, err := d.client.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return string(output), fmt.Errorf("failed to inspect exec result: %w", err)
	}

	if execInspect.ExitCode != 0 {
		return string(output), fmt.Errorf("command failed with exit code %d", execInspect.ExitCode)
	}

	d.logger.Debug("Command executed successfully",
		"instance_id", instance.ID,
		"exit_code", execInspect.ExitCode)

	return string(output), nil
}

// GetLogs retrieves logs from a container
func (d *DockerRuntime) GetLogs(ctx context.Context, instance *runtimeTypes.Instance, lines int) ([]string, error) {
	d.logger.Debug("Getting container logs",
		"instance_id", instance.ID,
		"lines", lines)

	// Configure log options
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       fmt.Sprintf("%d", lines),
	}

	// Get logs
	logsReader, err := d.client.ContainerLogs(ctx, instance.RuntimeID, options)
	if err != nil {
		return nil, fmt.Errorf("failed to get container logs: %w", err)
	}
	defer logsReader.Close()

	// Read logs
	logsData, err := io.ReadAll(logsReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read logs: %w", err)
	}

	// Split logs into lines
	// Note: Docker logs include headers, so we need to strip them
	logLines := strings.Split(string(logsData), "\n")

	// Filter out empty lines and strip Docker headers
	var cleanLines []string
	for _, line := range logLines {
		if line = strings.TrimSpace(line); line != "" {
			// Strip Docker log header (8 bytes) if present
			if len(line) > 8 && line[0] <= 2 {
				line = line[8:]
			}
			cleanLines = append(cleanLines, line)
		}
	}

	// Limit to requested number of lines
	if len(cleanLines) > lines {
		cleanLines = cleanLines[len(cleanLines)-lines:]
	}

	d.logger.Debug("Retrieved container logs",
		"instance_id", instance.ID,
		"log_lines", len(cleanLines))

	return cleanLines, nil
}

// CleanupOrphanedInstances removes containers that are not in the desired state
func (d *DockerRuntime) CleanupOrphanedInstances(ctx context.Context, desiredInstances []*runtimeTypes.Instance) error {
	d.logger.Info("Cleaning up orphaned containers")

	// Get all running zeitwork containers
	actualInstances, err := d.ListInstances(ctx)
	if err != nil {
		return fmt.Errorf("failed to list actual instances: %w", err)
	}

	// Create map of desired instance IDs
	desiredMap := make(map[string]bool)
	for _, instance := range desiredInstances {
		desiredMap[instance.ID] = true
	}

	// Find orphaned instances
	var orphanedInstances []*runtimeTypes.Instance
	for _, actual := range actualInstances {
		if !desiredMap[actual.ID] {
			orphanedInstances = append(orphanedInstances, actual)
		}
	}

	// Clean up orphaned instances
	cleanedCount := 0
	for _, orphaned := range orphanedInstances {
		d.logger.Info("Cleaning up orphaned container",
			"instance_id", orphaned.ID,
			"container_id", orphaned.RuntimeID[:12])

		if err := d.DeleteInstance(ctx, orphaned); err != nil {
			d.logger.Error("Failed to cleanup orphaned container",
				"instance_id", orphaned.ID,
				"error", err)
			continue
		}
		cleanedCount++
	}

	d.logger.Info("Cleanup completed",
		"orphaned_found", len(orphanedInstances),
		"cleaned_up", cleanedCount)

	return nil
}

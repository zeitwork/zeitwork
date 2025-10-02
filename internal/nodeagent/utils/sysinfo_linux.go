//go:build linux

package utils

import (
	"fmt"
	"syscall"
)

// GetSystemMemoryMB returns the total system memory in megabytes
func GetSystemMemoryMB() (uint64, error) {
	var si syscall.Sysinfo_t
	if err := syscall.Sysinfo(&si); err != nil {
		return 0, fmt.Errorf("failed to get system info: %w", err)
	}
	// Totalram is in bytes, convert to MB
	return si.Totalram / 1024 / 1024, nil
}

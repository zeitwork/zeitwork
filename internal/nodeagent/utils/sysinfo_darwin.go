//go:build darwin

package utils

import (
	"fmt"
	"syscall"
	"unsafe"
)

// GetSystemMemoryMB returns the total system memory in megabytes
func GetSystemMemoryMB() (uint64, error) {
	// On macOS, use sysctl to get hw.memsize
	mib := []int32{6, 24} // CTL_HW, HW_MEMSIZE
	var physicalMemory uint64
	length := unsafe.Sizeof(physicalMemory)

	_, _, errno := syscall.Syscall6(
		syscall.SYS___SYSCTL,
		uintptr(unsafe.Pointer(&mib[0])),
		uintptr(len(mib)),
		uintptr(unsafe.Pointer(&physicalMemory)),
		uintptr(unsafe.Pointer(&length)),
		0,
		0,
	)

	if errno != 0 {
		return 0, fmt.Errorf("sysctl failed: %v", errno)
	}

	// physicalMemory is in bytes, convert to MB
	return physicalMemory / 1024 / 1024, nil
}

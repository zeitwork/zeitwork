package config

import (
	"fmt"
	"os"
)

// RuntimeMode represents the execution mode of the node agent
type RuntimeMode string

const (
	RuntimeModeDevelopment RuntimeMode = "development"
	RuntimeModeProduction  RuntimeMode = "production"
)

// DetectRuntimeMode detects the current runtime mode based on environment
func DetectRuntimeMode() RuntimeMode {
	mode := os.Getenv("RUNTIME_MODE")
	switch mode {
	case "production":
		return RuntimeModeProduction
	case "development":
		return RuntimeModeDevelopment
	default:
		// Default to development for safety
		return RuntimeModeDevelopment
	}
}

// ValidateRuntimeMode validates if the runtime mode is supported
func ValidateRuntimeMode(mode RuntimeMode) error {
	switch mode {
	case RuntimeModeDevelopment, RuntimeModeProduction:
		return nil
	default:
		return fmt.Errorf("unsupported runtime mode: %s", mode)
	}
}

// IsProduction returns true if running in production mode
func IsProduction(mode RuntimeMode) bool {
	return mode == RuntimeModeProduction
}

// IsDevelopment returns true if running in development mode
func IsDevelopment(mode RuntimeMode) bool {
	return mode == RuntimeModeDevelopment
}

package internal

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// GenerateShortID generates a short random ID for logging purposes
func GenerateShortID() string {
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID
		return fmt.Sprintf("%x", time.Now().UnixNano()&0xFFFFFFFF)
	}
	return hex.EncodeToString(bytes)
}

// TruncateString truncates a string to a maximum length
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// FormatDuration formats a duration in a human-readable way
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	} else {
		return fmt.Sprintf("%.1fd", d.Hours()/24)
	}
}

// FormatBytes formats bytes in a human-readable way
func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// SanitizeContainerName sanitizes a string to be used as a container name
func SanitizeContainerName(name string) string {
	// Replace invalid characters with hyphens
	sanitized := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)

	// Remove leading/trailing hyphens
	sanitized = strings.Trim(sanitized, "-")

	// Ensure it's not empty
	if sanitized == "" {
		sanitized = "unknown"
	}

	return sanitized
}

// ParseImageTag parses a container image tag into registry, name, and tag components
func ParseImageTag(imageTag string) (registry, name, tag string) {
	// Default values
	registry = "docker.io"
	tag = "latest"

	// Split by slash to separate registry/name
	parts := strings.Split(imageTag, "/")

	if len(parts) == 1 {
		// Just image name, possibly with tag
		name = parts[0]
	} else if len(parts) == 2 {
		if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
			// First part looks like a registry
			registry = parts[0]
			name = parts[1]
		} else {
			// Assume it's namespace/image
			name = imageTag
		}
	} else {
		// Full registry/namespace/image format
		registry = parts[0]
		name = strings.Join(parts[1:], "/")
	}

	// Extract tag if present
	if strings.Contains(name, ":") {
		tagParts := strings.Split(name, ":")
		name = strings.Join(tagParts[:len(tagParts)-1], ":")
		tag = tagParts[len(tagParts)-1]
	}

	return registry, name, tag
}

// RetryConfig defines configuration for retry operations
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Multiplier  float64
}

// DefaultRetryConfig returns a default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Second,
		MaxDelay:    30 * time.Second,
		Multiplier:  2.0,
	}
}

// ExponentialBackoff calculates the delay for a given attempt using exponential backoff
func (c *RetryConfig) ExponentialBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return c.BaseDelay
	}

	delay := float64(c.BaseDelay)
	for i := 0; i < attempt; i++ {
		delay *= c.Multiplier
	}

	if time.Duration(delay) > c.MaxDelay {
		return c.MaxDelay
	}

	return time.Duration(delay)
}

// ContainsString checks if a slice contains a specific string
func ContainsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// RemoveString removes all occurrences of a string from a slice
func RemoveString(slice []string, item string) []string {
	var result []string
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

// MergeStringMaps merges multiple string maps, with later maps overriding earlier ones
func MergeStringMaps(maps ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

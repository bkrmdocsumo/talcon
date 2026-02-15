// Package util provides shared utility functions used across the Talon codebase.
package util

import "strings"

// IsImageMime returns true for image MIME types supported by the Anthropic API.
func IsImageMime(mime string) bool {
	switch strings.ToLower(mime) {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	}
	return false
}

// Truncate returns s truncated to max characters with "..." appended if truncated.
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// TruncateForUI truncates long strings for display in the UI, appending a
// truncation notice that's more appropriate for tool output display.
func TruncateForUI(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n... (truncated)"
}

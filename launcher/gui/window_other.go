//go:build !windows && !darwin && !linux

package main

// isWindowFocused checks if a window with the given title is focused.
// Fallback for unsupported platforms - always returns true.
func isWindowFocused(windowTitle string) bool {
	return true
}

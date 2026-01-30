//go:build linux

package main

import (
	"os/exec"
	"strings"
)

// isWindowFocused checks if a window with the given title is focused.
// Uses xdotool to get the active window title on Linux/X11.
func isWindowFocused(windowTitle string) bool {
	// Try xdotool first (X11)
	cmd := exec.Command("xdotool", "getactivewindow", "getwindowname")
	output, err := cmd.Output()
	if err == nil {
		title := strings.TrimSpace(string(output))
		return strings.Contains(title, windowTitle)
	}

	// Try xprop as fallback (also X11)
	cmd = exec.Command("sh", "-c", "xprop -id $(xprop -root _NET_ACTIVE_WINDOW | cut -d ' ' -f 5) WM_NAME 2>/dev/null | cut -d '\"' -f 2")
	output, err = cmd.Output()
	if err == nil {
		title := strings.TrimSpace(string(output))
		return strings.Contains(title, windowTitle)
	}

	// If we can't detect, assume focused to not block input
	return true
}

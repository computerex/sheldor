//go:build darwin

package main

import (
	"os/exec"
	"strings"
)

// isWindowFocused checks if a window with the given title is focused.
// Uses AppleScript to get the frontmost application window title on macOS.
func isWindowFocused(windowTitle string) bool {
	// AppleScript to get the frontmost window title
	script := `
		tell application "System Events"
			set frontApp to first application process whose frontmost is true
			set appName to name of frontApp
			try
				tell frontApp
					set windowTitle to name of front window
				end tell
				return appName & " - " & windowTitle
			on error
				return appName
			end try
		end tell
	`

	cmd := exec.Command("osascript", "-e", script)
	output, err := cmd.Output()
	if err == nil {
		title := strings.TrimSpace(string(output))
		return strings.Contains(title, windowTitle)
	}

	// If we can't detect, assume focused to not block input
	return true
}

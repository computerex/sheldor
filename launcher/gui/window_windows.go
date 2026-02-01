//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentProcessId      = kernel32.NewProc("GetCurrentProcessId")
)

func isWindowFocused(windowTitle string) bool {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return false
	}

	// Check if the focused window belongs to our process
	var processId uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&processId)))
	
	// If we couldn't get the process ID, assume not focused
	if processId == 0 {
		return false
	}
	
	currentPid, _, _ := procGetCurrentProcessId.Call()
	return processId == uint32(currentPid)
}

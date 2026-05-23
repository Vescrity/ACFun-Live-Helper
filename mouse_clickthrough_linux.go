//go:build linux

package main

import (
	"errors"
	"log"
)

// Mouse click-through and global hotkey stubs for Linux.
//
// On Linux, these features require X11/Wayland-specific window management
// and are not yet implemented. The mini window (floating danmaku overlay)
// will still work, but without click-through and global hotkey support.

const (
	modAlt   = 0x0001
	modCtrl  = 0x0002
	modShift = 0x0004

	vkG = 0x47 // 'G', default Ctrl+Alt+Shift+G
)

// findOwnVisibleHWND returns 0 on Linux (not implemented).
func findOwnVisibleHWND() uintptr {
	return 0
}

// applyMouseClickThrough is not supported on Linux.
func applyMouseClickThrough(hwnd uintptr, enable bool) error {
	return errors.New("mouse click-through is not supported on Linux")
}

// startGlobalHotkey is a no-op on Linux. Logs a warning.
func startGlobalHotkey(initialMods, initialVK uintptr, onTrigger func()) {
	log.Println("[Mini] Global hotkey is not supported on Linux; mouse click-through toggle via hotkey will not work")
}

// updateGlobalHotkey is a no-op on Linux.
func updateGlobalHotkey(mods, vk uintptr) error {
	return errors.New("global hotkey is not supported on Linux")
}

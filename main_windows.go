//go:build windows
// +build windows

package main

import (
	syswin "golang.org/x/sys/windows"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// watchParentProcess monitors the parent process (ACLIVE_PARENT_PID) to ensure the mini window
// closes when the main window closes.
func watchParentProcess() {
	pidStr := os.Getenv("ACLIVE_PARENT_PID")
	if pidStr == "" {
		return
	}
	pid64, err := strconv.ParseUint(pidStr, 10, 32)
	if err != nil || pid64 == 0 {
		return
	}
	pid := uint32(pid64)

	const stillActive uint32 = 259
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		handle, err := syswin.OpenProcess(syswin.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			log.Printf("[Mini] Parent process %d not reachable: %v; exiting", pid, err)
			os.Exit(0)
		}
		var code uint32
		err = syswin.GetExitCodeProcess(handle, &code)
		_ = syswin.CloseHandle(handle)
		if err != nil {
			log.Printf("[Mini] Failed to query parent exit code: %v; exiting", pid, err)
			os.Exit(0)
		}
		if code != stillActive {
			log.Printf("[Mini] Parent process %d exited (code=%d); exiting", pid, code)
			os.Exit(0)
		}
	}
}

func getWindowsOptions(webviewDataPath string, isMini bool) *windows.Options {
	return &windows.Options{
		WebviewUserDataPath:              webviewDataPath,
		WebviewGpuIsDisabled:             !isMini,
		Theme:                            windows.SystemDefault,
		WebviewIsTransparent:             isMini,
		WindowIsTranslucent:              isMini,
		BackdropType:                     windows.None,
		DisableFramelessWindowDecorations: isMini,
	}
}

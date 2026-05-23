//go:build windows

package main

import (
	"log"
	"os"
	"strconv"
	"time"

	syswin "golang.org/x/sys/windows"
)

// watchParentProcess monitors the parent process (ACLIVE_PARENT_PID) and exits
// when the parent terminates. This ensures the mini window closes when the main
// window is closed — whether normally, crashed, or restarted via wails dev.
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
			log.Printf("[Mini] Failed to query parent exit code: %v; exiting", err)
			os.Exit(0)
		}
		if code != stillActive {
			log.Printf("[Mini] Parent process %d exited (code=%d); exiting", pid, code)
			os.Exit(0)
		}
	}
}

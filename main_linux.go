//go:build linux && wails

package main

import (
	"log"
	"os"
	"strconv"
	"syscall"
	"time"
)

// watchParentProcess monitors the parent process (ACLIVE_PARENT_PID) and exits
// when the parent terminates. Uses signal 0 (null signal) to check liveness.
func watchParentProcess() {
	pidStr := os.Getenv("ACLIVE_PARENT_PID")
	if pidStr == "" {
		return
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		return
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		process, err := os.FindProcess(pid)
		if err != nil {
			log.Printf("[Mini] Parent process %d not reachable: %v; exiting", pid, err)
			os.Exit(0)
		}
		// Signal 0 checks if the process exists and we have permission to signal it.
		// ESRCH means the process no longer exists.
		err = process.Signal(syscall.Signal(0))
		if err != nil {
			log.Printf("[Mini] Parent process %d not alive: %v; exiting", pid, err)
			os.Exit(0)
		}
	}
}

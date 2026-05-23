//go:build linux

package main

import (
	"bufio"
	"log"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// sysStatsState holds platform-specific state for system stats tracking.
type sysStatsState struct {
	prevIdle, prevTotal uint64
}

func newSysStatsState() *sysStatsState {
	return &sysStatsState{}
}

func (s *sysStatsState) collect() (cpu, mem float64) {
	cpu = getSystemCPUUsage(&s.prevIdle, &s.prevTotal)
	mem, _ = getSystemMemoryUsage()
	return
}

// getSystemCPUUsage reads /proc/stat and returns the percentage of non-idle CPU time
// since the last call. It compares two snapshots (prevIdle/prevTotal are in/out params).
func getSystemCPUUsage(prevIdle, prevTotal *uint64) float64 {
	f, err := os.Open("/proc/stat")
	if err != nil {
		log.Printf("failed to open /proc/stat: %v", err)
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return 0
	}
	line := scanner.Text()
	if !strings.HasPrefix(line, "cpu ") {
		return 0
	}

	fields := strings.Fields(line)
	if len(fields) < 5 {
		return 0
	}

	// fields[0] = "cpu", fields[1] = user, fields[2] = nice, fields[3] = system, fields[4] = idle, ...
	var idle, total uint64
	for i := 1; i < len(fields); i++ {
		val, err := strconv.ParseUint(fields[i], 10, 64)
		if err != nil {
			return 0
		}
		total += val
		if i == 4 { // idle is the 5th field (index 4)
			idle = val
		}
	}

	if *prevTotal == 0 {
		*prevIdle = idle
		*prevTotal = total
		return 0
	}

	idleDiff := idle - *prevIdle
	totalDiff := total - *prevTotal

	*prevIdle = idle
	*prevTotal = total

	if totalDiff == 0 {
		return 0
	}
	return float64(totalDiff-idleDiff) / float64(totalDiff) * 100.0
}

// getSystemMemoryUsage reads /proc/meminfo and returns the memory usage percentage.
func getSystemMemoryUsage() (float64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var memTotal, memAvailable uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			memTotal = val
		case "MemAvailable:":
			memAvailable = val
		}
		if memTotal > 0 && memAvailable > 0 {
			break
		}
	}
	if memTotal == 0 {
		return 0, nil
	}
	return float64(memTotal-memAvailable) / float64(memTotal) * 100.0, nil
}

// getWindowsFonts is a stub on Linux (not applicable).
func getWindowsFonts() []string {
	return nil
}

// getLinuxFonts returns a sorted list of fonts available on the system using fc-list.
func getLinuxFonts() []string {
	fontSet := map[string]struct{}{
		"Arial":        {},
		"Noto Sans SC": {},
		"sans-serif":   {},
	}

	// Try fc-list for system fonts
	cmd := exec.Command("fc-list", "--format=%{family}\n")
	data, err := cmd.Output()
	if err != nil {
		log.Printf("failed to enumerate fonts via fc-list: %v", err)
	} else {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// fc-list can return comma-separated family names
			for _, family := range strings.Split(line, ",") {
				family = strings.TrimSpace(family)
				if family != "" {
					fontSet[family] = struct{}{}
				}
			}
		}
	}

	result := make([]string, 0, len(fontSet))
	for f := range fontSet {
		result = append(result, f)
	}
	sort.Strings(result)
	return result
}

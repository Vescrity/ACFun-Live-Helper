//go:build windows

package main

import (
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// readWindowsFontRegistry reads installed font names from the Windows registry.
func readWindowsFontRegistry(root registry.Key, fonts map[string]struct{}) {
	key, err := registry.OpenKey(root, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts`, registry.READ)
	if err != nil {
		return
	}
	defer key.Close()
	names, err := key.ReadValueNames(-1)
	if err != nil {
		return
	}
	for _, name := range names {
		font := strings.TrimSpace(name)
		if index := strings.LastIndex(font, "("); index > 0 {
			font = strings.TrimSpace(font[:index])
		}
		if font != "" {
			for _, part := range strings.Split(font, "&") {
				part = strings.TrimSpace(part)
				if part != "" {
					fonts[part] = struct{}{}
				}
			}
		}
	}
}

// getWindowsFonts returns a sorted list of fonts available on Windows.
func getWindowsFonts() []string {
	fonts := map[string]struct{}{
		"Arial":           {},
		"Microsoft YaHei": {},
		"Noto Sans SC":    {},
		"Segoe UI":        {},
		"SimHei":          {},
		"SimSun":          {},
		"sans-serif":      {},
	}
	readWindowsFontRegistry(registry.LOCAL_MACHINE, fonts)
	readWindowsFontRegistry(registry.CURRENT_USER, fonts)

	result := make([]string, 0, len(fonts))
	for font := range fonts {
		result = append(result, font)
	}
	sort.Strings(result)
	return result
}

// getLinuxFonts is a stub on Windows (not applicable).
func getLinuxFonts() []string {
	return nil
}

// --- Windows system stats types and functions ---

type FILETIME struct {
	DwLowDateTime  uint32
	DwHighDateTime uint32
}

type MEMORYSTATUSEX struct {
	DwLength               uint32
	DwMemoryLoad           uint32
	UllTotalPhys           uint64
	UllAvailPhys           uint64
	UllTotalPageFile       uint64
	UllAvailPageFile       uint64
	UllTotalVirtual        uint64
	UllAvailVirtual        uint64
	UllAvailExtendedVirtual uint64
}

var (
	modkernel32              = syscall.NewLazyDLL("kernel32.dll")
	procGetSystemTimes       = modkernel32.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx = modkernel32.NewProc("GlobalMemoryStatusEx")
)

// sysStatsState holds platform-specific state for system stats tracking.
type sysStatsState struct {
	idle1, kernel1, user1 FILETIME
}

func newSysStatsState() *sysStatsState {
	var s sysStatsState
	s.idle1, s.kernel1, s.user1, _ = getSystemTimes()
	return &s
}

func (s *sysStatsState) collect() (cpu, mem float64) {
	idle2, kernel2, user2, err := getSystemTimes()
	if err == nil {
		cpu = calculateCPULoad(s.idle1, s.kernel1, s.user1, idle2, kernel2, user2)
		s.idle1, s.kernel1, s.user1 = idle2, kernel2, user2
	}
	mem, _ = getSystemMemoryUsage()
	return
}

func getSystemTimes() (idle, kernel, user FILETIME, err error) {
	ret, _, errNo := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if ret == 0 {
		err = errNo
	}
	return
}

func getSystemMemoryUsage() (float64, error) {
	var memInfo MEMORYSTATUSEX
	memInfo.DwLength = uint32(unsafe.Sizeof(memInfo))
	ret, _, errNo := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memInfo)))
	if ret == 0 {
		return 0, errNo
	}
	return float64(memInfo.DwMemoryLoad), nil
}

func calculateCPULoad(idle1, kernel1, user1, idle2, kernel2, user2 FILETIME) float64 {
	i1 := (uint64(idle1.DwHighDateTime) << 32) | uint64(idle1.DwLowDateTime)
	k1 := (uint64(kernel1.DwHighDateTime) << 32) | uint64(kernel1.DwLowDateTime)
	u1 := (uint64(user1.DwHighDateTime) << 32) | uint64(user1.DwLowDateTime)

	i2 := (uint64(idle2.DwHighDateTime) << 32) | uint64(idle2.DwLowDateTime)
	k2 := (uint64(kernel2.DwHighDateTime) << 32) | uint64(kernel2.DwLowDateTime)
	u2 := (uint64(user2.DwHighDateTime) << 32) | uint64(user2.DwLowDateTime)

	idleDiff := i2 - i1
	kernelDiff := k2 - k1
	userDiff := u2 - u1

	totalDiff := kernelDiff + userDiff
	if totalDiff == 0 {
		return 0.0
	}
	if totalDiff < idleDiff {
		return 0.0
	}
	return float64(totalDiff-idleDiff) / float64(totalDiff) * 100.0
}

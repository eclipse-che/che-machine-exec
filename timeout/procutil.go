//
// Copyright (c) 2026 Red Hat, Inc.
// This program and the accompanying materials are made
// available under the terms of the Eclipse Public License 2.0
// which is available at https://www.eclipse.org/legal/epl-2.0/
//
// SPDX-License-Identifier: EPL-2.0
//
// Contributors:
//   Red Hat, Inc. - initial API and implementation
//

package timeout

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// systemClockTicks is the number of clock ticks per second (sysconf(_SC_CLK_TCK))
// Detected lazily on first use from /proc/self/auxv with platform-dependent fallback
var (
	systemClockTicks int64
	systemBootTime   time.Time
	systemInitOnce   sync.Once
)

// ensureSystemInfoInitialized lazily initializes system clock ticks and boot time
// Uses sync.Once to ensure initialization happens exactly once, thread-safe
// Only called when needed (avoids /proc reads on non-Linux systems or when CLI watcher unused)
func ensureSystemInfoInitialized() {
	systemInitOnce.Do(func() {
		systemClockTicks = detectClockTicks()
		if systemClockTicks <= 0 {
			logrus.Warnf("CLI Watcher: Failed to detect system clock ticks, using platform default")
			systemClockTicks = getPlatformDefaultClockTicks()
		}
		logrus.Debugf("CLI Watcher: System clock ticks: %d", systemClockTicks)

		systemBootTime = detectSystemBootTime()
		if systemBootTime.IsZero() {
			logrus.Warnf("CLI Watcher: Failed to detect system boot time")
		} else {
			logrus.Debugf("CLI Watcher: System boot time: %s", systemBootTime.Format(time.RFC3339))
		}
	})
}

// detectClockTicks reads AT_CLKTCK from /proc/self/auxv
func detectClockTicks() int64 {
	const AT_CLKTCK = 17 // Auxiliary vector entry for clock ticks

	auxv, err := os.ReadFile("/proc/self/auxv")
	if err != nil {
		return 0
	}

	// auxv is a series of (type, value) pairs as uintptr (native word size)
	// On 64-bit: 8 bytes per value, on 32-bit: 4 bytes per value
	// Use NativeEndian to support both little-endian (x86, ARM) and big-endian (s390x) platforms
	wordSize := strconv.IntSize / 8 // IntSize is 32 or 64 bits, convert to bytes

	for i := 0; i+wordSize*2 <= len(auxv); i += wordSize * 2 {
		var auxType, auxVal uint64

		if wordSize == 8 {
			// 64-bit: need 16 bytes total (8 + 8)
			if i+16 > len(auxv) {
				break
			}
			auxType = binary.NativeEndian.Uint64(auxv[i : i+8])
			auxVal = binary.NativeEndian.Uint64(auxv[i+8 : i+16])
		} else {
			// 32-bit: need 8 bytes total (4 + 4)
			if i+8 > len(auxv) {
				break
			}
			auxType = uint64(binary.NativeEndian.Uint32(auxv[i : i+4]))
			auxVal = uint64(binary.NativeEndian.Uint32(auxv[i+4 : i+8]))
		}

		if auxType == AT_CLKTCK {
			// Sanity check: clock ticks should be in reasonable range
			// Typical values: 100 (x86), 250 (ARM), 1000 (rare)
			// Reject values outside [1, 10000] as corrupted data
			if auxVal >= 1 && auxVal <= 10000 {
				return int64(auxVal)
			}
			// Invalid value detected, return 0 to trigger platform default
			logrus.Warnf("CLI Watcher: Invalid AT_CLKTCK value %d from auxv (expected 1-10000), using platform default", auxVal)
			return 0
		}
	}

	return 0
}

// getPlatformDefaultClockTicks returns platform-specific default clock ticks
// This is a FALLBACK used only if /proc/self/auxv detection fails (very rare)
// Most Linux systems use 100 ticks/sec (x86, RISC-V, PowerPC, MIPS, s390x)
// ARM is the main exception with 250 ticks/sec
func getPlatformDefaultClockTicks() int64 {
	switch runtime.GOARCH {
	case "arm", "arm64":
		return 250 // ARM systems typically use 250
	case "amd64", "386":
		return 100 // x86/x86_64 systems typically use 100
	default:
		// RISC-V, PowerPC, MIPS, s390x, and most others also use 100
		return 100
	}
}

// procStat holds parsed fields from /proc/[pid]/stat
type procStat struct {
	ppid       string // Parent PID (field 4)
	pgrp       int    // Process group ID (field 5)
	tpgid      int    // Foreground process group of TTY (field 8)
	utimeTicks int64  // CPU time in user mode, clock ticks (field 14)
	stimeTicks int64  // CPU time in kernel mode, clock ticks (field 15)
	startTicks int64  // Process start time in clock ticks (field 22)
}

// parseProcStat reads and parses /proc/[pid]/stat once, returning all needed fields
// This avoids multiple reads of the same file for different fields
//
// Note: During detection, the same PID's stat file may be read 2-3 times via different
// callers (getProcessAge, isInForegroundProcessGroup, hasEverReadFromTTY). Caching would
// require threading *procStat through many function layers. Current design prioritizes
// code clarity over the small perf cost (2-3 file reads per detected process per scan).
func parseProcStat(pid string) (*procStat, error) {
	statPath := filepath.Join("/proc", pid, "stat")

	// Add reasonable file size limit to prevent DoS via huge stat files
	const maxStatFileSize = 4096 // 4KB should be more than enough for any real stat file
	file, err := os.Open(statPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			logrus.Debugf("CLI Watcher: Failed to close %s: %v", statPath, closeErr)
		}
	}()

	// Read with size limit
	data := make([]byte, maxStatFileSize)
	n, err := file.Read(data)
	if err != nil && err != io.EOF {
		return nil, err
	}
	data = data[:n] // Truncate to actual read size

	str := string(data)
	// Parse /proc/[pid]/stat - format: pid (comm) state ppid pgrp session tty_nr tpgid ...
	// Need to handle process names with spaces/parens
	lastParen := strings.LastIndex(str, ")")
	if lastParen == -1 {
		return nil, fmt.Errorf("invalid stat format: no closing paren")
	}

	// Fields after ')': state ppid pgrp session tty_nr tpgid flags ... starttime
	fields := strings.Fields(str[lastParen+1:])

	// Add reasonable field count limit (normal stat files have ~50 fields)
	const maxStatFields = 100
	if len(fields) > maxStatFields {
		return nil, fmt.Errorf("stat file has too many fields (%d > %d)", len(fields), maxStatFields)
	}

	if len(fields) < 22 {
		return nil, fmt.Errorf("insufficient fields in stat: %d", len(fields))
	}

	stat := &procStat{}

	// Field 4 (index 1): ppid
	stat.ppid = fields[1]

	// Field 5 (index 2): pgrp
	if n, err := fmt.Sscanf(fields[2], "%d", &stat.pgrp); err != nil || n != 1 {
		return nil, fmt.Errorf("failed to parse pgrp")
	}

	// Field 8 (index 5): tpgid (foreground process group)
	if n, err := fmt.Sscanf(fields[5], "%d", &stat.tpgid); err != nil || n != 1 {
		return nil, fmt.Errorf("failed to parse tpgid")
	}

	// Field 14 (index 11): utime — CPU time in user mode (clock ticks)
	if n, err := fmt.Sscanf(fields[11], "%d", &stat.utimeTicks); err != nil || n != 1 {
		return nil, fmt.Errorf("failed to parse utime")
	}

	// Field 15 (index 12): stime — CPU time in kernel mode (clock ticks)
	if n, err := fmt.Sscanf(fields[12], "%d", &stat.stimeTicks); err != nil || n != 1 {
		return nil, fmt.Errorf("failed to parse stime")
	}

	// Field 22 (index 19): starttime (clock ticks since boot)
	// Validate > 0: starttime=0 is invalid (would mean process started at boot time),
	// and negative values indicate corrupted /proc data
	if n, err := fmt.Sscanf(fields[19], "%d", &stat.startTicks); err != nil || n != 1 || stat.startTicks <= 0 {
		return nil, fmt.Errorf("failed to parse starttime")
	}

	return stat, nil
}

// getProcessStartTime returns when the process started
// Returns zero time if process no longer exists or system info unavailable
func getProcessStartTime(pid string) time.Time {
	// Ensure system info is initialized (lazy init on first call)
	ensureSystemInfoInitialized()

	stat, err := parseProcStat(pid)
	if err != nil {
		// Normal: process may have exited between scan and read
		return time.Time{}
	}

	// Use cached system boot time (initialized lazily)
	bootTime := systemBootTime
	if bootTime.IsZero() {
		// Rare: system boot time detection failed
		return time.Time{}
	}

	// Use detected clock ticks (from /proc/self/auxv or platform default)
	clockTicks := systemClockTicks
	if clockTicks <= 0 {
		clockTicks = 100 // Ultimate fallback
	}

	// Calculate process start time avoiding integer overflow
	// Use floating point to prevent overflow: (startTicks * 1000) could overflow for long-running processes
	// Formula: bootTime + (startTicks / clockTicks) seconds
	startTimeMs := int64(float64(stat.startTicks) * 1000.0 / float64(clockTicks))
	startTime := bootTime.Add(time.Duration(startTimeMs) * time.Millisecond)
	return startTime
}

// detectSystemBootTime reads boot time from /proc/stat
// Called lazily via ensureSystemInfoInitialized(), cached in systemBootTime global
func detectSystemBootTime() time.Time {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "btime ") {
			var bootSec int64
			if n, err := fmt.Sscanf(line, "btime %d", &bootSec); err != nil || n != 1 || bootSec <= 0 {
				return time.Time{}
			}
			return time.Unix(bootSec, 0)
		}
	}
	return time.Time{}
}

// getProcessAge returns how long the process has been running
// Returns 0 if process start time cannot be determined or if system clock skew results in negative age
func getProcessAge(pid string) time.Duration {
	startTime := getProcessStartTime(pid)
	if startTime.IsZero() {
		return 0
	}
	age := time.Since(startTime)
	// Handle clock skew: if system clock was set backward after process started,
	// treat as age 0 (just started) to ensure grace period protection
	if age < 0 {
		return 0
	}
	return age
}

// getProcessCPUTicks returns the total CPU ticks (utime+stime) consumed by the process.
func getProcessCPUTicks(pid string) (int64, bool) {
	stat, err := parseProcStat(pid)
	if err != nil {
		return 0, false
	}
	return stat.utimeTicks + stat.stimeTicks, true
}

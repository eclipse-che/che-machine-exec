//
// Copyright (c) 2025-2026 Red Hat, Inc.
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
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type InteractiveMode string

const (
	InteractiveModeAuto  InteractiveMode = "auto"
	InteractiveModeTrue  InteractiveMode = "true"
	InteractiveModeFalse InteractiveMode = "false"
	InteractiveModeYes   InteractiveMode = "yes"
	InteractiveModeNo    InteractiveMode = "no"
)

type ForceWatchMode string

const (
	ForceWatchModeTrue  ForceWatchMode = "true"
	ForceWatchModeFalse ForceWatchMode = "false"
	ForceWatchModeYes   ForceWatchMode = "yes"
	ForceWatchModeNo    ForceWatchMode = "no"
)

// isForceWatchEnabled checks if ForceWatchMode is enabled (true/yes)
func (f ForceWatchMode) isEnabled() bool {
	return f == ForceWatchModeTrue || f == ForceWatchModeYes
}

const (
	DefaultInteractiveMode InteractiveMode = InteractiveModeNo // Backward compatible: always prevent idling
	DefaultCheckPeriod                     = 60                // Default check period: 60 seconds
	DefaultActivityWindow                  = 25 * time.Minute  // Default activity window: 25 minutes (fallback when idle timeout unavailable)
	DefaultGracePeriod                     = 5 * time.Minute   // Default grace period: 5 minutes
	DefaultMaxProcessAge                   = 6 * time.Hour     // Default max process age: 6 hours - safety limit

	MinActivityWindow    = 2 * time.Minute // Minimum activity window for very short idle timeouts
	MinGracePeriod       = 1 * time.Minute // Minimum grace period
	MinCheckPeriod       = 10              // Minimum check period in seconds
	SafetyBufferDuration = 5 * time.Minute // Safety buffer between activity window and idle timeout
	SafetyBufferPercent  = 0.2             // Or 20% of idle timeout, whichever is smaller
)

// ttyCache holds cached TTY device information to reduce redundant filesystem operations
type ttyCache struct {
	path       string    // TTY device path (e.g., "/dev/pts/1")
	atime      time.Time // Last access time
	cachedAt   time.Time // When this was cached
	valid      bool      // Whether the TTY path resolution was successful
}

// TTY cache with short TTL to avoid stale data across scan cycles
var (
	ttyPathCache      = make(map[string]*ttyCache)
	ttyPathCacheMutex sync.RWMutex
)
const (
	ttyCacheDuration  = 2 * time.Second
	ttyCacheMaxSize   = 1000 // Maximum entries to prevent unbounded growth
	ttyCacheCleanupAt = 800  // Trigger cleanup when reaching this size
)

// cleanupTTYCache removes expired and dead PID entries from the cache
// MUST be called with ttyPathCacheMutex write lock held
func cleanupTTYCache() {
	now := time.Now()
	for pid, entry := range ttyPathCache {
		// Remove if expired
		if now.Sub(entry.cachedAt) >= ttyCacheDuration {
			delete(ttyPathCache, pid)
			continue
		}

		// Remove if PID no longer exists (quick check without filesystem calls)
		if _, err := os.Stat(filepath.Join("/proc", pid)); os.IsNotExist(err) {
			delete(ttyPathCache, pid)
		}
	}
}

type WatchedCommand struct {
	Name        string          `yaml:"name"`
	Interactive InteractiveMode `yaml:"interactive"`
	ForceWatch  ForceWatchMode  `yaml:"forceWatch"`
}

// UnmarshalYAML allows WatchedCommand to be unmarshaled from either a string or an object
func (w *WatchedCommand) UnmarshalYAML(unmarshal func(any) error) error {
	// Try to unmarshal as a string first (backward compatible)
	var str string
	if err := unmarshal(&str); err == nil {
		w.Name = str
		w.Interactive = DefaultInteractiveMode
		return nil
	}

	// Otherwise, unmarshal as a struct
	type rawWatchedCommand WatchedCommand
	var raw rawWatchedCommand
	if err := unmarshal(&raw); err != nil {
		return err
	}

	*w = WatchedCommand(raw)
	return nil
}

type cliWatcherConfig struct {
	WatchedCommands       []WatchedCommand `yaml:"watchedCommands"`
	IgnoredCommands       []string         `yaml:"ignoredCommands" json:"-"`
	CheckPeriodSeconds    int              `yaml:"checkPeriodSeconds"` // Deprecated: use CheckPeriod instead (kept for backward compatibility)
	CheckPeriod           string           `yaml:"checkPeriod"`
	ActivityWindow        string           `yaml:"activityWindow"`
	GracePeriod           string           `yaml:"gracePeriod"`
	MaxProcessAge         string           `yaml:"maxProcessAge"`
	Enabled               bool             `yaml:"enabled"`
	_lastModTime          time.Time        `json:"-"`
	_checkPeriodParsed    time.Duration    `json:"-"`
	_activityWindowParsed time.Duration    `json:"-"`
	_gracePeriodParsed    time.Duration    `json:"-"`
	_maxProcessAgeParsed  time.Duration    `json:"-"`
}

// Watcher monitors CLI processes and invokes a tick callback when active ones are found
type cliWatcher struct {
	mu                  sync.Mutex    // Protects config, warnedMissingConfig, started
	config              *cliWatcherConfig
	warnedMissingConfig bool
	stopChan            chan struct{}
	stopOnce            sync.Once     // Ensures stopChan is only closed once
	started             bool
	tickFunc            func()        // Immutable after construction (safe to read without lock)
	myPID               string        // Immutable after construction (safe to read without lock)
	idleTimeout         time.Duration // Immutable after construction (safe to read without lock)
}

// Commands that should NEVER prevent workspace idling (passive monitoring tools)
var alwaysIgnoredCommands = []string{"tail", "watch", "top", "htop"}

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

// New creates a new Watcher with the given config and tick callback
func NewCliWatcher(tickFunc func(), idleTimeout time.Duration) *cliWatcher {
	if tickFunc == nil {
		logrus.Warnf("CLI Watcher: Created with nil tick callback - activity will not be reported")
	}
	return &cliWatcher{
		stopChan:    make(chan struct{}),
		tickFunc:    tickFunc,
		myPID:       fmt.Sprintf("%d", os.Getpid()),
		idleTimeout: idleTimeout,
	}
}

// Start begins the watcher loop
func (w *cliWatcher) Start() {
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	w.started = true
	w.mu.Unlock()

	go func() {
		var err error
		w.mu.Lock()
		w.config, err = w.loadConfig(getConfigPath(), w.config)
		w.mu.Unlock()
		if err != nil {
			logrus.Errorf("CLI Watcher: Failed to reload config: %v", err)
		}

		w.mu.Lock()
		chkPeriod := DefaultCheckPeriod
		if w.config != nil && w.config._checkPeriodParsed > 0 {
			chkPeriod = int(w.config._checkPeriodParsed.Seconds())
		}
		w.mu.Unlock()

		ticker := time.NewTicker(time.Duration(chkPeriod) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-w.stopChan:
				logrus.Infof("CLI Watcher: Stopped")
				return

			case <-ticker.C:
				oldPeriod := chkPeriod

				// Reload config (protected)
				w.mu.Lock()
				w.config, err = w.loadConfig(getConfigPath(), w.config)
				configSnapshot := w.config // Take snapshot for use outside lock
				w.mu.Unlock()
				if err != nil {
					logrus.Errorf("CLI Watcher: Failed to reload config: %v", err)
				}

				if configSnapshot == nil || !configSnapshot.Enabled {
					if chkPeriod != DefaultCheckPeriod {
						logrus.Infof("CLI Watcher: Config was removed or disabled — resetting check period to default (%ds)", DefaultCheckPeriod)
						chkPeriod = DefaultCheckPeriod
						ticker.Stop()
						// Recreate ticker with new period
						ticker = time.NewTicker(time.Duration(chkPeriod) * time.Second)
					}
					continue
				}

				newPeriod := int(configSnapshot._checkPeriodParsed.Seconds())
				if newPeriod > 0 && newPeriod != oldPeriod {
					logrus.Infof("CLI Watcher: Detected new check period: %d seconds (was %d), restarting ticker", newPeriod, oldPeriod)
					chkPeriod = newPeriod
					ticker.Stop()
					// Recreate ticker with new period
					ticker = time.NewTicker(time.Duration(chkPeriod) * time.Second)
				}

				found, name := isWatchedProcessRunning(configSnapshot, w.myPID)
				if found {
					logrus.Debugf("CLI Watcher: Detected CLI command: %s — reporting activity tick", name)
					if w.tickFunc != nil {
						w.tickFunc()
					}
				}
			}
		}
	}()

	logrus.Infof("CLI Watcher: Started")
}

// Stop terminates the watcher loop
func (w *cliWatcher) Stop() {
	w.mu.Lock()
	wasStarted := w.started
	if wasStarted {
		w.started = false
	}
	w.mu.Unlock()

	if !wasStarted {
		return
	}

	// Use sync.Once to ensure channel is only closed once, even if Stop() called concurrently
	w.stopOnce.Do(func() {
		close(w.stopChan)
	})
}

// Scans /proc to check if any watched process is running and active
func isWatchedProcessRunning(config *cliWatcherConfig, myPID string) (bool, string) {
	// Handle nil config
	if config == nil {
		return false, ""
	}

	procEntries, err := os.ReadDir("/proc")
	if err != nil {
		logrus.Warnf("CLI Watcher: Cannot read /proc: %v", err)
		return false, ""
	}

	for _, entry := range procEntries {
		if !entry.IsDir() || !isNumeric(entry.Name()) {
			continue
		}

		pid := entry.Name()
		if pid == "1" || pid == myPID { // Skip PID 1 and ourselves
			continue
		}

		// FIRST CHECK: Only process user-initiated work (has TTY + main user process exists)
		if !isUserInitiatedProcess(pid) {
			continue
		}

		// Get command name from /proc/[pid]/comm (shows invoked command name, not underlying binary)
		// This handles multicall binaries like coreutils where cmdline shows the actual binary
		// but comm shows the invoked command (e.g., "tail" not "coreutils")
		commPath := filepath.Join("/proc", pid, "comm")
		commData, err := os.ReadFile(commPath)
		if err != nil {
			continue
		}

		cmdName := strings.TrimSpace(string(commData))
		if cmdName == "" {
			continue
		}

		// STEP 1: Check if command is in always-ignored list OR config ignored list
		if slices.Contains(alwaysIgnoredCommands, cmdName) {
			logrus.Debugf("CLI Watcher: Process %s (PID %s) is in always-ignored list, skipping", cmdName, pid)
			continue
		}
		if slices.Contains(config.IgnoredCommands, cmdName) {
			logrus.Debugf("CLI Watcher: Process %s (PID %s) is in config ignored list, skipping", cmdName, pid)
			continue
		}

		// STEP 2: Check if command is explicitly configured
		var configuredCmd *WatchedCommand
		for i := range config.WatchedCommands {
			if config.WatchedCommands[i].Name == cmdName {
				configuredCmd = &config.WatchedCommands[i]
				break
			}
		}

		// STEP 3: Safety check - don't prevent idling for processes older than maxProcessAge
		processAge := getProcessAge(pid)
		maxAge := config._maxProcessAgeParsed
		if maxAge <= 0 {
			maxAge = DefaultMaxProcessAge
		}
		if processAge > 0 && processAge > maxAge {
			logrus.Warnf("CLI Watcher: Process %s (PID %s) exceeds max age (%v, limit: %v), no longer preventing idling (safety limit)", cmdName, pid, processAge, maxAge)
			continue
		}

		// STEP 4: Grace period - all young processes prevent idling
		gracePeriod := config._gracePeriodParsed
		if gracePeriod <= 0 {
			gracePeriod = DefaultGracePeriod
		}
		if processAge == 0 {
			// Can't determine age (getProcessStartTime failed) - give benefit of doubt with grace period
			logrus.Debugf("CLI Watcher: Process %s (PID %s) age unknown, applying grace period protection", cmdName, pid)
			return true, cmdName
		}
		if processAge < gracePeriod {
			logrus.Debugf("CLI Watcher: Process %s (PID %s) in grace period (age: %v), preventing idling", cmdName, pid, processAge)
			return true, cmdName
		}

		// STEP 5: Apply policy based on configuration or defaults
		var mode InteractiveMode
		var policySource string
		if configuredCmd != nil {
			mode = configuredCmd.Interactive
			if mode == "" {
				mode = DefaultInteractiveMode
			}
			policySource = "configured"
		} else {
			mode = InteractiveModeAuto // Auto-detect for unconfigured commands
			policySource = "default"
		}

		if !applyPolicy(pid, cmdName, mode, config._activityWindowParsed, policySource) {
			continue
		}

		return true, cmdName
	}

	return false, ""
}

func isNumeric(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// procStat holds parsed fields from /proc/[pid]/stat
type procStat struct {
	ppid       string // Parent PID (field 4)
	pgrp       int    // Process group ID (field 5)
	tpgid      int    // Foreground process group of TTY (field 8)
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
	defer file.Close()

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

	// Field 22 (index 19): starttime (clock ticks since boot)
	// Validate > 0: starttime=0 is invalid (would mean process started at boot time),
	// and negative values indicate corrupted /proc data
	if n, err := fmt.Sscanf(fields[19], "%d", &stat.startTicks); err != nil || n != 1 || stat.startTicks <= 0 {
		return nil, fmt.Errorf("failed to parse starttime")
	}

	return stat, nil
}

// applyPolicy applies the interactive policy for a command
// Returns true if process should prevent idling, false otherwise
// Unified function handling both configured and default policies
func applyPolicy(pid, cmdName string, mode InteractiveMode, activityWindow time.Duration, policySource string) bool {
	// Determine if process is interactive
	var checkActivity bool

	switch mode {
	case InteractiveModeAuto:
		// Auto-detect: use foreground + TTY read analysis
		checkActivity = isInteractiveProcess(pid)
		if checkActivity {
			logrus.Debugf("CLI Watcher: Process %s (PID %s) auto-detected as interactive (%s policy)", cmdName, pid, policySource)
		} else {
			logrus.Debugf("CLI Watcher: Process %s (PID %s) auto-detected as work process (%s policy)", cmdName, pid, policySource)
		}

	case InteractiveModeTrue, InteractiveModeYes:
		// Force interactive mode
		checkActivity = true
		logrus.Debugf("CLI Watcher: Process %s (PID %s) forced interactive (%s policy)", cmdName, pid, policySource)

	case InteractiveModeFalse, InteractiveModeNo:
		// Force non-interactive (work) mode
		checkActivity = false
		logrus.Debugf("CLI Watcher: Process %s (PID %s) forced non-interactive (%s policy)", cmdName, pid, policySource)
	}

	// If interactive, check for recent activity
	if checkActivity {
		if !hasRecentActivity(activityWindow, pid) {
			logrus.Debugf("CLI Watcher: Process %s (PID %s) is interactive but no recent activity (%s policy)", cmdName, pid, policySource)
			return false
		}
		logrus.Debugf("CLI Watcher: Process %s (PID %s) is interactive with recent activity (%s policy)", cmdName, pid, policySource)
	}

	return true
}

// getParentPID returns the parent PID of a given process
// Returns empty string if process no longer exists or /proc read fails
func getParentPID(pid string) string {
	stat, err := parseProcStat(pid)
	if err != nil {
		// Normal: process may have exited between scan and read
		return ""
	}
	return stat.ppid
}

// getMainUserProcess walks up the process tree to find the first parent without TTY
// Returns the main user process PID and true if found, empty string and false otherwise
// Protected against infinite loops with max depth limit and cycle detection
func getMainUserProcess(pid string) (string, bool) {
	// Maximum parent chain depth to prevent infinite loops
	// Rationale: Typical process chains are 2-5 deep (terminal → shell → command)
	// Even pathological cases (deeply nested tmux/screen/containers) rarely exceed 20
	// 64 provides ample headroom while preventing runaway traversal on corrupted /proc
	const maxDepth = 64
	current := pid
	visited := make(map[string]bool, maxDepth) // Pre-allocate for worst-case to avoid reallocations

	for depth := 0; depth < maxDepth; depth++ {
		// Mark current as visited BEFORE processing to detect cycles early
		if visited[current] {
			logrus.Warnf("CLI Watcher: Detected cycle in process tree at PID %s", current)
			return "", false
		}
		visited[current] = true

		parent := getParentPID(current)

		// Check for self-parent (corruption)
		if parent == current {
			logrus.Warnf("CLI Watcher: Process %s claims to be its own parent (corrupted /proc)", current)
			return "", false
		}

		// Reached top of process tree
		if parent == "" || parent == "0" || parent == "1" {
			return "", false // Reached top without finding main user process
		}

		// Check if parent has NO TTY - that's our main user process
		if !processHasTTY(parent) {
			return parent, true
		}

		current = parent
	}

	// Max depth exceeded - highly unlikely to be a user terminal process
	logrus.Warnf("CLI Watcher: Max depth (%d) exceeded walking process tree from PID %s", maxDepth, pid)
	return "", false
}

// isUserInitiatedProcess checks if a process is user-initiated by verifying:
// 1. It has a TTY
// 2. Its parent also has TTY (filters out shells themselves - bash/sh/zsh parent has no TTY)
// 3. Walking up the parent chain leads to a process without TTY (main user process)
func isUserInitiatedProcess(pid string) bool {
	// Must have TTY
	if !processHasTTY(pid) {
		return false
	}

	// Parent must exist and not be init process (PID 1) or kernel (PID 0)
	parent := getParentPID(pid)
	if parent == "" || parent == "0" || parent == "1" {
		return false
	}

	// Parent must also have TTY (filters out shells - shell has TTY but parent doesn't)
	if !processHasTTY(parent) {
		return false
	}

	// Find main user process (first parent without TTY in the chain)
	_, found := getMainUserProcess(pid)
	return found
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

// isInForegroundProcessGroup checks if process is in the foreground process group of its TTY
func isInForegroundProcessGroup(pid string) bool {
	stat, err := parseProcStat(pid)
	if err != nil {
		return false
	}

	// If tpgid == -1, no foreground process group
	// If pgrp == tpgid, this process is in foreground
	return stat.tpgid > 0 && stat.pgrp == stat.tpgid
}

// getWaitChannel returns what the process is waiting on
func getWaitChannel(pid string) string {
	wchanPath := filepath.Join("/proc", pid, "wchan")
	data, err := os.ReadFile(wchanPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// getCachedTTYInfo gets TTY device path and access time with caching to reduce filesystem operations
func getCachedTTYInfo(pid string) (string, time.Time, bool) {
	// Check cache first (read lock)
	ttyPathCacheMutex.RLock()
	cached, exists := ttyPathCache[pid]
	if exists && time.Since(cached.cachedAt) < ttyCacheDuration {
		// Cache hit and still valid
		if !cached.valid {
			ttyPathCacheMutex.RUnlock()
			return "", time.Time{}, false
		}
		// Use cached path to get fresh atime (filesystem call outside lock)
		cachedPath := cached.path
		ttyPathCacheMutex.RUnlock()

		// Update access time from cached path
		var stat syscall.Stat_t
		if err := syscall.Stat(cachedPath, &stat); err != nil {
			return cachedPath, cached.atime, false // Use stale atime if stat fails
		}
		return cachedPath, time.Unix(stat.Atim.Sec, stat.Atim.Nsec), true
	}
	ttyPathCacheMutex.RUnlock()

	// Cache miss or expired - resolve TTY path (expensive operations outside locks)
	fd0Path := filepath.Join("/proc", pid, "fd", "0")
	target, err := os.Readlink(fd0Path)
	if err != nil {
		// Cache failure result (write lock)
		ttyPathCacheMutex.Lock()
		ttyPathCache[pid] = &ttyCache{cachedAt: time.Now(), valid: false}
		ttyPathCacheMutex.Unlock()
		return "", time.Time{}, false
	}

	if !strings.HasPrefix(target, "/dev/pts/") && !strings.HasPrefix(target, "/dev/tty") {
		// Cache invalid TTY result (write lock)
		ttyPathCacheMutex.Lock()
		ttyPathCache[pid] = &ttyCache{cachedAt: time.Now(), valid: false}
		ttyPathCacheMutex.Unlock()
		return "", time.Time{}, false
	}

	// Get access time
	var stat syscall.Stat_t
	if err := syscall.Stat(target, &stat); err != nil {
		// Cache path but failed stat (write lock)
		ttyPathCacheMutex.Lock()
		ttyPathCache[pid] = &ttyCache{path: target, cachedAt: time.Now(), valid: false}
		ttyPathCacheMutex.Unlock()
		return target, time.Time{}, false
	}

	atime := time.Unix(stat.Atim.Sec, stat.Atim.Nsec)

	// Cache successful result (write lock)
	ttyPathCacheMutex.Lock()

	// Trigger cleanup if cache is getting large
	if len(ttyPathCache) >= ttyCacheCleanupAt {
		cleanupTTYCache()
	}

	// Enforce maximum cache size (fallback if cleanup didn't free enough space)
	if len(ttyPathCache) >= ttyCacheMaxSize {
		// Remove oldest entries until we're comfortably under the cleanup threshold
		targetSize := ttyCacheCleanupAt - 50 // Leave some headroom
		for len(ttyPathCache) > targetSize {
			// Find and remove the oldest entry
			oldestTime := time.Now()
			var oldestPID string
			for cachePID, entry := range ttyPathCache {
				if entry.cachedAt.Before(oldestTime) {
					oldestTime = entry.cachedAt
					oldestPID = cachePID
				}
			}
			if oldestPID != "" {
				delete(ttyPathCache, oldestPID)
			} else {
				// Safety break - shouldn't happen but prevents infinite loop
				break
			}
		}
	}

	ttyPathCache[pid] = &ttyCache{
		path:     target,
		atime:    atime,
		cachedAt: time.Now(),
		valid:    true,
	}
	ttyPathCacheMutex.Unlock()

	return target, atime, true
}

// getTTYAtime returns the access time of the process's TTY
func getTTYAtime(pid string) time.Time {
	_, atime, valid := getCachedTTYInfo(pid)
	if !valid {
		return time.Time{}
	}
	return atime
}

// hasEverReadFromTTY checks if the process has ever read from its TTY
// NOTE: This depends on filesystem access time (atime) being updated.
// On filesystems mounted with 'noatime' or 'relatime', this may not work reliably.
func hasEverReadFromTTY(pid string) bool {
	startTime := getProcessStartTime(pid)
	if startTime.IsZero() {
		return false
	}

	ttyAtime := getTTYAtime(pid)
	if ttyAtime.IsZero() {
		return false
	}

	// If TTY was accessed after process started, it has read input
	if ttyAtime.After(startTime) {
		return true
	}

	// Atime failed - fall back to alternative detection methods
	logrus.Debugf("CLI Watcher: TTY atime for PID %s unavailable or unreliable, using fallback detection", pid)
	return hasInteractiveBehaviorFallback(pid)
}

// hasInteractiveBehaviorFallback uses alternative methods when atime is unavailable
// Combines: process state analysis, enhanced wchan analysis, and FD analysis
func hasInteractiveBehaviorFallback(pid string) bool {
	score := 0

	// Method #1: Process State Analysis
	// Interactive processes are typically sleeping (waiting for input)
	if state := getProcessState(pid); state == "S" {
		score += 2 // Sleeping = likely waiting for input
	}

	// Method #2: Enhanced wchan Analysis (beyond basic TTY read)
	wchan := getWaitChannel(pid)
	if wchan == "poll_schedule_timeout" || // Polling with timeout (interactive pattern)
		wchan == "pipe_wait" || // Waiting on pipe input
		wchan == "unix_stream_read_generic" || // Reading from socket
		wchan == "select" || // Select/poll waiting for input
		wchan == "ep_poll" { // Epoll waiting (event-driven input)
		score += 3 // Strong indicator of waiting for input
	}

	// Method #3: File Descriptor Analysis
	// Check if stdin is actively connected to TTY
	if hasActiveTTYConnection(pid) {
		score += 2
	}

	// Threshold: score >= 4 indicates interactive behavior
	// This is conservative - when in doubt, assume interactive to prevent false negatives
	isInteractive := score >= 4
	if isInteractive {
		logrus.Debugf("CLI Watcher: PID %s detected as interactive via fallback (score: %d, wchan: %s)", pid, score, wchan)
	}
	return isInteractive
}

// getProcessState returns the process state from /proc/[pid]/stat field 3
func getProcessState(pid string) string {
	// Read the raw stat file for state (field 3)
	statPath := filepath.Join("/proc", pid, "stat")
	data, err := os.ReadFile(statPath)
	if err != nil {
		return ""
	}

	str := string(data)
	// Find the last ')' to handle process names with spaces/parens
	lastParen := strings.LastIndex(str, ")")
	if lastParen == -1 {
		return ""
	}

	// State is the first field after ')'
	fields := strings.Fields(str[lastParen+1:])
	if len(fields) > 0 {
		return fields[0] // State (R/S/D/Z/T)
	}
	return ""
}

// hasActiveTTYConnection checks if process has active TTY file descriptors
func hasActiveTTYConnection(pid string) bool {
	// Check if stdin (fd/0) points to a TTY and is recently accessed
	fd0Path := filepath.Join("/proc", pid, "fd", "0")
	target, err := os.Readlink(fd0Path)
	if err != nil {
		return false
	}

	// Must be a TTY device
	if !strings.HasPrefix(target, "/dev/pts/") && !strings.HasPrefix(target, "/dev/tty") {
		return false
	}

	// Check if the fd directory itself has been recently modified
	// This indicates recent file descriptor activity
	fdDir := filepath.Join("/proc", pid, "fd")
	stat, err := os.Stat(fdDir)
	if err != nil {
		return false
	}

	// If fd directory was modified recently, there's active FD usage
	return time.Since(stat.ModTime()) < 5*time.Minute
}

// isInteractiveProcess detects if a process is interactive by checking:
// 1. Is it in foreground process group?
// 2. Is it waiting on TTY read OR has it ever read from TTY?
func isInteractiveProcess(pid string) bool {
	if !isInForegroundProcessGroup(pid) {
		return false // Background processes are not interactive
	}

	wchan := getWaitChannel(pid)

	// Currently waiting on TTY/terminal read?
	// Use exact matching to avoid false positives (e.g., "spreadsheet", "thread_reading")
	if wchan == "read" || // Generic read syscall on TTY
		wchan == "wait_woken" || // Terminal I/O wait
		wchan == "n_tty_read" || // TTY line discipline read
		wchan == "tty_read" || // TTY read
		wchan == "tty_write" { // TTY write (also indicates terminal interaction)
		return true
	}

	// Has it ever read from TTY?
	if hasEverReadFromTTY(pid) {
		return true
	}

	return false // Foreground but never read input = work process
}

// getInteractiveModeDescription returns a human-readable description of the interactive mode
func getInteractiveModeDescription(mode InteractiveMode) string {
	switch mode {
	case InteractiveModeAuto:
		return "auto-detect TTY"
	case InteractiveModeTrue, InteractiveModeYes:
		return "interactive (activity check)"
	case InteractiveModeFalse, InteractiveModeNo:
		return "non-interactive (always active)"
	default:
		return "unknown"
	}
}

// processHasTTY checks if a process has a controlling TTY
func processHasTTY(pid string) bool {
	// Check stdin (fd 0) for TTY
	fd0Path := filepath.Join("/proc", pid, "fd", "0")
	target, err := os.Readlink(fd0Path)
	if err != nil {
		return false
	}

	// TTY devices are typically /dev/pts/N or /dev/tty*
	return strings.HasPrefix(target, "/dev/pts/") ||
		strings.HasPrefix(target, "/dev/tty")
}

// hasRecentActivity checks if a process has had recent I/O activity
func hasRecentActivity(activityWindow time.Duration, pid string) bool {
	window := activityWindow
	if window <= 0 {
		window = DefaultActivityWindow
	}

	// Check TTY access time for user input activity
	return hasTTYActivity(pid, window)
}

// hasTTYActivity checks if the TTY has been accessed recently
func hasTTYActivity(pid string, window time.Duration) bool {
	_, atime, valid := getCachedTTYInfo(pid)
	if !valid {
		return false
	}

	threshold := time.Now().Add(-window)
	return atime.After(threshold)
}

// Finds the CLI Watcher configuration file in:
// 1. Use explicit override by using "CLI_WATCHER_CONFIG" env. variable, or if not set then
// 2. Search for '.noidle' upward from current project directory up to "PROJECTS_ROOT" directory, or
// 3. Fallback to $HOME/.<binary> file, or if doesn't exist/isn't accessble then
// 4. Otherwise, give up. Repeating the search on next run (thus waiting for a config to appear)
func getConfigPath() string {

	// 1. Use explicit override
	if configEnv := os.Getenv("CLI_WATCHER_CONFIG"); configEnv != "" {
		return configEnv
	}

	const configFileName = ".noidle"

	// 2. Search upward from current project directory
	root := os.Getenv("PROJECTS_ROOT")
	if root == "" {
		root = "/"
	}

	start := os.Getenv("PROJECT_SOURCE")
	if start == "" {
		start = os.Getenv("PROJECTS_ROOT")
	}

	if start == "" {
		start, _ = os.Getwd()
	}

	if path := findUpward(start, root, configFileName); path != "" {
		return path
	}

	// 3. Fallback to $HOME/.<binary>
	if home := os.Getenv("HOME"); home != "" && home != "/" {
		homeCfg := filepath.Join(home, configFileName)
		if _, err := os.Stat(homeCfg); err == nil {
			return homeCfg
		}
	}

	// 4. Give up
	return ""
}

func findUpward(start, stop, filename string) string {
	const maxIterations = 100 // Safety limit to prevent infinite loops

	// Resolve symlinks in start path to ensure consistent traversal
	current, err := filepath.EvalSymlinks(start)
	if err != nil {
		// If symlink resolution fails (e.g., broken symlink), use original path
		current = start
	}

	// Also resolve stop to ensure comparison works correctly
	stopResolved, err := filepath.EvalSymlinks(stop)
	if err != nil {
		// If symlink resolution fails, use original path
		stopResolved = stop
	}

	for i := 0; i < maxIterations; i++ {
		candidate := filepath.Join(current, filename)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}

		if current == stopResolved || current == "/" {
			break
		}

		parent := filepath.Dir(current)
		if parent == current { // root reached
			break
		}
		current = parent
	}
	return ""
}

// Loads `.noidle` configuration file (or the one that is specified in ” environment variable) into the CLI Watcher configuration struct.
// Example configuraiton file:
// ```yaml
//
//	enabled: true
//	checkPeriodSeconds: 30
//	watchedCommands:
//	  - helm
//	  - odo
//	  - sleep
//
// ````
func (w *cliWatcher) loadConfig(path string, current *cliWatcherConfig) (*cliWatcherConfig, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		if current != nil {
			logrus.Infof("CLI Watcher: Config file at %s was removed, stopping config-based detection", path)
		} else if !w.warnedMissingConfig {
			if strings.TrimSpace(path) == "" {
				logrus.Infof("CLI Watcher: Config file not found, waiting for it to appear...")
			} else {
				logrus.Infof("CLI Watcher: Config file not found at %s, waiting for it to appear...", path)
			}
			w.warnedMissingConfig = true
		}
		return nil, nil
	} else if err != nil {
		return current, fmt.Errorf("CLI Watcher: Failed to stat config file: %w", err)
	}

	if w.warnedMissingConfig {
		logrus.Infof("CLI Watcher: Config file appeared at %s", path)
		w.warnedMissingConfig = false
	}

	if current != nil && !info.ModTime().After(current._lastModTime) {
		return current, nil // no change
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return current, fmt.Errorf("CLI Watcher: Failed to read config file: %w", err)
	}

	var newCfg cliWatcherConfig
	if err := yaml.Unmarshal(data, &newCfg); err != nil {
		// Log helpful error with context
		logrus.Errorf("CLI Watcher: Failed to parse config file at %s", path)
		logrus.Errorf("  Error: %v", err)
		logrus.Errorf("  Hint: Check that 'watchedCommands' entries are either strings or objects with 'name:' field")
		if current != nil {
			logrus.Errorf("  Keeping previous valid config until syntax is fixed.")
		}
		// Return error so caller can distinguish "config broken" from "config unchanged"
		return current, fmt.Errorf("failed to parse config file: %w", err)
	}

	newCfg._lastModTime = info.ModTime()
	newCfg = applyDefaults(newCfg, w.idleTimeout)
	newCfg = ignoreExclusions(alwaysIgnoredCommands, newCfg)

	// Log config changes
	logrus.Infof("CLI Watcher: Config reloaded from %s", path)
	if current != nil && current.Enabled {
		if current._checkPeriodParsed != newCfg._checkPeriodParsed {
			logrus.Infof("CLI Watcher:   Check period changed: %v → %v", current._checkPeriodParsed, newCfg._checkPeriodParsed)
		}
		if current._activityWindowParsed != newCfg._activityWindowParsed {
			logrus.Infof("CLI Watcher:   Activity window changed: %v → %v", current._activityWindowParsed, newCfg._activityWindowParsed)
		}
		if current._gracePeriodParsed != newCfg._gracePeriodParsed {
			logrus.Infof("CLI Watcher:   Grace period changed: %v → %v", current._gracePeriodParsed, newCfg._gracePeriodParsed)
		}
		if current._maxProcessAgeParsed != newCfg._maxProcessAgeParsed {
			logrus.Infof("CLI Watcher:   Max process age changed: %v → %v", current._maxProcessAgeParsed, newCfg._maxProcessAgeParsed)
		}
	}

	if newCfg.Enabled {
		if len(newCfg.WatchedCommands) > 0 {
			logrus.Infof("CLI Watcher:   Watching ALL user processes with %d explicit override(s):", len(newCfg.WatchedCommands))
			for _, cmd := range newCfg.WatchedCommands {
				modeDesc := getInteractiveModeDescription(cmd.Interactive)
				logrus.Infof("CLI Watcher:     - %s (mode: %s)", cmd.Name, modeDesc)
			}
		} else {
			logrus.Infof("CLI Watcher:   Watching ALL user processes (no explicit overrides)")
		}
		if len(newCfg.IgnoredCommands) > 0 {
			logrus.Warnf("CLI Watcher:   WARNING: You configured %v in watchedCommands, but these are globally excluded (always ignored). Remove them from your config to silence this warning.", newCfg.IgnoredCommands)
		}
		logrus.Infof("CLI Watcher:   Always-ignored commands (never prevent idling): %v", alwaysIgnoredCommands)
		logrus.Infof("CLI Watcher:   Detection period: %v", newCfg._checkPeriodParsed)
		logrus.Infof("CLI Watcher:   Activity window: %v", newCfg._activityWindowParsed)
		logrus.Infof("CLI Watcher:   Grace period: %v", newCfg._gracePeriodParsed)
		logrus.Infof("CLI Watcher:   Max process age: %v (safety limit)", newCfg._maxProcessAgeParsed)
	} else {
		logrus.Infof("CLI Watcher: Disabled by configuration. CLI idling prevention is turned off.")
	}

	return &newCfg, nil
}

// Remove excluded CLIs from the watcher configuration.
func ignoreExclusions(exclusions []string, cfg cliWatcherConfig) cliWatcherConfig {
	var filtered []WatchedCommand
	var ignored []string

	for _, cmd := range cfg.WatchedCommands {
		name := strings.ToLower(strings.TrimSpace(cmd.Name))
		isAlwaysIgnored := slices.ContainsFunc(exclusions, func(ex string) bool {
			return strings.EqualFold(strings.TrimSpace(ex), name)
		})

		if isAlwaysIgnored && !cmd.ForceWatch.isEnabled() {
			// Command is in always-ignored list and no override specified
			ignored = append(ignored, cmd.Name)
			continue
		} else if isAlwaysIgnored && cmd.ForceWatch.isEnabled() {
			// User explicitly wants to watch this normally-ignored command
			logrus.Warnf("CLI Watcher: Command '%s' is normally always-ignored but forceWatch=true overrides this. Use with caution.", cmd.Name)
		}

		filtered = append(filtered, cmd)
	}

	cfg.WatchedCommands = filtered
	// Preserve user-specified ignoredCommands and add filtered always-ignored ones
	cfg.IgnoredCommands = append(cfg.IgnoredCommands, ignored...)
	return cfg
}

// parseDuration parses a duration string or integer (treated as seconds)
func parseDuration(value string, fieldName string, defaultValue time.Duration) time.Duration {
	if value == "" {
		return defaultValue
	}

	// Try parsing as duration first (e.g., "6h", "30m", "3600s")
	duration, err := time.ParseDuration(value)
	if err != nil {
		// Fallback: try parsing as integer seconds (e.g., "21600" or "60")
		// Use strconv.ParseInt to ensure the ENTIRE string is numeric and avoid 32-bit overflow
		seconds, atoiErr := strconv.ParseInt(value, 10, 64)
		if atoiErr == nil && seconds > 0 {
			// Prevent time.Duration overflow: max safe value is ~292 years
			const maxSafeSeconds = int64(9223372036) // math.MaxInt64 / 1e9, rounded down
			if seconds > maxSafeSeconds {
				logrus.Warnf("CLI Watcher: %s value '%s' (%d seconds) too large (max ~292 years), using default (%v)", fieldName, value, seconds, defaultValue)
				return defaultValue
			}
			duration = time.Duration(seconds) * time.Second
		} else {
			// Invalid value - warn and use default
			logrus.Warnf("CLI Watcher: Invalid %s value '%s' (not a duration or integer), using default (%v)", fieldName, value, defaultValue)
			return defaultValue
		}
	}

	if duration <= 0 {
		logrus.Warnf("CLI Watcher: %s is zero or negative (%v), using default (%v)", fieldName, duration, defaultValue)
		return defaultValue
	}

	// Add reasonable upper bounds to prevent misconfiguration or potential DoS
	var maxAllowed time.Duration
	switch fieldName {
	case "checkPeriod":
		maxAllowed = 1 * time.Hour // No point checking less than once per hour
	case "activityWindow":
		maxAllowed = 24 * time.Hour // Activity windows longer than a day are impractical
	case "gracePeriod":
		maxAllowed = 1 * time.Hour // Grace periods longer than an hour are excessive
	case "maxProcessAge":
		maxAllowed = 7 * 24 * time.Hour // Week-long processes are likely stuck
	default:
		maxAllowed = 24 * time.Hour // Default maximum for unknown fields
	}

	if duration > maxAllowed {
		logrus.Warnf("CLI Watcher: %s value '%s' (%v) exceeds maximum (%v), using default (%v)", fieldName, value, duration, maxAllowed, defaultValue)
		return defaultValue
	}

	return duration
}

// applyDefaults sets fallback values (user values are never changed, only unspecified fields get smart defaults)
func applyDefaults(c cliWatcherConfig, idleTimeout time.Duration) cliWatcherConfig {
	// Parse checkPeriod (new field takes priority over deprecated checkPeriodSeconds)
	if c.CheckPeriod != "" {
		c._checkPeriodParsed = parseDuration(c.CheckPeriod, "checkPeriod", time.Duration(DefaultCheckPeriod)*time.Second)

		// Warn if both old and new fields are specified with different values
		if c.CheckPeriodSeconds > 0 {
			deprecatedValue := time.Duration(c.CheckPeriodSeconds) * time.Second
			if c._checkPeriodParsed != deprecatedValue {
				logrus.Warnf("CLI Watcher: Both 'checkPeriod' (%v) and deprecated 'checkPeriodSeconds' (%v) are set - using 'checkPeriod' value", c._checkPeriodParsed, deprecatedValue)
			}
		}
	} else if c.CheckPeriodSeconds > 0 {
		// Backward compatibility: use deprecated checkPeriodSeconds
		c._checkPeriodParsed = time.Duration(c.CheckPeriodSeconds) * time.Second
	} else {
		c._checkPeriodParsed = time.Duration(DefaultCheckPeriod) * time.Second
	}

	// Validate check period bounds (must be done AFTER parsing both fields)
	minCheckPeriod := time.Duration(MinCheckPeriod) * time.Second
	if c._checkPeriodParsed < minCheckPeriod {
		logrus.Warnf("CLI Watcher: checkPeriod (%v) is below minimum (%v), using minimum", c._checkPeriodParsed, minCheckPeriod)
		c._checkPeriodParsed = minCheckPeriod
	}

	// Maximum check period should be reasonable and less than idle timeout
	// Absolute max: 10 minutes (no point checking less frequently)
	// If idleTimeout known: max 1/4 of idle timeout (ensure we can detect activity in time)
	var maxCheckPeriod time.Duration
	if idleTimeout > 0 {
		maxCheckPeriod = idleTimeout / 4
		if maxCheckPeriod > 10*time.Minute {
			maxCheckPeriod = 10 * time.Minute
		}
	} else {
		maxCheckPeriod = 10 * time.Minute
	}

	if c._checkPeriodParsed > maxCheckPeriod {
		if idleTimeout > 0 {
			logrus.Warnf("CLI Watcher: checkPeriod (%v) exceeds maximum (%v, 1/4 of idle timeout %v), using maximum", c._checkPeriodParsed, maxCheckPeriod, idleTimeout)
		} else {
			logrus.Warnf("CLI Watcher: checkPeriod (%v) exceeds maximum (%v), using maximum", c._checkPeriodParsed, maxCheckPeriod)
		}
		c._checkPeriodParsed = maxCheckPeriod
	}

	// Parse gracePeriod (needed first to calculate activityWindow)
	var gracePeriodDefault time.Duration
	if c.GracePeriod == "" && idleTimeout > 0 {
		// Smart default: use smaller of 5m or 15% of idle timeout
		gracePeriodDefault = time.Duration(float64(idleTimeout) * 0.15)
		if gracePeriodDefault > DefaultGracePeriod {
			gracePeriodDefault = DefaultGracePeriod
		}
		if gracePeriodDefault < MinGracePeriod {
			gracePeriodDefault = MinGracePeriod
		}
	} else {
		gracePeriodDefault = DefaultGracePeriod
	}
	c._gracePeriodParsed = parseDuration(c.GracePeriod, "gracePeriod", gracePeriodDefault)

	// Parse activityWindow (depends on gracePeriod and idleTimeout)
	var activityWindowDefault time.Duration
	if c.ActivityWindow == "" && idleTimeout > 0 {
		// Smart default: idleTimeout - gracePeriod - buffer
		buffer := time.Duration(float64(idleTimeout) * SafetyBufferPercent)
		if buffer > SafetyBufferDuration {
			buffer = SafetyBufferDuration
		}

		calculated := idleTimeout - c._gracePeriodParsed - buffer
		if calculated < MinActivityWindow {
			activityWindowDefault = MinActivityWindow
			if calculated <= 0 {
				logrus.Warnf("CLI Watcher: Grace period (%v) + buffer (%v) exceeds idle timeout (%v), using minimum activity window (%v)", c._gracePeriodParsed, buffer, idleTimeout, MinActivityWindow)
			} else if idleTimeout < 10*time.Minute {
				logrus.Warnf("CLI Watcher: Workspace idle timeout (%v) is very short, using minimum activity window (%v)", idleTimeout, MinActivityWindow)
			} else {
				logrus.Warnf("CLI Watcher: Calculated activity window too short (%v), using minimum (%v)", calculated, MinActivityWindow)
			}
		} else {
			activityWindowDefault = calculated
		}
	} else if c.ActivityWindow == "" && c._gracePeriodParsed > 0 {
		// No idleTimeout but gracePeriod specified: ensure activityWindow > gracePeriod
		activityWindowDefault = c._gracePeriodParsed + 2*time.Minute
		if activityWindowDefault < DefaultActivityWindow {
			activityWindowDefault = DefaultActivityWindow
		}
	} else {
		activityWindowDefault = DefaultActivityWindow
	}
	c._activityWindowParsed = parseDuration(c.ActivityWindow, "activityWindow", activityWindowDefault)

	// Parse maxProcessAge
	c._maxProcessAgeParsed = parseDuration(c.MaxProcessAge, "maxProcessAge", DefaultMaxProcessAge)

	// Apply defaults to each watched command
	for i := range c.WatchedCommands {
		if c.WatchedCommands[i].Interactive == "" {
			c.WatchedCommands[i].Interactive = DefaultInteractiveMode
		}
	}

	// Validate configuration (warn about misconfigurations but never change user values)
	if c._activityWindowParsed < c._gracePeriodParsed {
		logrus.Warnf("CLI Watcher: activityWindow (%v) is less than gracePeriod (%v), interactive processes may not be detected correctly", c._activityWindowParsed, c._gracePeriodParsed)
	}

	if idleTimeout > 0 {
		if c._activityWindowParsed >= idleTimeout {
			logrus.Warnf("CLI Watcher: activityWindow (%v) exceeds workspace idle timeout (%v), may not work as expected", c._activityWindowParsed, idleTimeout)
		}
		if c._gracePeriodParsed >= idleTimeout*8/10 {
			logrus.Warnf("CLI Watcher: gracePeriod (%v) is very close to workspace idle timeout (%v)", c._gracePeriodParsed, idleTimeout)
		}
	}

	checkPeriodDuration := c._checkPeriodParsed
	if checkPeriodDuration > c._activityWindowParsed/2 {
		logrus.Warnf("CLI Watcher: checkPeriod (%v) may be too long for activityWindow (%v), activity might not be detected in time", checkPeriodDuration, c._activityWindowParsed)
	}

	return c
}

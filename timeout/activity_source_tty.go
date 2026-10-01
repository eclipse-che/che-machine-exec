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
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

// ttyActivitySource detects activity among interactive, TTY-attached user
// processes — the CLI Watcher's original (and default) detection strategy.
type ttyActivitySource struct{}

// newTTYActivitySource constructs the default tty ActivitySource.
func newTTYActivitySource() ActivitySource {
	return &ttyActivitySource{}
}

// Name returns the canonical identifier for this source.
func (t *ttyActivitySource) Name() string {
	return "tty"
}

// Scan scans /proc to check if any watched, TTY-attached process is running and active.
func (t *ttyActivitySource) Scan(ctx ActivityScanContext) (bool, string) {
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
		if pid == "1" || pid == ctx.MyPID { // Skip PID 1 and ourselves
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
			activityLogf(ctx.Verbose, "CLI Watcher: Process %s (PID %s) is in always-ignored list, skipping", cmdName, pid)
			continue
		}
		if slices.Contains(ctx.IgnoredCommands, cmdName) {
			activityLogf(ctx.Verbose, "CLI Watcher: Process %s (PID %s) is in config ignored list, skipping", cmdName, pid)
			continue
		}

		// STEP 2: Check if command is explicitly configured
		var configuredCmd *WatchedCommand
		for i := range ctx.WatchedCommands {
			if ctx.WatchedCommands[i].Name == cmdName {
				configuredCmd = &ctx.WatchedCommands[i]
				break
			}
		}

		// STEP 3: Safety check - don't prevent idling for processes older than maxProcessAge
		processAge := getProcessAge(pid)
		maxAge := ctx.MaxProcessAge
		if maxAge <= 0 {
			maxAge = DefaultMaxProcessAge
		}
		if processAge > 0 && processAge > maxAge {
			logrus.Warnf("CLI Watcher: Process %s (PID %s) exceeds max age (%v, limit: %v), no longer preventing idling (safety limit)", cmdName, pid, processAge, maxAge)
			continue
		}

		// STEP 4: Grace period - all young processes prevent idling
		gracePeriod := ctx.GracePeriod
		if gracePeriod <= 0 {
			gracePeriod = DefaultGracePeriod
		}
		if processAge == 0 {
			// Can't determine age (getProcessStartTime failed) - give benefit of doubt with grace period
			activityLogf(ctx.Verbose, "CLI Watcher: Process %s (PID %s) age unknown, applying grace period protection", cmdName, pid)
			return true, cmdName
		}
		if processAge < gracePeriod {
			activityLogf(ctx.Verbose, "CLI Watcher: Process %s (PID %s) in grace period (age: %v), preventing idling", cmdName, pid, processAge)
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

		if !applyPolicy(pid, cmdName, mode, ctx.ActivityWindow, policySource, ctx.Verbose) {
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

// applyPolicy applies the interactive policy for a command
// Returns true if process should prevent idling, false otherwise
// Unified function handling both configured and default policies
func applyPolicy(pid, cmdName string, mode InteractiveMode, activityWindow time.Duration, policySource string, verbose bool) bool {
	// Determine if process is interactive
	var checkActivity bool

	switch mode {
	case InteractiveModeAuto:
		// Auto-detect: use foreground + TTY read analysis
		checkActivity = isInteractiveProcess(pid, verbose)
		if checkActivity {
			activityLogf(verbose, "CLI Watcher: Process %s (PID %s) auto-detected as interactive (%s policy)", cmdName, pid, policySource)
		} else {
			activityLogf(verbose, "CLI Watcher: Process %s (PID %s) auto-detected as work process (%s policy)", cmdName, pid, policySource)
		}

	case InteractiveModeTrue, InteractiveModeYes:
		// Force interactive mode
		checkActivity = true
		activityLogf(verbose, "CLI Watcher: Process %s (PID %s) forced interactive (%s policy)", cmdName, pid, policySource)

	case InteractiveModeFalse, InteractiveModeNo:
		// Force non-interactive (work) mode
		checkActivity = false
		activityLogf(verbose, "CLI Watcher: Process %s (PID %s) forced non-interactive (%s policy)", cmdName, pid, policySource)
	}

	// If interactive, check for recent activity
	if checkActivity {
		if !hasRecentActivity(activityWindow, pid, verbose) {
			activityLogf(verbose, "CLI Watcher: Process %s (PID %s) is interactive but no recent activity (%s policy)", cmdName, pid, policySource)
			return false
		}
		activityLogf(verbose, "CLI Watcher: Process %s (PID %s) is interactive with recent activity (%s policy)", cmdName, pid, policySource)
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

// ttyCache holds cached TTY device information to reduce redundant filesystem operations
type ttyCache struct {
	path     string    // TTY device path (e.g., "/dev/pts/1")
	atime    time.Time // Last access time
	cachedAt time.Time // When this was cached
	valid    bool      // Whether the TTY path resolution was successful
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
func hasEverReadFromTTY(pid string, verbose bool) bool {
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
	activityLogf(verbose, "CLI Watcher: TTY atime for PID %s unavailable or unreliable, using fallback detection", pid)
	return hasInteractiveBehaviorFallback(pid, verbose)
}

// hasInteractiveBehaviorFallback checks whether the process is blocked in a syscall
// pattern consistent with waiting for user input, used when TTY atime is unavailable
// or unreliable.
//
// Process state ("S") and /proc/<pid>/fd's mtime were previously part of a weighted
// score, but both proved non-specific: any blocking syscall reports state "S", and
// /proc/<pid>/fd's mtime is set once when the fd table is created (effectively process
// start time) and never updates again for processes that don't open/close fds
// afterward — so it really measured "process age < 5 minutes," not activity. Verified
// on a live workspace: a non-interactive `sleep` was misclassified as interactive
// during its first ~5 minutes solely because of this.
func hasInteractiveBehaviorFallback(pid string, verbose bool) bool {
	wchan := getWaitChannel(pid)
	isInteractive := wchan == "poll_schedule_timeout" || // Polling with timeout (interactive pattern)
		wchan == "pipe_wait" || // Waiting on pipe input
		wchan == "unix_stream_read_generic" || // Reading from socket
		wchan == "select" || // Select/poll waiting for input
		wchan == "ep_poll" // Epoll waiting (event-driven input)

	if isInteractive {
		activityLogf(verbose, "CLI Watcher: PID %s detected as interactive via fallback (wchan: %s)", pid, wchan)
	}
	return isInteractive
}

// isInteractiveProcess detects if a process is interactive by checking:
// 1. Is it in foreground process group?
// 2. Is it waiting on TTY read OR has it ever read from TTY?
func isInteractiveProcess(pid string, verbose bool) bool {
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
	if hasEverReadFromTTY(pid, verbose) {
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
func hasRecentActivity(activityWindow time.Duration, pid string, verbose bool) bool {
	window := activityWindow
	if window <= 0 {
		window = DefaultActivityWindow
	}

	return hasTTYActivity(pid, window, verbose)
}

// checkDevptsAtimeSupport inspects /proc/mounts for the devpts filesystem (backing
// /dev/pts/*, i.e. workspace terminals) and warns once at startup if it's mounted with
// `noatime`. TTY atime is the primary signal for interactive activity; when disabled,
// CLI Watcher falls back to CPU-usage based detection (see hasTTYActivity /
// hasRecentCPUActivity), which is coarser — it can tell a process did *something*, not
// specifically that a user typed something.
func checkDevptsAtimeSupport() {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		logrus.Debugf("CLI Watcher: Could not read /proc/mounts to check devpts atime support: %v", err)
		return
	}

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[2] != "devpts" {
			continue
		}

		options := strings.Split(fields[3], ",")
		if slices.Contains(options, "noatime") {
			logrus.Warnf("CLI Watcher: devpts (%s) is mounted with 'noatime' — TTY access-time tracking is disabled, so interactive-process activity will be detected via a CPU-usage fallback instead of keystroke timing (coarser; may keep workspaces alive slightly longer than expected)", fields[1])
		} else {
			logrus.Debugf("CLI Watcher: devpts (%s) mount options: %s (atime tracking available)", fields[1], fields[3])
		}
		return
	}

	logrus.Debugf("CLI Watcher: No devpts mount found in /proc/mounts, cannot verify TTY atime support")
}

// hasTTYActivity checks if the TTY has been accessed recently, using atime when it's
// reliable (has advanced past process start), falling back to CPU-usage tracking when
// it hasn't — e.g. under a `noatime` devpts mount, where atime for a shared pty never
// advances past whatever value it had before this process even started.
func hasTTYActivity(pid string, window time.Duration, verbose bool) bool {
	startTime := getProcessStartTime(pid)
	_, atime, valid := getCachedTTYInfo(pid)

	if valid && !startTime.IsZero() && atime.After(startTime) {
		threshold := time.Now().Add(-window)
		return atime.After(threshold)
	}

	activityLogf(verbose, "CLI Watcher: TTY atime for PID %s unavailable or unreliable, using CPU-activity fallback", pid)
	return hasRecentCPUActivity(pid, window, verbose)
}

// cpuActivityCache tracks per-process CPU ticks (utime+stime) across check cycles, used
// as an atime-independent activity signal when TTY atime is unavailable or unreliable
// (e.g. a `noatime` devpts mount, where atime for a shared pty never advances).
var (
	cpuActivityCache      = make(map[string]*cpuActivitySample)
	cpuActivityCacheMutex sync.Mutex
)

type cpuActivitySample struct {
	lastTicks    int64
	lastActiveAt time.Time
}

const cpuActivityCacheCleanupAt = 800 // Trigger cleanup when reaching this size

// cleanupCPUActivityCache removes entries for PIDs that no longer exist.
// MUST be called with cpuActivityCacheMutex held.
func cleanupCPUActivityCache() {
	for pid := range cpuActivityCache {
		if _, err := os.Stat(filepath.Join("/proc", pid)); os.IsNotExist(err) {
			delete(cpuActivityCache, pid)
		}
	}
}

// hasRecentCPUActivity reports whether a process has consumed any CPU since it was last
// sampled, tracking a per-PID "last seen active" timestamp across check cycles. Used as
// a fallback for hasTTYActivity when TTY atime can't be trusted.
func hasRecentCPUActivity(pid string, window time.Duration, verbose bool) bool {
	ticks, ok := getProcessCPUTicks(pid)
	if !ok {
		return false
	}

	cpuActivityCacheMutex.Lock()
	defer cpuActivityCacheMutex.Unlock()

	now := time.Now()
	sample, exists := cpuActivityCache[pid]
	if !exists {
		if len(cpuActivityCache) >= cpuActivityCacheCleanupAt {
			cleanupCPUActivityCache()
		}
		cpuActivityCache[pid] = &cpuActivitySample{lastTicks: ticks, lastActiveAt: now}
		activityLogf(verbose, "CLI Watcher: PID %s has no CPU-activity baseline yet, assuming active", pid)
		return true
	}

	if ticks > sample.lastTicks {
		sample.lastTicks = ticks
		sample.lastActiveAt = now
	}

	recentlyActive := now.Sub(sample.lastActiveAt) < window
	activityLogf(verbose, "CLI Watcher: PID %s CPU-activity fallback: recent=%v (last active %v ago)", pid, recentlyActive, now.Sub(sample.lastActiveAt).Round(time.Second))
	return recentlyActive
}

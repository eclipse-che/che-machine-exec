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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// codexAppServerStatusFileName is the file codex's managed hooks (see
// timeout/codex-hooks/) write to on every relevant event. Located under
// codex's own "app-server-control" directory, alongside its control
// socket and log file.
const codexAppServerStatusFileName = "codex-app-server-activity-status.yaml"

// codexAppServerHooksActivitySource detects activity on a codex app-server by
// polling a status file that codex's own managed hooks maintain (see
// timeout/codex-hooks/README.md for the full design and rationale).
//
// This replaces an earlier /proc-only design (comm+cmdline discovery +
// rchar-delta cache + AF_UNIX connection-presence gate) that was verified
// live and then abandoned after uncovering three independent
// false-positive sources: an internal ~10s heartbeat write unrelated to
// any client, codex's own persistent background network traffic (a
// remote-control websocket to chatgpt.com, periodic model-list calls),
// and an async-runtime-internal socketpair() structurally indistinguishable
// from a real client connection via /proc/net/unix alone. See the
// activity-sources-plan project memory for the full history.
//
// /proc discovery is still used here, but ONLY to find which process's
// environment to read (to resolve CODEX_HOME/HOME reliably - see
// resolveCodexBaseDir) - never as the activity signal itself, and never
// for grace-period/process-age purposes. GracePeriod and MaxProcessAge are
// both deliberately not applied: there's no detection lag to compensate
// for (a hook firing IS a real-time activity event), and a codex app-server
// daemon is expected to run for the whole workspace lifetime.
type codexAppServerHooksActivitySource struct {
	// lastWarnedPID avoids repeating the "can't resolve status file
	// location" warning every scan cycle for the same process. Only ever
	// read/written from Scan, which runs solely on the watcher's single
	// ticker-loop goroutine - no concurrent access, no mutex needed. A
	// different pid later (e.g. codex restarted with corrected env)
	// naturally triggers a fresh attempt/warning.
	lastWarnedPID string
}

// newcodexAppServerHooksActivitySource constructs the codex-app-server-hooks ActivitySource.
func newcodexAppServerHooksActivitySource() ActivitySource {
	return &codexAppServerHooksActivitySource{}
}

// Name returns the canonical identifier for this source.
func (c *codexAppServerHooksActivitySource) Name() string {
	return "codex-app-server-hooks"
}

// Scan looks for running codex-app-server-hooks processes and reports active as
// soon as any one's status file (maintained by codex's own managed hooks)
// shows recent activity. Multiple candidates are possible - and each is
// checked, not just the first found - since distinct instances can use
// distinct CODEX_HOME values (e.g. different users, or different workspace
// configurations), so a stale first instance must not hide a recent event
// from another.
func (c *codexAppServerHooksActivitySource) Scan(ctx ActivityScanContext) (bool, string) {
	candidates := findCodexAppServerProcesses(ctx.MyPID)

	for _, cand := range candidates {
		if active, label := c.scanCandidate(cand, ctx); active {
			return true, label
		}
	}

	return false, ""
}

// scanCandidate applies the status-file freshness check to a single
// matched codex-app-server-hooks process.
func (c *codexAppServerHooksActivitySource) scanCandidate(cand codexAppServerCandidate, ctx ActivityScanContext) (bool, string) {
	socketPath := cand.socketPath
	if socketPath == "" {
		socketPath = "(daemon-assigned default)"
	}
	label := fmt.Sprintf("codex-app-server-hooks (pid %s, socket %s)", cand.pid, socketPath)

	baseDir := resolveCodexBaseDir(cand.pid)
	if baseDir == "" {
		if cand.pid != c.lastWarnedPID {
			logrus.Warnf("CLI Watcher: %s: cannot resolve status file location - neither CODEX_HOME nor HOME is set in its process environment; codex-app-server-hooks activity will not be detected until this is fixed", label)
			c.lastWarnedPID = cand.pid
		}
		return false, label
	}

	statusFilePath := filepath.Join(baseDir, "app-server-control", codexAppServerStatusFileName)
	stat, err := os.Stat(statusFilePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			// Missing is the normal "no hook has fired yet" state and stays
			// silent - matching the tty source, which likewise never logs
			// anything (not even at verbose) when a scan simply finds no
			// activity. Anything else (permission denied, I/O error) is
			// unexpected, but still only surfaced via activityLogf (Debug by
			// default, Info when verbose) rather than an unconditional Warn:
			// it's a per-instance issue, not a systemic one like /proc being
			// unreadable (see findCodexAppServerProcesses), so it shouldn't
			// spam production logs at the default level.
			activityLogf(ctx.Verbose, "CLI Watcher: %s: cannot stat status file %s: %v", label, statusFilePath, err)
		}
		return false, label
	}

	window := ctx.ActivityWindow
	if window <= 0 {
		window = DefaultActivityWindow
	}
	// The file's own mtime is exactly as authoritative as the last-activity
	// field inside it (the hook writes via temp-file-then-rename right
	// after computing "now"), and far cheaper to check: no open, no read,
	// no parsing needed for the actual decision.
	//
	// age can go negative if the file's mtime is in the future (clock skew,
	// or a persistent CODEX_HOME restored from a snapshot/backup taken
	// "later" than the current clock) - require a nonnegative age too, not
	// just age < window, otherwise a future-dated file would report active
	// indefinitely until the clock catches up to it.
	age := time.Since(stat.ModTime())
	recentlyActive := age >= 0 && age < window

	if ctx.Verbose {
		// Extra cost only paid when someone actually wants the detail -
		// unlike activityLogf elsewhere (which always formats the string,
		// just changes Info-vs-Debug level), this skips the file I/O
		// entirely when not verbose, not just the log level.
		if info, perr := readCodexStatusFile(statusFilePath); perr == nil {
			activityLogf(true, "CLI Watcher: %s status file: last-event=%s last-session-id=%s mtime=%v recentlyActive=%v",
				label, info.lastEvent, info.lastSessionID, stat.ModTime(), recentlyActive)
		} else {
			activityLogf(true, "CLI Watcher: %s status file present but unreadable: %v", label, perr)
		}
	}

	return recentlyActive, label
}

// resolveCodexBaseDir determines the codex "home" directory
// (${CODEX_HOME:-$HOME/.codex}) as seen by the given process itself, by
// reading its own environment rather than che-machine-exec's. This is
// deliberate: che-machine-exec's own $HOME/$CODEX_HOME may be unset, or
// may simply not match wherever codex was actually launched from (e.g. an
// SSH login shell with its own PAM/profile-resolved environment) - reading
// the target process's own /proc/[pid]/environ guarantees the identical
// path its hooks (children of that same process) resolve to. Returns ""
// if neither variable is set in that process's environment.
func resolveCodexBaseDir(pid string) string {
	env, err := getProcessEnviron(pid)
	if err != nil {
		return ""
	}
	return codexBaseDirFromEnv(env)
}

// codexBaseDirFromEnv implements the ${CODEX_HOME:-$HOME/.codex} fallback
// given an already-parsed environment map. Split out from
// resolveCodexBaseDir as a pure function so the fallback logic itself is
// unit-testable without needing a real /proc/[pid]/environ (which reflects
// the environment at exec() time, not live setenv() changes, so it can't
// be exercised reliably against the test process's own live env).
func codexBaseDirFromEnv(env map[string]string) string {
	if v := env["CODEX_HOME"]; v != "" {
		return v
	}
	if v := env["HOME"]; v != "" {
		return filepath.Join(v, ".codex")
	}
	return ""
}

// getProcessEnviron reads and parses /proc/[pid]/environ, which is a
// NUL-separated list of "KEY=value" entries.
func getProcessEnviron(pid string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join("/proc", pid, "environ"))
	if err != nil {
		return nil, err
	}

	env := make(map[string]string)
	for _, entry := range strings.Split(string(data), "\x00") {
		if entry == "" {
			continue
		}
		if key, val, ok := strings.Cut(entry, "="); ok {
			env[key] = val
		}
	}
	return env, nil
}

// codexStatusFileInfo holds the fields parsed from the codex-app-server-hooks
// status file, for verbose debug logging only - the active/not-active
// decision itself is made from the file's mtime, not these fields.
type codexStatusFileInfo struct {
	lastEvent     string
	lastSessionID string
}

// readCodexStatusFile parses the status file written by
// timeout/codex-hooks/managed-hooks/codex-app-server-activity-status.sh -
// simple flat "key: value" lines.
func readCodexStatusFile(path string) (codexStatusFileInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return codexStatusFileInfo{}, err
	}

	var info codexStatusFileInfo
	for _, line := range strings.Split(string(data), "\n") {
		if val, ok := strings.CutPrefix(line, "last-event:"); ok {
			info.lastEvent = strings.TrimSpace(val)
		} else if val, ok := strings.CutPrefix(line, "last-session-id:"); ok {
			info.lastSessionID = strings.TrimSpace(val)
		}
	}
	return info, nil
}

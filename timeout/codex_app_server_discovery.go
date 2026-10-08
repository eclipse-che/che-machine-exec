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
	"strings"

	"github.com/sirupsen/logrus"
)

// codexAppServerBinaryNamePrefix is the expected /proc/[pid]/comm prefix for
// a codex CLI binary. Matched as a case-insensitive prefix (not exact
// equality) to tolerate arch/version-suffixed builds (e.g. "codex-x86_64")
// without being wide open to any process that happens to invent a matching
// --listen flag.
const codexAppServerBinaryNamePrefix = "codex"

// codexAppServerCandidate identifies one matched codex-app-server process.
// This is purely "which process is this and what socket did it advertise,"
// with no source-specific interpretation of that
// fact.
type codexAppServerCandidate struct {
	pid        string
	socketPath string
}

// findCodexAppServerProcesses scans /proc for ALL codex app-server
// processes listening on a unix socket, returning each one's pid and (if
// resolvable) socket path. Returns an empty slice if none are running.
func findCodexAppServerProcesses(myPID string) []codexAppServerCandidate {
	procEntries, err := os.ReadDir("/proc")
	if err != nil {
		logrus.Warnf("CLI Watcher: Cannot read /proc: %v", err)
		return nil
	}

	var candidates []codexAppServerCandidate
	for _, entry := range procEntries {
		if !entry.IsDir() || !isNumeric(entry.Name()) {
			continue
		}

		candidatePID := entry.Name()
		if candidatePID == myPID {
			continue
		}

		// Identity check: comm must look like a codex binary.
		commData, err := os.ReadFile(filepath.Join("/proc", candidatePID, "comm"))
		if err != nil {
			continue
		}
		comm := strings.ToLower(strings.TrimSpace(string(commData)))
		if !strings.HasPrefix(comm, codexAppServerBinaryNamePrefix) {
			continue
		}

		// Behavior check: cmdline must be specifically the app-server
		// subcommand listening on a unix socket, not any other codex
		// invocation (interactive session, mcp-server, exec, etc.).
		args, err := getProcessCmdlineArgs(candidatePID)
		if err != nil {
			continue
		}
		matches, sp := codexAppServerCmdlineMatch(args)
		if !matches {
			continue
		}

		candidates = append(candidates, codexAppServerCandidate{pid: candidatePID, socketPath: sp})
	}

	return candidates
}

// getProcessCmdlineArgs reads and splits /proc/[pid]/cmdline, which is
// NUL-separated rather than space-separated (so argument values containing
// spaces are preserved intact).
func getProcessCmdlineArgs(pid string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join("/proc", pid, "cmdline"))
	if err != nil {
		return nil, err
	}
	raw := strings.TrimRight(string(data), "\x00")
	if raw == "" {
		return nil, nil
	}
	return strings.Split(raw, "\x00"), nil
}

// codexAppServerCmdlineMatch checks whether the given cmdline args
// correspond to `codex ... app-server ... --listen unix://...` (or
// `--listen=unix://...`), returning the socket path (without the "unix://"
// prefix; empty when the daemon auto-assigns its default path, e.g.
// `--listen unix://` with no path) when matched. Both the "app-server"
// subcommand and a unix-socket --listen value are required - an
// args-pattern check alone would be too loose (anything could invent a
// matching flag).
func codexAppServerCmdlineMatch(args []string) (matches bool, socketPath string) {
	hasAppServer := false
	hasListenUnix := false

	for i, a := range args {
		if a == "app-server" {
			hasAppServer = true
		}
		if a == "--listen" && i+1 < len(args) {
			if val := args[i+1]; strings.Contains(val, "unix") {
				hasListenUnix = true
				socketPath = strings.TrimPrefix(val, "unix://")
			}
		} else if val, ok := strings.CutPrefix(a, "--listen="); ok {
			if strings.Contains(val, "unix") {
				hasListenUnix = true
				socketPath = strings.TrimPrefix(val, "unix://")
			}
		}
	}

	return hasAppServer && hasListenUnix, socketPath
}

// resolveCodexBaseDir determines the codex "home" directory
// (${CODEX_HOME:-$HOME/.codex}) as seen by the given process itself, by
// reading its own environment rather than che-machine-exec's. This is
// deliberate: che-machine-exec's own $HOME/$CODEX_HOME may be unset, or
// may simply not match wherever codex was actually launched from (e.g. an
// SSH login shell with its own PAM/profile-resolved environment) - reading
// the target process's own /proc/[pid]/environ guarantees the identical
// path that process itself resolves to. Returns "" if neither variable is
// set in that process's environment.
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

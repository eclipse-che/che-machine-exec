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

// codexAppServerBinaryNamePrefix is the expected /proc/[pid]/comm prefix for a codex binary.
const codexAppServerBinaryNamePrefix = "codex"

// codexAppServerCandidate identifies one matched codex-app-server process.
type codexAppServerCandidate struct {
	pid        string
	socketPath string
}

// findCodexAppServerProcesses scans /proc for codex app-server processes listening on a unix socket.
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

		// Check cmdline for app-server subcommand with unix socket.
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

// getProcessCmdlineArgs reads and splits /proc/[pid]/cmdline (NUL-separated).
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

// codexAppServerCmdlineMatch checks for `codex ... app-server ... --listen unix://...`.
// Returns socket path without "unix://" prefix (empty if daemon auto-assigns).
func codexAppServerCmdlineMatch(args []string) (matches bool, socketPath string) {
	hasAppServer := false
	hasListenUnix := false

	for i, a := range args {
		if a == "app-server" {
			hasAppServer = true
		}
		if a == "--listen" && i+1 < len(args) {
			if val := args[i+1]; strings.HasPrefix(val, "unix://") {
				hasListenUnix = true
				socketPath = strings.TrimPrefix(val, "unix://")
			}
		} else if val, ok := strings.CutPrefix(a, "--listen="); ok {
			if strings.HasPrefix(val, "unix://") {
				hasListenUnix = true
				socketPath = strings.TrimPrefix(val, "unix://")
			}
		}
	}

	return hasAppServer && hasListenUnix, socketPath
}

// resolveCodexBaseDir determines ${CODEX_HOME:-$HOME/.codex} from the target process's environment.
func resolveCodexBaseDir(pid string) string {
	env, err := getProcessEnviron(pid)
	if err != nil {
		return ""
	}
	return codexBaseDirFromEnv(env)
}

// codexBaseDirFromEnv implements ${CODEX_HOME:-$HOME/.codex} fallback from an environment map.
func codexBaseDirFromEnv(env map[string]string) string {
	if v := env["CODEX_HOME"]; v != "" {
		return v
	}
	if v := env["HOME"]; v != "" {
		return filepath.Join(v, ".codex")
	}
	return ""
}

// getProcessEnviron reads and parses /proc/[pid]/environ (NUL-separated KEY=value entries).
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

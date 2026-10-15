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
	"path/filepath"
	"time"
)

// codexAppServerDefaultSocketName is the socket filename codex creates
// under ${CODEX_HOME:-$HOME/.codex}/app-server-control/ when started
// with `--listen unix://` (no explicit path).
const codexAppServerDefaultSocketName = "app-server-control.sock"

// defaultCodexAppServerSocketPath builds the daemon-assigned default socket path.
func defaultCodexAppServerSocketPath(baseDir string) string {
	return filepath.Join(baseDir, "app-server-control", codexAppServerDefaultSocketName)
}

// codexAppServerApiActivitySource detects activity by connecting to a codex
// app-server's control socket as a passive JSON-RPC client, polling
// thread/loaded/list + thread/read for the freshest updatedAt timestamp.
// Activity is based on updatedAt freshness (within ActivityWindow), not status
// (which empirically doesn't reflect real-time conversation state).
// GracePeriod/MaxProcessAge are not applied.
type codexAppServerApiActivitySource struct {
	// conns caches client connections by PID across scan ticks.
	// No mutex needed - only accessed from single-threaded scan loop.
	conns map[string]*codexAppServerClient
}

// newCodexAppServerApiActivitySource constructs the codex-app-server
// ActivitySource.
func newCodexAppServerApiActivitySource() ActivitySource {
	return &codexAppServerApiActivitySource{}
}

// Name returns the canonical identifier for this source.
func (s *codexAppServerApiActivitySource) Name() string {
	return "codex-app-server"
}

// Scan looks for running codex-app-server processes and reports active as
// soon as any one has a loaded thread whose updatedAt is within the
// activity window. Multiple candidates are checked, not just the first
// found: distinct instances can be genuinely distinct servers, so a quiet
// first instance must not hide activity on another.
func (s *codexAppServerApiActivitySource) Scan(ctx ActivityScanContext) (bool, string) {
	candidates := findCodexAppServerProcesses(ctx.MyPID)
	s.pruneStaleConnections(candidates)

	for _, cand := range candidates {
		if active, label := s.scanCandidate(cand, ctx); active {
			return true, label
		}
	}

	return false, ""
}

// scanCandidate checks a candidate's control socket for active threads.
func (s *codexAppServerApiActivitySource) scanCandidate(cand codexAppServerCandidate, ctx ActivityScanContext) (bool, string) {
	socketPath := cand.socketPath
	if socketPath == "" {
		// Daemon-assigned default socket - resolve via target process's environment.
		baseDir := resolveCodexBaseDir(cand.pid)
		if baseDir == "" {
			label := fmt.Sprintf("%s (pid %s, socket <daemon-assigned default>)", s.Name(), cand.pid)
			activityLogf(ctx.Verbose, "CLI Watcher: %s: cannot resolve default socket path - neither CODEX_HOME nor HOME is set in its process environment", label)
			return false, label
		}
		socketPath = defaultCodexAppServerSocketPath(baseDir)
	}

	label := fmt.Sprintf("%s (pid %s, socket %s)", s.Name(), cand.pid, socketPath)

	client, wasCached, err := s.getOrDialClient(cand, socketPath)
	if err != nil {
		activityLogf(ctx.Verbose, "CLI Watcher: %s: cannot connect: %v", label, err)
		return false, label
	}
	if !wasCached {
		activityLogf(ctx.Verbose, "CLI Watcher: %s: connected", label)
	}

	loadedIDs, err := client.threadLoadedList()
	if err != nil {
		// Connection may be stale - drop it so next scan dials fresh.
		s.dropConnection(cand.pid)
		activityLogf(ctx.Verbose, "CLI Watcher: %s: thread/loaded/list failed: %v", label, err)
		return false, label
	}
	activityLogf(ctx.Verbose, "CLI Watcher: %s: %d loaded thread(s): %v", label, len(loadedIDs), loadedIDs)

	threads := make([]codexThread, 0, len(loadedIDs))
	for _, id := range loadedIDs {
		thread, err := client.threadRead(id)
		if err != nil {
			var rpcErr *codexAppServerRPCError
			if !errors.As(err, &rpcErr) {
				// Transport failure - the conn is dead; drop it and
				// stop, else every remaining ID blocks up to the RPC
				// timeout on it. Next scan dials fresh.
				activityLogf(ctx.Verbose, "CLI Watcher: %s: thread/read(%s) failed: %v - dropping connection", label, id, err)
				s.dropConnection(cand.pid)
				break
			}
			// JSON-RPC error (e.g. thread just disappeared) - conn healthy, keep going.
			activityLogf(ctx.Verbose, "CLI Watcher: %s: thread/read(%s) failed: %v", label, id, err)
			continue
		}
		activityLogf(ctx.Verbose, "CLI Watcher: %s: thread %s status=%s activeFlags=%v updatedAt=%d",
			label, thread.ID, thread.Status.Type, thread.Status.ActiveFlags, thread.UpdatedAt)
		threads = append(threads, thread)
	}

	// status is diagnostic-only - log when active for debugging, but it's not used for the activity decision.
	if active, ok := findActiveThread(threads); ok {
		activityLogf(ctx.Verbose, "CLI Watcher: %s: thread %s currently reports status=active (activeFlags=%v) - informational only, not used for the activity decision",
			label, active.ID, active.Status.ActiveFlags)
	}

	var freshest codexThread
	haveThread := false
	for _, thread := range threads {
		if !haveThread || thread.UpdatedAt > freshest.UpdatedAt {
			freshest = thread
			haveThread = true
		}
	}

	if !haveThread {
		activityLogf(ctx.Verbose, "CLI Watcher: %s: checked %d loaded thread(s), none active", label, len(threads))
		return false, label
	}

	window := ctx.ActivityWindow
	if window <= 0 {
		window = DefaultActivityWindow
	}
	// Require nonnegative age to guard against clock skew.
	age := time.Since(time.Unix(freshest.UpdatedAt, 0))
	active := age >= 0 && age < window
	if active {
		activityLogf(ctx.Verbose, "CLI Watcher: %s: thread %s updated %v ago (within %v activity window) - reporting active",
			label, freshest.ID, age.Round(time.Second), window)
		return true, label
	}

	activityLogf(ctx.Verbose, "CLI Watcher: %s: most recently updated thread %s was updated %v ago, outside %v activity window - none active",
		label, freshest.ID, age.Round(time.Second), window)
	return false, label
}

// getOrDialClient returns the cached connection for this PID, or dials a fresh one.
func (s *codexAppServerApiActivitySource) getOrDialClient(cand codexAppServerCandidate, socketPath string) (client *codexAppServerClient, wasCached bool, err error) {
	if client, ok := s.conns[cand.pid]; ok {
		return client, true, nil
	}

	client, err = dialCodexAppServerClient(socketPath)
	if err != nil {
		return nil, false, err
	}

	if s.conns == nil {
		s.conns = make(map[string]*codexAppServerClient)
	}
	s.conns[cand.pid] = client
	return client, false, nil
}

// dropConnection closes and forgets the cached connection for a pid, if
// any.
func (s *codexAppServerApiActivitySource) dropConnection(pid string) {
	if client, ok := s.conns[pid]; ok {
		_ = client.Close()
		delete(s.conns, pid)
	}
}

// pruneStaleConnections closes cached connections for PIDs no longer found.
func (s *codexAppServerApiActivitySource) pruneStaleConnections(candidates []codexAppServerCandidate) {
	if len(s.conns) == 0 {
		return
	}

	live := make(map[string]bool, len(candidates))
	for _, cand := range candidates {
		live[cand.pid] = true
	}

	for pid := range s.conns {
		if !live[pid] {
			s.dropConnection(pid)
		}
	}
}

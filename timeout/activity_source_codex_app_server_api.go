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
	"fmt"
	"path/filepath"
	"time"
)

// codexAppServerDefaultSocketName is the socket filename codex itself
// creates under ${CODEX_HOME:-$HOME/.codex}/app-server-control/ when
// started with a bare `--listen unix://` (no explicit path) - confirmed
// live both by direct local testing and against a real production
// container startup script, which launches codex-app-server exactly this
// way (`codex ... app-server --listen unix:// >"$CODEX_HOME/app-server-control/app-server.log"`).
// codexAppServerCmdlineMatch (codex_app_server_discovery.go) correctly
// detects this case and returns an empty socketPath rather than guessing
// - resolving the actual default path is this file's job, done in
// scanCandidate below via resolveCodexBaseDir (codex_app_server_discovery.go).
const codexAppServerDefaultSocketName = "app-server-control.sock"

// defaultCodexAppServerSocketPath builds the daemon-assigned default
// socket path under a resolved codex base dir. Split out as a pure
// function so this specific join is unit-testable without needing a real
// /proc/[pid]/environ.
func defaultCodexAppServerSocketPath(baseDir string) string {
	return filepath.Join(baseDir, "app-server-control", codexAppServerDefaultSocketName)
}

// codexAppServerApiActivitySource detects activity on a codex app-server by
// connecting to its own `--listen unix://...` control socket as a plain,
// passive JSON-RPC client and polling `thread/loaded/list` + `thread/read`
// for the freshest `updatedAt` timestamp across every loaded thread. This
// needs no managed hooks, no /etc/codex provisioning, and no
// root/image-build-time step - only that che-machine-exec can reach the
// already-running app-server's socket, which it already needs to *find*
// via the /proc discovery in codex_app_server_discovery.go.
//
// This source never calls thread/start, thread/resume, or any method that
// would create or join a conversation - thread/loaded/list + thread/read
// are plain, unscoped request/response calls that report state for every
// thread currently loaded in the server's memory regardless of which
// client (if any) started or resumed it, so a fully passive observer is
// sufficient. `thread/list` was deliberately NOT used here despite
// looking like the more obvious choice: live-tested against a real
// app-server, it reflects the persisted/indexed thread store, not what's
// actually loaded/live right now - it omitted a thread that had just been
// created and was actively being driven in the very same moment. See
// codex_app_server_client.go's codexThreadLoadedListResponse doc comment
// and the codex-app-server-api plan (activity-sources-plan project
// memory) for the full investigation.
//
// The activity decision is gated entirely on `updatedAt` freshness
// (`now - updatedAt < ActivityWindow`), NOT on `status`. This was not the
// original design - `status == "active"` was tried first, on the
// assumption (from the protocol schema alone) that a thread stays
// `active` for the whole time a conversation is open, including while
// waiting on the user's next message. Live testing falsified that
// assumption directly: polling a real session every 5 seconds across
// three full, real, completed interactions, `status` was observed as
// `idle` on *every single poll* - never once caught as `active`, even
// right after a response finished streaming. `Thread.updatedAt`, by
// contrast, was cross-checked against `thread/turns/list`'s authoritative
// per-turn `completedAt` field and found to match it exactly, updating
// precisely at turn completion. So `status`/`activeFlags` are kept only
// for verbose diagnostic logging (e.g. surfacing `systemError`) - they do
// not gate the result. GracePeriod/MaxProcessAge remain deliberately NOT
// applied: there's no detection lag to compensate for (a turn's
// `updatedAt` is itself a precise, real-time timestamp, not something
// requiring a lag allowance), and a codex-app-server daemon is expected
// to run for the whole workspace lifetime.
type codexAppServerApiActivitySource struct {
	// conns caches one persistent client connection per candidate pid
	// across scan ticks, to avoid paying the dial+initialize handshake
	// cost on every scan. Keyed by pid rather than socket path so a
	// restarted codex-app-server (new pid, possibly same socket path)
	// naturally gets a fresh connection instead of reusing a stale one.
	// Only ever read/written from Scan, which runs solely on the
	// watcher's single ticker-loop goroutine - no concurrent access, no
	// mutex needed.
	conns map[string]*codexAppServerClient
}

// newCodexAppServerApiActivitySource constructs the codex-app-server-api
// ActivitySource.
func newCodexAppServerApiActivitySource() ActivitySource {
	return &codexAppServerApiActivitySource{}
}

// Name returns the canonical identifier for this source.
func (s *codexAppServerApiActivitySource) Name() string {
	return "codex-app-server-api"
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

// scanCandidate connects to (or reuses a connection to) one candidate's
// control socket and checks every currently-loaded thread for one whose
// status is active. In verbose mode this logs its full reasoning - not
// just the final active/not-active outcome - since there's no other way
// for an admin to sanity-check "is this source even seeing my app-server"
// without reaching for a raw JSON-RPC client themselves.
func (s *codexAppServerApiActivitySource) scanCandidate(cand codexAppServerCandidate, ctx ActivityScanContext) (bool, string) {
	socketPath := cand.socketPath
	if socketPath == "" {
		// Daemon-assigned default socket path - confirmed live this is
		// exactly how a real production container starts codex-app-server
		// (`--listen unix://` with no explicit path). Resolve it via the
		// target process's own environment, not che-machine-exec's own
		// (which may not match - e.g. an SSH login shell resolves its own
		// env independently).
		baseDir := resolveCodexBaseDir(cand.pid)
		if baseDir == "" {
			label := fmt.Sprintf("codex-app-server-api (pid %s, socket <daemon-assigned default>)", cand.pid)
			activityLogf(ctx.Verbose, "CLI Watcher: %s: cannot resolve default socket path - neither CODEX_HOME nor HOME is set in its process environment", label)
			return false, label
		}
		socketPath = defaultCodexAppServerSocketPath(baseDir)
	}

	label := fmt.Sprintf("codex-app-server-api (pid %s, socket %s)", cand.pid, socketPath)

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
		// The connection may have gone stale (app-server restarted,
		// socket closed, etc.) - drop it so the next scan attempt dials
		// fresh rather than repeatedly failing on the same dead connection.
		s.dropConnection(cand.pid)
		activityLogf(ctx.Verbose, "CLI Watcher: %s: thread/loaded/list failed: %v", label, err)
		return false, label
	}
	activityLogf(ctx.Verbose, "CLI Watcher: %s: %d loaded thread(s): %v", label, len(loadedIDs), loadedIDs)

	threads := make([]codexThread, 0, len(loadedIDs))
	for _, id := range loadedIDs {
		thread, err := client.threadRead(id)
		if err != nil {
			activityLogf(ctx.Verbose, "CLI Watcher: %s: thread/read(%s) failed: %v", label, id, err)
			continue
		}
		activityLogf(ctx.Verbose, "CLI Watcher: %s: thread %s status=%s activeFlags=%v updatedAt=%d",
			label, thread.ID, thread.Status.Type, thread.Status.ActiveFlags, thread.UpdatedAt)
		threads = append(threads, thread)
	}

	// status is diagnostic-only (see the type doc comment for why) - log
	// it separately from the actual decision below when it does happen to
	// say "active", since that's still a potentially useful detail.
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
	// age can go negative if updatedAt is clock-skewed into the future -
	// require nonnegative age too, not just age < window, same guard the
	// old hooks source used for its file-mtime check.
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

// getOrDialClient returns the cached connection for this candidate's pid
// (wasCached=true), dialing a fresh one at socketPath if none exists yet
// (wasCached=false). socketPath is passed in separately from cand rather
// than read off cand.socketPath, since the caller may have had to resolve
// a daemon-assigned default path that wasn't present on the command line.
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

// pruneStaleConnections closes and forgets any cached connection whose pid
// is no longer among the currently discovered candidates (process exited
// or was restarted under a new pid) - otherwise this source would leak
// one open socket per codex-app-server restart for the lifetime of the
// watcher.
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

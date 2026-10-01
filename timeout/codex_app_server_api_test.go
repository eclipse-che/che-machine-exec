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
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Test findActiveThread: the pure "does any thread report status active"
// helper, used only for an extra diagnostic log line in scanCandidate -
// NOT the activity decision itself (that's updatedAt freshness, see
// TestCodexAppServerApiScanCandidateUpdatedAtFreshness). Exercised
// against canned thread slices - no real socket needed.
func TestFindActiveThread(t *testing.T) {
	tests := []struct {
		name      string
		threads   []codexThread
		wantFound bool
		wantID    string
	}{
		{
			name:      "no threads",
			threads:   nil,
			wantFound: false,
		},
		{
			name: "all idle or notLoaded",
			threads: []codexThread{
				{ID: "t1", Status: codexThreadStatus{Type: "idle"}},
				{ID: "t2", Status: codexThreadStatus{Type: "notLoaded"}},
				{ID: "t3", Status: codexThreadStatus{Type: "systemError"}},
			},
			wantFound: false,
		},
		{
			name: "one active among several idle",
			threads: []codexThread{
				{ID: "t1", Status: codexThreadStatus{Type: "idle"}},
				{ID: "t2", Status: codexThreadStatus{Type: "active", ActiveFlags: []string{}}},
				{ID: "t3", Status: codexThreadStatus{Type: "idle"}},
			},
			wantFound: true,
			wantID:    "t2",
		},
		{
			name: "active while waiting on user input still counts as active",
			threads: []codexThread{
				{ID: "t1", Status: codexThreadStatus{Type: "active", ActiveFlags: []string{"waitingOnUserInput"}}},
			},
			wantFound: true,
			wantID:    "t1",
		},
		{
			name: "first active thread wins when multiple are active",
			threads: []codexThread{
				{ID: "t1", Status: codexThreadStatus{Type: "active"}},
				{ID: "t2", Status: codexThreadStatus{Type: "active"}},
			},
			wantFound: true,
			wantID:    "t1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := findActiveThread(tt.threads)
			if ok != tt.wantFound {
				t.Fatalf("findActiveThread() found = %v, want %v", ok, tt.wantFound)
			}
			if ok && got.ID != tt.wantID {
				t.Errorf("findActiveThread() ID = %q, want %q", got.ID, tt.wantID)
			}
		})
	}
}

// Test unmarshaling realistic thread/loaded/list and thread/read
// responses, matching the actual schema shape verified against a real
// `codex app-server generate-json-schema` dump (tagged ThreadStatus union
// with a "type" discriminant, activeFlags only present when type is
// "active") and against a real live app-server (see the codex-app-server-api
// plan / activity-sources-plan project memory for why thread/loaded/list +
// thread/read are used instead of thread/list).
func TestCodexThreadLoadedListAndReadResponseUnmarshal(t *testing.T) {
	rawLoaded := `{"data": ["t1", "t2", "t3", "t4", "t5"]}`
	var loaded codexThreadLoadedListResponse
	if err := json.Unmarshal([]byte(rawLoaded), &loaded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if len(loaded.Data) != 5 {
		t.Fatalf("got %d loaded thread IDs, want 5", len(loaded.Data))
	}

	rawReads := map[string]string{
		"t1": `{"thread": {"id": "t1", "status": {"type": "notLoaded"}}}`,
		"t2": `{"thread": {"id": "t2", "status": {"type": "idle"}}}`,
		"t3": `{"thread": {"id": "t3", "status": {"type": "systemError"}}}`,
		"t4": `{"thread": {"id": "t4", "status": {"type": "active", "activeFlags": []}}}`,
		"t5": `{"thread": {"id": "t5", "status": {"type": "active", "activeFlags": ["waitingOnUserInput"]}}}`,
	}
	var threads []codexThread
	for _, id := range loaded.Data {
		var resp codexThreadReadResponse
		if err := json.Unmarshal([]byte(rawReads[id]), &resp); err != nil {
			t.Fatalf("Unmarshal thread/read(%s) failed: %v", id, err)
		}
		threads = append(threads, resp.Thread)
	}

	active, ok := findActiveThread(threads)
	if !ok {
		t.Fatal("expected an active thread to be found")
	}
	if active.ID != "t4" {
		t.Errorf("first active thread ID = %q, want %q", active.ID, "t4")
	}

	if threads[4].Status.Type != codexThreadStatusActive {
		t.Errorf("t5 status.type = %q, want %q", threads[4].Status.Type, codexThreadStatusActive)
	}
	if len(threads[4].Status.ActiveFlags) != 1 || threads[4].Status.ActiveFlags[0] != "waitingOnUserInput" {
		t.Errorf("t5 activeFlags = %v, want [waitingOnUserInput]", threads[4].Status.ActiveFlags)
	}
}

// Test the JSON-RPC request envelope marshals with the expected field
// names/shape (jsonrpc, id, method, params), using thread/read's
// threadId param as a representative example.
func TestCodexJSONRPCRequestMarshal(t *testing.T) {
	req := codexJSONRPCRequest{
		JSONRPC: "2.0",
		ID:      7,
		Method:  "thread/read",
		Params:  codexThreadReadParams{ThreadID: "01a08835-2ff8-7571-8a8c-d2bc17cce158"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal of marshaled request failed: %v", err)
	}
	if got["jsonrpc"] != "2.0" || got["method"] != "thread/read" {
		t.Errorf("unexpected envelope: %v", got)
	}
	if id, ok := got["id"].(float64); !ok || int64(id) != 7 {
		t.Errorf("unexpected id: %v", got["id"])
	}
	params, ok := got["params"].(map[string]any)
	if !ok {
		t.Fatalf("params missing or wrong type: %v", got["params"])
	}
	if params["threadId"] != "01a08835-2ff8-7571-8a8c-d2bc17cce158" {
		t.Errorf("params.threadId = %v, want %q", params["threadId"], "01a08835-2ff8-7571-8a8c-d2bc17cce158")
	}
}

// fakeCodexAppServerConfig configures startCodexAppServerFakeServer's
// canned responses for thread/loaded/list and thread/read.
type fakeCodexAppServerConfig struct {
	loadedList    []string
	loadedListErr *codexJSONRPCError
	threads       map[string]codexThread        // threadId -> canned thread/read result
	threadReadErr map[string]*codexJSONRPCError // threadId -> canned thread/read error
}

// startCodexAppServerFakeServer starts a minimal fake codex app-server: a
// WebSocket endpoint over a real Unix socket that answers "initialize",
// "thread/loaded/list", and "thread/read" with canned responses per cfg.
// It also emits one unsolicited notification frame before every real
// response, to verify the client's read loop correctly skips traffic that
// doesn't match the request it's waiting for (real codex connections can
// carry such notifications even though this client never subscribes to
// anything).
func startCodexAppServerFakeServer(t *testing.T, cfg fakeCodexAppServerConfig) string {
	t.Helper()
	sockPath := filepath.Join(t.TempDir(), "app-server-control.sock")
	startCodexAppServerFakeServerAt(t, sockPath, cfg)
	return sockPath
}

// startCodexAppServerFakeServerAt is startCodexAppServerFakeServer but
// binding to a caller-chosen socket path, for tests that need to control
// the exact path (e.g. verifying the daemon-assigned-default resolution
// in scanCandidate, which computes a specific path and must find a real
// server listening there).
func startCodexAppServerFakeServerAt(t *testing.T, sockPath string, cfg fakeCodexAppServerConfig) {
	t.Helper()

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to listen on unix socket: %v", err)
	}

	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var req struct {
				ID     int64           `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := json.Unmarshal(msg, &req); err != nil {
				continue
			}

			// Unsolicited notification (no id) before the real response,
			// to exercise the client's skip-unrelated-frames behavior.
			notif, _ := json.Marshal(map[string]any{"method": "some/notification"})
			_ = conn.WriteMessage(websocket.TextMessage, notif)

			resp := map[string]any{"id": req.ID}
			switch req.Method {
			case "initialize":
				resp["result"] = map[string]any{
					"userAgent":      "fake-codex-app-server",
					"codexHome":      "/tmp/codex-home",
					"platformFamily": "unix",
					"platformOs":     "linux",
				}
			case "thread/loaded/list":
				if cfg.loadedListErr != nil {
					resp["error"] = cfg.loadedListErr
				} else {
					resp["result"] = map[string]any{"data": cfg.loadedList}
				}
			case "thread/read":
				var params codexThreadReadParams
				_ = json.Unmarshal(req.Params, &params)
				if rerr, ok := cfg.threadReadErr[params.ThreadID]; ok {
					resp["error"] = rerr
				} else if thread, ok := cfg.threads[params.ThreadID]; ok {
					resp["result"] = map[string]any{"thread": thread}
				} else {
					resp["error"] = codexJSONRPCError{Code: -32001, Message: "unknown thread id"}
				}
			default:
				resp["error"] = codexJSONRPCError{Code: -32601, Message: "method not found"}
			}

			data, err := json.Marshal(resp)
			if err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}
	})

	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Close()
		_ = listener.Close()
	})
}

// Test the full dial -> initialize -> thread/loaded/list -> thread/read
// round trip against a fake codex-app-server over a real Unix socket,
// including that an unsolicited notification frame arriving before the
// real response is correctly skipped rather than mistaken for it.
func TestCodexAppServerClientLoadedListAndRead(t *testing.T) {
	sockPath := startCodexAppServerFakeServer(t, fakeCodexAppServerConfig{
		loadedList: []string{"t1", "t2"},
		threads: map[string]codexThread{
			"t1": {ID: "t1", Status: codexThreadStatus{Type: "idle"}},
			"t2": {ID: "t2", Status: codexThreadStatus{Type: "active", ActiveFlags: []string{"waitingOnUserInput"}}},
		},
	})

	client, err := dialCodexAppServerClient(sockPath)
	if err != nil {
		t.Fatalf("dialCodexAppServerClient failed: %v", err)
	}
	defer client.Close()

	loadedIDs, err := client.threadLoadedList()
	if err != nil {
		t.Fatalf("threadLoadedList() failed: %v", err)
	}
	if len(loadedIDs) != 2 {
		t.Fatalf("got %d loaded thread IDs, want 2", len(loadedIDs))
	}

	var threads []codexThread
	for _, id := range loadedIDs {
		thread, err := client.threadRead(id)
		if err != nil {
			t.Fatalf("threadRead(%s) failed: %v", id, err)
		}
		threads = append(threads, thread)
	}

	active, ok := findActiveThread(threads)
	if !ok || active.ID != "t2" {
		t.Errorf("expected active thread t2, got found=%v id=%q", ok, active.ID)
	}
}

// Test that a JSON-RPC error response from thread/loaded/list surfaces as
// a Go error, rather than being silently treated as "no threads."
func TestCodexAppServerClientLoadedListError(t *testing.T) {
	sockPath := startCodexAppServerFakeServer(t, fakeCodexAppServerConfig{
		loadedListErr: &codexJSONRPCError{Code: -32000, Message: "boom"},
	})

	client, err := dialCodexAppServerClient(sockPath)
	if err != nil {
		t.Fatalf("dialCodexAppServerClient failed: %v", err)
	}
	defer client.Close()

	if _, err := client.threadLoadedList(); err == nil {
		t.Fatal("expected threadLoadedList() to return an error, got nil")
	}
}

// Test that a JSON-RPC error response from thread/read (e.g. a thread
// that disappeared between thread/loaded/list and thread/read) surfaces
// as a Go error for that specific call, rather than being silently
// treated as "not active."
func TestCodexAppServerClientThreadReadError(t *testing.T) {
	sockPath := startCodexAppServerFakeServer(t, fakeCodexAppServerConfig{
		loadedList:    []string{"t1"},
		threadReadErr: map[string]*codexJSONRPCError{"t1": {Code: -32001, Message: "thread not found"}},
	})

	client, err := dialCodexAppServerClient(sockPath)
	if err != nil {
		t.Fatalf("dialCodexAppServerClient failed: %v", err)
	}
	defer client.Close()

	if _, err := client.threadRead("t1"); err == nil {
		t.Fatal("expected threadRead() to return an error, got nil")
	}
}

// Test the updatedAt-freshness decision: a candidate's most recently
// updated loaded thread determines activity, regardless of its `status`
// - live testing showed `status` is never observably "active" via
// thread/read even polling every 5s across real completed interactions,
// while `updatedAt` precisely matches the real turn-completion time (see
// the codex-app-server-api plan / activity-sources-plan project memory
// for the full live investigation). All these threads report status
// "idle" throughout - only updatedAt varies.
func TestCodexAppServerApiScanCandidateUpdatedAtFreshness(t *testing.T) {
	cand := codexAppServerCandidate{pid: "99999"}

	t.Run("recently updated, within window - reports active", func(t *testing.T) {
		sockPath := startCodexAppServerFakeServer(t, fakeCodexAppServerConfig{
			loadedList: []string{"t1"},
			threads: map[string]codexThread{
				"t1": {ID: "t1", Status: codexThreadStatus{Type: "idle"}, UpdatedAt: time.Now().Add(-10 * time.Second).Unix()},
			},
		})
		c := cand
		c.socketPath = sockPath
		src := &codexAppServerApiActivitySource{}
		active, _ := src.scanCandidate(c, ActivityScanContext{ActivityWindow: time.Minute})
		if !active {
			t.Error("expected active: updatedAt is within the activity window")
		}
	})

	t.Run("updated outside window - reports not active", func(t *testing.T) {
		sockPath := startCodexAppServerFakeServer(t, fakeCodexAppServerConfig{
			loadedList: []string{"t1"},
			threads: map[string]codexThread{
				"t1": {ID: "t1", Status: codexThreadStatus{Type: "idle"}, UpdatedAt: time.Now().Add(-2 * time.Minute).Unix()},
			},
		})
		c := cand
		c.socketPath = sockPath
		src := &codexAppServerApiActivitySource{}
		active, _ := src.scanCandidate(c, ActivityScanContext{ActivityWindow: time.Minute})
		if active {
			t.Error("expected not active: updatedAt is outside the activity window")
		}
	})

	t.Run("status active but updatedAt stale - still reports not active", func(t *testing.T) {
		// Directly exercises that status no longer gates the decision:
		// a thread claiming status "active" with a stale updatedAt must
		// not be reported as active.
		sockPath := startCodexAppServerFakeServer(t, fakeCodexAppServerConfig{
			loadedList: []string{"t1"},
			threads: map[string]codexThread{
				"t1": {ID: "t1", Status: codexThreadStatus{Type: "active"}, UpdatedAt: time.Now().Add(-2 * time.Minute).Unix()},
			},
		})
		c := cand
		c.socketPath = sockPath
		src := &codexAppServerApiActivitySource{}
		active, _ := src.scanCandidate(c, ActivityScanContext{ActivityWindow: time.Minute})
		if active {
			t.Error("status=active with a stale updatedAt must not be reported as active")
		}
	})

	t.Run("most recently updated thread wins among several", func(t *testing.T) {
		sockPath := startCodexAppServerFakeServer(t, fakeCodexAppServerConfig{
			loadedList: []string{"t1", "t2"},
			threads: map[string]codexThread{
				"t1": {ID: "t1", Status: codexThreadStatus{Type: "idle"}, UpdatedAt: time.Now().Add(-5 * time.Minute).Unix()},
				"t2": {ID: "t2", Status: codexThreadStatus{Type: "idle"}, UpdatedAt: time.Now().Add(-5 * time.Second).Unix()},
			},
		})
		c := cand
		c.socketPath = sockPath
		src := &codexAppServerApiActivitySource{}
		active, _ := src.scanCandidate(c, ActivityScanContext{ActivityWindow: time.Minute})
		if !active {
			t.Error("expected active: t2's updatedAt is within the window even though t1's isn't")
		}
	})

	t.Run("no loaded threads - reports not active", func(t *testing.T) {
		sockPath := startCodexAppServerFakeServer(t, fakeCodexAppServerConfig{loadedList: []string{}})
		c := cand
		c.socketPath = sockPath
		src := &codexAppServerApiActivitySource{}
		active, _ := src.scanCandidate(c, ActivityScanContext{ActivityWindow: time.Minute})
		if active {
			t.Error("expected not active with zero loaded threads")
		}
	})
}

// waitForProcessEnviron polls /proc/<pid>/environ (via getProcessEnviron)
// until it contains key, up to a short timeout. Immediately after
// exec.Cmd.Start() returns, /proc/<pid>/environ can still briefly reflect
// a pre-exec state rather than the child's actual exec'd environment
// (confirmed empirically - reading it with zero delay after Start() misses
// env vars set via cmd.Env) - this closes that race for tests that need a
// real child process's resolved environment.
func waitForProcessEnviron(t *testing.T, pid int, key string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		env, err := getProcessEnviron(fmt.Sprintf("%d", pid))
		if err == nil && env[key] != "" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to appear in pid %d's environ", key, pid)
}

// Test defaultCodexAppServerSocketPath: the pure join logic behind the
// daemon-assigned-default fallback.
func TestDefaultCodexAppServerSocketPath(t *testing.T) {
	got := defaultCodexAppServerSocketPath("/home/user/.codex")
	want := "/home/user/.codex/app-server-control/app-server-control.sock"
	if got != want {
		t.Errorf("defaultCodexAppServerSocketPath() = %q, want %q", got, want)
	}
}

// Test the full daemon-assigned-default fallback end to end: when
// discovery returns an empty socketPath (codex started with a bare
// `--listen unix://`, confirmed this is what a real production container
// startup script actually does), scanCandidate must resolve
// ${CODEX_HOME:-$HOME/.codex}/app-server-control/app-server-control.sock
// via the target process's own environment and successfully connect
// there - not just via che-machine-exec's own environment, which is why
// this spawns a real child process with a controlled environment rather
// than using t.Setenv on the test binary itself (/proc/[pid]/environ
// reflects env at exec() time, not live setenv() changes).
func TestCodexAppServerApiScanCandidateResolvesDaemonAssignedDefaultSocket(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping /proc-based env resolution test on non-Linux platform")
	}

	// A short-prefixed temp dir, not t.TempDir(): AF_UNIX socket paths are
	// capped at ~108 bytes (sun_path), and t.TempDir() embeds this test's
	// (long) name in the path, which combined with
	// "/app-server-control/app-server-control.sock" overflows that limit.
	codexHome, err := os.MkdirTemp("", "cah")
	if err != nil {
		t.Fatalf("failed to create temp CODEX_HOME: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(codexHome) })
	sockPath := filepath.Join(codexHome, "app-server-control", "app-server-control.sock")
	if err := os.MkdirAll(filepath.Dir(sockPath), 0o755); err != nil {
		t.Fatalf("failed to create app-server-control dir: %v", err)
	}
	startCodexAppServerFakeServerAt(t, sockPath, fakeCodexAppServerConfig{
		loadedList: []string{"t1"},
		threads: map[string]codexThread{
			"t1": {ID: "t1", Status: codexThreadStatus{Type: "idle"}, UpdatedAt: time.Now().Add(-5 * time.Second).Unix()},
		},
	})

	// Stand-in for the real codex-app-server process: a long-lived child
	// with CODEX_HOME set in its own environment, exactly what
	// resolveCodexBaseDir reads via /proc/<pid>/environ.
	cmd := exec.Command("sleep", "30")
	cmd.Env = append(os.Environ(), "CODEX_HOME="+codexHome)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start stand-in process: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	waitForProcessEnviron(t, cmd.Process.Pid, "CODEX_HOME")

	cand := codexAppServerCandidate{pid: fmt.Sprintf("%d", cmd.Process.Pid), socketPath: ""}
	src := &codexAppServerApiActivitySource{}
	active, label := src.scanCandidate(cand, ActivityScanContext{ActivityWindow: time.Minute})
	if !active {
		t.Fatal("expected active thread to be found via the resolved default socket path")
	}
	if label != fmt.Sprintf("codex-app-server-api (pid %d, socket %s)", cmd.Process.Pid, sockPath) {
		t.Errorf("unexpected label: %s", label)
	}
}

// Test that when the default socket path can't be resolved either
// (neither CODEX_HOME nor HOME set in the target process's environment),
// scanCandidate reports not-active rather than guessing or panicking.
func TestCodexAppServerApiScanCandidateUnresolvableDefaultSocket(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping /proc-based env resolution test on non-Linux platform")
	}

	cmd := exec.Command("sleep", "30")
	cmd.Env = []string{} // deliberately empty: neither CODEX_HOME nor HOME
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start stand-in process: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	cand := codexAppServerCandidate{pid: fmt.Sprintf("%d", cmd.Process.Pid), socketPath: ""}
	src := &codexAppServerApiActivitySource{}
	active, _ := src.scanCandidate(cand, ActivityScanContext{ActivityWindow: time.Minute})
	if active {
		t.Error("expected not active when the default socket path cannot be resolved")
	}
}

// Test that dialing a socket with nothing listening fails cleanly (no
// panic, no hang) - the state a codexAppServerApiActivitySource will
// actually see whenever a discovered candidate's app-server has already
// exited between discovery and connect.
func TestDialCodexAppServerClientNoListener(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "nothing-here.sock")
	if _, err := dialCodexAppServerClient(sockPath); err == nil {
		t.Fatal("expected dial to a nonexistent socket to fail")
	}
}

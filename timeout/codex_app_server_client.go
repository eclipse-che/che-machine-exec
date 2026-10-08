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
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// codexAppServerRPCTimeout bounds how long a single JSON-RPC round trip
// (write + matching response) may take before the call is treated as
// failed. A live app-server answers requests like initialize/thread/list
// near-instantly; this is only a safety net against a hung/unresponsive
// daemon, not a normal-case budget.
const codexAppServerRPCTimeout = 5 * time.Second

// codexAppServerClientName/Version identify che-machine-exec to the codex
// app-server during the initialize handshake. Not tied to
// che-machine-exec's own product version (the VERSION file): codex does
// not validate this beyond requiring a non-empty string, so a fixed
// client-protocol identifier is simpler than wiring in file I/O for a
// purely informational field.
const (
	codexAppServerClientName    = "che-machine-exec-cli-watcher"
	codexAppServerClientVersion = "1.0"
)

// codexJSONRPCRequest is the minimal JSON-RPC 2.0 request envelope needed
// to drive the codex app-server's initialize/thread/list methods. Params
// is left as `any` (marshaled per-call) rather than modeled as a full
// request-type enum - this client only ever calls two fixed methods, not
// the full app-server protocol surface.
type codexJSONRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// codexJSONRPCResponse covers both normal responses (matched by ID, to our
// own requests) and unsolicited server-to-client traffic (notifications -
// no ID; server-initiated requests - has both ID and Method). This client
// only ever reads, never answers, server-initiated requests: it is a
// passive observer, never a full app-server client.
type codexJSONRPCResponse struct {
	ID     *int64             `json:"id"`
	Method string             `json:"method"`
	Result json.RawMessage    `json:"result"`
	Error  *codexJSONRPCError `json:"error"`
}

type codexJSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// codexThreadStatus mirrors codex-rs's `ThreadStatus` (v2 protocol):
// notLoaded | idle | systemError | active { activeFlags: [...] }. Only
// Type is used for the active/idle decision; ActiveFlags is carried
// through for verbose/debug logging only.
type codexThreadStatus struct {
	Type        string   `json:"type"`
	ActiveFlags []string `json:"activeFlags,omitempty"`
}

// codexThreadStatusActive is the ThreadStatus.Type value meaning "this
// thread has a genuinely open/in-progress conversation" - see the
// codex-app-server-api plan for why this is a richer signal than a
// rolling activity window (it stays active while waiting on the user's
// next message, not just mid-tool-execution).
const codexThreadStatusActive = "active"

// codexThread is the small subset of codex-rs's `Thread` (v2 protocol)
// this client actually needs - not a full mirror of that much larger type.
// UpdatedAt (Unix seconds, always present) is the actual activity-decision
// input - live-verified to match thread/turns/list's per-turn completedAt
// exactly, updating precisely at turn completion. Status/ActiveFlags are
// diagnostic-only: live-tested, status was never once observed as
// "active" via thread/read even polling every 5s across real completed
// interactions, so it cannot gate the decision - see
// activity_source_codex_app_server_api.go's type doc comment for the full
// investigation.
type codexThread struct {
	ID        string            `json:"id"`
	Status    codexThreadStatus `json:"status"`
	UpdatedAt int64             `json:"updatedAt"`
}

// findActiveThread returns the first thread whose status is "active" (if
// any), for the active/not-active decision in
// codexAppServerApiActivitySource.scanCandidate. Split out as a pure
// function so the decision logic is unit-testable against canned
// codexThread slices, independent of any real socket/connection.
func findActiveThread(threads []codexThread) (codexThread, bool) {
	for _, thread := range threads {
		if thread.Status.Type == codexThreadStatusActive {
			return thread, true
		}
	}
	return codexThread{}, false
}

// codexThreadReadParams mirrors the subset of codex-rs's `ThreadReadParams`
// this client uses: just the thread id, no turn-history hydration.
type codexThreadReadParams struct {
	ThreadID string `json:"threadId"`
}

// codexThreadReadResponse mirrors codex-rs's `ThreadReadResponse`.
type codexThreadReadResponse struct {
	Thread codexThread `json:"thread"`
}

// codexThreadLoadedListResponse mirrors codex-rs's `ThreadLoadedListResponse`:
// thread IDs currently loaded in the app-server's memory, regardless of
// whether they've been flushed to its persisted/indexed thread store yet.
//
// This client deliberately uses thread/loaded/list + thread/read per ID,
// NOT thread/list: live-tested against a real app-server, thread/list
// returned only older, already-`notLoaded` threads and omitted a thread
// that had just been created and was actively being driven in the same
// moment - i.e. thread/list reflects the persisted/indexed view, not
// what's actually loaded/live right now. thread/loaded/list + thread/read
// correctly surfaced that same thread's live status. Polling thread/list
// would have systematically missed real-time activity on any thread not
// yet flushed to that index - unacceptable for this source's whole
// purpose. See the codex-app-server-api plan/activity-sources-plan
// project memory for the full investigation.
type codexThreadLoadedListResponse struct {
	Data []string `json:"data"`
}

// codexAppServerClient is a minimal, read-only JSON-RPC client for a
// codex-app-server's `--listen unix://...` control socket. It never calls
// thread/start, thread/resume, or any method that would create or join a
// conversation - see the codex-app-server-api plan for why passive
// observation via thread/loaded/list + thread/read alone is sufficient
// and deliberate.
type codexAppServerClient struct {
	conn   *websocket.Conn
	nextID atomic.Int64
}

// dialCodexAppServerClient opens a WebSocket connection to a codex
// app-server over a Unix domain socket and completes the required
// `initialize` handshake. gorilla/websocket has no native "dial a unix
// socket" option, so NetDialContext is overridden to always dial the
// given socketPath regardless of the (dummy) URL's nominal host - the
// standard Go pattern for WS-over-UDS.
func dialCodexAppServerClient(socketPath string) (*codexAppServerClient, error) {
	dialer := websocket.Dialer{
		NetDialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
		HandshakeTimeout: codexAppServerRPCTimeout,
	}

	conn, _, err := dialer.Dial("ws://unix/", nil)
	if err != nil {
		return nil, fmt.Errorf("dial codex-app-server socket %s: %w", socketPath, err)
	}

	client := &codexAppServerClient{conn: conn}
	if err := client.initialize(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return client, nil
}

// Close closes the underlying WebSocket connection.
func (c *codexAppServerClient) Close() error {
	return c.conn.Close()
}

// initialize performs the JSON-RPC handshake every codex app-server
// connection must complete before any other method is accepted.
func (c *codexAppServerClient) initialize() error {
	params := map[string]any{
		"clientInfo": map[string]any{
			"name":    codexAppServerClientName,
			"version": codexAppServerClientVersion,
		},
	}
	return c.call("initialize", params, nil)
}

// threadLoadedList calls `thread/loaded/list` and returns the IDs of every
// thread currently loaded in the app-server's memory - a plain,
// unscoped request/response call, not tied to any thread this connection
// itself started or resumed.
func (c *codexAppServerClient) threadLoadedList() ([]string, error) {
	var resp codexThreadLoadedListResponse
	if err := c.call("thread/loaded/list", map[string]any{}, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// threadRead calls `thread/read` for one thread ID and returns its current
// live state (including `status`).
func (c *codexAppServerClient) threadRead(threadID string) (codexThread, error) {
	params := codexThreadReadParams{ThreadID: threadID}
	var resp codexThreadReadResponse
	if err := c.call("thread/read", params, &resp); err != nil {
		return codexThread{}, err
	}
	return resp.Thread, nil
}

// call sends one JSON-RPC request and blocks for its matching response,
// discarding any unsolicited notification/server-request traffic that
// arrives first (this client never subscribes to anything, but codex may
// still send connection-level notifications unrelated to our request).
func (c *codexAppServerClient) call(method string, params any, result any) error {
	id := c.nextID.Add(1)
	req := codexJSONRPCRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal %s request: %w", method, err)
	}
	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("write %s request: %w", method, err)
	}

	deadline := time.Now().Add(codexAppServerRPCTimeout)
	for {
		if err := c.conn.SetReadDeadline(deadline); err != nil {
			return fmt.Errorf("set read deadline for %s response: %w", method, err)
		}
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read %s response: %w", method, err)
		}

		var resp codexJSONRPCResponse
		if err := json.Unmarshal(msg, &resp); err != nil {
			// Malformed frame - not our response either way, keep waiting
			// for the real one rather than failing the whole call on
			// unrelated server chatter.
			continue
		}
		if resp.ID == nil || *resp.ID != id {
			continue
		}
		if resp.Error != nil {
			return fmt.Errorf("%s error %d: %s", method, resp.Error.Code, resp.Error.Message)
		}
		if result == nil {
			return nil
		}
		return json.Unmarshal(resp.Result, result)
	}
}

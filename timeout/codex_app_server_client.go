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

// codexAppServerRPCTimeout is the timeout for a JSON-RPC round trip.
const codexAppServerRPCTimeout = 5 * time.Second

// codexAppServerClientName/Version identify che-machine-exec during the initialize handshake.
const (
	codexAppServerClientName    = "che-machine-exec-cli-watcher"
	codexAppServerClientVersion = "1.0"
)

// codexJSONRPCRequest is the JSON-RPC 2.0 request envelope.
type codexJSONRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// codexJSONRPCResponse is the JSON-RPC 2.0 response envelope.
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

// codexAppServerRPCError is a JSON-RPC error response (as opposed to a
// transport failure - the connection is still healthy in this case).
type codexAppServerRPCError struct {
	Method  string
	Code    int
	Message string
}

func (e *codexAppServerRPCError) Error() string {
	return fmt.Sprintf("%s error %d: %s", e.Method, e.Code, e.Message)
}

// codexThreadStatus mirrors codex-rs's ThreadStatus (v2 protocol).
type codexThreadStatus struct {
	Type        string   `json:"type"`
	ActiveFlags []string `json:"activeFlags,omitempty"`
}

// codexThreadStatusActive is the ThreadStatus.Type value for active threads.
const codexThreadStatusActive = "active"

// codexThread is a subset of codex-rs's Thread (v2 protocol).
// UpdatedAt is used for activity decisions; Status/ActiveFlags are diagnostic-only.
type codexThread struct {
	ID        string            `json:"id"`
	Status    codexThreadStatus `json:"status"`
	UpdatedAt int64             `json:"updatedAt"`
}

// findActiveThread returns the first thread with status "active", if any.
func findActiveThread(threads []codexThread) (codexThread, bool) {
	for _, thread := range threads {
		if thread.Status.Type == codexThreadStatusActive {
			return thread, true
		}
	}
	return codexThread{}, false
}

// codexThreadReadParams mirrors a subset of codex-rs's ThreadReadParams.
type codexThreadReadParams struct {
	ThreadID string `json:"threadId"`
}

// codexThreadReadResponse mirrors codex-rs's `ThreadReadResponse`.
type codexThreadReadResponse struct {
	Thread codexThread `json:"thread"`
}

// codexThreadLoadedListResponse mirrors codex-rs's ThreadLoadedListResponse.
// Contains thread IDs currently loaded in memory (not just persisted threads).
type codexThreadLoadedListResponse struct {
	Data []string `json:"data"`
}

// codexAppServerClient is a read-only JSON-RPC client for a codex app-server control socket.
type codexAppServerClient struct {
	conn   *websocket.Conn
	nextID atomic.Int64
}

// dialCodexAppServerClient opens a WebSocket connection over a Unix socket and initializes it.
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

// initialize performs the required JSON-RPC handshake.
func (c *codexAppServerClient) initialize() error {
	params := map[string]any{
		"clientInfo": map[string]any{
			"name":    codexAppServerClientName,
			"version": codexAppServerClientVersion,
		},
	}
	return c.call("initialize", params, nil)
}

// threadLoadedList calls thread/loaded/list and returns loaded thread IDs.
func (c *codexAppServerClient) threadLoadedList() ([]string, error) {
	var resp codexThreadLoadedListResponse
	if err := c.call("thread/loaded/list", map[string]any{}, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// threadRead calls thread/read and returns the thread's current state.
func (c *codexAppServerClient) threadRead(threadID string) (codexThread, error) {
	params := codexThreadReadParams{ThreadID: threadID}
	var resp codexThreadReadResponse
	if err := c.call("thread/read", params, &resp); err != nil {
		return codexThread{}, err
	}
	return resp.Thread, nil
}

// call sends a JSON-RPC request and waits for the matching response.
func (c *codexAppServerClient) call(method string, params any, result any) error {
	id := c.nextID.Add(1)
	req := codexJSONRPCRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal %s request: %w", method, err)
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(codexAppServerRPCTimeout)); err != nil {
		return fmt.Errorf("set write deadline for %s request: %w", method, err)
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
			return &codexAppServerRPCError{Method: method, Code: resp.Error.Code, Message: resp.Error.Message}
		}
		if result == nil {
			return nil
		}
		return json.Unmarshal(resp.Result, result)
	}
}

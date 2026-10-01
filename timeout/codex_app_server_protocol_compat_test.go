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
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// TestCodexAppServerProtocolCompatibility guards against the codex
// app-server protocol shifting under this client: it runs `codex
// app-server generate-json-schema` against whatever `codex` CLI is
// actually installed and checks that the specific shapes/method names
// codex_app_server_client.go depends on still match. This is a
// compatibility smoke check against a real external tool, not a
// hermetic unit test - it's skipped entirely when no `codex` binary is on
// PATH (e.g. most CI environments), and skipped (not failed) if an
// installed codex doesn't support this (still-experimental) subcommand.
//
// If this test ever fails against a real installed codex, that's a real
// signal: the protocol has changed in a way that could silently break
// codex-app-server-api's activity detection at runtime with no other
// warning, since the pure-logic tests in codex_app_server_api_test.go
// only verify this client's own code against canned fixtures, not
// against what codex itself actually speaks.
func TestCodexAppServerProtocolCompatibility(t *testing.T) {
	codexPath, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("codex CLI not found on PATH, skipping protocol compatibility check")
	}

	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, codexPath, "app-server", "generate-json-schema", "--out", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("codex app-server generate-json-schema failed (installed codex may not support this experimental subcommand): %v\n%s", err, out)
	}

	// Thread must still carry a required "status" property - the field
	// this client's codexThread type depends on entirely.
	bundle := readCompatSchemaBundle(t, filepath.Join(dir, "v2", "ThreadListResponse.json"))

	var thread struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	threadDef, ok := bundle.Definitions["Thread"]
	if !ok {
		t.Fatal(`schema no longer defines "Thread"`)
	}
	if err := json.Unmarshal(threadDef, &thread); err != nil {
		t.Fatalf("parsing Thread definition: %v", err)
	}
	if _, ok := thread.Properties["status"]; !ok {
		t.Error(`Thread no longer has a "status" property`)
	}
	if !slices.Contains(thread.Required, "status") {
		t.Error("Thread.status is no longer a required field")
	}

	// ThreadStatus must still be a tagged oneOf whose "active" variant
	// requires "activeFlags" - codexThreadStatus/findActiveThread's
	// entire decision rests on this shape.
	var status struct {
		OneOf []struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		} `json:"oneOf"`
	}
	statusDef, ok := bundle.Definitions["ThreadStatus"]
	if !ok {
		t.Fatal(`schema no longer defines "ThreadStatus"`)
	}
	if err := json.Unmarshal(statusDef, &status); err != nil {
		t.Fatalf("parsing ThreadStatus definition: %v", err)
	}

	foundActive := false
	for _, variant := range status.OneOf {
		raw, ok := variant.Properties["type"]
		if !ok {
			continue
		}
		var typeSchema struct {
			Enum []string `json:"enum"`
		}
		if err := json.Unmarshal(raw, &typeSchema); err != nil || len(typeSchema.Enum) != 1 {
			continue
		}
		if typeSchema.Enum[0] != codexThreadStatusActive {
			continue
		}
		foundActive = true
		if _, ok := variant.Properties["activeFlags"]; !ok {
			t.Error(`ThreadStatus's "active" variant no longer has an "activeFlags" property`)
		}
		if !slices.Contains(variant.Required, "activeFlags") {
			t.Error(`ThreadStatus's "active" variant no longer requires "activeFlags"`)
		}
	}
	if !foundActive {
		t.Errorf("ThreadStatus no longer has a %q variant", codexThreadStatusActive)
	}

	// The three method names this client actually calls must still exist
	// in the request schema.
	clientRequestData, err := os.ReadFile(filepath.Join(dir, "ClientRequest.json"))
	if err != nil {
		t.Fatalf("reading ClientRequest.json: %v", err)
	}
	for _, method := range []string{"initialize", "thread/loaded/list", "thread/read"} {
		if !bytes.Contains(clientRequestData, []byte(`"`+method+`"`)) {
			t.Errorf("ClientRequest schema no longer mentions method %q", method)
		}
	}
}

// compatSchemaBundle is the minimal shape needed to look up a named
// definition out of a `codex app-server generate-json-schema` output
// file - the files bundle many unrelated type definitions together under
// a single top-level "definitions" map.
type compatSchemaBundle struct {
	Definitions map[string]json.RawMessage `json:"definitions"`
}

func readCompatSchemaBundle(t *testing.T, path string) compatSchemaBundle {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading generated schema %s: %v", path, err)
	}
	var bundle compatSchemaBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		t.Fatalf("parsing generated schema %s: %v", path, err)
	}
	return bundle
}

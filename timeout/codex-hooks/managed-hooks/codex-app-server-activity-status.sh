#!/bin/sh
#
# Copyright (c) 2026 Red Hat, Inc.
# This program and the accompanying materials are made
# available under the terms of the Eclipse Public License 2.0
# which is available at https://www.eclipse.org/legal/epl-2.0/
#
# SPDX-License-Identifier: EPL-2.0
#
# Contributors:
#   Red Hat, Inc. - initial API and implementation
#

#
# Codex managed hook handler: maintains a status file that
# che-machine-exec's CLI Watcher codex-app-server-hooks ActivitySource polls, so
# CLI Watcher's own config (activity window, grace period, verbose logging,
# admin source enable/disable) stays authoritative - this hook only feeds
# it a reliable signal, it doesn't bypass CLI Watcher.
#
# Codex invokes this with the hook's JSON payload on stdin. We extract
# hook_event_name and session_id without a JSON parser dependency (grep/sed
# on the known field shape) to avoid requiring jq in every workspace image.
#
# Deliberately stateless: every field is derived solely from the current
# event, with no read-modify-write against the previous file content (an
# earlier version tracked an active-sessions counter incremented/decremented
# on SessionStart/SessionEnd, but codex has no hook that fires when the
# app-server PROCESS itself starts - only per-thread SessionStart - so a
# crash that skips SessionEnd had no reliable way to get the counter back in
# sync; dropped rather than papered over). Being fully stateless also means
# no locking is needed: the write is a plain temp-file-then-rename, and the
# worst a race between two near-simultaneous hook invocations can do is
# "last writer wins" between two individually-valid sets of values, never a
# corrupt or partially-written file.
#
# Never fails the hook (always exits 0): a missing CODEX_HOME or a
# filesystem hiccup must never block or delay the user's actual codex
# session.

STATUS_DIR="${CODEX_HOME:-$HOME/.codex}/app-server-control"
STATUS_FILE="$STATUS_DIR/codex-app-server-activity-status.yaml"

mkdir -p "$STATUS_DIR" 2>/dev/null || exit 0

PAYLOAD=$(cat)
EVENT=$(printf '%s' "$PAYLOAD" | grep -o '"hook_event_name"[[:space:]]*:[[:space:]]*"[^"]*"' | sed -E 's/.*:[[:space:]]*"([^"]*)"/\1/')
SESSION_ID=$(printf '%s' "$PAYLOAD" | grep -o '"session_id"[[:space:]]*:[[:space:]]*"[^"]*"' | sed -E 's/.*:[[:space:]]*"([^"]*)"/\1/')
NOW=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# SessionEnd can fire well after the user actually went idle (e.g. codex's
# own disconnect timeout), not in real time like every other event here -
# refreshing the file's mtime at that point would reset CLI Watcher's idle
# clock right when it should be allowed to expire. So SessionEnd is the one
# event that must NOT touch the file.
if [ "$EVENT" = "SessionEnd" ]; then
    exit 0
fi

# Write via a temp file + rename so a reader never observes a
# partially-written file.
TMP_FILE="$STATUS_FILE.tmp.$$"
{
    printf 'last-event: %s\n' "$EVENT"
    printf 'last-session-id: %s\n' "$SESSION_ID"
    printf 'last-activity: %s\n' "$NOW"
} > "$TMP_FILE" 2>/dev/null && mv -f "$TMP_FILE" "$STATUS_FILE" 2>/dev/null

exit 0

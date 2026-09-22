# Codex Managed Hooks — Activity Detection Example

## Problem this solves

CLI Watcher needs to know when a user is actively working with `codex
app-server` so the workspace doesn't idle out from under them. Everything
inferred from `/proc` (I/O byte deltas, socket fd presence, etc.) turned out
to have real false-positive failure modes:

- codex's own periodic background writes (~every 10s, unrelated to any
  client) would look like "activity" forever if using a combined I/O delta.
- codex maintains a persistent outbound "remote control" websocket to
  `chatgpt.com` plus periodic model-list HTTP calls, independent of whether
  any client is attached - polluting a whole-process `rchar` reading.
- The async runtime's own internal `socketpair()` (used for its wakeup
  mechanism) is structurally indistinguishable from a real accepted client
  connection via `/proc/net/unix` alone - "is a client connected" can't be
  answered reliably from `/proc` without netlink `sock_diag` (what `ss`
  uses) or an external `iproute2` dependency.

Given a false positive can effectively disable idle-shutdown for an entire
detection cycle (0.5-3h depending on configured timeout), inference-based
detection was judged too risky. This directory implements the alternative:
**codex tells us directly** via its own hooks system, the same way the
existing Claude Code integration already works (`~/.claude/settings.json`
hooks POST to `http://127.0.0.1:${MACHINE_EXEC_PORT}/activity/tick`, which
already exists in `che-machine-exec` — see `main.go:73` /
`api/rest/activity.go`).

Unlike that Claude Code integration, this does **not** POST a tick directly
— doing so would bypass CLI Watcher's own config (activity window, grace
period, verbose logging, per-admin source enable/disable) entirely, turning
codex into a special case no other source gets. Instead, the hook script
only maintains a **status file** that CLI Watcher's `codex-app-server-hooks`
`ActivitySource` polls on its normal scan cycle, same as every other
source — the hook supplies a reliable signal, CLI Watcher's own config
stays authoritative over what to do with it.

## Why "managed" hooks, specifically

Codex hooks normally require **interactive trust**: the first time a hook
appears (or its command changes), Codex marks it for review and skips it
until a human runs `/hooks` in the CLI and approves it. That's an
unacceptable reliability gap for something CLI Watcher depends on — nobody
can guarantee an admin or user ever does that, and it could silently never
fire.

Codex has one documented way around this: **managed hooks**, sourced from
`/etc/codex/requirements.toml` (or genuine MDM/enterprise-cloud delivery).
Hooks defined there get `HookTrustStatus::Managed` — they run without any
interactive approval, and (per
`codex-rs/app-server-protocol/src/protocol/v2/hook.rs`) **cannot be disabled
by the user from the hook browser during the session**. This is the only
mechanism that satisfies both constraints CLI Watcher needs: guaranteed to
run, and not something the user can turn off mid-session.

There is **no runtime API** to create or trust a hook on an
already-running codex process (verified against `codex-rs/app-server/src/
request_processors/config_processor.rs` and the hooks discovery/engine
source) — `config/batchWrite` can push a live config update to an
already-running session, but a hook written that way still lands in the
ordinary "User" source and still needs interactive trust. So this only
works if `/etc/codex/requirements.toml` and the hook script under
`managed_dir` are **already in place before `codex app-server` starts** —
i.e. baked into the workspace image or written by a privileged init step,
not something `che-machine-exec` can do for itself at runtime as an
unprivileged workspace-user process.

## Files

- `requirements.toml` → install at `/etc/codex/requirements.toml`
- `managed-hooks/codex-app-server-activity-status.sh` → install at
  `/etc/codex/managed-hooks/codex-app-server-activity-status.sh` (must be
  executable; codex requires managed hook commands to be absolute paths
  under `managed_dir`)

## Status file

Written to `${CODEX_HOME:-$HOME/.codex}/app-server-control/codex-app-server-activity-status.yaml`
(same directory the real container startup script already uses for the
control socket + log — see the `app-server-control` dir referenced
elsewhere in this project's notes). Simple flat `key: value` lines (valid
YAML, trivially parseable, trivially emittable from POSIX `sh` without a
JSON/YAML library on the writer side):

```yaml
last-event: UserPromptSubmit
last-session-id: 01a08835-2ff8-7571-8a8c-d2bc17cce158
last-activity: 2026-09-21T14:32:07Z
```

Deliberately **stateless**: every field is derived solely from the current
hook event, with no read-modify-write against the previous file content.
Earlier iterations of this design also tracked an `active-sessions` counter
and a derived `status: started`/`stopped` label — both dropped. The counter
was dropped because codex has no hook that fires when the app-server
*process* itself starts, only per-thread `SessionStart`, so a crash that
skips `SessionEnd` had no reliable way to get the counter back in sync (it
could only ever drift upward across repeated crash/restart cycles on a
persistent `$CODEX_HOME`). `status` was dropped after live-testing showed
it: (1) never reflects whether the app-server *process* is running anyway
(`Stop` fires per-turn, not per-session — confirmed live: a single session
produced repeated `Stop` events, one per prompt/response cycle) — a fresh
`last-activity` already implies "a live codex process fired a hook
recently," which is the only thing that actually matters; and (2) was
entirely computed from `$EVENT` by us, adding no information beyond what
`last-event` already carries. Being fully stateless also means **no
locking is needed**: the write is a plain temp-file-then-rename, and the
worst a race between two near-simultaneous hook invocations can do is
"last writer wins" between two individually-valid sets of values, never a
corrupt file.

- `last-event` / `last-session-id` / `last-activity`: diagnostic fields,
  parsed and logged only when CLI Watcher runs verbose (see
  `readCodexStatusFile` in `activity_source_codex_app_server_hooks.go`) —
  none of them are read on the normal (non-verbose) scan path.
- The file's own **mtime** (updated via write-to-temp-then-rename, so a
  reader never observes a partial write, right after the hook computes
  "now") is the **only** input CLI Watcher's `Scan()` actually uses to
  make its active/not-active decision: no open, no read, no parsing
  needed. `last-activity` is refreshed at the same moment as the rename
  for every hook event **except `SessionEnd`** (`SessionStart`,
  `UserPromptSubmit`, `PostToolUse`, `Stop`), so in practice it always
  matches the mtime CLI Watcher actually compares against — it's kept as
  a human-readable echo of that same timestamp for verbose logs, not a
  second source of truth.
  `SessionEnd` is deliberately excluded: codex can fire it well after the
  user actually went idle (e.g. its own disconnect timeout), so refreshing
  the file (and therefore its mtime) at that point would reset CLI
  Watcher's idle clock right when it should be allowed to expire. The
  hook script exits without touching the file for that one event.

### Why staleness doesn't need explicit recovery
CLI Watcher gates activity on the status file's mtime — there's no other
signal left to gate on incorrectly now. If codex-app-server-hooks
crashes mid-session, or a leftover file survives a full workspace restart
on a persistent `$CODEX_HOME` volume, the mtime simply stops
advancing, so once it's older than the configured activity window, CLI
Watcher correctly reports "not active." No reset step, no cleanup job, no
"is this file stale" special-casing needed anywhere.

### Discovery consequence: `/proc` is no longer the activity signal
Because a hook can only fire from inside a live codex process, "the file
has a recent `last-activity`" and "the app-server process is running and
handling real activity" are the same observable fact — CLI Watcher's
`codex-app-server-hooks` `ActivitySource` doesn't need `/proc` comm+cmdline
discovery (carried over from the earlier `/proc`-polling design) to confirm
liveness or as the activity signal itself anymore; the status file's mtime
is authoritative for that. `/proc` discovery is still used for one thing:
finding the running codex process(es) so their own environment
(`/proc/<pid>/environ`) can be read to resolve `CODEX_HOME`/`HOME`
reliably — necessary because che-machine-exec's own environment may not
match wherever codex was actually launched from (see the implementation in
`timeout/activity_source_codex_app_server_hooks.go`). Grace-period handling was
dropped too: grace period exists elsewhere (the `tty` source) to cover a
*detection lag* for a freshly-started process that hasn't had a chance to
demonstrate activity yet — here there's no lag to compensate for, since
`SessionStart` itself is a real-time activity event with its own fresh
timestamp the moment it fires — which, per the live-verified behavior
below, is when the first *turn* begins, not merely when a thread/session
is opened (`thread/start` alone does not trigger it). Until that first
turn starts, no hook has fired yet, so the status file may not exist at
all and the source correctly reports inactive — same as the general
"no hook has fired yet" case described above, not a bug.

## Installing into a workspace image

```dockerfile
COPY timeout/codex-hooks/requirements.toml /etc/codex/requirements.toml
COPY timeout/codex-hooks/managed-hooks/codex-app-server-activity-status.sh /etc/codex/managed-hooks/codex-app-server-activity-status.sh
RUN chmod 0644 /etc/codex/requirements.toml && \
    chmod 0755 /etc/codex/managed-hooks/codex-app-server-activity-status.sh
```

## Verifying it actually works

Since `/etc/codex/requirements.toml` is a fixed system path with no env-var
override, verifying this for real requires actually writing there — which
needs root.

1. Install the files (needs sudo). `/etc/codex/requirements.toml` is a
   single shared file — if your machine already has one (site config, a
   different managed hook, etc.), back it up first so step 6 can restore
   it instead of deleting it:
   ```
   [ -e /etc/codex/requirements.toml ] && sudo cp /etc/codex/requirements.toml /etc/codex/requirements.toml.bak
   sudo mkdir -p /etc/codex/managed-hooks
   sudo cp requirements.toml /etc/codex/requirements.toml
   sudo cp managed-hooks/codex-app-server-activity-status.sh /etc/codex/managed-hooks/codex-app-server-activity-status.sh
   sudo chmod 0644 /etc/codex/requirements.toml
   sudo chmod 0755 /etc/codex/managed-hooks/codex-app-server-activity-status.sh
   ```

2. Start `codex` (plain interactive session is enough — hooks load from the
   same config layer stack regardless of interactive vs. app-server mode).

3. Confirm trust status without triggering anything: run `/hooks` inside
   the CLI. The five hooks from `requirements.toml` should already show as
   trusted/managed — no review prompt, no manual approval step.

4. Submit a prompt, let the agent run a tool call or two, let it finish.
   Then check the status file:
   ```
   cat "${CODEX_HOME:-$HOME/.codex}/app-server-control/codex-app-server-activity-status.yaml"
   stat "${CODEX_HOME:-$HOME/.codex}/app-server-control/codex-app-server-activity-status.yaml"
   ```
   `last-activity` (and the file's mtime) should reflect the most recent
   event; `last-event` should read `PostToolUse` or `Stop` depending on
   timing.

5. End the session (exit codex) and re-check the file — it should be
   **unchanged** from step 4 (`last-event` still `PostToolUse`/`Stop`,
   same mtime). `SessionEnd` deliberately does not touch the file (see
   "Status file" above), so this confirms that exclusion is working, not
   a bug.

6. Clean up: remove only this example's hook script, and either restore
   your backed-up `requirements.toml` or remove it if none existed before
   step 1 — never blanket-`rm -rf` the shared directory/file, since a real
   machine may have unrelated managed hooks or site config there:
   ```
   sudo rm -f /etc/codex/managed-hooks/codex-app-server-activity-status.sh
   if [ -e /etc/codex/requirements.toml.bak ]; then
       sudo mv /etc/codex/requirements.toml.bak /etc/codex/requirements.toml
   else
       sudo rm -f /etc/codex/requirements.toml
   fi
   ```

## Open items / not yet verified

- **Verified live** (2026-09-21): hooks installed via `requirements.toml`
  fired without any interactive trust prompt, and the status file updated
  correctly across `SessionStart` → `Stop` (repeated per-turn) →
  `SessionEnd`, e.g.:
  ```
  status: started
  last-event: SessionStart
  last-session-id: 01a0bfeb-9204-7381-ac94-66d31d5be15e
  last-activity: 2026-09-21T14:31:11Z
  ```
  (shown with the `status` field from the version tested — since removed,
  see above). Confirms the core mechanism works end-to-end: managed-hook
  trust, event delivery, stdin JSON extraction, and file writes are all
  real, not just theoretical.
- **Verified live in `codex app-server` mode specifically** (not just
  interactive `codex`): connected to a real `codex app-server --listen
  unix://...` instance, drove `initialize` → `thread/start` → `turn/start`
  over the socket, and observed `hook/started`/`hook/completed`
  notifications for both `sessionStart` and `userPromptSubmit` completing
  successfully (no errors), with the status file updated accordingly. Note
  `SessionStart` (and other hooks) only fire once an actual *turn* begins —
  `thread/start` alone does not trigger them. See
  `../cli-watcher-test/scripts/codex-app-server/manual-test-scenario.md`
  for the reusable local test setup (a Unix-socket WebSocket client, a
  status-file monitor, and an install script for pointing the managed hooks
  at a specific `CODEX_HOME`).
- The Go side (`timeout/activity_source_codex_app_server_hooks.go`) has been
  rewritten and reads this status file — its `Scan()` decision is based
  entirely on the file's mtime, not the old `/proc`-polling I/O-delta
  approach. `/proc` discovery is still used, but only to find the running
  codex process(es) so their own environment can be read to resolve
  `CODEX_HOME`/`HOME` reliably (see "Discovery consequence" above) — never
  as the activity signal itself.

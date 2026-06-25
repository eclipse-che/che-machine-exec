# CLI Watcher - Prevent Workspace Idling During Long-Running Commands

The CLI Watcher monitors running CLI processes and prevents workspace idling during active development. This is particularly useful in containerized development environments like Eclipse Che where long-running deployments, builds, or interactive sessions shouldn't trigger automatic workspace shutdown.

## How It Works

The watcher periodically scans `/proc` to detect **all user-initiated CLI processes** (processes with TTY from user terminals). When an active process is found, it triggers a callback that resets the workspace idle timeout.

**Key behavior**:
- **ALL user processes are watched** automatically (no explicit configuration needed)
- **Configured commands** (`watchedCommands`) allow you to override auto-detection behavior
- **Unconfigured commands** are intelligently classified as interactive or work processes after grace period

## Upgrading from Previous Versions

**⚠️ BREAKING BEHAVIORAL CHANGE:** The CLI Watcher now watches **ALL user-initiated terminal processes** by default, not just those explicitly listed in `watchedCommands`.

### What Changed

**Before (old behavior):**
- Only commands listed in `watchedCommands` were monitored
- Other processes were completely ignored
- Only `tail` was globally excluded

**After (new behavior):**
- **ALL user processes with TTY are monitored automatically**
- `watchedCommands` now **overrides auto-detection** for specific commands (not required to enable watching)
- `tail`, `watch`, `top`, `htop` are now **always ignored** (expanded exclusion list)

### Impact on Your Workspace

1. **Workspaces may stay active longer** - processes that were previously ignored (shells, scripts, REPLs) now prevent idling
2. **Commands in `watchedCommands` may behave differently**:
   - If you configured `watch`, `top`, or `htop` → now ignored with a warning
   - If you only listed specific commands → other user processes are now also monitored
3. **Auto-detection may differ from your expectations** - interactive processes (vim, python REPL) only prevent idling when actively used

### Migration Steps

**If you have an existing `.noidle` configuration:**

1. **Review your current `watchedCommands` list**
   ```yaml
   # Old config - only these were watched
   watchedCommands:
     - helm
     - kubectl
     - watch  # ⚠️ Now globally ignored!
   ```

2. **Understand the new behavior**:
   - All user terminal processes are now watched (helm, kubectl, vim, bash scripts, etc.)
   - `watchedCommands` is now for **overriding** auto-detection, not enabling watching
   - Remove `watch`, `top`, `htop` from your config (they're always ignored)

3. **Option A: Embrace auto-detection** (recommended for most users)
   ```yaml
   # Minimal config - let auto-detection handle everything
   enabled: true
   ```
   The watcher will automatically distinguish interactive (vim, REPLs) from work processes (builds, deploys).

4. **Option B: Restrict to specific commands only**
   ```yaml
   enabled: true
   
   # Override auto-detection for specific commands
   watchedCommands:
     - helm
     - kubectl
   
   # Add all other commands you DON'T want watched
   ignoredCommands:
     - bash
     - sh
     - python3
     - node
     # ... any other commands you want to ignore
   ```

5. **Test in a non-production workspace first** - verify idle timeout behavior matches your expectations

### Examples

**Example 1: Old config watching only deployments**
```yaml
# Before
watchedCommands:
  - helm
  - kubectl
  - odo
```

**After migration (Option A - auto-detect):**
```yaml
# After - auto-detection handles everything
enabled: true

# Optional: Force these to always prevent idling (skip auto-detection)
watchedCommands:
  - helm
  - kubectl
  - odo
```

**Example 2: Old config with globally-excluded commands**
```yaml
# Before
watchedCommands:
  - watch  # Monitoring logs
  - kubectl
```

**After migration:**
```yaml
# After - remove 'watch' (now globally ignored)
enabled: true

watchedCommands:
  - kubectl  # Keep kubectl if you want to override auto-detection
  
# WARNING will be logged:
# "You configured [watch] in watchedCommands, but these are globally excluded"
```

### Verification

After updating your configuration:

1. Check logs for warnings about globally-excluded commands
2. Monitor workspace idle timeout behavior
3. Use `LOG_LEVEL=debug` to see which processes are detected and classified
4. Refer to the [Testing](#testing) section for validation scenarios

### Rollback

If the new behavior doesn't suit your workflow:

1. Use `ignoredCommands` to exclude unwanted processes
2. Set explicit `interactive` modes in `watchedCommands` to override auto-detection
3. Contact your platform administrator if workspace idle policies need adjustment

## Configuration

### Configuration File Requirement

**IMPORTANT**: The CLI Watcher requires a configuration file to enable watching. Without a config file, NO processes will be watched and workspaces will idle normally.

**Minimum Required Configuration**:
```yaml
enabled: true
```

That's it! With just this one line:
- ✅ ALL user processes are automatically watched
- ✅ Interactive vs. work processes are auto-detected
- ✅ All settings use smart defaults (grace period: 5min, activity window: 25min, max age: 6h)
- ✅ Passive monitoring tools (tail, watch, top, htop) are automatically ignored

### Configuration File Locations

The CLI Watcher looks for a `.noidle` configuration file in the following order:

1. **Explicit override**: Set via `CLI_WATCHER_CONFIG` environment variable
2. **Project directory**: Search upward from `$PROJECT_SOURCE` to `$PROJECTS_ROOT` for `.noidle`
3. **Home directory**: Fallback to `$HOME/.noidle`

If no config file is found, the watcher runs but does NOT prevent idling (waits for config to appear).

### Basic Configuration (Backward Compatible)

```yaml
enabled: true
checkPeriod: 30
watchedCommands:
  - helm
  - odo
  - kubectl
```

**Note**: The `watchedCommands` list is **optional** - it's used to **override** auto-detection, not to enable watching. Without this list, ALL user processes are still watched with smart defaults.

This simple string format forces listed commands to be **non-interactive** - they always prevent idling when running, regardless of whether they're actively doing work.

### Advanced Configuration - Override Auto-Detection (Optional)

**⚠️ You probably don't need this section!** The CLI Watcher auto-detects process types correctly in most cases.

**Only override auto-detection when:**
- Auto-detection misclassifies a specific command
- You need to completely ignore a command that shouldn't be watched
- You have special requirements or are debugging

**Two escape hatches available:**

#### 1. **`watchedCommands`** - Fix misclassification (process still watched, mode corrected)
```yaml
watchedCommands:
  - name: myBuildTool
    interactive: false  # Auto-detected as interactive, but it's actually a build → force non-interactive
  
  - name: myREPL
    interactive: true   # Auto-detected as work process, but it's interactive → force interactive
```

#### 2. **`ignoredCommands`** - Stop watching entirely (process never prevents idling)
```yaml
ignoredCommands:
  - weirdSystemDaemon   # Has TTY but shouldn't be watched at all
  - debugTool           # Picked up by auto-detection but irrelevant
```

**Warning:** Misconfiguring can break workspace idling:
- Setting `sleep` as `interactive: true` → Long-running tasks interrupted ❌
- Setting `vim` as `interactive: false` → Idle editor prevents idling forever ❌
- Over-using `ignoredCommands` → Important work not tracked ❌

#### Full example with time settings:

```yaml
enabled: true
checkPeriod: 30                     # How often to check for active processes (default: 60 seconds)
activityWindow: 25m                 # How long to wait for activity from interactive processes (default: 25m)
gracePeriod: 5m                     # All processes prevent idling when this young (default: 5m)

# Optional: Override auto-detection for specific commands
watchedCommands:
  # Force long-running commands to always prevent idling (skip auto-detection)
  - helm
  - kubectl
  
  # Force interactive CLIs to always check for user input activity
  - name: claude
    interactive: true              # Force interactive (always check for user input)
  
  # Let auto-detection decide (foreground + TTY read → interactive)
  - name: vim
    interactive: auto              # Auto-detect (same as unconfigured, but explicit)
  
  # Force non-interactive mode (always prevent idling)
  - name: npm
    interactive: false             # Force non-interactive (always prevent idling)

# Optional: Completely ignore certain commands
ignoredCommands:
  - systemDaemon
  - debugHelper
```

**Remember**: 
- **Unconfigured commands**: Auto-detected with `interactive: auto` behavior after grace period
- **`watchedCommands` entries**: Use your explicit `interactive` setting instead of auto-detection
- **`ignoredCommands` entries**: Never watched, never prevent idling (like `tail`, `watch`, `top`, `htop`)

## Interactive Mode Options

The `interactive` field controls how the watcher determines if a process should prevent idling:

| Mode | Values | Behavior |
|------|--------|----------|
| **Non-interactive** (default) | `false`, `no`, or omit field | Always prevent idling when the process is running. Best for build tools, deployment commands, etc. |
| **Interactive** | `true`, `yes` | Force activity checking. Only prevent idling if process has recent user input (TTY access time). Best for interactive CLIs like editors, REPLs, or AI assistants. |
| **Auto-detect** | `auto` | Detect interactivity by checking if process is foreground AND has read from TTY. If yes → check activity; if no → always prevent idling. |

## ForceWatch Override Option

**⚠️ USE WITH EXTREME CAUTION ⚠️**

The `forceWatch` field allows you to override the always-ignored commands list for specific commands. This should **rarely be needed** as always-ignored commands (`tail`, `watch`, `top`, `htop`) are passive monitoring tools that don't indicate active work.

| Mode | Values | Behavior |
|------|--------|----------|
| **Respect always-ignored** (default) | `false`, `no`, or omit field | Commands in the always-ignored list will never prevent idling, even if explicitly configured |
| **Override always-ignored** | `true`, `yes` | Force this specific command to be watched, even if it's normally always-ignored |

### When to Use ForceWatch

**Valid use cases** (rare):
- Custom scripts named `watch`, `top`, etc. that actually perform work
- Debugging workspace idle behavior with monitoring tools
- Specialized monitoring tools that indicate active development

**Invalid use cases** (common mistakes):
- Making `tail -f logfile` prevent idling → Logs aren't active work
- Making `top` prevent idling → Process monitoring isn't active work  
- Making `watch kubectl get pods` prevent idling → Passive monitoring isn't active work

### Configuration Example

```yaml
watchedCommands:
  # WRONG: Don't do this for actual monitoring tools
  - name: watch
    forceWatch: true               # ❌ Bad - passive monitoring shouldn't prevent idling
  
  # VALID: Custom work script that happens to be named 'watch'
  - name: watch
    interactive: false
    forceWatch: true               # ✅ OK - custom script that actually does work
```

### Accepted Values

- `true`, `yes` → Override always-ignored list (monitor this command)
- `false`, `no` → Respect always-ignored list (default behavior) 
- Omit field → Same as `false` (respect always-ignored list)

### Warning Messages

When you configure always-ignored commands without `forceWatch: true`, you'll see:

```
WARNING: You configured [watch, top] in watchedCommands, but these are globally excluded (always ignored)
```

This usually means you should **remove those commands from your config**, not add `forceWatch: true`.

### Default Values

The CLI Watcher uses **smart defaults** that adapt to your workspace idle timeout when possible:

#### Fixed Defaults (always the same):
- `interactive`: `no` (backward compatible - always prevent idling)
- `maxProcessAge`: `6h` (safety limit to prevent indefinite idling prevention)
- `checkPeriod`: `60` seconds

#### Adaptive Defaults (calculated from workspace idle timeout):

When **workspace idle timeout is available** (e.g., 30 minutes):
- `gracePeriod`: Smaller of `5m` or `15%` of idle timeout
- `activityWindow`: `idle timeout - gracePeriod - buffer`
  - Buffer is smaller of `5m` or `20%` of idle timeout
  - Example: 30m idle → 5m grace → 20m activity window

When **workspace idle timeout is unavailable or disabled** (`-1`):
- `gracePeriod`: `5m`
- `activityWindow`: `25m`

#### Minimum Values (enforced even for very short idle timeouts):
- `gracePeriod`: At least `1m`
- `activityWindow`: At least `2m`
- `checkPeriod`: At least `10` seconds

**User-specified values always take priority** - smart defaults only apply to unspecified fields.

### How It Works

1. **User Process Detection**: Only watches processes with TTY that are children of user terminals (filters out system processes automatically)
2. **Always-Ignored Check**: Skips passive monitoring tools (`tail`, `watch`, `top`, `htop`) 
3. **Safety Limit**: Processes older than `maxProcessAge` (default 6h) don't prevent idling - protects against hung/forgotten/misconfigured processes
4. **Grace Period**: All user processes < 5 minutes old prevent idling (gives builds time to start)
5. **Interactive Detection** (after grace period):
   - **Configured commands**: Use their `interactive` setting
   - **Unconfigured commands**: Auto-detect (foreground + has read from TTY → interactive, otherwise → work process)
6. **Activity Checking**: Interactive processes only prevent idling if user input detected within `activityWindow`

**Note on time formats**: All time settings (`checkPeriod`, `activityWindow`, `gracePeriod`, `maxProcessAge`) accept:
- Duration strings: `6h`, `30m`, `21600s`, `6h30m`
- Plain integers: `21600` (treated as seconds)
- Invalid values log a warning and use the calculated or fixed default

### Configuration Validation

The CLI Watcher validates your configuration and warns about potential issues **without changing your specified values**:

**Warnings you might see**:
```
WARN: activityWindow (35m) exceeds workspace idle timeout (30m), may not work as expected
WARN: gracePeriod (25m) is very close to workspace idle timeout (30m)
WARN: activityWindow (3m) is less than gracePeriod (5m), interactive processes may not be detected correctly
WARN: checkPeriod (10m) may be too long for activityWindow (15m), activity might not be detected in time
WARN: Workspace idle timeout (8m) is very short, using minimum activity window (2m)
WARN: Both 'checkPeriod' (30s) and deprecated 'checkPeriodSeconds' (45) are set - using 'checkPeriod' value
```

These warnings help you identify misconfigurations but **your specified values are always respected**.

## Activity Detection

### Interactive Process Detection (`auto` mode)

A process is considered interactive if:
1. It's in the **foreground process group** of its TTY, AND
2. Either:
   - Currently waiting on `read` (from wchan), OR
   - Has **ever read from its TTY** (TTY access time is after process start time)

This detects:
- ✅ **Interactive**: `vim`, `python3` (REPL), `node` (REPL), `less` → Check for recent user input
- ✅ **Work**: `./compile.sh`, `go build`, `npm run build` → Always prevent idling

### Activity Monitoring

For interactive processes, recent activity is detected by monitoring **TTY Access Time (Atime)**:
- Atime updates when the TTY is **read from** (user types)
- Atime does NOT update from output (program writes)
- Process prevents idling if Atime is within the `activityWindow`

**Examples**:
- `claude` actively used → Prevents idling ✅
- `claude` idle for 30 minutes → Doesn't prevent idling ✅  
- `vim` with active typing → Prevents idling ✅
- `vim` left open but untouched → Doesn't prevent idling after activity window ✅
- `go build` running → Always prevents idling ✅
- Background `node` (VS Code) → Skipped (system process) ✅

## Always-Ignored Commands

The following commands are globally excluded and will NEVER prevent workspace idling, even if explicitly configured or detected as user processes:

- `tail` - Log file monitoring
- `watch` - Repeated command execution monitoring  
- `top` - Process monitoring
- `htop` - Enhanced process monitoring

These are passive monitoring tools that don't indicate active work.

## Use Cases

### Long-Running Deployments (Override Auto-Detection)

```yaml
# Optional: Force these to always prevent idling (skip auto-detection)
watchedCommands:
  - helm
  - kubectl
  - odo
```

These always prevent idling during deployment operations, even if auto-detection would classify them differently.

### Interactive Development with AI (Custom Activity Window)

```yaml
activityWindow: 300  # Override global default (25min) to 5 minutes

# Optional: Force claude to be interactive (it would likely auto-detect correctly anyway)
watchedCommands:
  - name: claude
    interactive: true
```

Workspace stays alive during active Claude Code sessions, but idles if left idle for 5+ minutes.

**Note**: Without `watchedCommands`, claude would still be watched and likely auto-detected as interactive. This config just makes it explicit and adjusts the activity window.

### Mixed Workload - Fine-Tuned Control

```yaml
activityWindow: 1500         # Global default: 25 minutes
gracePeriod: 300             # All processes: 5 minute grace period

# Override auto-detection for specific commands only when needed
watchedCommands:
  # Force deployment tools to always prevent idling
  - helm
  - kubectl
  
  # Force claude interactive with custom activity window
  - name: claude
    interactive: true
  
  # Let vim auto-detect (would likely work the same without this entry)
  - name: vim
    interactive: auto
  
  # Force npm to always prevent idling (in case auto-detection misclassifies)
  - name: npm
    interactive: false
```

**Remember**: All user processes are watched. This config just overrides auto-detection for specific commands.

## Environment Variables

- `CLI_WATCHER_CONFIG`: Override config file path
- `PROJECT_SOURCE`: Starting point for upward `.noidle` search
- `PROJECTS_ROOT`: Stop point for upward `.noidle` search (defaults to `/`)

## Logging

The watcher logs its activity at INFO level:

```
CLI Watcher: Started
CLI Watcher: Config reloaded from /home/user/.noidle
CLI Watcher:   Watching ALL user processes with 3 explicit override(s):
CLI Watcher:     - helm (mode: non-interactive (always active))
CLI Watcher:     - claude (mode: interactive (activity check))
CLI Watcher:     - vim (mode: auto-detect TTY)
CLI Watcher:   Detection period: 30 seconds
CLI Watcher:   Activity window: 1500 seconds
CLI Watcher:   Grace period: 300 seconds
CLI Watcher: Detected CLI command: helm — reporting activity tick
```

**Note**: The log shows configured overrides, but ALL user processes are monitored.

Use DEBUG level for detailed process scanning:

```
CLI Watcher: Process claude (PID 12345) has recent activity
```

## Migration Guide

### From Simple String List

**Before:**
```yaml
watchedCommands:
  - helm
  - claude
```

**After (to enable activity checking for claude):**
```yaml
activityWindow: 300  # Set global activity window

watchedCommands:
  - helm  # Still simple string - always active (or auto-detected after grace period)
  - name: claude
    interactive: true  # Force interactive mode for explicit control
```

**Note**: With the new implementation, even unconfigured commands are automatically watched and intelligently classified as interactive or work processes after the grace period. Explicit configuration is only needed to override the auto-detection.

## Deployment Requirements

### Filesystem Access Time (atime) Dependency

**CRITICAL**: Interactive process detection depends on filesystem access time (atime) updates for TTY devices. 

**Problem**: If containers or systems run on filesystems mounted with `noatime` or `relatime`:
- TTY access times won't update when users interact with terminals
- Interactive processes will appear "idle" even when actively used
- Workspaces may shutdown unexpectedly during active terminal sessions

**Verification**: Check if `/dev/pts` is mounted with atime support:
```bash
# Check mount options for devpts filesystem
mount | grep devpts

# Should NOT show 'noatime' - example of GOOD output:
devpts on /dev/pts type devpts (rw,nosuid,noexec,relatime,gid=5,mode=620,ptmxmode=000)

# Example of PROBLEMATIC output:
devpts on /dev/pts type devpts (rw,nosuid,noexec,noatime,gid=5,mode=620,ptmxmode=000)
```

**Fix for Problematic Systems**:
- **Container environments**: Ensure devpts is mounted without `noatime`
- **Kubernetes**: Use appropriate volume mounts or security policies
- **Manual fix**: Remount devpts with atime support:
  ```bash
  sudo mount -o remount,relatime /dev/pts
  ```

**Robust Fallback Detection**: When atime is unavailable or unreliable, the CLI Watcher automatically uses sophisticated alternative detection methods:

1. **Process State Analysis** - Analyzes if process is sleeping (waiting for input)
2. **Enhanced Wait Channel Analysis** - Detects specific input-waiting syscalls:
   - `poll_schedule_timeout` - polling with timeout (interactive pattern)  
   - `pipe_wait` - waiting on pipe input
   - `unix_stream_read_generic` - reading from socket
   - `select`, `ep_poll` - event-driven input waiting
3. **File Descriptor Activity** - Monitors recent TTY file descriptor usage

**Scoring System**: Multiple detection signals are combined with a scoring threshold to reliably identify interactive processes, even without atime support.

**Automatic Fallback**: No configuration needed - the system automatically detects atime issues and switches to alternative methods with debug logging.

**Symptoms Indicating Fallback Mode**:
- Debug logs show: "TTY atime for PID X unavailable or unreliable, using fallback detection"
- Debug logs show: "PID X detected as interactive via fallback (score: N, wchan: Y)"

**Result**: Interactive detection remains highly reliable even on `noatime` filesystems, though atime support is still preferred for optimal performance.

## Testing

### Monitoring Activity Ticks

To watch CLI watcher activity ticks in real-time in a DevWorkspace environment, open a terminal and run:

```bash
tail -f /checode/entrypoint-logs.txt
```

This will show continuous log output including:
- CLI Watcher startup messages
- Config reload events
- Detected CLI commands and activity ticks
- Process scanning debug messages (if `LOG_LEVEL=debug`)

Example output:
```
CLI Watcher: Started
CLI Watcher: Config reloaded from /projects/.noidle
CLI Watcher:   Watching 3 command(s):
CLI Watcher:     - sleep (mode: non-interactive (always active))
CLI Watcher:     - vi (mode: auto-detect TTY, activity window: 120s)
CLI Watcher:   Detection period is 15 seconds
CLI Watcher: Detected CLI command: sleep — reporting activity tick
```

### Test Scenarios

#### Available Commands in UBI9 Go-Toolset

First, verify what commands are available in your dev container:

```bash
# Check for interactive tools
which vi vim nano less more top python python3 bash sh 2>&1 | grep -v "not found"

# Check for background/non-interactive tools
which sleep ping curl wget nc watch yes 2>&1 | grep -v "not found"
```

Typically available:
- **Interactive**: `vi`, `less`, `more`, `bash`, `sh`
- **Non-interactive**: `sleep`, `ping`, `curl`, `wget`, `yes`

#### Scenario 1: Non-Interactive Long-Running Commands

**Test Config** (`/tmp/.noidle.test`):
```yaml
enabled: true
checkPeriod: 15

watchedCommands:
  - sleep
  - ping
```

**Test Steps**:
```bash
export CLI_WATCHER_CONFIG=/tmp/.noidle.test

# Start a long-running background process (no TTY)
sleep 1800 &

# Watch logs in another terminal
tail -f /checode/entrypoint-logs.txt

# Expected: "Detected CLI command: sleep — reporting activity tick" every 15s
```

**Cleanup**: `pkill sleep`

#### Scenario 2: Interactive Command with Activity Tracking

**Test Config** (`/tmp/.noidle.interactive`):
```yaml
enabled: true
checkPeriod: 15
activityWindow: 120  # 2 minutes for easy testing

watchedCommands:
  - name: vi
    interactive: auto
```

**Test Steps**:
```bash
export CLI_WATCHER_CONFIG=/tmp/.noidle.interactive

# Terminal 1: Watch logs
tail -f /checode/entrypoint-logs.txt

# Terminal 2: Open vi interactively
vi /tmp/testfile.txt

# Type occasionally and watch for activity ticks
# Stop typing for 3+ minutes - activity ticks should stop
```

#### Scenario 3: Auto-Detection of Interactive vs Work Processes

**Purpose**: Verify that the watcher correctly distinguishes between interactive CLIs (vim, REPLs) and work processes (builds, scripts) without explicit configuration.

**Test Config** (`/tmp/.noidle.autodetect`):
```yaml
enabled: true
checkPeriod: 15
activityWindow: 120  # 2 minutes for easy testing
gracePeriod: 1m      # Short grace period for faster testing

# No watchedCommands - everything is auto-detected!
```

**Test Steps**:
```bash
export CLI_WATCHER_CONFIG=/tmp/.noidle.autodetect

# Terminal 1: Watch logs
tail -f /checode/entrypoint-logs.txt

# Terminal 2: Test interactive process (should require activity)
vi /tmp/test.txt
# Type something, watch for activity tick
# Stop typing for 3+ minutes - ticks should stop

# Terminal 3: Test work process (should always prevent idling)
sleep 300
# Should see activity ticks every 15 seconds even without user interaction
```

**Expected behavior**:
- `vi` detected as **interactive** (foreground + reads from TTY) → Only ticks when typing
- `sleep` detected as **work process** (not interactive) → Always ticks while running
- Grace period (1min): Both prevent idling immediately when started

**Debugging**: Set `LOG_LEVEL=debug` to see detailed detection:
```
CLI Watcher: Process vi (PID 12345) auto-detected as interactive
CLI Watcher: Process vi (PID 12345) has recent activity
CLI Watcher: Process sleep (PID 12346) auto-detected as work process
```

#### Scenario 4: Excluded Commands (Negative Test)

**Test Config** (`/tmp/.noidle.exclusion`):
```yaml
enabled: true
checkPeriod: 10

watchedCommands:
  - tail   # Globally excluded
  - sleep
```

**Expected**: Logs show `tail` is skipped:
```
CLI Watcher:   WARNING: You configured [tail] in watchedCommands, but these are globally excluded (always ignored)
CLI Watcher:   Watching ALL user processes with 1 explicit override(s):
CLI Watcher:     - sleep (mode: non-interactive (always active))
```

### Debugging

Enable debug logging for detailed process scanning:

```bash
export LOG_LEVEL=debug
```

This shows:
```
CLI Watcher: Process vi (PID 12345) has recent activity
CLI Watcher: Process vi (PID 12345) found but no recent activity
```

### Quick Test Setup

Create a test configuration file:

```yaml
# /tmp/.noidle.quicktest
enabled: true
checkPeriod: 10
activityWindow: 120  # 2 minutes for easy testing
watchedCommands:
  - sleep
  - name: vi
    interactive: auto
```

**Testing Steps**:

1. **Stop existing server** (use devfile command: `stop-exec-server`)

2. **Start server with test config**:
   ```bash
   export CLI_WATCHER_CONFIG=/tmp/.noidle.quicktest
   ```
   Then run devfile command: `start-exec-server`

3. **Monitor activity ticks**:
   ```bash
   tail -f /checode/entrypoint-logs.txt
   ```

4. **Start test processes**:
   ```bash
   # Terminal 1: Non-interactive (always active)
   sleep 600 &
   
   # Terminal 2: Interactive (activity tracked)
   vi /tmp/test.txt
   ```

5. **Watch logs** - you should see:
   ```
   CLI Watcher: Config reloaded from /tmp/.noidle.quicktest
   CLI Watcher: Detected CLI command: sleep — reporting activity tick
   ```

6. **Cleanup**: Stop the server using `stop-exec-server` command

### Expected Test Results

| Command | Mode | Has TTY? | Active I/O? | Prevents Idling? |
|---------|------|----------|-------------|------------------|
| `sleep 3600 &` | default (no) | No | N/A | ✅ Always |
| `vi file.txt` (typing) | auto | Yes | Yes | ✅ Yes |
| `vi file.txt` (idle) | auto | Yes | No | ❌ No (after window) |
| `tail -f file` | any | any | any | ❌ Never (excluded) |

### Developer Testing

For contributors working on the CLI Watcher code:

**Run unit tests:**
```bash
go test ./timeout -v
```

**Check test coverage:**
```bash
go test ./timeout -cover
```

**Note on test coverage:**
- **Unit tests cover pure functions** (parsing, configuration, validation, defaults, YAML unmarshaling)
- **Core detection logic is untested** (process tree walking, TTY analysis, interactive process detection, `isWatchedProcessRunning`, `isUserInitiatedProcess`)

**Why core detection logic requires manual testing:**
- Requires real `/proc` filesystem (not available in standard Go test environment)
- Needs multiple process scenarios (shells, interactive CLIs, work processes, TTY states)
- Depends on actual system process behavior and file descriptor states

**For detection logic verification**: Use the manual test scenarios described above with real processes in a containerized development environment.

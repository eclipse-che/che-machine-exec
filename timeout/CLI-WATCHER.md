# CLI Watcher - Prevent Workspace Idling During Long-Running Commands

The CLI Watcher monitors running CLI processes and prevents workspace idling during active development. This is particularly useful in containerized development environments like Eclipse Che where long-running deployments, builds, or interactive sessions shouldn't trigger automatic workspace shutdown.

## How It Works

The watcher periodically scans `/proc` to detect **all user-initiated CLI processes** (processes with TTY from user terminals). When an active process is found, it triggers a callback that resets the workspace idle timeout.

**Key behavior**:
- **ALL user processes are watched** automatically (no explicit configuration needed)
- **Configured commands** (`watchedCommands`) allow you to override auto-detection behavior
- **Unconfigured commands** are intelligently classified as interactive or work processes after grace period

## Configuration

CLI Watcher configuration has three layers:

1. **Administrator configuration** (CheCluster CR) - cluster-wide policy via `spec.devEnvironments.cliActivityTracker` fields, propagated by the Che operator as environment variables to all workspace containers
2. **Administrator configuration** (environment variables / ConfigMap) - namespace-level or per-workspace overrides via `CLI_ACTIVITY_TRACKER_*` env vars
3. **User configuration** (`.noidle` file) - per-project or workspace-wide tuning within admin-defined bounds

### Configuration Precedence

```
Environment variables (admin)  >  .noidle file (user, stricter only)  >  Adaptive defaults
```

- **`enabled`**: Always controlled by the `CLI_ACTIVITY_TRACKER_ENABLED` env var or its default. The `.noidle` `enabled` field is **deprecated and ignored**.
- **Timing params** (`checkPeriod`, `activityWindow`, `gracePeriod`, `maxProcessAge`): Admin env vars set ceilings. Users can make values **stricter** (shorter) via `.noidle`, but **cannot loosen** (lengthen) beyond the admin ceiling.
- **`watchedCommands` / `ignoredCommands`**: User-only (`.noidle` file). Not configurable via env vars.
- **`verbose`**: Admin/env-only (`CLI_ACTIVITY_TRACKER_VERBOSE`). Not configurable via `.noidle` — it's an operational logging toggle, not a per-project idling policy.

### Administrator Configuration (Environment Variables)

Cluster and DevWorkspace administrators control CLI Watcher behavior through environment variables injected into the che-machine-exec container. These are set at pod creation time and are immutable for the container lifetime.

#### Environment Variables

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CLI_ACTIVITY_TRACKER_ENABLED` | boolean | `false` | Master switch. Set to `true` to enable CLI Watcher cluster-wide. |
| `CLI_ACTIVITY_TRACKER_CHECK_PERIOD` | duration | `60s` | How often to scan `/proc` for active processes. |
| `CLI_ACTIVITY_TRACKER_ACTIVITY_WINDOW` | duration | adaptive (see [Adaptive Defaults](#adaptive-defaults-calculated-from-workspace-idle-timeout)) | How long to wait for input from interactive processes before considering them idle. |
| `CLI_ACTIVITY_TRACKER_GRACE_PERIOD` | duration | adaptive (see [Adaptive Defaults](#adaptive-defaults-calculated-from-workspace-idle-timeout)) | All processes unconditionally prevent idling when younger than this. |
| `CLI_ACTIVITY_TRACKER_MAX_PROCESS_AGE` | duration | `6h` | Safety limit. Processes older than this stop preventing idling. |
| `CLI_ACTIVITY_TRACKER_VERBOSE` | boolean | `false` | Promotes activity-detection details (which process was detected, why it does or doesn't prevent idling) from Debug to Info level, without needing `LOG_LEVEL=debug` for the whole application. |

**Duration format**: Accepts Go duration strings (`30s`, `5m`, `1h`, `1h30m`) or plain integers (treated as seconds).

**Boolean format**: Accepts `true`, `false`, `1`, `0`, `t`, `f`, `TRUE`, `FALSE`, `True`, `False`, `T`, `F`.

#### Additional Environment Variables

| Variable | Description |
|----------|-------------|
| `CLI_ACTIVITY_TRACKER_CONFIG` | Override `.noidle` config file path (for user-level config) |
| `PROJECT_SOURCE` | Starting point for upward `.noidle` search |
| `PROJECTS_ROOT` | Stop point for upward `.noidle` search (defaults to `/`) |

#### Configuring via CheCluster Custom Resource (Recommended)

The recommended way to configure CLI Watcher cluster-wide is through the CheCluster custom resource. The Che operator reads these fields and propagates them as `CLI_ACTIVITY_TRACKER_*` environment variables to all workspace containers via the `che-user-settings` ConfigMap.

##### CheCluster CR Fields

| Field (under `spec.devEnvironments.cliActivityTracker`) | Type | Default | Maps to env var |
|---|---|---|---|
| `enabled` | bool | `false` | `CLI_ACTIVITY_TRACKER_ENABLED` |
| `secondsOfCheckPeriod` | int32 | not set (adaptive) | `CLI_ACTIVITY_TRACKER_CHECK_PERIOD` |
| `secondsOfActivityWindow` | int32 | not set (adaptive) | `CLI_ACTIVITY_TRACKER_ACTIVITY_WINDOW` |
| `secondsOfGracePeriod` | int32 | not set (adaptive) | `CLI_ACTIVITY_TRACKER_GRACE_PERIOD` |
| `secondsOfMaxProcessAge` | int32 | not set (`6h`) | `CLI_ACTIVITY_TRACKER_MAX_PROCESS_AGE` |

Timing fields are in **seconds**. Set to `-1` to use the default value calculated by che-machine-exec (see [Adaptive Defaults](#adaptive-defaults-calculated-from-workspace-idle-timeout)). When a timing field is not set (or set to `-1`), the corresponding env var is not written to the ConfigMap, and che-machine-exec uses its adaptive defaults.

##### Full Example

Save as `cli-watcher-config.yaml`:

```yaml
apiVersion: org.eclipse.che/v2
kind: CheCluster
metadata:
  name: eclipse-che
  namespace: eclipse-che
spec:
  devEnvironments:
    cliActivityTracker:
      enabled: true
      secondsOfCheckPeriod: 60
      secondsOfActivityWindow: 900    # 15 minutes
      secondsOfGracePeriod: 180       # 3 minutes
      secondsOfMaxProcessAge: 14400   # 4 hours
```

Apply with:

```bash
kubectl apply -f cli-watcher-config.yaml
```

##### Minimal Example (enable with adaptive defaults)

Save as `cli-watcher-enable.yaml`:

```yaml
apiVersion: org.eclipse.che/v2
kind: CheCluster
metadata:
  name: eclipse-che
  namespace: eclipse-che
spec:
  devEnvironments:
    cliActivityTracker:
      enabled: true
```

Apply with:

```bash
kubectl apply -f cli-watcher-enable.yaml
```

##### Quick Changes with `kubectl patch`

For one-off changes without a file:

```bash
# Enable CLI Watcher with adaptive defaults
kubectl patch checluster/eclipse-che -n eclipse-che --type=merge \
  -p '{"spec":{"devEnvironments":{"cliActivityTracker":{"enabled":true}}}}'

# Enable with custom timing
kubectl patch checluster/eclipse-che -n eclipse-che --type=merge \
  -p '{"spec":{"devEnvironments":{"cliActivityTracker":{"enabled":true,"secondsOfActivityWindow":900,"secondsOfGracePeriod":180}}}}'

# Disable CLI Watcher
kubectl patch checluster/eclipse-che -n eclipse-che --type=merge \
  -p '{"spec":{"devEnvironments":{"cliActivityTracker":{"enabled":false}}}}'
```

##### Important Notes

- **Scope**: Cluster-wide — the operator propagates these values to all user namespaces automatically.
- **Restart required**: Changes to the CheCluster CR require a workspace restart (stop and start) to take effect, since environment variables are set at pod creation time.
- **Do not mix with custom ConfigMaps**: If you configure CLI Watcher via the CheCluster CR, do not also set the same `CLI_ACTIVITY_TRACKER_*` keys in a custom ConfigMap with `controller.devfile.io/mount-to-devworkspace` label. Both ConfigMaps will be mounted, and the effective value depends on unpredictable mount order.

#### Configuring via Kubernetes ConfigMap

**Note**: If the Che operator is deployed and the CheCluster CR includes `cliActivityTracker` fields, the [CheCluster CR approach](#configuring-via-checluster-custom-resource-recommended) is preferred. Use the ConfigMap approach below for environments without the Che operator, or for per-namespace overrides that differ from the cluster-wide CheCluster CR settings (using different, non-overlapping env var keys only).

To inject CLI Watcher env vars into workspace containers, create a labeled ConfigMap in the **user's namespace**. The env vars are mounted into **all DevWorkspace containers** (including the che-machine-exec sidecar).

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cli-watcher-config
  namespace: <user-namespace>
  labels:
    controller.devfile.io/mount-to-devworkspace: "true"
    controller.devfile.io/watch-configmap: "true"
  annotations:
    controller.devfile.io/mount-as: env
    controller.devfile.io/mount-on-start: "true"
data:
  CLI_ACTIVITY_TRACKER_ENABLED: "true"
  CLI_ACTIVITY_TRACKER_ACTIVITY_WINDOW: "15m"
  CLI_ACTIVITY_TRACKER_GRACE_PERIOD: "3m"
  CLI_ACTIVITY_TRACKER_MAX_PROCESS_AGE: "4h"
```

**Minimal admin configuration** (enable with all defaults):

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cli-watcher-config
  namespace: <user-namespace>
  labels:
    controller.devfile.io/mount-to-devworkspace: "true"
    controller.devfile.io/watch-configmap: "true"
  annotations:
    controller.devfile.io/mount-as: env
    controller.devfile.io/mount-on-start: "true"
data:
  CLI_ACTIVITY_TRACKER_ENABLED: "true"
```

**Important notes**:

- **Scope**: The ConfigMap applies to all workspaces in the namespace where it is created. In Eclipse Che, each user has their own namespace — a ConfigMap created in one user's namespace only affects that user's workspaces. To enforce a cluster-wide policy, an administrator must create the ConfigMap in every user's namespace.
- **Restart behavior**: The `controller.devfile.io/mount-on-start` annotation (included above) ensures the ConfigMap is only mounted when a workspace starts. Without it, creating or updating a ConfigMap with the `controller.devfile.io/mount-to-devworkspace` label **restarts all running workspaces** in that namespace.
- **Selective targeting**: Use `controller.devfile.io/mount-to-devworkspace-include` or `controller.devfile.io/mount-to-devworkspace-exclude` annotations with comma-separated workspace name patterns to target specific workspaces.

See the Eclipse Che documentation for details:
- [Mounting ConfigMaps](https://eclipse.dev/che/docs/stable/end-user-guide/mounting-configmaps/)
- [Customizing Cloud Development Environments](https://che.eclipseprojects.io/2024/02/05/@mario.loriedo-cde-customization.html)

#### Configuring via DevWorkspace Attribute

To configure CLI Watcher for a **single workspace**, use the `workspaceEnv` attribute in the DevWorkspace spec. This injects env vars into all containers in that workspace:

```yaml
apiVersion: workspace.devfile.io/v1alpha2
kind: DevWorkspace
metadata:
  name: my-workspace
spec:
  template:
    attributes:
      workspaceEnv:
        - name: CLI_ACTIVITY_TRACKER_ENABLED
          value: "true"
        - name: CLI_ACTIVITY_TRACKER_ACTIVITY_WINDOW
          value: "15m"
```

**Scope**: Per-workspace only. For namespace-wide or cluster-wide configuration, use the [ConfigMap approach](#configuring-via-kubernetes-configmap) instead.

#### Ceiling Enforcement

When an admin sets a timing env var, it becomes a **ceiling** that users cannot exceed via `.noidle`:

- If a user sets `activityWindow: 30m` in `.noidle` but the admin set `CLI_ACTIVITY_TRACKER_ACTIVITY_WINDOW=15m`, the value is **clamped to 15m** and a log message explains why.
- If a user sets `activityWindow: 10m` (stricter than the 15m ceiling), it is **accepted**.
- If the admin does not set a timing env var, the user's `.noidle` value is used without restriction.

Every resolved parameter is logged with its source (see [Logging](#logging)).

#### Important: Environment Variables Are Immutable

Environment variables are set at **pod creation time** and cannot be changed for a running workspace. Changing env var values in the DevWorkspace spec requires a workspace restart (stop and start). For runtime tuning without restart, users can modify the `.noidle` file (which is hot-reloaded) within admin-defined bounds.

### User Configuration (`.noidle` File)

Users can tune CLI Watcher behavior per-project using a `.noidle` YAML file. This is optional when the administrator has enabled the watcher via `CLI_ACTIVITY_TRACKER_ENABLED`.

#### Configuration File Locations

The CLI Watcher looks for a `.noidle` file in this order:

1. **Explicit override**: Path from `CLI_ACTIVITY_TRACKER_CONFIG` environment variable
2. **Project directory**: Search upward from `$PROJECT_SOURCE` to `$PROJECTS_ROOT` for `.noidle`
3. **Home directory**: Fallback to `$HOME/.noidle`

If no `.noidle` file is found, the watcher uses env var values and adaptive defaults.

#### Deprecated: `enabled` Field

**The `enabled` field in `.noidle` is deprecated and ignored.** Enablement is controlled exclusively by the `CLI_ACTIVITY_TRACKER_ENABLED` environment variable (or its default value). If `.noidle` contains `enabled: true` or `enabled: false`, a deprecation warning is logged and the value is disregarded.

**Before** (old behavior):
```yaml
# .noidle - this no longer controls enablement
enabled: true
```

**After** (new behavior):
```bash
# Enablement is admin-controlled via environment variable
CLI_ACTIVITY_TRACKER_ENABLED=true
```

#### What `.noidle` Still Controls

- **Timing overrides** (within admin ceilings): `checkPeriod`, `activityWindow`, `gracePeriod`, `maxProcessAge`
- **Command-specific behavior**: `watchedCommands`, `ignoredCommands`
- **Hot-reload**: The `.noidle` file is re-read on every check cycle. Changes take effect without workspace restart.

#### Basic Configuration

```yaml
# .noidle - timing and command overrides only
checkPeriod: 30
activityWindow: 20m
gracePeriod: 3m

watchedCommands:
  - helm
  - kubectl
```

**Note**: The `watchedCommands` list is **optional**. It overrides auto-detection for specific commands, not enables watching. Without this list, ALL user processes are still watched with smart defaults.

#### Advanced Configuration - Override Auto-Detection (Optional)

**You probably don't need this section.** The CLI Watcher auto-detects process types correctly in most cases.

**Only override auto-detection when:**
- Auto-detection misclassifies a specific command
- You need to completely ignore a command that shouldn't be watched
- You have special requirements or are debugging

**Two escape hatches available:**

##### 1. `watchedCommands` - Fix misclassification (process still watched, mode corrected)
```yaml
watchedCommands:
  - name: myBuildTool
    interactive: false  # Auto-detected as interactive, but it's actually a build

  - name: myREPL
    interactive: true   # Auto-detected as work process, but it's interactive
```

##### 2. `ignoredCommands` - Stop watching entirely (process never prevents idling)
```yaml
ignoredCommands:
  - weirdSystemDaemon   # Has TTY but shouldn't be watched at all
  - debugTool           # Picked up by auto-detection but irrelevant
```

**Warning:** Misconfiguring can break workspace idling:
- Setting `sleep` as `interactive: true` - Long-running tasks interrupted
- Setting `vim` as `interactive: false` - Idle editor prevents idling forever
- Over-using `ignoredCommands` - Important work not tracked

#### Full Example with Time Settings

```yaml
checkPeriod: 30                     # How often to check for active processes (default: 60 seconds)
activityWindow: 25m                 # How long to wait for activity from interactive processes
gracePeriod: 5m                     # All processes prevent idling when this young

# Optional: Override auto-detection for specific commands
watchedCommands:
  # Force long-running commands to always prevent idling (skip auto-detection)
  - helm
  - kubectl

  # Force interactive CLIs to always check for user input activity
  - name: claude
    interactive: true

  # Let auto-detection decide (foreground + TTY read -> interactive)
  - name: vim
    interactive: auto

  # Force non-interactive mode (always prevent idling)
  - name: npm
    interactive: false

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
| **Auto-detect** | `auto` | Detect interactivity by checking if process is foreground AND has read from TTY. If yes - check activity; if no - always prevent idling. |

## ForceWatch Override Option

**USE WITH EXTREME CAUTION**

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
- Making `tail -f logfile` prevent idling - Logs aren't active work
- Making `top` prevent idling - Process monitoring isn't active work
- Making `watch kubectl get pods` prevent idling - Passive monitoring isn't active work

### Accepted Values

- `true`, `yes` - Override always-ignored list (monitor this command)
- `false`, `no` - Respect always-ignored list (default behavior)
- Omit field - Same as `false` (respect always-ignored list)

## Default Values and Adaptive Calculation

The CLI Watcher uses **smart defaults** that adapt to the workspace idle timeout (`SECONDS_OF_DW_INACTIVITY_BEFORE_IDLING`) when available.

### Fixed Defaults (always the same)

| Parameter | Default | Description |
|-----------|---------|-------------|
| `enabled` | `false` | CLI Watcher is disabled by default |
| `interactive` | `no` | Backward compatible - always prevent idling |
| `maxProcessAge` | `6h` | Safety limit to prevent indefinite idling prevention |
| `checkPeriod` | `60s` | Process scan interval |

### Adaptive Defaults (calculated from workspace idle timeout)

When **workspace idle timeout is available** (e.g., 30 minutes), the timing defaults are calculated to fit within the idle window:

#### Grace Period Calculation

```
gracePeriod = min(5m, 15% of idleTimeout)
```

- Clamped to minimum `1m`
- Examples:
  - 30m idle timeout: `min(5m, 4m30s)` = **4m30s**
  - 15m idle timeout: `min(5m, 2m15s)` = **2m15s**
  - 60m idle timeout: `min(5m, 9m)` = **5m**

#### Activity Window Calculation

```
activityWindow = idleTimeout - gracePeriod - safetyBuffer
safetyBuffer   = min(5m, 20% of idleTimeout)
```

- Clamped to minimum `2m`
- Examples:
  - 30m idle timeout: `30m - 4m30s - min(5m, 6m)` = `30m - 4m30s - 5m` = **20m30s**
  - 15m idle timeout: `15m - 2m15s - min(5m, 3m)` = `15m - 2m15s - 3m` = **9m45s**
  - 60m idle timeout: `60m - 5m - min(5m, 12m)` = `60m - 5m - 5m` = **50m**

#### Why Adaptive?

The goal is to ensure `gracePeriod + activityWindow + safetyBuffer <= idleTimeout`, so that:
- A new process gets grace period protection immediately
- An interactive process has enough time to show activity
- There's a safety buffer before the workspace actually idles

When **workspace idle timeout is unavailable or disabled** (`-1`):
- `gracePeriod`: `5m`
- `activityWindow`: `25m`

### Minimum Values (enforced even for very short idle timeouts)

| Parameter | Minimum |
|-----------|---------|
| `gracePeriod` | `1m` |
| `activityWindow` | `2m` |
| `checkPeriod` | `10s` |

### How Defaults, Env Vars, and `.noidle` Interact

For each timing parameter, the resolution order is:

1. Start with the **adaptive default** (calculated from idle timeout, or fixed if idle timeout unavailable)
2. If the parameter is specified in `.noidle`, use the `.noidle` value instead
3. If an admin env var is set, enforce it as a **ceiling**: if the resolved value from steps 1-2 exceeds the env var, clamp it down

The final value and its source are always logged (see [Logging](#logging)).

### Note on Time Formats

All time settings (`checkPeriod`, `activityWindow`, `gracePeriod`, `maxProcessAge`) accept:
- Duration strings: `6h`, `30m`, `21600s`, `6h30m`
- Plain integers: `21600` (treated as seconds)
- Invalid values log a warning and use the calculated or fixed default

## How It Works (Detail)

1. **User Process Detection**: Only watches processes with TTY that are children of user terminals (filters out system processes automatically)
2. **Always-Ignored Check**: Skips passive monitoring tools (`tail`, `watch`, `top`, `htop`)
3. **Safety Limit**: Processes older than `maxProcessAge` (default 6h) don't prevent idling - protects against hung/forgotten/misconfigured processes
4. **Grace Period**: All user processes younger than `gracePeriod` prevent idling (gives builds time to start)
5. **Interactive Detection** (after grace period):
   - **Configured commands**: Use their `interactive` setting
   - **Unconfigured commands**: Auto-detect (foreground + has read from TTY - interactive, otherwise - work process)
6. **Activity Checking**: Interactive processes only prevent idling if user input detected within `activityWindow`

### Configuration Validation

The CLI Watcher validates your configuration and warns about potential issues **without changing your specified values**:

```
WARN: activityWindow (35m) exceeds workspace idle timeout (30m), may not work as expected
WARN: gracePeriod (25m) is very close to workspace idle timeout (30m)
WARN: activityWindow (3m) is less than gracePeriod (5m), interactive processes may not be detected correctly
WARN: checkPeriod (10m) may be too long for activityWindow (15m), activity might not be detected in time
WARN: Workspace idle timeout (8m) is very short, using minimum activity window (2m)
WARN: Both 'checkPeriod' (30s) and deprecated 'checkPeriodSeconds' (45) are set - using 'checkPeriod' value
```

## Activity Detection

### Interactive Process Detection (`auto` mode)

A process is considered interactive if:
1. It's in the **foreground process group** of its TTY, AND
2. Either:
   - Currently waiting on `read` (from wchan), OR
   - Has **ever read from its TTY** (TTY access time is after process start time)

This detects:
- **Interactive**: `vim`, `python3` (REPL), `node` (REPL), `less` - Check for recent user input
- **Work**: `./compile.sh`, `go build`, `npm run build` - Always prevent idling

### Activity Monitoring

For interactive processes, recent activity is detected by monitoring **TTY Access Time (Atime)**:
- Atime updates when the TTY is **read from** (user types)
- Atime does NOT update from output (program writes)
- Process prevents idling if Atime is within the `activityWindow`

**Examples**:
- `claude` actively used - Prevents idling
- `claude` idle for 30 minutes - Doesn't prevent idling
- `vim` with active typing - Prevents idling
- `vim` left open but untouched - Doesn't prevent idling after activity window
- `go build` running - Always prevents idling
- Background `node` (VS Code) - Skipped (system process)

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
# .noidle
watchedCommands:
  - helm
  - kubectl
  - odo
```

These always prevent idling during deployment operations, even if auto-detection would classify them differently.

### Interactive Development with AI (Custom Activity Window)

```yaml
# .noidle
activityWindow: 300  # Override global default to 5 minutes

watchedCommands:
  - name: claude
    interactive: true
```

Workspace stays alive during active Claude Code sessions, but idles if left idle for 5+ minutes.

### Mixed Workload - Fine-Tuned Control

```yaml
# .noidle
activityWindow: 25m
gracePeriod: 5m

watchedCommands:
  - helm
  - kubectl
  - name: claude
    interactive: true
  - name: vim
    interactive: auto
  - name: npm
    interactive: false
```

## Logging

### Startup: Admin Config Summary

At startup, the watcher logs all admin env var values:

```
CLI Watcher: Admin config from environment:
CLI Watcher:   CLI_ACTIVITY_TRACKER_ENABLED = true
CLI Watcher:   CLI_ACTIVITY_TRACKER_CHECK_PERIOD not set
CLI Watcher:   CLI_ACTIVITY_TRACKER_ACTIVITY_WINDOW = 15m0s
CLI Watcher:   CLI_ACTIVITY_TRACKER_GRACE_PERIOD not set
CLI Watcher:   CLI_ACTIVITY_TRACKER_MAX_PROCESS_AGE not set
CLI Watcher:   CLI_ACTIVITY_TRACKER_VERBOSE not set (default: false)
```

### Config Load: Resolved Values with Source

Every parameter is logged with its final value and source. This makes it easy to understand why a particular value is in effect.

**Env var used directly (no `.noidle` override):**
```
CLI Watcher: 'enabled' = true (from CLI_ACTIVITY_TRACKER_ENABLED)
CLI Watcher: 'activityWindow' = 15m0s (from CLI_ACTIVITY_TRACKER_ACTIVITY_WINDOW)
CLI Watcher: 'checkPeriod' = 1m0s (default)
```

**`.noidle` value accepted (stricter than admin ceiling):**
```
CLI Watcher: 'activityWindow' = 10m0s (from .noidle; within admin limit 15m0s)
```

**`.noidle` value rejected (exceeds admin ceiling):**
```
CLI Watcher: 'activityWindow' = 15m0s (admin limit; .noidle value 30m0s rejected — exceeds admin ceiling)
```

**`.noidle` `enabled` deprecated:**
```
CLI Watcher: 'enabled' = true (from CLI_ACTIVITY_TRACKER_ENABLED; .noidle 'enabled' is deprecated — admin-controlled)
CLI Watcher: 'enabled' = false (from CLI_ACTIVITY_TRACKER_ENABLED; .noidle 'enabled: true' rejected — deprecated, admin-controlled)
CLI Watcher: 'enabled' = false (default; .noidle 'enabled: true' rejected — deprecated, use CLI_ACTIVITY_TRACKER_ENABLED env var)
```

**Default used (no env var, no `.noidle`):**
```
CLI Watcher: 'enabled' = false (default)
CLI Watcher: 'activityWindow' = 25m0s (default)
```

### Runtime: Activity Detection

```
CLI Watcher: Config reloaded from /home/user/.noidle
CLI Watcher:   Watching ALL user processes with 3 explicit override(s):
CLI Watcher:     - helm (mode: non-interactive (always active))
CLI Watcher:     - claude (mode: interactive (activity check))
CLI Watcher:     - vim (mode: auto-detect TTY)
CLI Watcher:   Detection period: 30s
CLI Watcher:   Activity window: 15m0s
CLI Watcher:   Grace period: 4m30s
CLI Watcher:   Max process age: 6h0m0s (safety limit)
CLI Watcher: Detected CLI command: helm — reporting activity tick
```

Use DEBUG level for detailed process scanning:

```
CLI Watcher: Process claude (PID 12345) has recent activity
CLI Watcher: Process vi (PID 12345) found but no recent activity
```

### Verbose Activity Logging

By default, detailed activity-detection reasoning (which process was detected, why it does or doesn't prevent idling, interactive/auto-detection decisions) is logged at Debug level. Enabling global `LOG_LEVEL=debug` shows this, but also produces debug output from every other component in che-machine-exec.

Set `CLI_ACTIVITY_TRACKER_VERBOSE=true` to promote just the CLI Watcher's activity-detection messages to Info level, without touching the global log level:

```
CLI Watcher: Detected CLI command: helm — reporting activity tick
CLI Watcher: Process vi (PID 12345) auto-detected as interactive (default policy)
CLI Watcher: Process vi (PID 12345) is interactive with recent activity (default policy)
CLI Watcher: Process npm (PID 12346) is in config ignored list, skipping
```

## Upgrading from Previous Versions

### Breaking Changes

#### 1. `enabled` Field in `.noidle` is Deprecated

**Before**: The `enabled: true` field in `.noidle` was the only way to enable the CLI Watcher.

**After**: Enablement is controlled exclusively by the `CLI_ACTIVITY_TRACKER_ENABLED` environment variable (or its default). The `.noidle` `enabled` field is **ignored** with a deprecation warning logged.

**Migration**: Ask your cluster administrator to set `CLI_ACTIVITY_TRACKER_ENABLED=true` in the DevWorkspace configuration.

#### 2. Timing Parameters Have Admin Ceilings

**Before**: `.noidle` timing values were always used as-is.

**After**: If an administrator sets timing env vars (`CLI_ACTIVITY_TRACKER_ACTIVITY_WINDOW`, etc.), `.noidle` values can only be **stricter** (shorter). Looser values are clamped to the admin ceiling with a warning.

#### 3. ALL User Processes Are Watched by Default

**Before (older versions)**: Only commands listed in `watchedCommands` were monitored.

**After**: **ALL user processes with TTY are monitored automatically**. `watchedCommands` now **overrides auto-detection** for specific commands (not required to enable watching). `tail`, `watch`, `top`, `htop` are **always ignored**.

### Impact on Your Workspace

1. **Workspaces may stay active longer** - processes that were previously ignored (shells, scripts, REPLs) now prevent idling
2. **Commands in `watchedCommands` may behave differently**:
   - If you configured `watch`, `top`, or `htop` - now ignored with a warning
   - If you only listed specific commands - other user processes are now also monitored
3. **Auto-detection may differ from your expectations** - interactive processes (vim, python REPL) only prevent idling when actively used

### Migration Steps

**If you have an existing `.noidle` configuration:**

1. **Remove `enabled: true`** - enablement is now admin-controlled via `CLI_ACTIVITY_TRACKER_ENABLED` env var
2. **Review your `watchedCommands` list** - remove `watch`, `top`, `htop` (always ignored now)
3. **Check timing values** - if admin ceilings are set, your values may be clamped

**If you're an administrator enabling CLI Watcher for the first time:**

1. Set `CLI_ACTIVITY_TRACKER_ENABLED=true` in DevWorkspace env vars
2. Optionally set timing ceilings to enforce policy bounds
3. Users can create `.noidle` files to tune within your bounds

### Verification

After updating:

1. Check logs for deprecation warnings about `.noidle` `enabled` field
2. Check logs for ceiling enforcement messages (rejected/accepted overrides)
3. Monitor workspace idle timeout behavior
4. Use `LOG_LEVEL=debug` to see which processes are detected and classified

### Rollback

If the new behavior doesn't suit your workflow:

1. Use `ignoredCommands` to exclude unwanted processes
2. Set explicit `interactive` modes in `watchedCommands` to override auto-detection
3. Contact your platform administrator if workspace idle policies need adjustment

## Deployment Requirements

### Filesystem Access Time (atime) Dependency

**CRITICAL**: Interactive-process classification and activity-freshness tracking both prefer filesystem access time (atime) updates for TTY devices as their primary signal.

**Problem**: If the `devpts` filesystem (backing `/dev/pts/*`, i.e. workspace terminals) is mounted with `noatime`:
- TTY access times never update, regardless of real terminal activity
- Both interactive-process classification and "is this process still active" checks fall back to less precise, atime-independent detection (see below)

`relatime` (the default on most systems) is generally *not* a problem — atime can lag real activity by up to tens of seconds under heavy load, but it does track it. `noatime` is the actual failure mode: atime freezes at whatever value it had before, forever.

**Automatic Detection**: CLI Watcher checks `/proc/mounts` for the `devpts` mount options once at startup and logs a warning if `noatime` is detected:
```
CLI Watcher: devpts (/dev/pts) is mounted with 'noatime' — TTY access-time tracking is disabled, so interactive-process activity will be detected via a CPU-usage fallback instead of keystroke timing (coarser; may keep workspaces alive slightly longer than expected)
```

**Manual Verification**: Check if `/dev/pts` is mounted with atime support:
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

**Fallback Detection**: When atime hasn't advanced past a process's start time (the sign that it's genuinely unusable — `noatime`, or a TTY never read from at all), CLI Watcher substitutes two independent, atime-free signals for the two different questions it needs to answer:

1. **Classification** ("is this the kind of process that waits for input at all?") — checks the process's wait channel (`wchan`) for patterns consistent with an input-driven event loop:
   - `poll_schedule_timeout` - polling with timeout (interactive pattern)
   - `pipe_wait` - waiting on pipe input
   - `unix_stream_read_generic` - reading from socket
   - `select`, `ep_poll` - event-driven input waiting

2. **Activity freshness** ("is this already-classified-interactive process still being used right now?") — tracks CPU time (`utime`+`stime` from `/proc/<pid>/stat`) sampled once per check cycle. If a process has consumed any CPU since it was last observed, it's treated as active.

The CPU-usage signal can't distinguish "the user typed something" from "the process did something on its own" (e.g. background timers), so it's coarser than atime — but unlike atime under `noatime`, it doesn't get stuck reporting "never active" forever.

**Symptoms Indicating Fallback Mode**:
- Startup log: `"devpts (...) is mounted with 'noatime' ..."` (see Automatic Detection above)
- `"CLI Watcher: TTY atime for PID X unavailable or unreliable, using fallback detection"` — classification fallback triggered
- `"CLI Watcher: PID X detected as interactive via fallback (wchan: Y)"` — classified interactive via wchan
- `"CLI Watcher: TTY atime for PID X unavailable or unreliable, using CPU-activity fallback"` — freshness fallback triggered
- `"CLI Watcher: PID X CPU-activity fallback: recent=... (last active ... ago)"` — freshness fallback's verdict (set `CLI_ACTIVITY_TRACKER_VERBOSE=true` to see this at Info level instead of Debug — see [Verbose Activity Logging](#verbose-activity-logging))

**Result**: Interactive detection remains reliable even on `noatime` filesystems, though atime support is still preferable for both precision (per-keystroke timing vs. per-check-cycle CPU sampling) and correctness (CPU-based freshness can't tell genuine user input from unrelated background work).

## Testing

### Testing Administrator Configuration (Environment Variables)

These scenarios verify that admin env vars correctly control CLI Watcher behavior, enforce ceilings, and produce the expected log output.

#### Scenario A1: Enable CLI Watcher via Env Var Only (No `.noidle` File)

**Setup**: No `.noidle` file exists anywhere.

```bash
# Set env vars before starting che-machine-exec
export CLI_ACTIVITY_TRACKER_ENABLED=true

# In a DevWorkspace, set in the container spec:
# env:
#   - name: CLI_ACTIVITY_TRACKER_ENABLED
#     value: "true"
```

**Expected startup logs**:
```
CLI Watcher: Admin config from environment:
CLI Watcher:   CLI_ACTIVITY_TRACKER_ENABLED = true
CLI Watcher:   CLI_ACTIVITY_TRACKER_CHECK_PERIOD not set
CLI Watcher:   CLI_ACTIVITY_TRACKER_ACTIVITY_WINDOW not set
CLI Watcher:   CLI_ACTIVITY_TRACKER_GRACE_PERIOD not set
CLI Watcher:   CLI_ACTIVITY_TRACKER_MAX_PROCESS_AGE not set
```

**Expected config resolution logs** (no `.noidle` file):
```
CLI Watcher: Config file not found, waiting for it to appear...
CLI Watcher: 'enabled' = true (from CLI_ACTIVITY_TRACKER_ENABLED)
CLI Watcher: 'checkPeriod' = 1m0s (default)
CLI Watcher: 'activityWindow' = 20m30s (default)
CLI Watcher: 'gracePeriod' = 4m30s (default)
CLI Watcher: 'maxProcessAge' = 6h0m0s (default)
```

**Verify**: The watcher is active and scanning processes despite no `.noidle` file.

#### Scenario A2: Admin Disables CLI Watcher, User `.noidle` Says `enabled: true`

**Setup**: Create a `.noidle` file:
```yaml
enabled: true
activityWindow: 20m
```

```bash
export CLI_ACTIVITY_TRACKER_ENABLED=false
```

**Expected config resolution logs**:
```
CLI Watcher: 'enabled' = false (from CLI_ACTIVITY_TRACKER_ENABLED; .noidle 'enabled: true' rejected — deprecated, admin-controlled)
CLI Watcher: 'activityWindow' = 20m0s (from .noidle)
...
```

**Verify**: The watcher is NOT scanning processes despite `.noidle` having `enabled: true`.

#### Scenario A3: Admin Sets Timing Ceilings, User `.noidle` Exceeds Them

**Setup**: Create a `.noidle` file:
```yaml
activityWindow: 30m
gracePeriod: 10m
checkPeriod: 120
maxProcessAge: 12h
```

```bash
export CLI_ACTIVITY_TRACKER_ENABLED=true
export CLI_ACTIVITY_TRACKER_ACTIVITY_WINDOW=15m
export CLI_ACTIVITY_TRACKER_GRACE_PERIOD=3m
export CLI_ACTIVITY_TRACKER_CHECK_PERIOD=45s
export CLI_ACTIVITY_TRACKER_MAX_PROCESS_AGE=4h
```

**Expected config resolution logs**:
```
CLI Watcher: 'enabled' = true (from CLI_ACTIVITY_TRACKER_ENABLED; .noidle 'enabled' is deprecated — admin-controlled)
CLI Watcher: 'checkPeriod' = 45s (admin limit; .noidle value 2m0s rejected — exceeds admin ceiling)
CLI Watcher: 'activityWindow' = 15m0s (admin limit; .noidle value 30m0s rejected — exceeds admin ceiling)
CLI Watcher: 'gracePeriod' = 3m0s (admin limit; .noidle value 10m0s rejected — exceeds admin ceiling)
CLI Watcher: 'maxProcessAge' = 4h0m0s (admin limit; .noidle value 12h0m0s rejected — exceeds admin ceiling)
```

**Verify**: All timing params are clamped to admin values, not `.noidle` values.

#### Scenario A4: Admin Sets Ceilings, User `.noidle` Is Stricter

**Setup**: Create a `.noidle` file:
```yaml
activityWindow: 10m
gracePeriod: 2m
```

```bash
export CLI_ACTIVITY_TRACKER_ENABLED=true
export CLI_ACTIVITY_TRACKER_ACTIVITY_WINDOW=15m
export CLI_ACTIVITY_TRACKER_GRACE_PERIOD=5m
```

**Expected config resolution logs**:
```
CLI Watcher: 'enabled' = true (from CLI_ACTIVITY_TRACKER_ENABLED)
CLI Watcher: 'activityWindow' = 10m0s (from .noidle; within admin limit 15m0s)
CLI Watcher: 'gracePeriod' = 2m0s (from .noidle; within admin limit 5m0s)
```

**Verify**: User's stricter values are accepted.

#### Scenario A5: Hot-Reload `.noidle` While Running

**Setup**: Start with `CLI_ACTIVITY_TRACKER_ENABLED=true` and `CLI_ACTIVITY_TRACKER_ACTIVITY_WINDOW=15m`.

1. Create a `.noidle` file with `activityWindow: 10m` - observe "accepted" log
2. Edit it to `activityWindow: 30m` - observe "rejected" log on next check cycle
3. Delete the `.noidle` file - observe config reverts to env var values

**Verify**: Changes take effect on the next check cycle without restart.

### Testing User Configuration (`.noidle` File)

#### Monitoring Activity Ticks

To watch CLI watcher activity ticks in real-time in a DevWorkspace environment, open a terminal and run:

```bash
tail -f /checode/entrypoint-logs.txt
```

This will show continuous log output including:
- CLI Watcher startup messages
- Config reload events
- Detected CLI commands and activity ticks
- Process scanning debug messages (if `LOG_LEVEL=debug`)

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

#### Scenario U1: Non-Interactive Long-Running Commands

**Test Config** (`/tmp/.noidle.test`):
```yaml
checkPeriod: 15

watchedCommands:
  - sleep
  - ping
```

**Test Steps**:
```bash
export CLI_ACTIVITY_TRACKER_ENABLED=true
export CLI_ACTIVITY_TRACKER_CONFIG=/tmp/.noidle.test

# Start a long-running background process (no TTY)
sleep 1800 &

# Watch logs in another terminal
tail -f /checode/entrypoint-logs.txt

# Expected: "Detected CLI command: sleep — reporting activity tick" every 15s
```

**Cleanup**: `pkill sleep`

#### Scenario U2: Interactive Command with Activity Tracking

**Test Config** (`/tmp/.noidle.interactive`):
```yaml
checkPeriod: 15
activityWindow: 120  # 2 minutes for easy testing

watchedCommands:
  - name: vi
    interactive: auto
```

**Test Steps**:
```bash
export CLI_ACTIVITY_TRACKER_ENABLED=true
export CLI_ACTIVITY_TRACKER_CONFIG=/tmp/.noidle.interactive

# Terminal 1: Watch logs
tail -f /checode/entrypoint-logs.txt

# Terminal 2: Open vi interactively
vi /tmp/testfile.txt

# Type occasionally and watch for activity ticks
# Stop typing for 3+ minutes - activity ticks should stop
```

#### Scenario U3: Auto-Detection of Interactive vs Work Processes

**Purpose**: Verify that the watcher correctly distinguishes between interactive CLIs (vim, REPLs) and work processes (builds, scripts) without explicit configuration.

**Test Config** (`/tmp/.noidle.autodetect`):
```yaml
checkPeriod: 15
activityWindow: 120  # 2 minutes for easy testing
gracePeriod: 1m      # Short grace period for faster testing

# No watchedCommands - everything is auto-detected!
```

**Test Steps**:
```bash
export CLI_ACTIVITY_TRACKER_ENABLED=true
export CLI_ACTIVITY_TRACKER_CONFIG=/tmp/.noidle.autodetect

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
- `vi` detected as **interactive** (foreground + reads from TTY) - Only ticks when typing
- `sleep` detected as **work process** (not interactive) - Always ticks while running
- Grace period (1min): Both prevent idling immediately when started

**Debugging**: Set `LOG_LEVEL=debug` to see detailed detection:
```
CLI Watcher: Process vi (PID 12345) auto-detected as interactive
CLI Watcher: Process vi (PID 12345) has recent activity
CLI Watcher: Process sleep (PID 12346) auto-detected as work process
```

#### Scenario U4: Excluded Commands (Negative Test)

**Test Config** (`/tmp/.noidle.exclusion`):
```yaml
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

**Scoped alternative**: To see CLI Watcher activity-detection details without enabling debug logging application-wide, set `CLI_ACTIVITY_TRACKER_VERBOSE=true` instead. This promotes only the CLI Watcher's own detection/reasoning messages to Info level.

### Quick Test Setup

Create a test configuration file:

```yaml
# /tmp/.noidle.quicktest
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
   export CLI_ACTIVITY_TRACKER_ENABLED=true
   export CLI_ACTIVITY_TRACKER_CONFIG=/tmp/.noidle.quicktest
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
| `sleep 3600 &` | default (no) | No | N/A | Always |
| `vi file.txt` (typing) | auto | Yes | Yes | Yes |
| `vi file.txt` (idle) | auto | Yes | No | No (after window) |
| `tail -f file` | any | any | any | Never (excluded) |

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
- **Unit tests cover pure functions** (parsing, configuration, validation, defaults, YAML unmarshaling, env var loading, ceiling enforcement)
- **Core detection logic is untested** (process tree walking, TTY analysis, interactive process detection, `isWatchedProcessRunning`, `isUserInitiatedProcess`)

**Why core detection logic requires manual testing:**
- Requires real `/proc` filesystem (not available in standard Go test environment)
- Needs multiple process scenarios (shells, interactive CLIs, work processes, TTY states)
- Depends on actual system process behavior and file descriptor states

**For detection logic verification**: Use the manual test scenarios described above with real processes in a containerized development environment.

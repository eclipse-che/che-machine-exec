//
// Copyright (c) 2025-2026 Red Hat, Inc.
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
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type InteractiveMode string

const (
	InteractiveModeAuto  InteractiveMode = "auto"
	InteractiveModeTrue  InteractiveMode = "true"
	InteractiveModeFalse InteractiveMode = "false"
	InteractiveModeYes   InteractiveMode = "yes"
	InteractiveModeNo    InteractiveMode = "no"
)

type ForceWatchMode string

const (
	ForceWatchModeTrue  ForceWatchMode = "true"
	ForceWatchModeFalse ForceWatchMode = "false"
	ForceWatchModeYes   ForceWatchMode = "yes"
	ForceWatchModeNo    ForceWatchMode = "no"
)

// isForceWatchEnabled checks if ForceWatchMode is enabled (true/yes)
func (f ForceWatchMode) isEnabled() bool {
	return f == ForceWatchModeTrue || f == ForceWatchModeYes
}

const (
	DefaultInteractiveMode InteractiveMode = InteractiveModeNo // Backward compatible: always prevent idling
	DefaultCheckPeriod                     = 60                // Default check period: 60 seconds
	DefaultActivityWindow                  = 25 * time.Minute  // Default activity window: 25 minutes (fallback when idle timeout unavailable)
	DefaultGracePeriod                     = 5 * time.Minute   // Default grace period: 5 minutes
	DefaultMaxProcessAge                   = 6 * time.Hour     // Default max process age: 6 hours - safety limit

	MinActivityWindow    = 2 * time.Minute // Minimum activity window for very short idle timeouts
	MinGracePeriod       = 1 * time.Minute // Minimum grace period
	MinCheckPeriod       = 10              // Minimum check period in seconds
	SafetyBufferDuration = 5 * time.Minute // Safety buffer between activity window and idle timeout
	SafetyBufferPercent  = 0.2             // Or 20% of idle timeout, whichever is smaller
)

// Environment variable names for admin-level CLI Watcher configuration
const (
	EnvCliWatcherEnabled         = "CLI_ACTIVITY_TRACKER_ENABLED"
	EnvCliWatcherCheckPeriod     = "CLI_ACTIVITY_TRACKER_CHECK_PERIOD"
	EnvCliWatcherActivityWindow  = "CLI_ACTIVITY_TRACKER_ACTIVITY_WINDOW"
	EnvCliWatcherGracePeriod     = "CLI_ACTIVITY_TRACKER_GRACE_PERIOD"
	EnvCliWatcherMaxProcessAge   = "CLI_ACTIVITY_TRACKER_MAX_PROCESS_AGE"
	EnvCliWatcherVerbose         = "CLI_ACTIVITY_TRACKER_VERBOSE"
	EnvCliWatcherActivitySources = "CLI_ACTIVITY_TRACKER_ACTIVITY_SOURCES"
)

// DefaultCliWatcherEnabled is the default for CLI_ACTIVITY_TRACKER_ENABLED (flip to true when ready for general rollout)
const DefaultCliWatcherEnabled = false

type WatchedCommand struct {
	Name        string          `yaml:"name"`
	Interactive InteractiveMode `yaml:"interactive"`
	ForceWatch  ForceWatchMode  `yaml:"forceWatch"`
}

// UnmarshalYAML allows WatchedCommand to be unmarshaled from either a string or an object
func (w *WatchedCommand) UnmarshalYAML(unmarshal func(any) error) error {
	// Try to unmarshal as a string first (backward compatible)
	var str string
	if err := unmarshal(&str); err == nil {
		w.Name = str
		w.Interactive = DefaultInteractiveMode
		return nil
	}

	// Otherwise, unmarshal as a struct
	type rawWatchedCommand WatchedCommand
	var raw rawWatchedCommand
	if err := unmarshal(&raw); err != nil {
		return err
	}

	*w = WatchedCommand(raw)
	return nil
}

type cliWatcherConfig struct {
	WatchedCommands       []WatchedCommand `yaml:"watchedCommands"`
	IgnoredCommands       []string         `yaml:"ignoredCommands" json:"-"`
	CheckPeriodSeconds    int              `yaml:"checkPeriodSeconds"` // Deprecated: use CheckPeriod instead (kept for backward compatibility)
	CheckPeriod           string           `yaml:"checkPeriod"`
	ActivityWindow        string           `yaml:"activityWindow"`
	GracePeriod           string           `yaml:"gracePeriod"`
	MaxProcessAge         string           `yaml:"maxProcessAge"`
	Enabled               bool             `yaml:"enabled"`
	_lastModTime          time.Time        `json:"-"`
	_checkPeriodParsed    time.Duration    `json:"-"`
	_activityWindowParsed time.Duration    `json:"-"`
	_gracePeriodParsed    time.Duration    `json:"-"`
	_maxProcessAgeParsed  time.Duration    `json:"-"`
	_fromFile             bool             `json:"-"` // true only if loaded from an actual .noidle file
	_verbose              bool             `json:"-"` // resolved from CLI_ACTIVITY_TRACKER_VERBOSE
}

// cliWatcherEnvConfig holds admin-level configuration from environment variables.
// Pointer fields: nil = not set by admin, non-nil = admin-enforced ceiling.
type cliWatcherEnvConfig struct {
	enabled         *bool
	checkPeriod     *time.Duration
	activityWindow  *time.Duration
	gracePeriod     *time.Duration
	maxProcessAge   *time.Duration
	verbose         *bool
	activitySources []activitySourceSelector
}

// Watcher monitors CLI processes and invokes a tick callback when active ones are found
type cliWatcher struct {
	mu                    sync.Mutex // Protects config, warnedMissingConfig, started
	config                *cliWatcherConfig
	warnedMissingConfig   bool
	stopChan              chan struct{}
	stopOnce              sync.Once // Ensures stopChan is only closed once
	started               bool
	tickFunc              func()              // Immutable after construction (safe to read without lock)
	myPID                 string              // Immutable after construction (safe to read without lock)
	idleTimeout           time.Duration       // Immutable after construction (safe to read without lock)
	envConfig             cliWatcherEnvConfig // Immutable after Start() (safe to read without lock)
	activeActivitySources []ActivitySource    // Immutable after Start() (safe to read without lock)
}

// Commands that should NEVER prevent workspace idling (passive monitoring tools)
var alwaysIgnoredCommands = []string{"tail", "watch", "top", "htop"}

func loadEnvConfig() cliWatcherEnvConfig {
	var cfg cliWatcherEnvConfig

	if v, ok := os.LookupEnv(EnvCliWatcherEnabled); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.enabled = &b
		} else {
			logrus.Errorf("CLI Watcher: Invalid value '%s' for %s, expected boolean", v, EnvCliWatcherEnabled)
		}
	}

	parseDurationEnv := func(envName string, target **time.Duration) {
		if v, ok := os.LookupEnv(envName); ok && len(v) > 0 {
			d := parseDuration(v, envName, 0)
			if d > 0 {
				*target = &d
			} else {
				logrus.Errorf("CLI Watcher: Invalid value '%s' for %s, expected positive duration (e.g. 30s, 5m, 1h)", v, envName)
			}
		}
	}

	parseDurationEnv(EnvCliWatcherCheckPeriod, &cfg.checkPeriod)
	parseDurationEnv(EnvCliWatcherActivityWindow, &cfg.activityWindow)
	parseDurationEnv(EnvCliWatcherGracePeriod, &cfg.gracePeriod)
	parseDurationEnv(EnvCliWatcherMaxProcessAge, &cfg.maxProcessAge)

	if v, ok := os.LookupEnv(EnvCliWatcherVerbose); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.verbose = &b
		} else {
			logrus.Errorf("CLI Watcher: Invalid value '%s' for %s, expected boolean", v, EnvCliWatcherVerbose)
		}
	}

	if v, ok := os.LookupEnv(EnvCliWatcherActivitySources); ok {
		cfg.activitySources = parseActivitySourcesEnv(v)
	}

	return cfg
}

// activityLogf logs CLI Watcher activity-detection details at Info level when verbose
// is true (CLI_ACTIVITY_TRACKER_VERBOSE), otherwise at Debug level.
func activityLogf(verbose bool, format string, args ...interface{}) {
	if verbose {
		logrus.Infof(format, args...)
	} else {
		logrus.Debugf(format, args...)
	}
}

// New creates a new Watcher with the given config and tick callback
func NewCliWatcher(tickFunc func(), idleTimeout time.Duration) *cliWatcher {
	if tickFunc == nil {
		logrus.Warnf("CLI Watcher: Created with nil tick callback - activity will not be reported")
	}
	return &cliWatcher{
		stopChan:    make(chan struct{}),
		tickFunc:    tickFunc,
		myPID:       fmt.Sprintf("%d", os.Getpid()),
		idleTimeout: idleTimeout,
	}
}

// Start begins the watcher loop
func (w *cliWatcher) Start() {
	// Held for the whole function, not just the started-check/envConfig
	// write: w.envConfig/w.activeActivitySources are read (for logging and
	// resolution) further down without re-acquiring the lock, on the
	// assumption that they're immutable once Start() has finished. That's
	// only true if a concurrent Stop() can't reset w.started and let a
	// second Start() call re-enter and overwrite them while this call is
	// still in here — holding the lock for the entire body closes that
	// window. Start() only runs once per watcher lifetime in real usage
	// and does no I/O beyond a few /proc-free logging calls, so holding
	// the lock this long costs nothing in practice.
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.started {
		return
	}
	w.started = true
	w.envConfig = loadEnvConfig()

	logrus.Infof("CLI Watcher: Admin config from environment:")
	if w.envConfig.enabled != nil {
		logrus.Infof("CLI Watcher:   %s = %t", EnvCliWatcherEnabled, *w.envConfig.enabled)
	} else {
		logrus.Infof("CLI Watcher:   %s not set (default: %t)", EnvCliWatcherEnabled, DefaultCliWatcherEnabled)
	}
	logEnvDuration := func(envName string, val *time.Duration) {
		if val != nil {
			logrus.Infof("CLI Watcher:   %s = %v", envName, *val)
		} else {
			logrus.Infof("CLI Watcher:   %s not set", envName)
		}
	}
	logEnvDuration(EnvCliWatcherCheckPeriod, w.envConfig.checkPeriod)
	logEnvDuration(EnvCliWatcherActivityWindow, w.envConfig.activityWindow)
	logEnvDuration(EnvCliWatcherGracePeriod, w.envConfig.gracePeriod)
	logEnvDuration(EnvCliWatcherMaxProcessAge, w.envConfig.maxProcessAge)
	if w.envConfig.verbose != nil {
		logrus.Infof("CLI Watcher:   %s = %t", EnvCliWatcherVerbose, *w.envConfig.verbose)
	} else {
		logrus.Infof("CLI Watcher:   %s not set (default: false)", EnvCliWatcherVerbose)
	}

	logrus.Infof("CLI Watcher: Compiled-in activity sources:")
	for _, src := range allActivitySources {
		logrus.Infof("CLI Watcher:   %s", src.Name())
	}
	if v, ok := os.LookupEnv(EnvCliWatcherActivitySources); ok {
		logrus.Infof("CLI Watcher:   %s = %q", EnvCliWatcherActivitySources, v)
	} else {
		logrus.Infof("CLI Watcher:   %s not set (default: tty)", EnvCliWatcherActivitySources)
	}

	activeSources, sourceWarnings := resolveActiveActivitySources(allActivitySources, w.envConfig.activitySources)
	for _, warning := range sourceWarnings {
		logrus.Warnf("CLI Watcher: %s", warning)
	}
	w.activeActivitySources = activeSources

	if len(w.activeActivitySources) == 0 {
		logrus.Infof("CLI Watcher: No activity sources active — CLI Watcher will not scan for activity")
		return
	}

	activeNames := make([]string, len(w.activeActivitySources))
	ttyActive := false
	for i, src := range w.activeActivitySources {
		activeNames[i] = src.Name()
		if src.Name() == "tty" {
			ttyActive = true
		}
	}
	logrus.Infof("CLI Watcher: Activity source scan order: %s", strings.Join(activeNames, ", "))

	if ttyActive {
		checkDevptsAtimeSupport()
	}

	go func() {
		var err error
		w.mu.Lock()
		w.config, err = w.loadConfig(getConfigPath(), w.config)
		w.mu.Unlock()
		if err != nil {
			logrus.Errorf("CLI Watcher: Failed to reload config: %v", err)
		}

		w.mu.Lock()
		chkPeriod := DefaultCheckPeriod
		if w.config != nil && w.config._checkPeriodParsed > 0 {
			chkPeriod = int(w.config._checkPeriodParsed.Seconds())
		}
		w.mu.Unlock()

		ticker := time.NewTicker(time.Duration(chkPeriod) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-w.stopChan:
				logrus.Infof("CLI Watcher: Stopped")
				return

			case <-ticker.C:
				oldPeriod := chkPeriod

				// Reload config (protected)
				w.mu.Lock()
				w.config, err = w.loadConfig(getConfigPath(), w.config)
				configSnapshot := w.config // Take snapshot for use outside lock
				w.mu.Unlock()
				if err != nil {
					logrus.Errorf("CLI Watcher: Failed to reload config: %v", err)
				}

				if configSnapshot == nil || !configSnapshot.Enabled {
					if chkPeriod != DefaultCheckPeriod {
						logrus.Infof("CLI Watcher: Config was removed or disabled — resetting check period to default (%ds)", DefaultCheckPeriod)
						chkPeriod = DefaultCheckPeriod
						ticker.Stop()
						// Recreate ticker with new period
						ticker = time.NewTicker(time.Duration(chkPeriod) * time.Second)
					}
					continue
				}

				newPeriod := int(configSnapshot._checkPeriodParsed.Seconds())
				if newPeriod > 0 && newPeriod != oldPeriod {
					logrus.Infof("CLI Watcher: Detected new check period: %d seconds (was %d), restarting ticker", newPeriod, oldPeriod)
					chkPeriod = newPeriod
					ticker.Stop()
					// Recreate ticker with new period
					ticker = time.NewTicker(time.Duration(chkPeriod) * time.Second)
				}

				scanCtx := ActivityScanContext{
					MyPID:           w.myPID,
					ActivityWindow:  configSnapshot._activityWindowParsed,
					GracePeriod:     configSnapshot._gracePeriodParsed,
					MaxProcessAge:   configSnapshot._maxProcessAgeParsed,
					Verbose:         configSnapshot._verbose,
					WatchedCommands: configSnapshot.WatchedCommands,
					IgnoredCommands: configSnapshot.IgnoredCommands,
				}
				for _, src := range w.activeActivitySources {
					active, label := src.Scan(scanCtx)
					if active {
						activityLogf(configSnapshot._verbose, "CLI Watcher: [%s] Detected activity: %s — reporting activity tick", src.Name(), label)
						if w.tickFunc != nil {
							w.tickFunc()
						}
						break // one tick per cycle is enough; skip remaining sources
					}
				}
			}
		}
	}()

	logrus.Infof("CLI Watcher: Started")
}

// Stop terminates the watcher loop
func (w *cliWatcher) Stop() {
	w.mu.Lock()
	wasStarted := w.started
	if wasStarted {
		w.started = false
	}
	w.mu.Unlock()

	if !wasStarted {
		return
	}

	// Use sync.Once to ensure channel is only closed once, even if Stop() called concurrently
	w.stopOnce.Do(func() {
		close(w.stopChan)
	})
}

// Finds the CLI Watcher configuration file in:
// 1. Use explicit override by using "CLI_ACTIVITY_TRACKER_CONFIG" env. variable, or if not set then
// 2. Search for '.noidle' upward from current project directory up to "PROJECTS_ROOT" directory, or
// 3. Fallback to $HOME/.<binary> file, or if doesn't exist/isn't accessble then
// 4. Otherwise, give up. Repeating the search on next run (thus waiting for a config to appear)
func getConfigPath() string {

	// 1. Use explicit override
	if configEnv := os.Getenv("CLI_ACTIVITY_TRACKER_CONFIG"); configEnv != "" {
		return configEnv
	}

	const configFileName = ".noidle"

	// 2. Search upward from current project directory
	root := os.Getenv("PROJECTS_ROOT")
	if root == "" {
		root = "/"
	}

	start := os.Getenv("PROJECT_SOURCE")
	if start == "" {
		start = os.Getenv("PROJECTS_ROOT")
	}

	if start == "" {
		start, _ = os.Getwd()
	}

	if path := findUpward(start, root, configFileName); path != "" {
		return path
	}

	// 3. Fallback to $HOME/.<binary>
	if home := os.Getenv("HOME"); home != "" && home != "/" {
		homeCfg := filepath.Join(home, configFileName)
		if _, err := os.Stat(homeCfg); err == nil {
			return homeCfg
		}
	}

	// 4. Give up
	return ""
}

func findUpward(start, stop, filename string) string {
	const maxIterations = 100 // Safety limit to prevent infinite loops

	// Resolve symlinks in start path to ensure consistent traversal
	current, err := filepath.EvalSymlinks(start)
	if err != nil {
		// If symlink resolution fails (e.g., broken symlink), use original path
		current = start
	}

	// Also resolve stop to ensure comparison works correctly
	stopResolved, err := filepath.EvalSymlinks(stop)
	if err != nil {
		// If symlink resolution fails, use original path
		stopResolved = stop
	}

	for i := 0; i < maxIterations; i++ {
		candidate := filepath.Join(current, filename)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}

		if current == stopResolved || current == "/" {
			break
		}

		parent := filepath.Dir(current)
		if parent == current { // root reached
			break
		}
		current = parent
	}
	return ""
}

// Loads `.noidle` configuration file (or the one that is specified in ” environment variable) into the CLI Watcher configuration struct.
// Example configuraiton file:
// ```yaml
//
//	enabled: true
//	checkPeriodSeconds: 30
//	watchedCommands:
//	  - helm
//	  - odo
//	  - sleep
//
// ````
func (w *cliWatcher) loadConfig(path string, current *cliWatcherConfig) (*cliWatcherConfig, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		if current != nil && current._fromFile {
			logrus.Infof("CLI Watcher: Config file at %s was removed, stopping config-based detection", path)
		} else if !w.warnedMissingConfig {
			if strings.TrimSpace(path) == "" {
				logrus.Infof("CLI Watcher: Config file not found, waiting for it to appear...")
			} else {
				logrus.Infof("CLI Watcher: Config file not found at %s, waiting for it to appear...", path)
			}
			w.warnedMissingConfig = true
		}

		// Already on env vars + defaults, nothing changed — stay quiet
		if current != nil && !current._fromFile {
			return current, nil
		}

		// No .noidle file — build config from env vars and defaults only
		var defaultCfg cliWatcherConfig
		defaultCfg = applyDefaults(defaultCfg, w.idleTimeout)
		defaultCfg = w.applyEnvCeilings(defaultCfg, false)
		return &defaultCfg, nil
	} else if err != nil {
		return current, fmt.Errorf("CLI Watcher: Failed to stat config file: %w", err)
	}

	if w.warnedMissingConfig {
		logrus.Infof("CLI Watcher: Config file appeared at %s", path)
		w.warnedMissingConfig = false
	}

	if current != nil && !info.ModTime().After(current._lastModTime) {
		return current, nil // no change
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return current, fmt.Errorf("CLI Watcher: Failed to read config file: %w", err)
	}

	var newCfg cliWatcherConfig
	if err := yaml.Unmarshal(data, &newCfg); err != nil {
		// Log helpful error with context
		logrus.Errorf("CLI Watcher: Failed to parse config file at %s", path)
		logrus.Errorf("  Error: %v", err)
		logrus.Errorf("  Hint: Check that 'watchedCommands' entries are either strings or objects with 'name:' field")
		if current != nil {
			logrus.Errorf("  Keeping previous valid config until syntax is fixed.")
		}
		// Return error so caller can distinguish "config broken" from "config unchanged"
		return current, fmt.Errorf("failed to parse config file: %w", err)
	}

	newCfg._lastModTime = info.ModTime()
	newCfg._fromFile = true
	newCfg = applyDefaults(newCfg, w.idleTimeout)
	newCfg = ignoreExclusions(alwaysIgnoredCommands, newCfg)
	newCfg = w.applyEnvCeilings(newCfg, true)

	// Log config changes
	logrus.Infof("CLI Watcher: Config reloaded from %s", path)
	if current != nil && current.Enabled {
		if current._checkPeriodParsed != newCfg._checkPeriodParsed {
			logrus.Infof("CLI Watcher:   Check period changed: %v → %v", current._checkPeriodParsed, newCfg._checkPeriodParsed)
		}
		if current._activityWindowParsed != newCfg._activityWindowParsed {
			logrus.Infof("CLI Watcher:   Activity window changed: %v → %v", current._activityWindowParsed, newCfg._activityWindowParsed)
		}
		if current._gracePeriodParsed != newCfg._gracePeriodParsed {
			logrus.Infof("CLI Watcher:   Grace period changed: %v → %v", current._gracePeriodParsed, newCfg._gracePeriodParsed)
		}
		if current._maxProcessAgeParsed != newCfg._maxProcessAgeParsed {
			logrus.Infof("CLI Watcher:   Max process age changed: %v → %v", current._maxProcessAgeParsed, newCfg._maxProcessAgeParsed)
		}
	}

	if newCfg.Enabled {
		if len(newCfg.WatchedCommands) > 0 {
			logrus.Infof("CLI Watcher:   Watching ALL user processes with %d explicit override(s):", len(newCfg.WatchedCommands))
			for _, cmd := range newCfg.WatchedCommands {
				modeDesc := getInteractiveModeDescription(cmd.Interactive)
				logrus.Infof("CLI Watcher:     - %s (mode: %s)", cmd.Name, modeDesc)
			}
		} else {
			logrus.Infof("CLI Watcher:   Watching ALL user processes (no explicit overrides)")
		}
		if len(newCfg.IgnoredCommands) > 0 {
			logrus.Warnf("CLI Watcher:   WARNING: You configured %v in watchedCommands, but these are globally excluded (always ignored). Remove them from your config to silence this warning.", newCfg.IgnoredCommands)
		}
		logrus.Infof("CLI Watcher:   Always-ignored commands (never prevent idling): %v", alwaysIgnoredCommands)
		logrus.Infof("CLI Watcher:   Detection period: %v", newCfg._checkPeriodParsed)
		logrus.Infof("CLI Watcher:   Activity window: %v", newCfg._activityWindowParsed)
		logrus.Infof("CLI Watcher:   Grace period: %v", newCfg._gracePeriodParsed)
		logrus.Infof("CLI Watcher:   Max process age: %v (safety limit)", newCfg._maxProcessAgeParsed)
	} else {
		logrus.Infof("CLI Watcher: Disabled by configuration. CLI idling prevention is turned off.")
	}

	return &newCfg, nil
}

// Remove excluded CLIs from the watcher configuration.
func ignoreExclusions(exclusions []string, cfg cliWatcherConfig) cliWatcherConfig {
	var filtered []WatchedCommand
	var ignored []string

	for _, cmd := range cfg.WatchedCommands {
		name := strings.ToLower(strings.TrimSpace(cmd.Name))
		isAlwaysIgnored := slices.ContainsFunc(exclusions, func(ex string) bool {
			return strings.EqualFold(strings.TrimSpace(ex), name)
		})

		if isAlwaysIgnored && !cmd.ForceWatch.isEnabled() {
			// Command is in always-ignored list and no override specified
			ignored = append(ignored, cmd.Name)
			continue
		} else if isAlwaysIgnored && cmd.ForceWatch.isEnabled() {
			// User explicitly wants to watch this normally-ignored command
			logrus.Warnf("CLI Watcher: Command '%s' is normally always-ignored but forceWatch=true overrides this. Use with caution.", cmd.Name)
		}

		filtered = append(filtered, cmd)
	}

	cfg.WatchedCommands = filtered
	// Preserve user-specified ignoredCommands and add filtered always-ignored ones
	cfg.IgnoredCommands = append(cfg.IgnoredCommands, ignored...)
	return cfg
}

// parseDuration parses a duration string or integer (treated as seconds)
func parseDuration(value string, fieldName string, defaultValue time.Duration) time.Duration {
	if value == "" {
		return defaultValue
	}

	// Try parsing as duration first (e.g., "6h", "30m", "3600s")
	duration, err := time.ParseDuration(value)
	if err != nil {
		// Fallback: try parsing as integer seconds (e.g., "21600" or "60")
		// Use strconv.ParseInt to ensure the ENTIRE string is numeric and avoid 32-bit overflow
		seconds, atoiErr := strconv.ParseInt(value, 10, 64)
		if atoiErr == nil && seconds > 0 {
			// Prevent time.Duration overflow: max safe value is ~292 years
			const maxSafeSeconds = int64(9223372036) // math.MaxInt64 / 1e9, rounded down
			if seconds > maxSafeSeconds {
				logrus.Warnf("CLI Watcher: %s value '%s' (%d seconds) too large (max ~292 years), using default (%v)", fieldName, value, seconds, defaultValue)
				return defaultValue
			}
			duration = time.Duration(seconds) * time.Second
		} else {
			// Invalid value - warn and use default
			logrus.Warnf("CLI Watcher: Invalid %s value '%s' (not a duration or integer), using default (%v)", fieldName, value, defaultValue)
			return defaultValue
		}
	}

	if duration <= 0 {
		logrus.Warnf("CLI Watcher: %s is zero or negative (%v), using default (%v)", fieldName, duration, defaultValue)
		return defaultValue
	}

	// Add reasonable upper bounds to prevent misconfiguration or potential DoS
	var maxAllowed time.Duration
	switch fieldName {
	case "checkPeriod":
		maxAllowed = 1 * time.Hour // No point checking less than once per hour
	case "activityWindow":
		maxAllowed = 24 * time.Hour // Activity windows longer than a day are impractical
	case "gracePeriod":
		maxAllowed = 1 * time.Hour // Grace periods longer than an hour are excessive
	case "maxProcessAge":
		maxAllowed = 7 * 24 * time.Hour // Week-long processes are likely stuck
	default:
		maxAllowed = 24 * time.Hour // Default maximum for unknown fields
	}

	if duration > maxAllowed {
		logrus.Warnf("CLI Watcher: %s value '%s' (%v) exceeds maximum (%v), using default (%v)", fieldName, value, duration, maxAllowed, defaultValue)
		return defaultValue
	}

	return duration
}

// applyDefaults sets fallback values (user values are never changed, only unspecified fields get smart defaults)
func applyDefaults(c cliWatcherConfig, idleTimeout time.Duration) cliWatcherConfig {
	// Parse checkPeriod (new field takes priority over deprecated checkPeriodSeconds)
	if c.CheckPeriod != "" {
		c._checkPeriodParsed = parseDuration(c.CheckPeriod, "checkPeriod", time.Duration(DefaultCheckPeriod)*time.Second)

		// Warn if both old and new fields are specified with different values
		if c.CheckPeriodSeconds > 0 {
			deprecatedValue := time.Duration(c.CheckPeriodSeconds) * time.Second
			if c._checkPeriodParsed != deprecatedValue {
				logrus.Warnf("CLI Watcher: Both 'checkPeriod' (%v) and deprecated 'checkPeriodSeconds' (%v) are set - using 'checkPeriod' value", c._checkPeriodParsed, deprecatedValue)
			}
		}
	} else if c.CheckPeriodSeconds > 0 {
		// Backward compatibility: use deprecated checkPeriodSeconds
		c._checkPeriodParsed = time.Duration(c.CheckPeriodSeconds) * time.Second
	} else {
		c._checkPeriodParsed = time.Duration(DefaultCheckPeriod) * time.Second
	}

	// Validate check period bounds (must be done AFTER parsing both fields)
	minCheckPeriod := time.Duration(MinCheckPeriod) * time.Second
	if c._checkPeriodParsed < minCheckPeriod {
		logrus.Warnf("CLI Watcher: checkPeriod (%v) is below minimum (%v), using minimum", c._checkPeriodParsed, minCheckPeriod)
		c._checkPeriodParsed = minCheckPeriod
	}

	// Maximum check period should be reasonable and less than idle timeout
	// Absolute max: 10 minutes (no point checking less frequently)
	// If idleTimeout known: max 1/4 of idle timeout (ensure we can detect activity in time)
	var maxCheckPeriod time.Duration
	if idleTimeout > 0 {
		maxCheckPeriod = idleTimeout / 4
		if maxCheckPeriod > 10*time.Minute {
			maxCheckPeriod = 10 * time.Minute
		}
	} else {
		maxCheckPeriod = 10 * time.Minute
	}

	if c._checkPeriodParsed > maxCheckPeriod {
		if idleTimeout > 0 {
			logrus.Warnf("CLI Watcher: checkPeriod (%v) exceeds maximum (%v, 1/4 of idle timeout %v), using maximum", c._checkPeriodParsed, maxCheckPeriod, idleTimeout)
		} else {
			logrus.Warnf("CLI Watcher: checkPeriod (%v) exceeds maximum (%v), using maximum", c._checkPeriodParsed, maxCheckPeriod)
		}
		c._checkPeriodParsed = maxCheckPeriod
	}

	// Parse gracePeriod (needed first to calculate activityWindow)
	var gracePeriodDefault time.Duration
	if c.GracePeriod == "" && idleTimeout > 0 {
		// Smart default: use smaller of 5m or 15% of idle timeout
		gracePeriodDefault = time.Duration(float64(idleTimeout) * 0.15)
		if gracePeriodDefault > DefaultGracePeriod {
			gracePeriodDefault = DefaultGracePeriod
		}
		if gracePeriodDefault < MinGracePeriod {
			gracePeriodDefault = MinGracePeriod
		}
	} else {
		gracePeriodDefault = DefaultGracePeriod
	}
	c._gracePeriodParsed = parseDuration(c.GracePeriod, "gracePeriod", gracePeriodDefault)

	// Parse activityWindow (depends on gracePeriod and idleTimeout)
	var activityWindowDefault time.Duration
	if c.ActivityWindow == "" && idleTimeout > 0 {
		// Smart default: idleTimeout - gracePeriod - buffer
		buffer := time.Duration(float64(idleTimeout) * SafetyBufferPercent)
		if buffer > SafetyBufferDuration {
			buffer = SafetyBufferDuration
		}

		calculated := idleTimeout - c._gracePeriodParsed - buffer
		if calculated < MinActivityWindow {
			activityWindowDefault = MinActivityWindow
			if calculated <= 0 {
				logrus.Warnf("CLI Watcher: Grace period (%v) + buffer (%v) exceeds idle timeout (%v), using minimum activity window (%v)", c._gracePeriodParsed, buffer, idleTimeout, MinActivityWindow)
			} else if idleTimeout < 10*time.Minute {
				logrus.Warnf("CLI Watcher: Workspace idle timeout (%v) is very short, using minimum activity window (%v)", idleTimeout, MinActivityWindow)
			} else {
				logrus.Warnf("CLI Watcher: Calculated activity window too short (%v), using minimum (%v)", calculated, MinActivityWindow)
			}
		} else {
			activityWindowDefault = calculated
		}
	} else if c.ActivityWindow == "" && c._gracePeriodParsed > 0 {
		// No idleTimeout but gracePeriod specified: ensure activityWindow > gracePeriod
		activityWindowDefault = c._gracePeriodParsed + 2*time.Minute
		if activityWindowDefault < DefaultActivityWindow {
			activityWindowDefault = DefaultActivityWindow
		}
	} else {
		activityWindowDefault = DefaultActivityWindow
	}
	c._activityWindowParsed = parseDuration(c.ActivityWindow, "activityWindow", activityWindowDefault)

	// Parse maxProcessAge
	c._maxProcessAgeParsed = parseDuration(c.MaxProcessAge, "maxProcessAge", DefaultMaxProcessAge)

	// Apply defaults to each watched command
	for i := range c.WatchedCommands {
		if c.WatchedCommands[i].Interactive == "" {
			c.WatchedCommands[i].Interactive = DefaultInteractiveMode
		}
	}

	// Validate configuration (warn about misconfigurations but never change user values)
	if c._activityWindowParsed < c._gracePeriodParsed {
		logrus.Warnf("CLI Watcher: activityWindow (%v) is less than gracePeriod (%v), interactive processes may not be detected correctly", c._activityWindowParsed, c._gracePeriodParsed)
	}

	if idleTimeout > 0 {
		if c._activityWindowParsed >= idleTimeout {
			logrus.Warnf("CLI Watcher: activityWindow (%v) exceeds workspace idle timeout (%v), may not work as expected", c._activityWindowParsed, idleTimeout)
		}
		if c._gracePeriodParsed >= idleTimeout*8/10 {
			logrus.Warnf("CLI Watcher: gracePeriod (%v) is very close to workspace idle timeout (%v)", c._gracePeriodParsed, idleTimeout)
		}
	}

	checkPeriodDuration := c._checkPeriodParsed
	if checkPeriodDuration > c._activityWindowParsed/2 {
		logrus.Warnf("CLI Watcher: checkPeriod (%v) may be too long for activityWindow (%v), activity might not be detected in time", checkPeriodDuration, c._activityWindowParsed)
	}

	return c
}

func (w *cliWatcher) applyEnvCeilings(c cliWatcherConfig, noidle bool) cliWatcherConfig {
	env := &w.envConfig

	// --- enabled: always admin-controlled ---
	resolvedEnabled := DefaultCliWatcherEnabled
	if env.enabled != nil {
		resolvedEnabled = *env.enabled
	}

	if noidle && c.Enabled != resolvedEnabled {
		if env.enabled != nil {
			logrus.Infof("CLI Watcher: 'enabled' = %t (from %s; .noidle 'enabled: %t' rejected — deprecated, admin-controlled)", resolvedEnabled, EnvCliWatcherEnabled, c.Enabled)
		} else {
			logrus.Infof("CLI Watcher: 'enabled' = %t (default; .noidle 'enabled: %t' rejected — deprecated, use %s env var)", resolvedEnabled, c.Enabled, EnvCliWatcherEnabled)
		}
	} else if noidle {
		if env.enabled != nil {
			logrus.Infof("CLI Watcher: 'enabled' = %t (from %s; .noidle 'enabled' is deprecated — admin-controlled)", resolvedEnabled, EnvCliWatcherEnabled)
		} else {
			logrus.Infof("CLI Watcher: 'enabled' = %t (default; .noidle 'enabled' is deprecated — use %s env var)", resolvedEnabled, EnvCliWatcherEnabled)
		}
	} else {
		if env.enabled != nil {
			logrus.Infof("CLI Watcher: 'enabled' = %t (from %s)", resolvedEnabled, EnvCliWatcherEnabled)
		} else {
			logrus.Infof("CLI Watcher: 'enabled' = %t (default)", resolvedEnabled)
		}
	}
	c.Enabled = resolvedEnabled

	// --- verbose: admin-controlled activity-detection logging, no .noidle equivalent ---
	c._verbose = env.verbose != nil && *env.verbose

	// --- timing params: env var is ceiling, .noidle can only tighten ---
	applyDurationCeiling := func(fieldName, envName, noidleRaw string, envVal *time.Duration, parsed *time.Duration) {
		noifileSet := noidle && noidleRaw != ""
		if envVal != nil {
			if noifileSet && *parsed > *envVal {
				logrus.Infof("CLI Watcher: '%s' = %v (admin limit; .noidle value %v rejected — exceeds admin ceiling)", fieldName, *envVal, *parsed)
				*parsed = *envVal
			} else if noifileSet {
				logrus.Infof("CLI Watcher: '%s' = %v (from .noidle; within admin limit %v)", fieldName, *parsed, *envVal)
			} else {
				logrus.Infof("CLI Watcher: '%s' = %v (from %s)", fieldName, *envVal, envName)
				*parsed = *envVal
			}
		} else {
			if noifileSet {
				logrus.Infof("CLI Watcher: '%s' = %v (from .noidle)", fieldName, *parsed)
			} else {
				logrus.Infof("CLI Watcher: '%s' = %v (default)", fieldName, *parsed)
			}
		}
	}

	applyDurationCeiling("checkPeriod", EnvCliWatcherCheckPeriod, c.CheckPeriod, env.checkPeriod, &c._checkPeriodParsed)
	applyDurationCeiling("activityWindow", EnvCliWatcherActivityWindow, c.ActivityWindow, env.activityWindow, &c._activityWindowParsed)
	applyDurationCeiling("gracePeriod", EnvCliWatcherGracePeriod, c.GracePeriod, env.gracePeriod, &c._gracePeriodParsed)
	applyDurationCeiling("maxProcessAge", EnvCliWatcherMaxProcessAge, c.MaxProcessAge, env.maxProcessAge, &c._maxProcessAgeParsed)

	return c
}

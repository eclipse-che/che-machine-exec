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
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// ActivityScanContext carries the resolved, per-cycle configuration a
// ActivitySource needs to decide whether its target(s) are active. Fields
// are not all meaningful to every source — e.g. MaxProcessAge is a
// safety-cutoff concept the tty source honors but a long-lived daemon
// source may deliberately ignore; WatchedCommands/IgnoredCommands are
// tty-source-specific (named-command overrides) and other sources ignore
// them.
type ActivityScanContext struct {
	MyPID           string
	ActivityWindow  time.Duration
	GracePeriod     time.Duration
	MaxProcessAge   time.Duration
	Verbose         bool
	WatchedCommands []WatchedCommand
	IgnoredCommands []string
}

// ActivitySource detects activity for one kind of watched channel (e.g. TTY
// processes, a codex app-server socket). Scan performs one full detection
// pass and reports whether it found activity; it does not report ticks
// itself — that is the caller's (cliWatcher's) responsibility, so a single
// tick per scan cycle is reported regardless of how many sources are active.
type ActivitySource interface {
	// Name returns the canonical, lowercase identifier for this source
	// (e.g. "tty", "codex-app-server-hooks"), used in admin config
	// (CLI_ACTIVITY_TRACKER_ACTIVITY_SOURCES), logging, and registry lookup.
	Name() string

	// Scan reports whether this source's target(s) are currently active,
	// along with a human-readable label describing what was found, for
	// logging.
	Scan(ctx ActivityScanContext) (active bool, label string)
}

// allActivitySources is the compiled-in registry of activity sources, in
// declaration order. This order is BOTH the default admin-config-listing
// order AND the default scan order when CLI_ACTIVITY_TRACKER_ACTIVITY_SOURCES
// doesn't otherwise reorder things (see resolveActiveActivitySources) — this
// is the one place a new in-tree source gets wired in.
var allActivitySources = []ActivitySource{
	newTTYActivitySource(),
	newcodexAppServerHooksActivitySource(),
}

// defaultOnActivitySources lists sources active by default, even when
// CLI_ACTIVITY_TRACKER_ACTIVITY_SOURCES doesn't mention them at all. An
// admin can still turn one off explicitly via "name:disabled".
var defaultOnActivitySources = map[string]bool{
	"tty": true,
}

// activitySourceSelector is one parsed, validated entry from
// CLI_ACTIVITY_TRACKER_ACTIVITY_SOURCES: a source name plus whether it
// should be enabled (default true when no flag is given).
type activitySourceSelector struct {
	name    string
	enabled bool
}

// validActivitySourceNamePattern is an allow-list (not a deny-list) for
// activity source identifiers: letters, digits, and a handful of
// "segment"-friendly separators (., _, /, @, #, $, &, -). Deliberately
// excludes anything that could interfere with splitting
// CLI_ACTIVITY_TRACKER_ACTIVITY_SOURCES on "," or a name:flag entry on ":"
// (commas, semicolons, quotes, colons, whitespace, etc.).
var validActivitySourceNamePattern = regexp.MustCompile(`^[A-Za-z0-9._/@#$&-]+$`)

// parseActivitySourcesEnv parses CLI_ACTIVITY_TRACKER_ACTIVITY_SOURCES, a
// comma-separated list of "name" or "name:flag" entries (flag one of
// enabled/disabled/true/false, case-insensitive; default enabled when no
// flag is given). Malformed entries and duplicates are dropped with a
// warning rather than failing — CLI Watcher never crashes/exits over admin
// config typos. Registry lookup (does the name correspond to a compiled-in
// source) happens later, in resolveActiveActivitySources, once the registry
// is known.
func parseActivitySourcesEnv(raw string) []activitySourceSelector {
	var result []activitySourceSelector
	seen := make(map[string]bool)

	for _, token := range strings.Split(raw, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		parts := strings.Split(token, ":")
		var namePart, flagPart string
		switch len(parts) {
		case 1:
			namePart = parts[0]
		case 2:
			namePart, flagPart = parts[0], parts[1]
		default:
			logrus.Warnf("CLI Watcher: Malformed %s entry %q (more than one ':'), expected name[:enabled|disabled], ignoring", EnvCliWatcherActivitySources, token)
			continue
		}

		name := strings.ToLower(strings.TrimSpace(namePart))
		if !validActivitySourceNamePattern.MatchString(name) {
			logrus.Warnf("CLI Watcher: Invalid activity source identifier %q in %s (must match %s), ignoring", name, EnvCliWatcherActivitySources, validActivitySourceNamePattern.String())
			continue
		}

		enabled := true
		if flagPart != "" {
			switch strings.ToLower(strings.TrimSpace(flagPart)) {
			case "enabled", "true":
				enabled = true
			case "disabled", "false":
				enabled = false
			default:
				logrus.Warnf("CLI Watcher: Invalid flag %q for activity source %q in %s (expected enabled|disabled|true|false), ignoring entry", flagPart, name, EnvCliWatcherActivitySources)
				continue
			}
		}

		if seen[name] {
			logrus.Warnf("CLI Watcher: Duplicate activity source %q in %s, first occurrence wins", name, EnvCliWatcherActivitySources)
			continue
		}
		seen[name] = true

		result = append(result, activitySourceSelector{name: name, enabled: enabled})
	}

	return result
}

// resolveActiveActivitySources computes the final, ordered set of active
// activity sources from the compiled-in registry and the admin's parsed
// selector list, per the two-pass rule: explicit order first (as given in
// CLI_ACTIVITY_TRACKER_ACTIVITY_SOURCES), then any default-on source not
// mentioned at all, appended in registry order. Unknown selector names are
// returned as warnings rather than logged directly, keeping this function
// pure/testable.
func resolveActiveActivitySources(all []ActivitySource, selectors []activitySourceSelector) (active []ActivitySource, warnings []string) {
	byName := make(map[string]ActivitySource, len(all))
	validNames := make([]string, 0, len(all))
	for _, src := range all {
		name := strings.ToLower(src.Name())
		byName[name] = src
		validNames = append(validNames, name)
	}

	mentioned := make(map[string]bool, len(selectors))

	// Explicit pass: admin's order wins.
	for _, sel := range selectors {
		mentioned[sel.name] = true
		src, ok := byName[sel.name]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("Unknown activity source %q in %s (valid: %s), ignoring", sel.name, EnvCliWatcherActivitySources, strings.Join(validNames, ", ")))
			continue
		}
		if !sel.enabled {
			continue
		}
		active = append(active, src)
	}

	// Implicit pass: default-on sources not mentioned at all, in registry order.
	for _, src := range all {
		name := strings.ToLower(src.Name())
		if mentioned[name] {
			continue
		}
		if defaultOnActivitySources[name] {
			active = append(active, src)
		}
	}

	return active, warnings
}

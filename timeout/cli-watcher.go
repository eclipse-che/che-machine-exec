//
// Copyright (c) 2025 Red Hat, Inc.
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
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type cliWatcherConfig struct {
	WatchedCommands    []string  `yaml:"watchedCommands"`
	IgnoredCommands    []string  `json:"-"`
	CheckPeriodSeconds int       `yaml:"checkPeriodSeconds"`
	Enabled            bool      `yaml:"enabled"`
	_lastModTime       time.Time `json:"-"`
}

// Watcher monitors CLI processes and invokes a tick callback when active ones are found
type cliWatcher struct {
	config              *cliWatcherConfig
	warnedMissingConfig bool
	stopChan            chan struct{}
	started             bool
	tickFunc            func()
}

// CLIs that should never prevent idling
var excludedCommands = []string{"tail"}

// New creates a new Watcher with the given config and tick callback
func NewCliWatcher(tickFunc func()) *cliWatcher {
	return &cliWatcher{
		stopChan: make(chan struct{}),
		tickFunc: tickFunc,
	}
}

// Start begins the watcher loop
func (w *cliWatcher) Start() {
	if w.started {
		return
	}
	w.started = true

	go func() {
		var err error
		w.config, err = w.loadConfig(getConfigPath(), w.config)
		if err != nil {
			logrus.Errorf("CLI Watcher: Failed to reload config: %v", err)
		}

		chkPeriod := 60
		if w.config != nil {
			chkPeriod = w.config.CheckPeriodSeconds
		}

		ticker := time.NewTicker(time.Duration(chkPeriod) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-w.stopChan:
				logrus.Infof("CLI Watcher: Stopped")
				return

			case <-ticker.C:
				oldPeriod := chkPeriod

				// Reload config
				w.config, err = w.loadConfig(getConfigPath(), w.config)
				if err != nil {
					logrus.Errorf("CLI Watcher: Failed to reload config: %v", err)
				}

				if w.config == nil || !w.config.Enabled {
					if chkPeriod != 60 {
						logrus.Infof("CLI Watcher: Config was removed or disabled — resetting check period to default (60s)")
						chkPeriod = 60
						ticker.Stop()
						ticker = time.NewTicker(time.Duration(chkPeriod) * time.Second)
					}
					continue
				}

				if w.config.CheckPeriodSeconds > 0 && w.config.CheckPeriodSeconds != oldPeriod {
					logrus.Infof("CLI Watcher: Detected new check period: %d seconds (was %d), restarting ticker", w.config.CheckPeriodSeconds, oldPeriod)
					chkPeriod = w.config.CheckPeriodSeconds
					ticker.Stop()
					ticker = time.NewTicker(time.Duration(chkPeriod) * time.Second)
				}

				found, name := isWatchedProcessRunning(w.config.WatchedCommands)
				if found {
					logrus.Infof("CLI Watcher: Detected CLI command: %s — reporting activity tick", name)
					if w.tickFunc != nil {
						w.tickFunc()
					}
				}
			}
		}
	}()

	logrus.Infof("CLI Watcher: Started")
}

// Stop terminates the watcher loop
func (w *cliWatcher) Stop() {
	if !w.started {
		return
	}
	close(w.stopChan)
	w.started = false
}

// Scans /proc to check if any watched process is running
func isWatchedProcessRunning(watched []string) (bool, string) {
	procEntries, err := os.ReadDir("/proc")
	if err != nil {
		logrus.Warnf("CLI Watcher: Cannot read /proc: %v", err)
		return false, ""
	}

	for _, entry := range procEntries {
		if !entry.IsDir() || !isNumeric(entry.Name()) {
			continue
		}

		pid := entry.Name()
		if pid == "1" { // Skip PID 1 (main container process)
			continue
		}

		cmdlinePath := filepath.Join("/proc", pid, "cmdline")
		data, err := os.ReadFile(cmdlinePath)
		if err != nil || len(data) == 0 {
			continue
		}

		cmdParts := strings.Split(string(data), "\x00")
		if len(cmdParts) == 0 {
			continue
		}

		// Match against all command line parts, not just the first
		for _, part := range cmdParts {
			partName := filepath.Base(part)
			for _, keyword := range watched {
				if partName == keyword {
					return true, keyword
				}
			}
		}
	}

	return false, ""
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Finds the CLI Watcher configuration file in:
// 1. Use explicit override by using "CLI_WATCHER_CONFIG" env. variable, or if not set then
// 2. Search for '.noidle' upward from current project directory up to "PROJECTS_ROOT" directory, or
// 3. Fallback to $HOME/.<binary> file, or if doesn't exist/isn't accessble then
// 4. Otherwise, give up. Repeating the search on next run (thus waiting for a config to appear)
func getConfigPath() string {

	// 1. Use explicit override
	if configEnv := os.Getenv("CLI_WATCHER_CONFIG"); configEnv != "" {
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
	current := start
	for {
		candidate := filepath.Join(current, filename)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}

		if current == stop || current == "/" {
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
		if current != nil {
			logrus.Infof("CLI Watcher: Config file at %s was removed, stopping config-based detection", path)
		} else if !w.warnedMissingConfig {
			if strings.TrimSpace(path) == "" {
				logrus.Infof("CLI Watcher: Config file not found, waiting for it to appear...")
			} else {
				logrus.Infof("CLI Watcher: Config file not found at %s, waiting for it to appear...", path)
			}
			w.warnedMissingConfig = true
		}
		return nil, nil
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
		return current, fmt.Errorf("CLI Watcher: Failed to parse config file: %w", err)
	}

	newCfg._lastModTime = info.ModTime()
	newCfg = applyDefaults(newCfg)
	newCfg = ignoreExclusions(excludedCommands, newCfg)

	logrus.Infof("CLI Watcher: Config reloaded from %s", path)
	if newCfg.Enabled {
		logrus.Infof("CLI Watcher:   Detecting active commands: %v...", newCfg.WatchedCommands)
		if len(newCfg.IgnoredCommands) > 0 {
			logrus.Infof("CLI Watcher:   Skipping watch for: %v...", newCfg.IgnoredCommands)
		}
		logrus.Infof("CLI Watcher:   Detection period is %d seconds", newCfg.CheckPeriodSeconds)
	} else {
		logrus.Infof("CLI Watcher: Disabled by configuration. CLI idling prevention is turned off.")
	}

	return &newCfg, nil
}

// Remove excluded CLIs from the watcher configuration.
func ignoreExclusions(exclusions []string, cfg cliWatcherConfig) cliWatcherConfig {
	var filtered, ignored []string

	for _, cmd := range cfg.WatchedCommands {
		name := strings.ToLower(strings.TrimSpace(cmd))
		if slices.ContainsFunc(exclusions, func(ex string) bool {
			return strings.EqualFold(strings.TrimSpace(ex), name)
		}) {
			ignored = append(ignored, cmd)
			continue
		}
		filtered = append(filtered, cmd)
	}

	cfg.WatchedCommands = filtered
	cfg.IgnoredCommands = ignored
	return cfg
}

// applyDefaults sets fallback values
func applyDefaults(c cliWatcherConfig) cliWatcherConfig {
	if c.CheckPeriodSeconds <= 0 {
		c.CheckPeriodSeconds = 60
	}
	return c
}

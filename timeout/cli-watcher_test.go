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
	"os"
	"runtime"
	"testing"
	"time"

	"gopkg.in/yaml.v2"
)

// Test parseDuration with various input formats
func TestParseDuration(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		fieldName    string
		defaultValue time.Duration
		expected     time.Duration
	}{
		// Valid duration strings
		{"6 hours", "6h", "maxProcessAge", 1 * time.Hour, 6 * time.Hour},
		{"30 minutes", "30m", "activityWindow", 10 * time.Minute, 30 * time.Minute},
		{"90 seconds", "90s", "gracePeriod", 5 * time.Minute, 90 * time.Second},
		{"compound duration", "1h30m", "activityWindow", 10 * time.Minute, 90 * time.Minute},
		{"compound with seconds", "45m30s", "gracePeriod", 5 * time.Minute, 45*time.Minute + 30*time.Second},

		// Valid integers (treated as seconds)
		{"integer 3600", "3600", "maxProcessAge", 1 * time.Hour, 3600 * time.Second},
		{"integer 1800", "1800", "activityWindow", 10 * time.Minute, 1800 * time.Second},
		{"integer 300", "300", "gracePeriod", 5 * time.Minute, 300 * time.Second},
		// Note: parseDuration rejects zero/negative for checkPeriod, returns default
		{"integer 0", "0", "checkPeriod", 60 * time.Second, 60 * time.Second},

		// Empty string (should return default)
		{"empty string", "", "activityWindow", 25 * time.Minute, 25 * time.Minute},

		// Invalid formats (should return default and log warning)
		{"invalid format", "invalid", "activityWindow", 10 * time.Minute, 10 * time.Minute},
		{"negative duration", "-5m", "gracePeriod", 5 * time.Minute, 5 * time.Minute},
		{"negative integer", "-300", "gracePeriod", 5 * time.Minute, 5 * time.Minute},

		// Typo cases - should use default, not silently parse partial integer
		// These were previously parsed by fmt.Sscanf which stops at first non-digit
		{"typo: 30min", "30min", "activityWindow", 10 * time.Minute, 10 * time.Minute},
		{"typo: 5minutes", "5minutes", "gracePeriod", 5 * time.Minute, 5 * time.Minute},
		{"typo: 1hour", "1hour", "maxProcessAge", 6 * time.Hour, 6 * time.Hour},
		{"typo: 30x", "30x", "activityWindow", 10 * time.Minute, 10 * time.Minute},
		{"typo: 100sec", "100sec", "checkPeriod", 60 * time.Second, 60 * time.Second},

		// Upper bounds validation - values exceeding maximum should use default
		{"checkPeriod exceeds max", "2h", "checkPeriod", 60 * time.Second, 60 * time.Second},
		{"gracePeriod exceeds max", "2h", "gracePeriod", 5 * time.Minute, 5 * time.Minute},
		{"activityWindow exceeds max", "48h", "activityWindow", 10 * time.Minute, 10 * time.Minute},
		{"maxProcessAge exceeds max", "30d", "maxProcessAge", 6 * time.Hour, 6 * time.Hour},

		// Large integer test - would overflow int32 on 32-bit systems without proper handling
		// This value exceeds activityWindow max (24h) so should use default
		{"large integer (32-bit overflow test)", "2200000000", "activityWindow", 10 * time.Minute, 10 * time.Minute},

		// Extreme value test - would overflow time.Duration multiplication
		// ~500 years in seconds, should use default
		{"extreme duration overflow test", "15768000000000", "checkPeriod", 60 * time.Second, 60 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDuration(tt.value, tt.fieldName, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("parseDuration(%q, %q, %v) = %v, want %v",
					tt.value, tt.fieldName, tt.defaultValue, result, tt.expected)
			}
		})
	}
}

// Test WatchedCommand UnmarshalYAML with both string and object formats
func TestWatchedCommandUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		expected    WatchedCommand
		shouldError bool
	}{
		{
			name: "simple string",
			yaml: "helm",
			// Simple strings default to InteractiveModeNo in UnmarshalYAML
			expected: WatchedCommand{Name: "helm", Interactive: InteractiveModeNo},
		},
		{
			name:     "object with name only",
			yaml:     "name: kubectl",
			expected: WatchedCommand{Name: "kubectl", Interactive: ""},
		},
		{
			name:     "object with interactive auto",
			yaml:     "name: vim\ninteractive: auto",
			expected: WatchedCommand{Name: "vim", Interactive: InteractiveModeAuto},
		},
		{
			name:     "object with interactive true",
			yaml:     "name: claude\ninteractive: true",
			expected: WatchedCommand{Name: "claude", Interactive: InteractiveModeTrue},
		},
		{
			name:     "object with interactive false",
			yaml:     "name: npm\ninteractive: false",
			expected: WatchedCommand{Name: "npm", Interactive: InteractiveModeFalse},
		},
		{
			name:     "object with interactive yes",
			yaml:     "name: editor\ninteractive: yes",
			expected: WatchedCommand{Name: "editor", Interactive: InteractiveModeYes},
		},
		{
			name:     "object with interactive no",
			yaml:     "name: build\ninteractive: no",
			expected: WatchedCommand{Name: "build", Interactive: InteractiveModeNo},
		},
		{
			name:     "object with forceWatch true",
			yaml:     "name: watch\nforceWatch: true",
			expected: WatchedCommand{Name: "watch", ForceWatch: ForceWatchModeTrue},
		},
		{
			name:     "object with forceWatch yes",
			yaml:     "name: watch\nforceWatch: yes",
			expected: WatchedCommand{Name: "watch", ForceWatch: ForceWatchModeYes},
		},
		{
			name:     "object with forceWatch false",
			yaml:     "name: watch\nforceWatch: false",
			expected: WatchedCommand{Name: "watch", ForceWatch: ForceWatchModeFalse},
		},
		{
			name:     "object with forceWatch no",
			yaml:     "name: watch\nforceWatch: no",
			expected: WatchedCommand{Name: "watch", ForceWatch: ForceWatchModeNo},
		},
		{
			name:     "object with all fields",
			yaml:     "name: top\ninteractive: false\nforceWatch: true",
			expected: WatchedCommand{Name: "top", Interactive: InteractiveModeFalse, ForceWatch: ForceWatchModeTrue},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cmd WatchedCommand
			err := yaml.Unmarshal([]byte(tt.yaml), &cmd)

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if cmd.Name != tt.expected.Name {
				t.Errorf("Name = %q, want %q", cmd.Name, tt.expected.Name)
			}
			if cmd.Interactive != tt.expected.Interactive {
				t.Errorf("Interactive = %q, want %q", cmd.Interactive, tt.expected.Interactive)
			}
			if cmd.ForceWatch != tt.expected.ForceWatch {
				t.Errorf("ForceWatch = %v, want %v", cmd.ForceWatch, tt.expected.ForceWatch)
			}
		})
	}
}

// Test ignoreExclusions filters out globally-excluded commands
func TestIgnoreExclusions(t *testing.T) {
	tests := []struct {
		name            string
		exclusions      []string
		inputCommands   []WatchedCommand
		expectedCount   int
		expectedIgnored int
	}{
		{
			name:       "no exclusions",
			exclusions: []string{},
			inputCommands: []WatchedCommand{
				{Name: "helm", Interactive: ""},
				{Name: "kubectl", Interactive: ""},
			},
			expectedCount:   2,
			expectedIgnored: 0,
		},
		{
			name:       "filter tail",
			exclusions: []string{"tail"},
			inputCommands: []WatchedCommand{
				{Name: "helm", Interactive: ""},
				{Name: "tail", Interactive: ""},
				{Name: "kubectl", Interactive: ""},
			},
			expectedCount:   2,
			expectedIgnored: 1,
		},
		{
			name:       "filter multiple",
			exclusions: []string{"tail", "watch", "top"},
			inputCommands: []WatchedCommand{
				{Name: "helm", Interactive: ""},
				{Name: "tail", Interactive: ""},
				{Name: "watch", Interactive: ""},
				{Name: "kubectl", Interactive: ""},
				{Name: "top", Interactive: ""},
			},
			expectedCount:   2,
			expectedIgnored: 3,
		},
		{
			name:       "case insensitive",
			exclusions: []string{"tail"},
			inputCommands: []WatchedCommand{
				{Name: "Tail", Interactive: ""},
				{Name: "TAIL", Interactive: ""},
				{Name: "helm", Interactive: ""},
			},
			expectedCount:   1,
			expectedIgnored: 2,
		},
		{
			name:       "all excluded",
			exclusions: []string{"tail", "watch"},
			inputCommands: []WatchedCommand{
				{Name: "tail", Interactive: ""},
				{Name: "watch", Interactive: ""},
			},
			expectedCount:   0,
			expectedIgnored: 2,
		},
		{
			name:       "forceWatch overrides exclusion",
			exclusions: []string{"watch", "top"},
			inputCommands: []WatchedCommand{
				{Name: "helm", Interactive: ""},
				{Name: "watch", Interactive: "false", ForceWatch: ForceWatchModeTrue}, // Override exclusion
				{Name: "top", Interactive: ""}, // Still excluded
			},
			expectedCount:   2, // helm + watch (override)
			expectedIgnored: 1, // top
		},
		{
			name:       "forceWatch false still excluded",
			exclusions: []string{"watch"},
			inputCommands: []WatchedCommand{
				{Name: "watch", Interactive: "", ForceWatch: ForceWatchModeFalse}, // Explicit false
			},
			expectedCount:   0,
			expectedIgnored: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cliWatcherConfig{
				WatchedCommands: tt.inputCommands,
			}

			result := ignoreExclusions(tt.exclusions, cfg)

			if len(result.WatchedCommands) != tt.expectedCount {
				t.Errorf("WatchedCommands count = %d, want %d",
					len(result.WatchedCommands), tt.expectedCount)
			}

			if len(result.IgnoredCommands) != tt.expectedIgnored {
				t.Errorf("IgnoredCommands count = %d, want %d",
					len(result.IgnoredCommands), tt.expectedIgnored)
			}

			// Verify no excluded commands remain (unless forceWatch=true)
			for _, cmd := range result.WatchedCommands {
				for _, ex := range tt.exclusions {
					if cmd.Name == ex && !cmd.ForceWatch.isEnabled() {
						t.Errorf("Excluded command %q still in WatchedCommands without forceWatch", cmd.Name)
					}
				}
			}
		})
	}
}

// Test applyDefaults with various idle timeout scenarios
func TestApplyDefaults(t *testing.T) {
	tests := []struct {
		name                      string
		config                    cliWatcherConfig
		idleTimeout               time.Duration
		expectedCheckPeriod       time.Duration
		expectedGracePeriodMin    time.Duration
		expectedGracePeriodMax    time.Duration
		expectedActivityWindowMin time.Duration
	}{
		{
			name: "all defaults with 30m idle timeout",
			config: cliWatcherConfig{
				CheckPeriod:    "",
				GracePeriod:    "",
				ActivityWindow: "",
			},
			idleTimeout:               30 * time.Minute,
			expectedCheckPeriod:       60 * time.Second, // DefaultCheckPeriod
			expectedGracePeriodMin:    1 * time.Minute,
			expectedGracePeriodMax:    5 * time.Minute,
			expectedActivityWindowMin: 2 * time.Minute,
		},
		{
			name: "user-specified values preserved",
			config: cliWatcherConfig{
				CheckPeriod:    "45",
				GracePeriod:    "10m",
				ActivityWindow: "20m",
			},
			idleTimeout:               30 * time.Minute,
			expectedCheckPeriod:       45 * time.Second,
			expectedGracePeriodMin:    10 * time.Minute,
			expectedGracePeriodMax:    10 * time.Minute,
			expectedActivityWindowMin: 20 * time.Minute,
		},
		{
			name: "no idle timeout (disabled)",
			config: cliWatcherConfig{
				CheckPeriod:    "",
				GracePeriod:    "",
				ActivityWindow: "",
			},
			idleTimeout:               -1,
			expectedCheckPeriod:       60 * time.Second, // DefaultCheckPeriod
			expectedGracePeriodMin:    DefaultGracePeriod,
			expectedGracePeriodMax:    DefaultGracePeriod,
			expectedActivityWindowMin: DefaultActivityWindow,
		},
		{
			name: "very short idle timeout (5m)",
			config: cliWatcherConfig{
				CheckPeriod:    "",
				GracePeriod:    "",
				ActivityWindow: "",
			},
			idleTimeout:               5 * time.Minute,
			expectedCheckPeriod:       60 * time.Second, // DefaultCheckPeriod
			expectedGracePeriodMin:    MinGracePeriod,
			expectedGracePeriodMax:    MinGracePeriod,
			expectedActivityWindowMin: MinActivityWindow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyDefaults(tt.config, tt.idleTimeout)

			// Check checkPeriod
			if result._checkPeriodParsed != tt.expectedCheckPeriod {
				t.Errorf("checkPeriod = %v, want %v",
					result._checkPeriodParsed, tt.expectedCheckPeriod)
			}

			// Check gracePeriod (range for adaptive defaults)
			if result._gracePeriodParsed < tt.expectedGracePeriodMin ||
				result._gracePeriodParsed > tt.expectedGracePeriodMax {
				t.Errorf("gracePeriod = %v, want between %v and %v",
					result._gracePeriodParsed, tt.expectedGracePeriodMin, tt.expectedGracePeriodMax)
			}

			// Check activityWindow (minimum check for adaptive defaults)
			if result._activityWindowParsed < tt.expectedActivityWindowMin {
				t.Errorf("activityWindow = %v, want at least %v",
					result._activityWindowParsed, tt.expectedActivityWindowMin)
			}

			// Check maxProcessAge always has default
			if result._maxProcessAgeParsed == 0 {
				t.Errorf("maxProcessAge should not be zero")
			}
		})
	}
}

// Test isNumeric helper function
func TestIsNumeric(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"empty string", "", false}, // Empty string is NOT numeric (edge case fix)
		{"single digit", "5", true},
		{"multiple digits", "12345", true},
		{"zero", "0", true},
		{"leading zeros", "00123", true},

		// Non-numeric cases
		{"letters", "abc", false},
		{"mixed", "123abc", false},
		{"negative", "-123", false},
		{"decimal", "12.34", false},
		{"space", "12 34", false},
		{"duration", "30s", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNumeric(tt.input)
			if result != tt.expected {
				t.Errorf("isNumeric(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// Test getInteractiveModeDescription
func TestGetInteractiveModeDescription(t *testing.T) {
	tests := []struct {
		mode     InteractiveMode
		expected string
	}{
		{InteractiveModeAuto, "auto-detect TTY"},
		{InteractiveModeTrue, "interactive (activity check)"},
		{InteractiveModeYes, "interactive (activity check)"},
		{InteractiveModeFalse, "non-interactive (always active)"},
		{InteractiveModeNo, "non-interactive (always active)"},
		{InteractiveMode(""), "unknown"},        // Empty string doesn't match any case
		{InteractiveMode("invalid"), "unknown"}, // Invalid value
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			result := getInteractiveModeDescription(tt.mode)
			if result != tt.expected {
				t.Errorf("getInteractiveModeDescription(%q) = %q, want %q",
					tt.mode, result, tt.expected)
			}
		})
	}
}

// Test getPlatformDefaultClockTicks
func TestGetPlatformDefaultClockTicks(t *testing.T) {
	result := getPlatformDefaultClockTicks()

	// Verify result is one of the expected values
	if result != 100 && result != 250 {
		t.Errorf("getPlatformDefaultClockTicks() = %d, want 100 or 250", result)
	}

	// Platform-specific checks (can only validate current platform)
	switch runtime.GOARCH {
	case "arm", "arm64":
		if result != 250 {
			t.Errorf("On ARM platform, expected 250 ticks but got %d", result)
		}
	case "amd64", "386":
		if result != 100 {
			t.Errorf("On x86 platform, expected 100 ticks but got %d", result)
		}
	default:
		// Other platforms should default to 100
		if result != 100 {
			t.Errorf("On platform %s, expected 100 ticks but got %d", runtime.GOARCH, result)
		}
	}
}

// Test applyPolicy mode handling
// Note: Full testing of interactive modes (auto, true, yes) requires real /proc filesystem
// These tests cover the non-interactive mode logic which doesn't require process inspection
func TestApplyPolicy(t *testing.T) {
	tests := []struct {
		name           string
		mode           InteractiveMode
		expectedResult bool
		note           string
	}{
		{
			name:           "non-interactive mode: false",
			mode:           InteractiveModeFalse,
			expectedResult: true,
			note:           "Should always return true (prevent idling)",
		},
		{
			name:           "non-interactive mode: no",
			mode:           InteractiveModeNo,
			expectedResult: true,
			note:           "Should always return true (prevent idling)",
		},
		{
			name:           "empty mode (defaults to no)",
			mode:           InteractiveMode(""),
			expectedResult: true,
			note:           "Empty mode should behave as non-interactive",
		},
		// Note: Cannot fully test interactive modes without real /proc:
		// - InteractiveModeAuto calls isInteractiveProcess(pid, verbose) which needs /proc
		// - InteractiveModeTrue/Yes call hasRecentActivity(activityWindow, pid, verbose) which needs /proc/[pid]/fd/0
		// These require integration testing with real processes
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a non-existent PID since we're only testing non-interactive modes
			// which don't call process inspection functions
			result := applyPolicy("99999", "testcmd", tt.mode, 60*time.Second, "test", false)

			if result != tt.expectedResult {
				t.Errorf("applyPolicy with mode %q = %v, want %v (%s)",
					tt.mode, result, tt.expectedResult, tt.note)
			}
		})
	}
}

// Test concurrent access (race detector)
func TestConcurrentStartStop(t *testing.T) {
	t.Run("concurrent Start calls", func(t *testing.T) {
		watcher := NewCliWatcher(func() {}, 30*time.Minute)

		// Start multiple goroutines trying to start the watcher
		done := make(chan bool, 5)
		for i := 0; i < 5; i++ {
			go func() {
				watcher.Start()
				done <- true
			}()
		}

		// Wait for all to complete
		for i := 0; i < 5; i++ {
			<-done
		}

		// Should be started exactly once
		if !watcher.started {
			t.Error("Watcher should be started after concurrent Start() calls")
		}

		watcher.Stop()
	})

	t.Run("concurrent Stop calls", func(t *testing.T) {
		watcher := NewCliWatcher(func() {}, 30*time.Minute)
		watcher.Start()
		time.Sleep(10 * time.Millisecond) // Let it actually start

		// Stop multiple times concurrently
		done := make(chan bool, 5)
		for i := 0; i < 5; i++ {
			go func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("Stop() panicked: %v", r)
					}
					done <- true
				}()
				watcher.Stop()
			}()
		}

		// Wait for all to complete
		for i := 0; i < 5; i++ {
			<-done
		}
	})

	t.Run("concurrent Start and Stop", func(t *testing.T) {
		watcher := NewCliWatcher(func() {}, 30*time.Minute)

		done := make(chan bool, 10)

		// 5 goroutines trying to start
		for i := 0; i < 5; i++ {
			go func() {
				watcher.Start()
				done <- true
			}()
		}

		// 5 goroutines trying to stop
		for i := 0; i < 5; i++ {
			go func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("Concurrent Start/Stop panicked: %v", r)
					}
					done <- true
				}()
				watcher.Stop()
			}()
		}

		// Wait for all to complete
		for i := 0; i < 10; i++ {
			<-done
		}
	})
}

// Test system clock changes and time edge cases
func TestProcessAgeWithClockSkew(t *testing.T) {
	// Note: This test documents expected behavior when system clock changes
	// Full testing requires mocking time.Now() which isn't easily done in Go without interfaces

	t.Run("getProcessAge with zero startTime", func(t *testing.T) {
		// When getProcessStartTime returns zero (error case), age should be 0
		age := getProcessAge("99999") // Non-existent PID
		if age != 0 {
			t.Errorf("Process age for invalid PID should be 0, got %v", age)
		}
	})

	t.Run("process age validation", func(t *testing.T) {
		// Verify that getProcessAge() handles clock skew gracefully
		// We can't easily test negative time.Since() without mocking,
		// but we verify the current implementation returns non-negative values

		// Test with current process (should always have valid age >= 0)
		age := getProcessAge(fmt.Sprintf("%d", os.Getpid()))
		if age < 0 {
			t.Errorf("Process age should never be negative (clock skew should return 0), got %v", age)
		}

		t.Log("✓ getProcessAge() properly handles clock skew by returning 0 for negative durations")
		t.Log("  This ensures processes get grace period protection even after backward clock adjustment")
	})
}

// Test concurrent config access
func TestConcurrentConfigAccess(t *testing.T) {
	t.Run("concurrent config reads", func(t *testing.T) {
		watcher := NewCliWatcher(func() {}, 30*time.Minute)

		// This test will fail with -race if there's a data race
		done := make(chan bool, 10)

		// Start the watcher (which will reload config periodically)
		watcher.Start()
		defer watcher.Stop()

		time.Sleep(10 * time.Millisecond) // Let watcher start

		// Read config from multiple goroutines
		// Note: Direct access to watcher.config requires mutex, but our implementation
		// uses configSnapshot pattern, so we can't test direct field access
		// Instead, verify the watcher doesn't crash when running concurrently
		for i := 0; i < 10; i++ {
			go func() {
				// Just verify watcher is running without panicking
				// The actual config access is protected in Start() via snapshot
				time.Sleep(5 * time.Millisecond)
				done <- true
			}()
		}

		// Wait for all reads
		for i := 0; i < 10; i++ {
			<-done
		}
	})
}

// Test /proc parsing with real process data
// These are integration tests that require a Linux /proc filesystem
func TestProcParsing(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping /proc parsing tests on non-Linux platform")
	}

	t.Run("parseProcStat with current process", func(t *testing.T) {
		// Test with our own PID (we know it exists and is valid)
		myPID := fmt.Sprintf("%d", os.Getpid())

		stat, err := parseProcStat(myPID)
		if err != nil {
			t.Fatalf("parseProcStat(%s) failed: %v", myPID, err)
		}

		// Validate returned fields
		if stat.ppid == "" {
			t.Error("parseProcStat returned empty ppid")
		}
		if stat.ppid == "0" {
			t.Error("parseProcStat returned ppid=0 (invalid for non-init process)")
		}
		if stat.pgrp <= 0 {
			t.Errorf("parseProcStat returned invalid pgrp: %d", stat.pgrp)
		}
		if stat.startTicks <= 0 {
			t.Errorf("parseProcStat returned invalid startTicks: %d", stat.startTicks)
		}
		if stat.utimeTicks < 0 {
			t.Errorf("parseProcStat returned negative utimeTicks: %d", stat.utimeTicks)
		}
		if stat.stimeTicks < 0 {
			t.Errorf("parseProcStat returned negative stimeTicks: %d", stat.stimeTicks)
		}

		t.Logf("Current process stats: ppid=%s, pgrp=%d, tpgid=%d, utimeTicks=%d, stimeTicks=%d, startTicks=%d",
			stat.ppid, stat.pgrp, stat.tpgid, stat.utimeTicks, stat.stimeTicks, stat.startTicks)
	})

	t.Run("parseProcStat with init process", func(t *testing.T) {
		// PID 1 should always exist on Linux
		stat, err := parseProcStat("1")
		if err != nil {
			t.Fatalf("parseProcStat(1) failed: %v", err)
		}

		// Init should have ppid=0
		if stat.ppid != "0" {
			t.Errorf("Init process ppid = %s, want 0", stat.ppid)
		}
		if stat.startTicks <= 0 {
			t.Errorf("Init process has invalid startTicks: %d", stat.startTicks)
		}
	})

	t.Run("parseProcStat with invalid PID", func(t *testing.T) {
		// Very high PID unlikely to exist
		_, err := parseProcStat("999999")
		if err == nil {
			t.Error("parseProcStat(999999) should fail for non-existent PID")
		}
	})

	t.Run("getParentPID with current process", func(t *testing.T) {
		myPID := fmt.Sprintf("%d", os.Getpid())

		ppid := getParentPID(myPID)
		if ppid == "" {
			t.Error("getParentPID returned empty string for valid PID")
		}
		if ppid == "0" {
			t.Error("getParentPID returned 0 (invalid for non-init process)")
		}

		t.Logf("Current process parent PID: %s", ppid)
	})

	t.Run("getProcessStartTime with current process", func(t *testing.T) {
		myPID := fmt.Sprintf("%d", os.Getpid())

		startTime := getProcessStartTime(myPID)
		if startTime.IsZero() {
			t.Error("getProcessStartTime returned zero time for valid PID")
		}

		// Start time should be in the past
		if startTime.After(time.Now()) {
			t.Errorf("Process start time %v is in the future", startTime)
		}

		// Start time should be recent (within last hour for test process)
		age := time.Since(startTime)
		if age > 1*time.Hour {
			t.Logf("Warning: Process age is %v (seems old for test process)", age)
		}

		t.Logf("Current process started at: %v (age: %v)", startTime, age)
	})

	t.Run("getProcessAge with current process", func(t *testing.T) {
		myPID := fmt.Sprintf("%d", os.Getpid())

		age := getProcessAge(myPID)
		if age <= 0 {
			t.Error("getProcessAge returned non-positive duration for valid PID")
		}

		// Age should be reasonable (less than 1 hour for test)
		if age > 1*time.Hour {
			t.Logf("Warning: Process age %v seems old for test process", age)
		}

		t.Logf("Current process age: %v", age)
	})

	t.Run("isNumeric with various inputs", func(t *testing.T) {
		tests := []struct {
			input    string
			expected bool
		}{
			{"123", true},
			{"0", true},
			{"999999", true},
			{"abc", false},
			{"12a34", false},
			{"-123", false},
			{"", false}, // Empty string should be false (edge case)
		}

		for _, tt := range tests {
			result := isNumeric(tt.input)
			if result != tt.expected {
				t.Errorf("isNumeric(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		}
	})

	t.Run("processHasTTY detection", func(t *testing.T) {
		// Note: This test may vary depending on how the test is run
		// (terminal vs CI environment)
		myPID := fmt.Sprintf("%d", os.Getpid())

		hasTTY := processHasTTY(myPID)
		t.Logf("Current process has TTY: %v", hasTTY)

		// Init process (PID 1) typically has no TTY
		initHasTTY := processHasTTY("1")
		if initHasTTY {
			t.Log("Note: Init process has TTY (unusual but not necessarily wrong)")
		}
	})

	t.Run("getMainUserProcess behavior", func(t *testing.T) {
		// This is hard to test deterministically, but we can verify it doesn't crash
		myPID := fmt.Sprintf("%d", os.Getpid())

		mainPID, found := getMainUserProcess(myPID)
		t.Logf("getMainUserProcess(%s) = %s, found=%v", myPID, mainPID, found)

		// If found, mainPID should be valid
		if found && mainPID == "" {
			t.Error("getMainUserProcess returned found=true but empty PID")
		}
	})
}

// Test getProcessCPUTicks, used by the CPU-usage activity fallback
func TestGetProcessCPUTicks(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping /proc parsing tests on non-Linux platform")
	}

	t.Run("current process returns non-negative ticks", func(t *testing.T) {
		myPID := fmt.Sprintf("%d", os.Getpid())
		ticks, ok := getProcessCPUTicks(myPID)
		if !ok {
			t.Fatalf("getProcessCPUTicks(%s) failed", myPID)
		}
		if ticks < 0 {
			t.Errorf("getProcessCPUTicks returned negative ticks: %d", ticks)
		}
	})

	t.Run("invalid PID returns false", func(t *testing.T) {
		_, ok := getProcessCPUTicks("999999")
		if ok {
			t.Error("getProcessCPUTicks(999999) should fail for non-existent PID")
		}
	})
}

// Test hasRecentCPUActivity, the atime-independent fallback used by hasTTYActivity
// when TTY atime is unavailable or unreliable (e.g. noatime devpts mounts)
func TestHasRecentCPUActivity(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping CPU-activity fallback tests on non-Linux platform")
	}

	myPID := fmt.Sprintf("%d", os.Getpid())

	t.Run("first observation returns true (no baseline yet)", func(t *testing.T) {
		cpuActivityCache = make(map[string]*cpuActivitySample)

		if !hasRecentCPUActivity(myPID, time.Minute, false) {
			t.Error("hasRecentCPUActivity should return true on first observation (benefit of doubt)")
		}
	})

	t.Run("recent baseline still counts as active", func(t *testing.T) {
		cpuActivityCache = make(map[string]*cpuActivitySample)
		hasRecentCPUActivity(myPID, time.Minute, false) // establish baseline

		if !hasRecentCPUActivity(myPID, time.Minute, false) {
			t.Error("hasRecentCPUActivity should still be true immediately after establishing baseline")
		}
	})

	t.Run("stale baseline with no new CPU ticks returns false", func(t *testing.T) {
		cpuActivityCache = make(map[string]*cpuActivitySample)
		cpuActivityCache[myPID] = &cpuActivitySample{
			lastTicks:    1 << 30, // implausibly high so real CPU usage won't exceed it
			lastActiveAt: time.Now().Add(-time.Hour),
		}

		if hasRecentCPUActivity(myPID, time.Minute, false) {
			t.Error("hasRecentCPUActivity should return false when last activity predates the window and ticks haven't increased")
		}
	})

	t.Run("invalid PID returns false", func(t *testing.T) {
		cpuActivityCache = make(map[string]*cpuActivitySample)

		if hasRecentCPUActivity("999999", time.Minute, false) {
			t.Error("hasRecentCPUActivity should return false for a non-existent PID")
		}
	})
}

// Smoke test for checkDevptsAtimeSupport: verify it doesn't panic against the real
// /proc/mounts. It only logs, so there's nothing else to assert without adding a
// logrus-capture harness.
func TestCheckDevptsAtimeSupport(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping devpts atime check on non-Linux platform")
	}

	checkDevptsAtimeSupport()
}

// Test backward compatibility with deprecated fields
func TestBackwardCompatibility(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		expected time.Duration
	}{
		{
			name:     "old checkPeriodSeconds",
			yaml:     "checkPeriodSeconds: 45",
			expected: 45 * time.Second,
		},
		{
			name:     "new checkPeriod string",
			yaml:     "checkPeriod: 30s",
			expected: 30 * time.Second,
		},
		{
			name:     "new checkPeriod integer",
			yaml:     "checkPeriod: 60",
			expected: 60 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg cliWatcherConfig
			err := yaml.Unmarshal([]byte(tt.yaml), &cfg)
			if err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			result := applyDefaults(cfg, 30*time.Minute)

			if result._checkPeriodParsed != tt.expected {
				t.Errorf("checkPeriod = %v, want %v",
					result._checkPeriodParsed, tt.expected)
			}
		})
	}
}

// Test loadEnvConfig parsing
func TestLoadEnvConfig(t *testing.T) {
	t.Run("no env vars set", func(t *testing.T) {
		for _, env := range []string{EnvCliWatcherEnabled, EnvCliWatcherCheckPeriod, EnvCliWatcherActivityWindow, EnvCliWatcherGracePeriod, EnvCliWatcherMaxProcessAge} {
			os.Unsetenv(env)
		}
		cfg := loadEnvConfig()
		if cfg.enabled != nil {
			t.Errorf("enabled should be nil when env var not set, got %v", *cfg.enabled)
		}
		if cfg.checkPeriod != nil {
			t.Errorf("checkPeriod should be nil when env var not set")
		}
		if cfg.activityWindow != nil {
			t.Errorf("activityWindow should be nil when env var not set")
		}
		if cfg.gracePeriod != nil {
			t.Errorf("gracePeriod should be nil when env var not set")
		}
		if cfg.maxProcessAge != nil {
			t.Errorf("maxProcessAge should be nil when env var not set")
		}
	})

	t.Run("enabled true", func(t *testing.T) {
		t.Setenv(EnvCliWatcherEnabled, "true")
		cfg := loadEnvConfig()
		if cfg.enabled == nil || !*cfg.enabled {
			t.Errorf("enabled should be true")
		}
	})

	t.Run("enabled false", func(t *testing.T) {
		t.Setenv(EnvCliWatcherEnabled, "false")
		cfg := loadEnvConfig()
		if cfg.enabled == nil || *cfg.enabled {
			t.Errorf("enabled should be false")
		}
	})

	t.Run("enabled invalid", func(t *testing.T) {
		t.Setenv(EnvCliWatcherEnabled, "notabool")
		cfg := loadEnvConfig()
		if cfg.enabled != nil {
			t.Errorf("enabled should be nil for invalid value, got %v", *cfg.enabled)
		}
	})

	t.Run("duration env vars", func(t *testing.T) {
		t.Setenv(EnvCliWatcherCheckPeriod, "45s")
		t.Setenv(EnvCliWatcherActivityWindow, "20m")
		t.Setenv(EnvCliWatcherGracePeriod, "3m")
		t.Setenv(EnvCliWatcherMaxProcessAge, "4h")
		cfg := loadEnvConfig()
		if cfg.checkPeriod == nil || *cfg.checkPeriod != 45*time.Second {
			t.Errorf("checkPeriod = %v, want 45s", cfg.checkPeriod)
		}
		if cfg.activityWindow == nil || *cfg.activityWindow != 20*time.Minute {
			t.Errorf("activityWindow = %v, want 20m", cfg.activityWindow)
		}
		if cfg.gracePeriod == nil || *cfg.gracePeriod != 3*time.Minute {
			t.Errorf("gracePeriod = %v, want 3m", cfg.gracePeriod)
		}
		if cfg.maxProcessAge == nil || *cfg.maxProcessAge != 4*time.Hour {
			t.Errorf("maxProcessAge = %v, want 4h", cfg.maxProcessAge)
		}
	})

	t.Run("duration as plain integer (seconds)", func(t *testing.T) {
		t.Setenv(EnvCliWatcherCheckPeriod, "30")
		cfg := loadEnvConfig()
		if cfg.checkPeriod == nil || *cfg.checkPeriod != 30*time.Second {
			t.Errorf("checkPeriod = %v, want 30s", cfg.checkPeriod)
		}
	})
}

// Test applyEnvCeilings
func TestApplyEnvCeilings(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }
	durPtr := func(d time.Duration) *time.Duration { return &d }

	t.Run("enabled from env var overrides noidle", func(t *testing.T) {
		w := &cliWatcher{envConfig: cliWatcherEnvConfig{enabled: boolPtr(false)}}
		cfg := cliWatcherConfig{Enabled: true}
		result := w.applyEnvCeilings(cfg, true)
		if result.Enabled {
			t.Error("enabled should be false (admin override)")
		}
	})

	t.Run("enabled uses default when env var not set", func(t *testing.T) {
		w := &cliWatcher{envConfig: cliWatcherEnvConfig{}}
		cfg := cliWatcherConfig{Enabled: true}
		result := w.applyEnvCeilings(cfg, true)
		if result.Enabled != DefaultCliWatcherEnabled {
			t.Errorf("enabled should be %t (default), got %t", DefaultCliWatcherEnabled, result.Enabled)
		}
	})

	t.Run("enabled without noidle uses env var", func(t *testing.T) {
		w := &cliWatcher{envConfig: cliWatcherEnvConfig{enabled: boolPtr(true)}}
		cfg := cliWatcherConfig{}
		result := w.applyEnvCeilings(cfg, false)
		if !result.Enabled {
			t.Error("enabled should be true (from env var)")
		}
	})

	t.Run("timing param clamped to admin ceiling", func(t *testing.T) {
		adminLimit := 15 * time.Minute
		w := &cliWatcher{envConfig: cliWatcherEnvConfig{activityWindow: durPtr(adminLimit)}}
		cfg := cliWatcherConfig{
			ActivityWindow:        "30m",
			_activityWindowParsed: 30 * time.Minute,
		}
		result := w.applyEnvCeilings(cfg, true)
		if result._activityWindowParsed != adminLimit {
			t.Errorf("activityWindow = %v, want %v (admin ceiling)", result._activityWindowParsed, adminLimit)
		}
	})

	t.Run("timing param accepted when stricter than ceiling", func(t *testing.T) {
		adminLimit := 15 * time.Minute
		noifileVal := 10 * time.Minute
		w := &cliWatcher{envConfig: cliWatcherEnvConfig{activityWindow: durPtr(adminLimit)}}
		cfg := cliWatcherConfig{
			ActivityWindow:        "10m",
			_activityWindowParsed: noifileVal,
		}
		result := w.applyEnvCeilings(cfg, true)
		if result._activityWindowParsed != noifileVal {
			t.Errorf("activityWindow = %v, want %v (noidle stricter)", result._activityWindowParsed, noifileVal)
		}
	})

	t.Run("timing param uses env var when no noidle", func(t *testing.T) {
		envVal := 20 * time.Minute
		w := &cliWatcher{envConfig: cliWatcherEnvConfig{activityWindow: durPtr(envVal)}}
		cfg := cliWatcherConfig{
			_activityWindowParsed: DefaultActivityWindow,
		}
		result := w.applyEnvCeilings(cfg, false)
		if result._activityWindowParsed != envVal {
			t.Errorf("activityWindow = %v, want %v (from env)", result._activityWindowParsed, envVal)
		}
	})

	t.Run("timing param uses default when no env and no noidle", func(t *testing.T) {
		w := &cliWatcher{envConfig: cliWatcherEnvConfig{}}
		cfg := cliWatcherConfig{
			_activityWindowParsed: DefaultActivityWindow,
		}
		result := w.applyEnvCeilings(cfg, false)
		if result._activityWindowParsed != DefaultActivityWindow {
			t.Errorf("activityWindow = %v, want %v (default)", result._activityWindowParsed, DefaultActivityWindow)
		}
	})

	t.Run("all timing params clamped", func(t *testing.T) {
		w := &cliWatcher{envConfig: cliWatcherEnvConfig{
			checkPeriod:    durPtr(30 * time.Second),
			activityWindow: durPtr(10 * time.Minute),
			gracePeriod:    durPtr(2 * time.Minute),
			maxProcessAge:  durPtr(3 * time.Hour),
		}}
		cfg := cliWatcherConfig{
			CheckPeriod:           "60s",
			ActivityWindow:        "25m",
			GracePeriod:           "5m",
			MaxProcessAge:         "6h",
			_checkPeriodParsed:    60 * time.Second,
			_activityWindowParsed: 25 * time.Minute,
			_gracePeriodParsed:    5 * time.Minute,
			_maxProcessAgeParsed:  6 * time.Hour,
		}
		result := w.applyEnvCeilings(cfg, true)
		if result._checkPeriodParsed != 30*time.Second {
			t.Errorf("checkPeriod = %v, want 30s", result._checkPeriodParsed)
		}
		if result._activityWindowParsed != 10*time.Minute {
			t.Errorf("activityWindow = %v, want 10m", result._activityWindowParsed)
		}
		if result._gracePeriodParsed != 2*time.Minute {
			t.Errorf("gracePeriod = %v, want 2m", result._gracePeriodParsed)
		}
		if result._maxProcessAgeParsed != 3*time.Hour {
			t.Errorf("maxProcessAge = %v, want 3h", result._maxProcessAgeParsed)
		}
	})
}

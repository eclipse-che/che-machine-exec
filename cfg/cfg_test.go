//
// Copyright (c) 2019-2026 Red Hat, Inc.
// This program and the accompanying materials are made
// available under the terms of the Eclipse Public License 2.0
// which is available at https://www.eclipse.org/legal/epl-2.0/
//
// SPDX-License-Identifier: EPL-2.0
//
// Contributors:
//   Red Hat, Inc. - initial API and implementation
//

package cfg

import "testing"

func TestCheckAuthMode(t *testing.T) {
	tests := []struct {
		name      string
		noAuth    bool
		useBearer bool
		wantErr   bool
	}{
		{name: "bearer only is valid", noAuth: false, useBearer: true, wantErr: false},
		{name: "no-auth only is valid", noAuth: true, useBearer: false, wantErr: false},
		{name: "neither selected is rejected", noAuth: false, useBearer: false, wantErr: true},
		{name: "both selected is rejected", noAuth: true, useBearer: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkAuthMode(tt.noAuth, tt.useBearer)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkAuthMode(noAuth=%v, useBearer=%v) error = %v, wantErr %v",
					tt.noAuth, tt.useBearer, err, tt.wantErr)
			}
		})
	}
}

func TestParseAllowedOrigins(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "empty string yields no origins", input: "", want: nil},
		{name: "single origin", input: "https://a.example", want: []string{"https://a.example"}},
		{name: "trims whitespace and drops blanks", input: " https://a.example , , https://b.example ", want: []string{"https://a.example", "https://b.example"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAllowedOrigins(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseAllowedOrigins(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseAllowedOrigins(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

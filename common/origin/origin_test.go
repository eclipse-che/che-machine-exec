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

package origin

import (
	"net/http"
	"testing"

	"github.com/eclipse-che/che-machine-exec/cfg"
)

func TestIsOriginAllowed(t *testing.T) {
	tests := []struct {
		name           string
		host           string
		origin         string
		allowedOrigins []string
		want           bool
	}{
		{
			name:   "no origin header is allowed (non-browser client)",
			host:   "machine-exec:4444",
			origin: "",
			want:   true,
		},
		{
			name:   "same-origin request is allowed",
			host:   "machine-exec:4444",
			origin: "https://machine-exec:4444",
			want:   true,
		},
		{
			name:           "cross-origin request is rejected by default",
			host:           "machine-exec:4444",
			origin:         "https://attacker.example.com",
			allowedOrigins: nil,
			want:           false,
		},
		{
			name:           "cross-origin request listed in allow-list is accepted",
			host:           "machine-exec:4444",
			origin:         "https://console-openshift-console.apps.cluster",
			allowedOrigins: []string{"https://console-openshift-console.apps.cluster"},
			want:           true,
		},
		{
			name:           "cross-origin request not in allow-list is rejected",
			host:           "machine-exec:4444",
			origin:         "https://attacker.example.com",
			allowedOrigins: []string{"https://console-openshift-console.apps.cluster"},
			want:           false,
		},
		{
			name:           "allow-list entry may be a bare host",
			host:           "machine-exec:4444",
			origin:         "https://dashboard.apps.cluster",
			allowedOrigins: []string{"dashboard.apps.cluster"},
			want:           true,
		},
		{
			name:           "allow-list entry with trailing slash still matches",
			host:           "machine-exec:4444",
			origin:         "https://dashboard.apps.cluster",
			allowedOrigins: []string{"https://dashboard.apps.cluster/"},
			want:           true,
		},
		{
			name:   "unparsable origin is rejected",
			host:   "machine-exec:4444",
			origin: "://not a url",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := cfg.AllowedOrigins
			cfg.AllowedOrigins = tt.allowedOrigins
			defer func() { cfg.AllowedOrigins = original }()

			r := &http.Request{Host: tt.host, Header: http.Header{}}
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}

			if got := IsOriginAllowed(r); got != tt.want {
				t.Errorf("IsOriginAllowed(origin=%q, host=%q, allowed=%v) = %v, want %v",
					tt.origin, tt.host, tt.allowedOrigins, got, tt.want)
			}
		})
	}
}

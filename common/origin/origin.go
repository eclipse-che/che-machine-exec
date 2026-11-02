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

// Package origin provides Cross-Site WebSocket Hijacking (CSWSH) protection for
// the WebSocket endpoints. It lives in its own leaf package so the logic can be
// unit-tested without pulling in the exec manager, which requires an in-cluster
// service account to initialize.
package origin

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/eclipse-che/che-machine-exec/cfg"
	"github.com/sirupsen/logrus"
)

// IsOriginAllowed reports whether a WebSocket upgrade request may proceed.
//
// A request is allowed when:
//   - it carries no Origin header (non-browser clients such as che-code's
//     loopback connection never set one), or
//   - the Origin is same-origin with the request Host, or
//   - the Origin is listed in cfg.AllowedOrigins.
//
// Any other (cross-origin, browser-initiated) request is rejected. Leaving
// cfg.AllowedOrigins empty therefore keeps the current behavior for the
// supported deployments while still blocking a cross-site browser hijack.
func IsOriginAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}

	originURL, err := url.Parse(origin)
	if err != nil {
		logrus.Warnf("Rejecting WebSocket connection with unparsable Origin %q", origin)
		return false
	}

	// Same-origin requests (Origin host matches the request Host) are always allowed.
	if strings.EqualFold(originURL.Host, r.Host) {
		return true
	}

	for _, allowed := range cfg.AllowedOrigins {
		if originMatches(originURL, origin, allowed) {
			return true
		}
	}

	logrus.Warnf("Rejecting WebSocket connection from disallowed Origin %q", origin)
	return false
}

// originMatches reports whether the request Origin matches a configured entry.
// An entry may be a full origin (scheme://host[:port]) or a bare host[:port].
func originMatches(originURL *url.URL, origin, allowed string) bool {
	allowed = strings.TrimRight(strings.TrimSpace(allowed), "/")
	if allowed == "" {
		return false
	}
	if strings.EqualFold(origin, allowed) {
		return true
	}
	// Allow configuring a bare host[:port] without a scheme.
	return strings.EqualFold(originURL.Host, allowed)
}

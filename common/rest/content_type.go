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

package rest

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// corsSafelistedContentTypes are the Content-Type values a browser is allowed to
// send on a cross-origin request without triggering a CORS preflight. Because
// the state-changing POST handlers decode their body as JSON regardless of the
// Content-Type, rejecting these values blocks cross-site request forgery while
// still accepting application/json (what the OpenShift console proxy sends),
// requests with no Content-Type, and any non-browser client.
var corsSafelistedContentTypes = map[string]struct{}{
	"text/plain":                        {},
	"application/x-www-form-urlencoded": {},
	"multipart/form-data":               {},
}

// RejectCrossSiteContentType writes a 415 response and returns true when the
// request carries a CORS-safelisted Content-Type. gin's ContentType() returns
// the media type without parameters (e.g. charset or multipart boundary), so a
// plain string comparison is sufficient.
func RejectCrossSiteContentType(c *gin.Context) bool {
	if _, ok := corsSafelistedContentTypes[c.ContentType()]; ok {
		WriteResponse(c, http.StatusUnsupportedMediaType, "Unsupported Content-Type: expected application/json")
		return true
	}
	return false
}

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
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRejectCrossSiteContentType(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		contentType  string
		wantRejected bool
	}{
		{name: "application/json is accepted", contentType: "application/json", wantRejected: false},
		{name: "application/json with charset is accepted", contentType: "application/json; charset=utf-8", wantRejected: false},
		{name: "missing content-type is accepted", contentType: "", wantRejected: false},
		{name: "text/plain is rejected", contentType: "text/plain", wantRejected: true},
		{name: "text/plain with charset is rejected", contentType: "text/plain; charset=utf-8", wantRejected: true},
		{name: "form-urlencoded is rejected", contentType: "application/x-www-form-urlencoded", wantRejected: true},
		{name: "multipart/form-data is rejected", contentType: "multipart/form-data; boundary=xyz", wantRejected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			req := httptest.NewRequest(http.MethodPost, "/exec/init", strings.NewReader("{}"))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			c.Request = req

			got := RejectCrossSiteContentType(c)
			if got != tt.wantRejected {
				t.Errorf("RejectCrossSiteContentType(%q) = %v, want %v", tt.contentType, got, tt.wantRejected)
			}
			if tt.wantRejected && rec.Code != http.StatusUnsupportedMediaType {
				t.Errorf("expected status %d for %q, got %d", http.StatusUnsupportedMediaType, tt.contentType, rec.Code)
			}
		})
	}
}

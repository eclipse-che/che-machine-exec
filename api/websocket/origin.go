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

package websocket

import (
	"github.com/eclipse-che/che-machine-exec/common/origin"
	"github.com/eclipse/che-go-jsonrpc/jsonrpcws"
)

func init() {
	// che-go-jsonrpc's default upgrader accepts any origin. Override it so that
	// the /connect endpoint uses the same Cross-Site WebSocket Hijacking
	// protection as /attach. This runs once, when the package is loaded; the
	// CheckOrigin callback reads cfg.AllowedOrigins at request time (after the
	// configuration has been parsed).
	jsonrpcws.DefaultUpgrader.CheckOrigin = origin.IsOriginAllowed
}

//
// Copyright (c) 2012-2019 Red Hat, Inc.
// This program and the accompanying materials are made
// available under the terms of the Eclipse Public License 2.0
// which is available at https://www.eclipse.org/legal/epl-2.0/
//
// SPDX-License-Identifier: EPL-2.0
//
// Contributors:
//   Red Hat, Inc. - initial API and implementation
//

package filter

import "github.com/eclipse/che-machine-exec/api/model"

// Container filter to find container information if it's possible by unique
// workspaceId and machineName.
type ContainerFilter interface {
	// Find container information by machine identifier.
	// Machine identifier stores workspaceId and machineName.
	// Return error in case fail filter operation.
	FindContainerInfo(identifier *model.MachineIdentifier) (containerInfo map[string]string, err error)
}

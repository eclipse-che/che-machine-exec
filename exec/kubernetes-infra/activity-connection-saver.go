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

package kubernetes_infra

import (
	"github.com/eclipse/che-machine-exec/api/model"
	"time"
)

const (
	ActivityTimeOut = 30
)

// To prevent close exec connection
// (https://blog.openshift.com/executing-commands-in-pods-using-k8s-api/ - Connection lifecycle)
// let's send empty byte array each 30 sec.
func saveActivity(machineExec *model.MachineExec) {
	ticker := time.NewTicker(ActivityTimeOut * time.Second)
	for range ticker.C {
		machineExec.MsgChan <- make([]byte, 0)
	}
}

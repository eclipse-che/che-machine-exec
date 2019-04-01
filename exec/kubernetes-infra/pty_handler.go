//
// Copyright (c) 2012-2018 Red Hat, Inc.
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
	"k8s.io/client-go/tools/remotecommand"
)

// Kubernetes pty handler
type PtyHandlerImpl struct {
	machineExec *model.MachineExec
	filter      *model.Utf8StreamFilter
}

func (t PtyHandlerImpl) Read(p []byte) (int, error) {
	data := <-t.machineExec.MsgChan

	return copy(p, data), nil
}

func (t PtyHandlerImpl) Write(p []byte) (int, error) {

	filteredCharacters := t.filter.ProcessRaw(p)
	t.machineExec.Buffer.Write(filteredCharacters)
	t.machineExec.WriteDataToWsConnections(filteredCharacters)

	return len(filteredCharacters), nil
}

func (t PtyHandlerImpl) Next() *remotecommand.TerminalSize {
	select {
	case size := <-t.machineExec.SizeChan:
		return &size
	}
}

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

package exec

import (
	"errors"
	"github.com/eclipse/che-go-jsonrpc/event"
	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/eclipse/che-machine-exec/mocks"
	"testing"
	"time"
)

const Exec1ID = 0

func TestShouldCleanUpExecOnExit(t *testing.T) {
	machineExec := &model.MachineExec{ID: Exec1ID, ErrorChan: make(chan error), ExitChan: make(chan bool)}
	execManagerMock := &mocks.ExecManager{}
	eventBus := event.NewBus()

	execManagerMock.On("Remove", Exec1ID).Return()

	healthWatcher := NewHealthWatcher(machineExec, eventBus, execManagerMock)
	healthWatcher.CleanUpOnExitOrError()

	machineExec.ExitChan <- true
	time.Sleep(1000 * time.Millisecond)

	execManagerMock.AssertExpectations(t)
}

func TestShouldCleanUpExecOnError(t *testing.T) {
	machineExec := &model.MachineExec{ID: Exec1ID, ErrorChan: make(chan error), ExitChan: make(chan bool)}
	execManagerMock := &mocks.ExecManager{}
	eventBus := event.NewBus()

	execManagerMock.On("Remove", Exec1ID).Return()

	healthWatcher := NewHealthWatcher(machineExec, eventBus, execManagerMock)
	healthWatcher.CleanUpOnExitOrError()

	machineExec.ErrorChan <- errors.New("unable to create exec")
	time.Sleep(1000 * time.Millisecond)

	execManagerMock.AssertExpectations(t)
}

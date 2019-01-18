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

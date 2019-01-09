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
	"errors"
	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/eclipse/che-machine-exec/exec-info"
	"github.com/eclipse/che-machine-exec/filter"
	"github.com/eclipse/che-machine-exec/line-buffer"
	"github.com/eclipse/che-machine-exec/shell"
	ws "github.com/eclipse/che-machine-exec/ws-conn"
	"github.com/gorilla/websocket"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"strconv"
	"sync"
	"sync/atomic"
)

type MachineExecs struct {
	mutex   *sync.Mutex
	execMap map[int]*model.MachineExec
}

// Manager to manipulate kubernetes container execs.
type KubernetesExecManager struct {
	shell.ContainerShellDetector
	filter.ContainerFilter

	api    corev1.CoreV1Interface
	config *rest.Config

	nameSpace string
}

var (
	machineExecs = MachineExecs{
		mutex:   &sync.Mutex{},
		execMap: make(map[int]*model.MachineExec),
	}
	prevExecID uint64 = 0
)

/**
 * Create new instance of the kubernetes exec manager.
 */
func New(
	namespace string,
	api corev1.CoreV1Interface,
	config *rest.Config,
	filter filter.ContainerFilter,
	shellDetector shell.ContainerShellDetector,
) *KubernetesExecManager {
	return &KubernetesExecManager{
		api:                    api,
		nameSpace:              namespace,
		ContainerFilter:        filter,
		ContainerShellDetector: shellDetector,
		config:                 config,
	}
}

func (manager KubernetesExecManager) setUpExecShellPath(exec *model.MachineExec, containerInfo map[string]string) {
	if exec.Tty && len(exec.Cmd) == 0 {
		if containerShell, err := manager.DetectShell(containerInfo); err == nil {
			exec.Cmd = []string{containerShell}
		} else {
			exec.Cmd = []string{shell.DefaultShell}
		}
	}
}

func (manager *KubernetesExecManager) Create(machineExec *model.MachineExec) (int, error) {
	containerInfo, err := manager.FindContainerInfo(&machineExec.Identifier)
	if err != nil {
		return -1, err
	}

	manager.setUpExecShellPath(machineExec, containerInfo)

	req := manager.api.RESTClient().
		Post().
		Namespace(manager.nameSpace).
		Resource(exec_info.Pods).
		Name(containerInfo[exec_info.PodName]).
		SubResource(exec_info.Exec).
		// set up params
		VersionedParams(&v1.PodExecOptions{
			Container: containerInfo[exec_info.ContainerName],
			Command:   machineExec.Cmd,
			Stdout:    true,
			Stderr:    true,
			Stdin:     true,
			TTY:       machineExec.Tty,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(manager.config, exec_info.Post, req.URL())
	if err != nil {
		return -1, err
	}

	defer machineExecs.mutex.Unlock()
	machineExecs.mutex.Lock()

	machineExec.Executor = executor
	machineExec.ID = int(atomic.AddUint64(&prevExecID, 1))
	machineExec.MsgChan = make(chan []byte)
	machineExec.SizeChan = make(chan remotecommand.TerminalSize)
	machineExec.ExitChan = make(chan bool)
	machineExec.ErrorChan = make(chan error)
	machineExec.ConnectionHandler = ws.NewConnHandler()

	machineExecs.execMap[machineExec.ID] = machineExec

	return machineExec.ID, nil
}

// Clean up information about exec
func (*KubernetesExecManager) Remove(execId int) {
	defer machineExecs.mutex.Unlock()

	machineExecs.mutex.Lock()
	delete(machineExecs.execMap, execId)
}

func (*KubernetesExecManager) Check(id int) (int, error) {
	machineExec := getById(id)
	if machineExec == nil {
		return -1, errors.New("Exec '" + strconv.Itoa(id) + "' was not found")
	}
	return machineExec.ID, nil
}

func (*KubernetesExecManager) Attach(id int, conn *websocket.Conn) error {
	machineExec := getById(id)
	if machineExec == nil {
		return errors.New("Exec '" + strconv.Itoa(id) + "' to attach was not found")
	}

	machineExec.ReadConnection(conn, machineExec.MsgChan)

	if machineExec.Buffer != nil {
		// restore previous output.
		restoreContent := machineExec.Buffer.GetContent()
		return conn.WriteMessage(websocket.TextMessage, []byte(restoreContent))
	}

	go saveActivity(machineExec)

	ptyHandler := PtyHandlerImpl{machineExec: machineExec}
	machineExec.Buffer = line_buffer.New()

	err := machineExec.Executor.Stream(remotecommand.StreamOptions{
		Stdin:             ptyHandler,
		Stdout:            ptyHandler,
		Stderr:            ptyHandler,
		TerminalSizeQueue: ptyHandler,
		Tty:               machineExec.Tty,
	})

	if err != nil {
		machineExec.ErrorChan <- err
	} else {
		machineExec.ExitChan <- true
	}

	return err
}

func (*KubernetesExecManager) Resize(id int, cols uint, rows uint) error {
	machineExec := getById(id)
	if machineExec == nil {
		return errors.New("Exec to resize '" + strconv.Itoa(id) + "' was not found")
	}

	machineExec.SizeChan <- remotecommand.TerminalSize{Width: uint16(cols), Height: uint16(rows)}
	return nil
}

func getById(id int) *model.MachineExec {
	defer machineExecs.mutex.Unlock()

	machineExecs.mutex.Lock()
	return machineExecs.execMap[id]
}

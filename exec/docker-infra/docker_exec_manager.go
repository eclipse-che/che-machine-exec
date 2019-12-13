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

package docker_infra

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/eclipse/che-machine-exec/filter"
	line_buffer "github.com/eclipse/che-machine-exec/output/line-buffer"
	"github.com/eclipse/che-machine-exec/shell"
	ws "github.com/eclipse/che-machine-exec/ws-conn"
	"github.com/gorilla/websocket"
	"golang.org/x/net/context"
)

// Manager to manipulate docker container execs.
type DockerMachineExecManager struct {
	client client.ContainerAPIClient

	filter.ContainerFilter
	shell.ContainerShellDetector
}

type MachineExecs struct {
	mutex   *sync.Mutex
	execMap map[int]*model.MachineExec
}

var (
	machineExecs = MachineExecs{
		mutex:   &sync.Mutex{},
		execMap: make(map[int]*model.MachineExec),
	}
	prevExecID uint64 = 0
)

/**
 * Create new instance of the docker exec manager.
 */
func New(dockerClient client.ContainerAPIClient, filter filter.ContainerFilter, shellDetector shell.ContainerShellDetector) *DockerMachineExecManager {
	return &DockerMachineExecManager{
		client:                 dockerClient,
		ContainerFilter:        filter,
		ContainerShellDetector: shellDetector,
	}
}

func (manager DockerMachineExecManager) setUpExecShellPath(exec *model.MachineExec, containerInfo *model.ContainerInfo) {
	if exec.Tty && len(exec.Cmd) == 0 {
		if containerShell, err := manager.DetectShell(containerInfo); err == nil {
			exec.Cmd = []string{containerShell}
		} else {
			exec.Cmd = []string{shell.DefaultShell}
		}
	}
}

func (manager *DockerMachineExecManager) Create(machineExec *model.MachineExec) (int, error) {
	containerInfo, err := manager.FindContainerInfo(&machineExec.Identifier)
	if err != nil {
		return -1, err
	}

	manager.setUpExecShellPath(machineExec, containerInfo)

	resp, err := manager.client.ContainerExecCreate(context.Background(), containerInfo.ContainerName, types.ExecConfig{
		Tty:          machineExec.Tty,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Detach:       false,
		Cmd:          machineExec.Cmd,
	})
	if err != nil {
		return -1, err
	}

	defer machineExecs.mutex.Unlock()
	machineExecs.mutex.Lock()

	machineExec.ExecId = resp.ID
	machineExec.ID = int(atomic.AddUint64(&prevExecID, 1))
	machineExec.MsgChan = make(chan []byte)
	machineExec.ExitChan = make(chan bool)
	machineExec.ErrorChan = make(chan error)
	machineExec.ConnectionHandler = ws.NewConnHandler()

	machineExecs.execMap[machineExec.ID] = machineExec

	fmt.Println("Create exec ", machineExec.ID, "execId", machineExec.ExecId)

	return machineExec.ID, nil
}

func (manager *DockerMachineExecManager) Remove(execId int) {
	defer machineExecs.mutex.Unlock()

	machineExecs.mutex.Lock()
	delete(machineExecs.execMap, execId)
}

func (manager *DockerMachineExecManager) Check(id int) (int, error) {
	machineExec := getById(id)
	if machineExec == nil {
		return -1, errors.New("Exec '" + strconv.Itoa(id) + "' was not found")
	}
	return machineExec.ID, nil
}

func (manager *DockerMachineExecManager) Attach(id int, conn *websocket.Conn) error {
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

	hjr, err := manager.client.ContainerExecAttach(context.Background(), machineExec.ExecId, types.ExecConfig{
		Detach: false,
		Tty:    machineExec.Tty,
	})
	if err != nil {
		return errors.New("Failed to attach to exec " + err.Error())
	}

	machineExec.Hjr = &hjr
	machineExec.Buffer = line_buffer.New()

	machineExec.Start()

	return nil
}

func (manager *DockerMachineExecManager) Resize(id int, cols uint, rows uint) error {
	machineExec := getById(id)
	if machineExec == nil {
		return errors.New("Exec to resize '" + strconv.Itoa(id) + "' was not found")
	}

	resizeParam := types.ResizeOptions{Height: rows, Width: cols}
	if err := manager.client.ContainerExecResize(context.Background(), machineExec.ExecId, resizeParam); err != nil {
		return err
	}

	return nil
}

func getById(id int) *model.MachineExec {
	defer machineExecs.mutex.Unlock()

	machineExecs.mutex.Lock()
	return machineExecs.execMap[id]
}

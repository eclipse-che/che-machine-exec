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

package exec

import (
	"errors"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/eclipse/che-lib/websocket"
	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/eclipse/che-machine-exec/line-buffer"
	"golang.org/x/net/context"
	"strconv"
	"sync"
	"sync/atomic"
)

type MachineExecs struct {
	mutex   *sync.Mutex
	execMap map[int]*model.MachineExec
}

var (
	cli          = createDockerClient()
	machineExecs = MachineExecs{
		mutex:   &sync.Mutex{},
		execMap: make(map[int]*model.MachineExec),
	}
	prevExecID uint64 = 0
)

func createDockerClient() *client.Client {
	cli, err := client.NewEnvClient()
	// set up minimal docker version 1.13.0(api version 1.25).
	cli.UpdateClientVersion("1.25")
	if err != nil {
		panic(err)
	}
	return cli
}

func Create(machineExec *model.MachineExec) (int, error) {
	container, err := findMachineContainer(&machineExec.Identifier)
	if err != nil {
		return -1, err
	}

	fmt.Println("found container for creation exec! id=", container.ID)

	resp, err := cli.ContainerExecCreate(context.Background(), container.ID, types.ExecConfig{
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
	machineExec.Buffer = line_buffer.CreateNewLineRingBuffer()
	machineExec.MsgChan = make(chan []byte)
	machineExec.WsConnsLock = &sync.Mutex{}
	machineExec.WsConns = make([]*websocket.Conn, 0)

	machineExecs.execMap[machineExec.ID] = machineExec

	fmt.Println("Create exec ", machineExec.ID, "execId", machineExec.ExecId)

	return machineExec.ID, nil
}

func Check(id int) (int, error) {
	machineExec := getById(id)
	if machineExec == nil {
		return -1, errors.New("Exec '" + strconv.Itoa(id) + "' was not found")
	}
	return machineExec.ID, nil
}

func Attach(id int) (*model.MachineExec, error) {
	machineExec := getById(id)
	if machineExec == nil {
		return nil, errors.New("Exec '" + strconv.Itoa(id) + "' to attach was not found")
	}

	if machineExec.Hjr != nil {
		return machineExec, nil
	}

	hjr, err := cli.ContainerExecAttach(context.Background(), machineExec.ExecId, types.ExecConfig{
		Detach: false,
		Tty:    machineExec.Tty,
	})
	if err != nil {
		return nil, errors.New("Failed to attach to exec " + err.Error())
	}
	machineExec.Hjr = &hjr

	return machineExec, nil
}

func Resize(id int, cols uint, rows uint) error {
	machineExec := getById(id)
	if machineExec == nil {
		return errors.New("Exec to resize '" + strconv.Itoa(id) + "' was not found")
	}

	resizeParam := types.ResizeOptions{Height: rows, Width: cols}
	if err := cli.ContainerExecResize(context.Background(), machineExec.ExecId, resizeParam); err != nil {
		return err
	}

	return nil
}

func getById(id int) *model.MachineExec {
	defer machineExecs.mutex.Unlock()

	machineExecs.mutex.Lock()
	return machineExecs.execMap[id]
}

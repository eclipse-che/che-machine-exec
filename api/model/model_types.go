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

package model

import (
	"fmt"
	"io"
	"log"

	"github.com/docker/docker/api/types"
	"github.com/eclipse/che-go-jsonrpc/event"
	line_buffer "github.com/eclipse/che-machine-exec/output/line-buffer"
	"github.com/eclipse/che-machine-exec/output/utf8stream"
	ws_conn "github.com/eclipse/che-machine-exec/ws-conn"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	BufferSize = 8192

	// method names to send events with information about exec to the clients.
	OnExecExit  = "onExecExit"
	OnExecError = "onExecError"
)

// todo inside workspace we can get workspace id from env variables.
type MachineIdentifier struct {
	MachineName string `json:"machineName"`
	WsId        string `json:"workspaceId"`
}

// Todo code Refactoring: MachineExec should be simple object for exec creation, without any business logic
type MachineExec struct {
	Identifier MachineIdentifier `json:"identifier"`
	Cmd        []string          `json:"cmd"`
	// Supported value for now 'shell', "process". If type is empty "", than type will be "shell" like default valuechan type.
	Type string `json:"type"`

	Tty  bool   `json:"tty"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
	Cwd  string `json:"cwd"`

	ExitChan  chan bool
	ErrorChan chan error

	// unique client id, real execId should be hidden from client to prevent serialization
	ID int `json:"id"`

	// Todo Refactoring this code is docker specific. Create separated code layer and move it.
	ExecId string
	Hjr    *types.HijackedResponse

	ws_conn.ConnectionHandler

	MsgChan chan []byte

	// Todo Refactoring: this code is kubernetes specific. Create separated code layer and move it.
	Executor remotecommand.Executor
	SizeChan chan remotecommand.TerminalSize

	// Todo Refactoring: Create separated code layer and move it.
	Buffer *line_buffer.LineRingBuffer
}

type ExecExitEvent struct {
	event.E `json:"-"`

	ExecId int `json:"id"`
}

func (*ExecExitEvent) Type() string {
	return OnExecExit
}

type ExecErrorEvent struct {
	event.E `json:"-"`

	ExecId int    `json:"id"`
	Stack  string `json:"stack"`
}

func (*ExecErrorEvent) Type() string {
	return OnExecError
}

func (machineExec *MachineExec) Start() {
	if machineExec.Hjr == nil {
		return
	}

	go sendClientInputToExec(machineExec)
	go sendExecOutputToWebsockets(machineExec)
}

func sendClientInputToExec(machineExec *MachineExec) {
	for {
		data := <-machineExec.MsgChan
		if _, err := machineExec.Hjr.Conn.Write(data); err != nil {
			fmt.Println("Failed to write data to exec with id ", machineExec.ID, " Cause: ", err.Error())
			return
		}
	}
}

func sendExecOutputToWebsockets(machineExec *MachineExec) {
	hjReader := machineExec.Hjr.Reader
	buffer := make([]byte, BufferSize)
	filter := &utf8stream.Utf8StreamFilter{}

	for {
		_, err := hjReader.Read(buffer)
		if err != nil {
			remainder := filter.FlushBuffer()
			if len(remainder) > 0 {
				machineExec.Buffer.Write(remainder)
				machineExec.WriteDataToWsConnections(remainder)
			}

			if err == io.EOF {
				machineExec.ExitChan <- true
			} else {
				machineExec.ErrorChan <- err
				log.Println("failed to read exec stdOut/stdError stream. " + err.Error())
			}
			return
		}

		filteredBuffer := filter.ProcessRaw(buffer)

		if len(filteredBuffer) > 0 {
			machineExec.Buffer.Write(filteredBuffer)
			machineExec.WriteDataToWsConnections(filteredBuffer)
		}
	}
}

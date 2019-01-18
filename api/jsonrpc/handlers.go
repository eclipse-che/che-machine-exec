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

package jsonrpc

import (
	"github.com/eclipse/che-machine-exec/api/events"
	"github.com/eclipse/che-machine-exec/api/model"

	"github.com/eclipse/che-go-jsonrpc"
	"github.com/eclipse/che-machine-exec/exec"
	"log"
	"strconv"
)

type IdParam struct {
	Id int `json:"id"`
}

type OperationResult struct {
	Id   int    `json:"id"`
	Text string `json:"text"`
}

type ResizeParam struct {
	Id   int  `json:"id"`
	Cols uint `json:"cols"`
	Rows uint `json:"rows"`
}

var (
	execManager = exec.GetExecManager()
)

func jsonRpcCreateExec(_ *jsonrpc.Tunnel, params interface{}, t jsonrpc.RespTransmitter) {
	machineExec := params.(*model.MachineExec)

	id, err := execManager.Create(machineExec)

	healthWatcher := exec.NewHealthWatcher(machineExec, events.EventBus, execManager)
	healthWatcher.CleanUpOnExitOrError()

	if err != nil {
		log.Println("Unable to create machine exec. Cause: ", err.Error())
		t.SendError(jsonrpc.NewArgsError(err))
	}

	t.Send(id)
}

func jsonRpcCheckExec(_ *jsonrpc.Tunnel, params interface{}, t jsonrpc.RespTransmitter) {
	idParam := params.(*IdParam)

	id, err := execManager.Check(idParam.Id)
	if err != nil {
		t.SendError(jsonrpc.NewArgsError(err))
	}

	t.Send(id)
}

func jsonRpcResizeExec(_ *jsonrpc.Tunnel, params interface{}) (interface{}, error) {
	resizeParam := params.(*ResizeParam)

	if err := execManager.Resize(resizeParam.Id, resizeParam.Cols, resizeParam.Rows); err != nil {
		return nil, jsonrpc.NewArgsError(err)
	}

	return &OperationResult{
		Id: resizeParam.Id, Text: "Exec with id " + strconv.Itoa(resizeParam.Id) + "  was successfully resized",
	}, nil
}

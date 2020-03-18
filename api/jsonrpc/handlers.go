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

package jsonrpc

import (
	"errors"

	"github.com/eclipse/che-machine-exec/api/events"
	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/eclipse/che-machine-exec/cfg"
	"github.com/sirupsen/logrus"

	"strconv"

	"github.com/eclipse/che-go-jsonrpc"
	"github.com/eclipse/che-machine-exec/exec"
)

const (
	// BearerTokenAttr attribute name.
	BearerTokenAttr = "bearerToken"
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

func jsonRpcCreateExec(tunnel *jsonrpc.Tunnel, params interface{}, t jsonrpc.RespTransmitter) {
	machineExec := params.(*model.MachineExec)
	if cfg.UseBearerToken {
		if token, ok := tunnel.Attributes[BearerTokenAttr]; ok && len(token) > 0 {
			machineExec.BearerToken = token
		} else {
			err := errors.New("Bearer token should not be an empty")
			logrus.Errorf(err.Error())
			t.SendError(jsonrpc.NewArgsError(err))
			return
		}
	}

	id, err := execManager.Create(machineExec)

	healthWatcher := exec.NewHealthWatcher(machineExec, events.EventBus, execManager)
	healthWatcher.CleanUpOnExitOrError()

	if err != nil {
		logrus.Errorf("Unable to initialize terminal. Cause: %s", err.Error())
		t.SendError(jsonrpc.NewArgsError(err))
		return
	}

	if id == -1 {
		logrus.Errorln("A container where it's possible to initialize terminal is not found")
		t.SendError(jsonrpc.NewArgsError(errors.New("A container where it's possible to initialize terminal is not found")))
		return
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

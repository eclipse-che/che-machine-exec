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
	"github.com/eclipse/che-go-jsonrpc"
	"github.com/eclipse/che-machine-exec/api/model"
)

type TunnelWithUserToken struct {
	UserToken string

	*jsonrpc.Tunnel
}

// Constants that represent RPC methods identifiers.
const (
	// methods to manage exec life cycle
	CreateMethod = "create"
	CheckMethod  = "check"
	ResizeMethod = "resize"
)

// type ExecRouteGroup struct {
// 	jsonrpc.Route
// }

// RPCRoutes defines json-rpc exec api. This api uses to manage exec's life cycle.
var RPCRoutes = jsonrpc.RoutesGroup{
	Name: "Json-rpc MachineExec Routes",
	Items: []jsonrpc.Route{
		{
			Method: CreateMethod,
			Decode: jsonrpc.FactoryDec(func() interface{} { return &model.MachineExec{} }),
			Handle: jsonRpcCreateExec,
		},
		{
			Method: CheckMethod,
			Decode: jsonrpc.FactoryDec(func() interface{} { return &IdParam{} }),
			Handle: jsonRpcCheckExec,
		},
		{
			Method: ResizeMethod,
			Decode: jsonrpc.FactoryDec(func() interface{} { return &ResizeParam{} }),
			Handle: jsonrpc.HandleRet(jsonRpcResizeExec),
		},
	},
}

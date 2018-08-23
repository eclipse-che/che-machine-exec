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

package main

import (
	"flag"
	jsonRpcApi "github.com/eclipse/che-machine-exec/api/jsonrpc"
	"github.com/eclipse/che-machine-exec/api/websocket"
	"github.com/eclipse/che/agents/go-agents/core/jsonrpc"
	"github.com/eclipse/che/agents/go-agents/core/jsonrpc/jsonrpcws"
	"github.com/eclipse/che/agents/go-agents/core/rest"
	"net/http"
	"time"
)

var url string

func init() {
	flag.StringVar(&url, "url", ":4444", "Host:Port address. ")
}

func main() {
	flag.Parse()

	appRoutes := []rest.RoutesGroup{
		{
			Name: "Exec-Machine WebSocket routes",
			Items: []rest.Route{
				{
					Method: "GET",
					Path:   "/connect",
					Name:   "MachineExec api end point(websocket)", // json-rpc
					HandleFunc: func(w http.ResponseWriter, r *http.Request, _ rest.Params) error {
						conn, err := jsonrpcws.Upgrade(w, r)
						if err != nil {
							return err
						}
						tunnel := jsonrpc.NewManagedTunnel(conn)
						tunnel.SayHello()
						return nil
					},
				},
				// Todo: use json-rpc to send output too.
				{
					Method:     "GET",
					Path:       "/attach/:id",
					Name:       "Attach to exec(pure websocket)",
					HandleFunc: websocket.Attach,
				},
			},
		},
	}

	// create json-rpc routs group
	appOpRoutes := []jsonrpc.RoutesGroup{
		jsonRpcApi.RPCRoutes,
	}

	// register routes and http handlers
	baseHandler := rest.NewDefaultRouter(url, appRoutes)
	rest.PrintRoutes(appRoutes)
	jsonrpc.RegRoutesGroups(appOpRoutes)
	jsonrpc.PrintRoutes(appOpRoutes)

	server := &http.Server{
		Handler:      baseHandler,
		Addr:         url,
		WriteTimeout: 10 * time.Second,
		ReadTimeout:  10 * time.Second,
	}

	server.ListenAndServe()
}

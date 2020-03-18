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

package main

import (
	"errors"
	"flag"
	"net/http"
	"os"

	"github.com/eclipse/che-go-jsonrpc"
	"github.com/eclipse/che-go-jsonrpc/jsonrpcws"
	"github.com/eclipse/che-machine-exec/api/events"
	jsonRpcApi "github.com/eclipse/che-machine-exec/api/jsonrpc"
	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/eclipse/che-machine-exec/api/websocket"
	"github.com/eclipse/che-machine-exec/client"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)



func main() {
	// setLogLevel()
	flag.Parse()

	r := gin.Default()

	if staticPath != "" {
		r.StaticFS("/static", http.Dir(staticPath))
		r.GET("/", func(c *gin.Context) {
			c.Redirect(http.StatusMovedPermanently, "/static")
		})
	}

	// connect to exec api end point(websocket with json-rpc)
	r.GET("/connect", func(c *gin.Context) {
		var token string
		if client.UseBearerToken {
			token = c.Request.Header.Get("X-Forwarded-Access-Token")
			if len(token) == 0 {
				err := errors.New("unable to find user token header")
				logrus.Debug(err)
				c.JSON(c.Writer.Status(), err.Error())
				return
			}
		}

		conn, err := jsonrpcws.Upgrade(c.Writer, c.Request)
		if err != nil {
			c.JSON(c.Writer.Status(), err.Error())
			return
		}

		logrus.Debug("Create json-rpc channel for new websocket connnection")
		tunnel := jsonrpc.NewManagedTunnel(conn)
		if len(token) > 0 {
			tunnel.Attributes[client.BearerTokenAttr] = token
		}

		execConsumer := &events.ExecEventConsumer{Tunnel: tunnel}
		events.EventBus.SubAny(execConsumer, model.OnExecError, model.OnExecExit)

		tunnel.SayHello()
	})

	// attach to get exec output and sent user input(by simple websocket)
	// Todo: rework to use only one websocket connection https://github.com/eclipse/che-machine-exec/issues/4
	r.GET("/attach/:id", func(c *gin.Context) {
		if err := websocket.Attach(c.Writer, c.Request, c.Param("id")); err != nil {
			c.JSON(c.Writer.Status(), err.Error())
		}
	})

	// create json-rpc routs group
	appOpRoutes := []jsonrpc.RoutesGroup{
		jsonRpcApi.RPCRoutes,
	}
	// register routes
	jsonrpc.RegRoutesGroups(appOpRoutes)
	jsonrpc.PrintRoutes(appOpRoutes)

	if err := r.Run(url); err != nil {
		logrus.Fatal("Unable to start server. Cause: ", err.Error())
	}
}

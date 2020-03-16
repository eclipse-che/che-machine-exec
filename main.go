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

var (
	url, staticPath string
)

func setLogLevel() {
	logLevel, isFound := os.LookupEnv("LOG_LEVEL")
	if isFound && len(logLevel) > 0 {
		parsedLevel, err := logrus.ParseLevel(logLevel)
		if err == nil {
			logrus.SetLevel(parsedLevel)
			logrus.Infof("Configured '%s' log level is applied", logLevel)
		} else {
			logrus.Errorf("Failed to parse log level `%s`. Possible values: panic, fatal, error, warn, info, debug. Default 'info' is applied", logLevel)
			logrus.SetLevel(logrus.InfoLevel)
		}
	} else {
		logrus.Infof("Default 'info' log level is applied")
		logrus.SetLevel(logrus.InfoLevel)
	}
}

func init() {
	flag.StringVar(&url, "url", ":4444", "Host:Port address.")
	flag.StringVar(&staticPath, "static", "", "/home/user/frontend - absolute path to folder with static resources.")
}

func main() {
	setLogLevel()
	flag.Parse()

	r := gin.Default()

	if staticPath != "" {
		r.StaticFS("/static", http.Dir(staticPath))
		r.GET("/", func(c *gin.Context) {
			c.Redirect(http.StatusMovedPermanently, "/static")
		})
	}

	// connect to exec api end point(websocket with json-rpc)
	r.GET("/connect", func(c *gin.Context) { // separated handler
		var userToken string
		if client.UseUserToken {
			userToken = c.Request.Header.Get("X-Forwarded-Access-Token")
			if len(userToken) == 0 {
				err := errors.New("unable to find user token header")
				logrus.Debug(err)
				c.JSON(c.Writer.Status(), err.Error())
				return
			}
		}

		// logrus.Infof("Cookies: %+v", c.Request.Cookies())
		conn, err := jsonrpcws.Upgrade(c.Writer, c.Request)
		if err != nil {
			c.JSON(c.Writer.Status(), err.Error()) // todo error code
			return
		}

		logrus.Debug("Create json-rpc channel for new websocket connnection")
		tunnel := jsonrpc.NewManagedTunnel(conn)
		if len(userToken) > 0 {
			tunnel.Attributes[client.UserTokenAttr] = userToken
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

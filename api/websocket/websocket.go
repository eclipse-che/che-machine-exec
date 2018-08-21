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

package websocket

import (
	"errors"
	"fmt"
	"github.com/eclipse/che-lib/websocket"
	"github.com/eclipse/che-machine-exec/api/model"
	execManager "github.com/eclipse/che-machine-exec/exec"
	"github.com/eclipse/che/agents/go-agents/core/rest"
	"log"
	"net/http"
	"strconv"
	"time"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	PingPeriod = 30 * time.Second
)

func Attach(w http.ResponseWriter, r *http.Request, restParmas rest.Params) error {
	id, err := strconv.Atoi(restParmas.Get("id"))
	if err != nil {
		return errors.New("failed to parse id")
	}
	fmt.Println("Parsed id", id)

	machineExec, err := execManager.Attach(id)
	if err != nil {
		return err
	}

	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Unable to upgrade connection to websocket " + err.Error())
		return err
	}

	restoreContent := machineExec.Buffer.GetContent()
	wsConn.WriteMessage(websocket.TextMessage, []byte(restoreContent))

	machineExec.AddWebSocket(wsConn)

	if !machineExec.Started {
		machineExec.Start()
	}

	go sendPingMessage(wsConn)
	go readWebSocketData(machineExec, wsConn)

	return nil
}

func sendPingMessage(wsConn *websocket.Conn) {
	ticker := time.NewTicker(PingPeriod)
	defer ticker.Stop()

	for range ticker.C {
		if err := wsConn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
			log.Printf("Error occurs on sending ping message to websocket. %v", err)
			return
		}
	}
}

func readWebSocketData(machineExec *model.MachineExec, wsConn *websocket.Conn) {
	defer machineExec.RemoveWebSocket(wsConn)

	for {
		msgType, wsBytes, err := wsConn.ReadMessage()
		if err != nil {
			log.Printf("failed to get read websocket message")
			return
		}

		if msgType != websocket.TextMessage {
			continue
		}

		machineExec.MsgChan <- wsBytes
	}
}

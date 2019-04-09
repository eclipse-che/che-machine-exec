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

package websocket

import (
	"errors"
	"github.com/eclipse/che-machine-exec/exec"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"strconv"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func Attach(w http.ResponseWriter, r *http.Request, idParam string) error {
	id, err := strconv.Atoi(idParam)
	if err != nil {
		return errors.New("failed to parse id")
	}

	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Unable to upgrade connection to ws-conn " + err.Error())
		return err
	}

	if err = exec.GetExecManager().Attach(id, wsConn); err != nil {
		log.Println("Attach to exec", strconv.Itoa(id), " failed. Cause:  ", err.Error())
		return err
	}

	return nil
}

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

package rest

import (
	"net/http"

	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/eclipse/che-machine-exec/exec"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

var (
	execManager = exec.GetExecManager()
)

func HandleKubeConfig(c *gin.Context) {
	token := c.Request.Header.Get(model.BearerTokenHeader)
	if token == "" {
		c.Writer.WriteHeader(http.StatusUnauthorized)
		_, err := c.Writer.Write([]byte("Authorization token must not be empty"))
		if err != nil {
			logrus.Error("Failed to write error response", err)
		}
		return
	}

	var kubeConfigParams model.KubeConfigParams
	if c.BindJSON(&kubeConfigParams) != nil {
		c.Writer.WriteHeader(http.StatusInternalServerError)
		_, err := c.Writer.Write([]byte("Failed to convert body args into internal structure"))
		if err != nil {
			logrus.Error("Failed to write error response", err)
		}
		return
	}

	if kubeConfigParams.ContainerName == "" {
		c.Writer.WriteHeader(http.StatusBadRequest)
		_, err := c.Writer.Write([]byte("Container name is required"))
		if err != nil {
			logrus.Error("Failed to write error response", err)
		}
		return
	}

	if kubeConfigParams.Username == "" {
		kubeConfigParams.Username = "Developer"
	}
	kubeConfigParams.BearerToken = token
	err := execManager.CreateKubeConfig(&kubeConfigParams)

	if err != nil {
		logrus.Errorf("Unable to create kubeconfig. Cause: %s", err.Error())
		c.Writer.WriteHeader(http.StatusInternalServerError)
		_, err := c.Writer.Write([]byte(err.Error()))
		if err != nil {
			logrus.Error("Failed to write error response", err)
		}
	}
}

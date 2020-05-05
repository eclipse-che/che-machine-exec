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
	"encoding/json"
	"fmt"
	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/eclipse/che-machine-exec/exec"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"net/http"
)

var (
	execManager = exec.GetExecManager()
)

func HandleResolve(c *gin.Context) {
	token := c.Request.Header.Get(model.BearerTokenHeader)
	if token == "" {
		c.Writer.WriteHeader(http.StatusUnauthorized)
		_, err := c.Writer.Write([]byte("Authorization token must not be empty"))
		if err != nil {
			logrus.Error("Failed to write error response", err)
		}
	}

	container := c.Param("container")

	resolvedExec, err := execManager.Resolve(container, token)

	if err != nil {
		c.Writer.WriteHeader(http.StatusInternalServerError)
		_, err := c.Writer.Write([]byte(fmt.Sprintf("Unable to resolve exec. Cause: %s", err.Error())))
		if err != nil {
			logrus.Error("Failed to write error response", err)
		}
		return
	}

	marshal, err := json.Marshal(resolvedExec)
	if err != nil {
		c.Writer.WriteHeader(http.StatusInternalServerError)
		_, err := c.Writer.Write([]byte(fmt.Sprintf("Failed to marshal resolved exec. Cause: %s", err.Error())))
		if err != nil {
			logrus.Error("Failed to write error response", err)
		}
		return
	}

	_, err = c.Writer.Write(marshal)
	if err != nil {
		logrus.Error("Failed to write response", err)
	}
}

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

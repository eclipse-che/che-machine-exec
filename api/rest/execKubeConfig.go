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
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func HandleKubeConfig(c *gin.Context) {
	token := c.Request.Header.Get(model.BearerTokenHeader)
	if token == "" {
		writeResponse(c, http.StatusUnauthorized, "Authorization token must not be empty")
		return
	}

	var initConfigParams model.InitConfigParams
	if c.BindJSON(&initConfigParams) != nil {
		writeResponse(c, http.StatusInternalServerError, "Failed to convert body args into internal structure")
		return
	}

	if initConfigParams.ContainerName == "" {
		c.Writer.WriteHeader(http.StatusBadRequest)
		_, err := c.Writer.Write([]byte("Container name is required"))
		if err != nil {
			logrus.Error("Failed to write error response", err)
		}
		return
	}

	err := HandleKubeConfigCreation(c, &initConfigParams, token)

	if err != nil {
		logrus.Errorf("Unable to create kubeconfig. Cause: %s", err.Error())
		writeResponse(c, http.StatusInternalServerError, err.Error())
	}
}

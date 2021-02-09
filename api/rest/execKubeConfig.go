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
	"github.com/eclipse/che-machine-exec/auth"
	"github.com/eclipse/che-machine-exec/common/rest"
	"net/http"

	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func HandleKubeConfig(c *gin.Context) {
	var token string
	if auth.IsEnabled() {
		var err error
		token, err = auth.Authenticate(c)
		if err != nil {
			rest.WriteErrorResponse(c, err)
			return
		}
	}

	var initConfigParams model.InitConfigParams
	if c.BindJSON(&initConfigParams) != nil {
		rest.WriteResponse(c, http.StatusInternalServerError, "Failed to convert body args into internal structure")
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

	err := HandleKubeConfigCreation(&initConfigParams.KubeConfigParams, token, initConfigParams.ContainerName)

	if err != nil {
		logrus.Errorf("Unable to create kubeconfig. Cause: %s", err.Error())
		rest.WriteResponse(c, http.StatusInternalServerError, err.Error())
	}
}

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
	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/gin-gonic/gin"
)

func HandleKubeConfigCreation(c *gin.Context, initConfigParams *model.InitConfigParams, token string) error {
	if initConfigParams.Username == "" {
		initConfigParams.Username = "Developer"
	}

	initConfigParams.BearerToken = token
	err := execManager.CreateKubeConfig(initConfigParams)
	return err
}

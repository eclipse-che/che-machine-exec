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
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func writeResponse(c *gin.Context, httpStatus int, message string) {
	c.Writer.WriteHeader(httpStatus)
	_, err := c.Writer.Write([]byte(message))
	if err != nil {
		logrus.Error("Failed to write error response", err)
	}
}

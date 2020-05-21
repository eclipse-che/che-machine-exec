//
// Copyright (c) 2012-2020 Red Hat, Inc.
// This program and the accompanying materials are made
// available under the terms of the Eclipse Public License 2.0
// which is available at https://www.eclipse.org/legal/epl-2.0/
//
// SPDX-License-Identifier: EPL-2.0
//
// Contributors:
//   Red Hat, Inc. - initial API and implementation
//

package auth

import (
	restUtil "github.com/eclipse/che-machine-exec/common/rest"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"net/http"
	"strings"
)

const (
	AccessTokenHeader          = "X-Access-Token"
	ForwardedAccessTokenHeader = "X-Forwarded-Access-Token"
)

func extractToken(c *gin.Context) (string, error) {
	token := c.Request.Header.Get(AccessTokenHeader)
	if token != "" {
		logrus.Debug("Access token is found " + token)
		token = strings.TrimSuffix(token, "Bearer ")
		return token, nil
	}

	token = c.Request.Header.Get(ForwardedAccessTokenHeader)
	if token != "" {
		logrus.Debug("Forwarded access token is found. " + token)
		return token, nil
	}

	return "", restUtil.NewError(http.StatusUnauthorized, "authorization header is missing")
}

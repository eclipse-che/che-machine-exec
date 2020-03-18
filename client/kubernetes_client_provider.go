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

package client

import (
	"errors"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"os"
	"strconv"
)

var (
	// UseBearerToken - flag to enable/disable using bearer token to have access to k8s api.
	UseBearerToken bool
)

const (
	// BearerTokenAttr attribute name.
	BearerTokenAttr = "bearerToken"
	// UseBearerTokenEnvName env variable, boolean flag to enable/disable using bearer token to have access to k8s api.
	UseBearerTokenEnvName = "USE_BEARER_TOKEN"
)

func init() {
	tokenEnvValue, isFound := os.LookupEnv(BearerTokenAttr)
	logrus.Debugf("Use bearer token env value: %s", tokenEnvValue)

	if isFound && len(tokenEnvValue) > 0 {
		if value, err := strconv.ParseBool(tokenEnvValue); err != nil {
			logrus.Panicf("Invalid value '%s' for env varible key '%s'. Supported values: 'true', 'false'.", tokenEnvValue, BearerTokenAttr)
		} else {
			UseBearerToken = value
		}
	}
}

// K8sAPI object to access k8s api.
type K8sAPI struct {
	config *rest.Config
	client *kubernetes.Clientset
}

// NewK8sAPI constructor for creation new k8s api access object.
func NewK8sAPI(config *rest.Config, client *kubernetes.Clientset) *K8sAPI {
	return &K8sAPI{config: config, client: client}
}

// GetClient returns k8s client.
func (api *K8sAPI) GetClient() *kubernetes.Clientset {
	return api.client
}

// GetConfig returns k8s config.
func (api *K8sAPI) GetConfig() *rest.Config {
	return api.config
}

// K8sAPIProvider for creation new K8sAPI.
type K8sAPIProvider struct {
	k8sAPI *K8sAPI
}

// NewK8sAPIProvider creates new K8sAPI provider.
func NewK8sAPIProvider() *K8sAPIProvider {
	return &K8sAPIProvider{}
}

// GetK8sAPI returns k8sApi.
func (clientProvider *K8sAPIProvider) GetK8sAPI() (*K8sAPI, error) {
	var err error
	if clientProvider.k8sAPI == nil {
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		client, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, err
		}

		clientProvider.k8sAPI = NewK8sAPI(config, client)
	}

	return clientProvider.k8sAPI, err
}

// GetK8sAPIWithBearerToken returns k8sApi with bearer token.
func (clientProvider *K8sAPIProvider) GetK8sAPIWithBearerToken(token string) (*K8sAPI, error) {
	if len(token) == 0 {
		return nil, errors.New("Failed to create k8sAPI. Token should not be an empty")
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	config.BearerToken = token

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return NewK8sAPI(config, client), nil
}

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

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	// UseUserToken flag to use user token
	UseUserToken bool // TODO grab this value from env!!!!
)

// K8sAPI object to access k8s api
type K8sAPI struct {
	config *rest.Config
	client *kubernetes.Clientset
}

// NewK8sAPI constructor for creation new k8s api access object.
func NewK8sAPI(config *rest.Config, client *kubernetes.Clientset) *K8sAPI {
	return &K8sAPI{config: config, client: client}
}

// GetClient returns k8s client
func (api *K8sAPI) GetClient() *kubernetes.Clientset {
	return api.client
}

// GetConfig returns k8s config
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

// Getk8sAPI returns k8sApi.
func (clientProvider *K8sAPIProvider) Getk8sAPI() (*K8sAPI, error) {
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

// Getk8sAPIWithUserToken returns k8sApi with user token.
func (clientProvider *K8sAPIProvider) Getk8sAPIWithUserToken(userBearerToken string) (*K8sAPI, error) {
	if len(userBearerToken) == 0 {
		return nil, errors.New("Failed to create k8sAPI. User token should not be an empty")
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	config.BearerToken = userBearerToken

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return NewK8sAPI(config, client), nil
}

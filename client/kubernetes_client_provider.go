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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Provider for creation new kubernetes api client.
type KubernetesClientProvider struct {
	config *rest.Config
	client *kubernetes.Clientset
}

// Create new kubernetes api client provider.
func NewKubernetesClientProvider() *KubernetesClientProvider {
	config := createConfig()
	return &KubernetesClientProvider{
		config: config,
		client: createClient(config),
	}
}

// Create configuration to work with kubernetes api inside pod.
func createConfig() *rest.Config {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	return config
}

// Create client set to use kubernetes api inside pod.
func createClient(config *rest.Config) *kubernetes.Clientset {
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return clientset
}

// Get kubernetes configuration inside cluster pod.
func (clientProvider *KubernetesClientProvider) GetKubernetesConfig() *rest.Config {
	return clientProvider.config
}

// Get kubernetes client set inside cluster pod.
func (clientProvider *KubernetesClientProvider) GetKubernetesClient() *kubernetes.Clientset {
	return clientProvider.client
}

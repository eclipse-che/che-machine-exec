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
	"github.com/docker/docker/client"
)

// Provider to get docker api client.
type DockerClientProvider struct {
	dockerClient *client.Client
}

// Create new docker api client provider.
func NewDockerClientProvider() *DockerClientProvider {
	return &DockerClientProvider{dockerClient: createDockerClient()}
}

func createDockerClient() *client.Client {
	dockerClient, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	// set up minimal docker version 1.13.0(api version 1.25).
	dockerClient.UpdateClientVersion("1.25")

	return dockerClient
}

// Return new docker client api to work with container and exec api.
func (clientProvider *DockerClientProvider) GetDockerClient() client.ContainerAPIClient {
	return clientProvider.dockerClient
}

//
// Copyright (c) 2012-2018 Red Hat, Inc.
// This program and the accompanying materials are made
// available under the terms of the Eclipse Public License 2.0
// which is available at https://www.eclipse.org/legal/epl-2.0/
//
// SPDX-License-Identifier: EPL-2.0
//
// Contributors:
//   Red Hat, Inc. - initial API and implementation
//

package exec_info

import (
	"github.com/docker/docker/client"
)

// Component to creation new info execs on the docker infrastructure.
type DockerInfoExecCreator struct {
	InfoExecCreator

	client client.ContainerAPIClient
}

// Return new instance of the docker info exec creator.
func NewDockerInfoExecCreator(client client.ContainerAPIClient) *DockerInfoExecCreator {
	return &DockerInfoExecCreator{client: client}
}

// Create new docker info exec.
func (creator *DockerInfoExecCreator) CreateInfoExec(command []string, containerInfo map[string]string) InfoExec {
	containerId := containerInfo[ContainerId]
	return NewDockerInfoExec(command, containerId, creator.client)
}

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

package filter

import (
	"errors"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/eclipse/che-machine-exec/exec-info"
	"golang.org/x/net/context"
)

const (
	Label       = "label"
	WsId        = "org.eclipse.che.workspace.id"
	MachineName = "org.eclipse.che.machine.name"
)

// Create new container filter for docker infrastructure.
type DockerContainerFilter struct {
	ContainerFilter

	client client.ContainerAPIClient
}

// Create new docker container filter.
func NewDockerContainerFilter(client client.ContainerAPIClient) *DockerContainerFilter {
	return &DockerContainerFilter{client: client}
}

// Filter container by labels: wsId and machineName.
func (filter *DockerContainerFilter) FindContainerInfo(identifier *model.MachineIdentifier) (containerInfo map[string]string, err error) {
	containers, err := filter.client.ContainerList(context.Background(), types.ContainerListOptions{
		Filters: createContainerFilter(identifier),
	})
	if err != nil {
		return nil, err
	}

	if len(containers) > 1 {
		return nil, errors.New("filter found more than one machine")
	}
	if len(containers) == 0 {
		return nil, errors.New("machine " + identifier.MachineName + " was not found")
	}

	containerInfo = make(map[string]string)
	containerInfo[exec_info.ContainerId] = containers[0].ID
	return containerInfo, nil
}

func createContainerFilter(identifier *model.MachineIdentifier) filters.Args {
	filterArgs := filters.NewArgs()
	filterArgs.Add(Label, WsId+"="+identifier.WsId)
	filterArgs.Add(Label, MachineName+"="+identifier.MachineName)

	return filterArgs
}

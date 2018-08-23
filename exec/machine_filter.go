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

package exec

import (
	"errors"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/eclipse/che-machine-exec/api/model"
	"golang.org/x/net/context"
)

const (
	WsId        = "org.eclipse.che.workspace.id"
	MachineName = "org.eclipse.che.machine.name"
	Label       = "label"
)

// Filter container by labels: wsId and machineName.
func findMachineContainer(identifier *model.MachineIdentifier) (*types.Container, error) {
	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{
		Filters: createMachineFilter(identifier),
	})
	if err != nil {
		return nil, err
	}

	if len(containers) > 1 {
		return nil, errors.New("filter found more than one machine")
	}
	if len(containers) == 0 {
		return nil, errors.New("machine was not found")
	}

	return &containers[0], nil
}

func createMachineFilter(identifier *model.MachineIdentifier) filters.Args {
	filterArgs := filters.NewArgs()
	filterArgs.Add(Label, WsId+"="+identifier.WsId)
	filterArgs.Add(Label, MachineName+"="+identifier.MachineName)

	return filterArgs
}

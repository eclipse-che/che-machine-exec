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

package filter

import (
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/eclipse/che-machine-exec/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"testing"
)

const (
	ContainerId  = "someId"
	ContainerId2 = "someId2"
	ContainerId3 = "someId3"
)

var machineIdentifier = &model.MachineIdentifier{"dev-machine", "workspaceIdSome"}

func TestShouldFindContainerFromLinsWithOneContainersAndGetInfo(t *testing.T) {
	mockContainerClient := &mocks.ContainerAPIClient{}

	container := types.Container{ID: ContainerId}
	containers := []types.Container{container}
	mockContainerClient.On("ContainerList", mock.Anything, mock.Anything).Return(containers, nil)

	filter := NewDockerContainerFilter(mockContainerClient)

	containerInfo, err := filter.FindContainerInfo(machineIdentifier)
	fmt.Println(err)

	assert.Nil(t, err)

	assert.Equal(t, containerInfo["containerId"], ContainerId)

	mockContainerClient.AssertExpectations(t)
	mockContainerClient.AssertExpectations(t)
}

func TestShouldThrowErrorIfWasFoundFewContainersWithSameMachineIdentifier(t *testing.T) {
	mockContainerClient := &mocks.ContainerAPIClient{}

	container := types.Container{ID: ContainerId}
	container2 := types.Container{ID: ContainerId2}
	container3 := types.Container{ID: ContainerId3}
	containers := []types.Container{container, container2, container3}

	mockContainerClient.On("ContainerList", mock.Anything, mock.Anything).Return(containers, nil)

	filter := NewDockerContainerFilter(mockContainerClient)

	_, err := filter.FindContainerInfo(machineIdentifier)
	fmt.Println(err)

	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), "filter found more than one machine")

	mockContainerClient.AssertExpectations(t)
	mockContainerClient.AssertExpectations(t)
}

func TestShouldThrowErrorIfContainerListIsEmpty(t *testing.T) {
	mockContainerClient := &mocks.ContainerAPIClient{}

	containers := []types.Container{}
	mockContainerClient.On("ContainerList", mock.Anything, mock.Anything).Return(containers, nil)

	filter := NewDockerContainerFilter(mockContainerClient)

	_, err := filter.FindContainerInfo(machineIdentifier)
	fmt.Println(err)

	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), "machine "+machineIdentifier.MachineName+" was not found")

	mockContainerClient.AssertExpectations(t)
	mockContainerClient.AssertExpectations(t)
}

func TestShouldReThrowErrorFromApi(t *testing.T) {
	mockContainerClient := &mocks.ContainerAPIClient{}

	containers := []types.Container{}
	apiErr := errors.New("Some Error")
	mockContainerClient.On("ContainerList", mock.Anything, mock.Anything).Return(containers, apiErr)

	filter := NewDockerContainerFilter(mockContainerClient)

	_, err := filter.FindContainerInfo(machineIdentifier)
	fmt.Println(err)

	assert.NotNil(t, err)
	assert.Equal(t, err.Error(), "Some Error")

	mockContainerClient.AssertExpectations(t)
	mockContainerClient.AssertExpectations(t)
}

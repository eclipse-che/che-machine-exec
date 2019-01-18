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
	"bytes"
	"context"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"io"
)

const (
	ContainerId = "containerId"
)

// Exec to get some information from container.
// Command for such exec should be
// "endless" and simple(For example: "whoami", "arch", "env").
// It should not be shell based command.
// This exec is always not "tty" and doesn't provide sending input to the command.
type DockerInfoExec struct {
	InfoExec

	// Command with arguments
	command []string

	// Unique docker container id
	containerId string

	// Buffer to store exec output.
	stdOut *bytes.Buffer

	// Docker api client
	client client.ContainerAPIClient
}

// Create mew docker info exec.
func NewDockerInfoExec(command []string, containerId string, client client.ContainerAPIClient) *DockerInfoExec {
	var stdOut bytes.Buffer
	return &DockerInfoExec{
		command:     command,
		containerId: containerId,
		stdOut:      &stdOut,
		client:      client,
	}
}

// Start new docker info exec. Return err in case fail.
func (exec *DockerInfoExec) Start() (err error) {
	resp, err := exec.client.ContainerExecCreate(context.Background(), exec.containerId, types.ExecConfig{
		Tty:          false,
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
		Detach:       false,
		Cmd:          exec.command,
	})
	if err != nil {
		return err
	}

	hjr, err := exec.client.ContainerExecAttach(context.Background(), resp.ID, types.ExecConfig{
		Detach: false,
		Tty:    false,
	})
	if err != nil {
		return err
	}

	_, err = io.Copy(exec.stdOut, hjr.Reader)
	return err
}

// Get exec output content.
func (exec *DockerInfoExec) GetOutput() string {
	return string(exec.stdOut.Bytes())
}

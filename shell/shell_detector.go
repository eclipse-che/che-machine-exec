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

package shell

import (
	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/eclipse/che-machine-exec/exec-info"
)

const (
	DefaultShell = "sh"
)

// ContainerShellDetector used to get information about preferable exec shell
// defined inside container for current active user.
// Information about preferable shell we get from /etc/passwd file and user Id.
type ContainerShellDetector interface {
	// Detect preferable shell inside container for current user id.
	// Create new info exec get information about preferable default shell.
	DetectShell(containerInfo *model.ContainerInfo) (shell string, err error)
}

// Component to detect shell inside container with help creation information execs.
type ShellDetector struct {
	ContainerShellDetector
	exec_info.InfoExecCreator
	ExecInfoParser
}

// Create new shell detector inside container.
func NewShellDetector(execInfoCreator exec_info.InfoExecCreator, parser ExecInfoParser) *ShellDetector {
	return &ShellDetector{
		InfoExecCreator: execInfoCreator,
		ExecInfoParser:  parser,
	}
}

// Detect default shell inside container by container info.
func (detector *ShellDetector) DetectShell(containerInfo *model.ContainerInfo) (shell string, err error) {
	getUserIdCommand := []string{"id", "-u"}
	userIdContent, err := detector.spawnExecInfo(getUserIdCommand, containerInfo)
	if err != nil {
		return "", err
	}

	userId, err := detector.ParseUID(userIdContent)
	if err != nil {
		return "", err
	}

	getEtcPassWdCommand := []string{"cat", "/etc/passwd"}
	etcPassWdContent, err := detector.spawnExecInfo(getEtcPassWdCommand, containerInfo)
	if err != nil {
		return "", err
	}

	return detector.ParseShellFromEtcPassWd(etcPassWdContent, userId)
}

func (detector ShellDetector) spawnExecInfo(command []string, containerInfo *model.ContainerInfo) (execOutPut string, err error) {
	execInfo := detector.CreateInfoExec(command, containerInfo)

	if err := execInfo.Start(); err != nil {
		return "", err
	}

	return execInfo.GetOutput(), nil
}

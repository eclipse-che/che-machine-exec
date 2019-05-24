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

package kubernetes_infra

import (
	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/eclipse/che-machine-exec/shell"
	"strings"
	"fmt"
	"net/url"
)

type CmdResolver struct {
	shell.ContainerShellDetector
}

func NewCmdResolver(shellDetector shell.ContainerShellDetector) *CmdResolver {
	return &CmdResolver{
		ContainerShellDetector: shellDetector,
	}
}

func (cmdRslv *CmdResolver) resolveCmd(exec *model.MachineExec, containerInfo map[string]string) (resolvedCmd []string) {
	var (
		argsPart = exec.Cmd
		cmd = exec.Cmd;
		shell, cdCommand string
	)

	if exec.IsShell && len(cmd) >= 2 && cmd[1] == "-c" {
		shell = cmd[0]
		argsPart = cmd[2:len(cmd)]
	} else {
		shell = cmdRslv.setUpExecShellPath(exec, containerInfo)
	}

	if exec.Cwd != "" {
		if strings.HasPrefix(exec.Cwd, "file://") {
			if res, err := url.Parse(exec.Cwd); err == nil {
				exec.Cwd = res.Path
			}
		}
		cdCommand = fmt.Sprintf("cd %s;", exec.Cwd)
	}

	return []string{shell, "-c", cdCommand + strings.Join(argsPart, " ")}
}

func (cmdRslv *CmdResolver) setUpExecShellPath(exec *model.MachineExec, containerInfo map[string]string) (shellPath string) {
	if containerShell, err := cmdRslv.DetectShell(containerInfo); err == nil {
		return containerShell
	}
	return shell.DefaultShell
}

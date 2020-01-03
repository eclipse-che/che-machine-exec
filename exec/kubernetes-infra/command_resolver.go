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
	"fmt"
	"net/url"
	"strings"

	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/eclipse/che-machine-exec/shell"
)

// CmdResolver resolves exec command - MachineExec#Cmd. Needed to patch command
// to apply some features which missed up in the original kubernetes exec api.
type CmdResolver struct {
	shell.ContainerShellDetector
}

// NewCmdResolver creates new instance CmdResolver.
func NewCmdResolver(shellDetector shell.ContainerShellDetector) *CmdResolver {
	return &CmdResolver{
		ContainerShellDetector: shellDetector,
	}
}

// Gets original command from exec model(MachineExec#Cmd) and returns patched command
// to support some features which original kubernetes api doesn't provide.
func (cmdRslv *CmdResolver) ResolveCmd(exec model.MachineExec, containerInfo *model.ContainerInfo) (resolvedCmd []string) {
	var (
		shell, cdCommand string
		cmd              = exec.Cmd
	)

	if cmd == nil {
		cmd = []string{}
	}

	if (exec.Type == "" || exec.Type == "shell") && len(cmd) > 0 {
		shell = cmd[0]
	}

	if shell == "" {
		shell = cmdRslv.setUpExecShellPath(exec, containerInfo)
	}

	if len(cmd) >= 2 && cmd[1] == "-c" {
		cmd = cmd[2:len(cmd)]
	}
	if len(cmd) == 0 {
		cmd = []string{shell}
	}

	if exec.Cwd != "" {
		if strings.HasPrefix(exec.Cwd, "file://") {
			if res, err := url.Parse(exec.Cwd); err == nil {
				exec.Cwd = res.Path
			}
		}
		cdCommand = fmt.Sprintf("cd %s; ", exec.Cwd)
	}

	return []string{shell, "-c", cdCommand + strings.Join(cmd, " ")}
}

func (cmdRslv *CmdResolver) setUpExecShellPath(exec model.MachineExec, containerInfo *model.ContainerInfo) (shellPath string) {
	if containerShell, err := cmdRslv.DetectShell(containerInfo); err == nil && cmdRslv.shellIsDefined(containerShell) {
		return containerShell
	}
	return shell.DefaultShell
}

func (cmdRslv *CmdResolver) shellIsDefined(shell string) bool {
	if strings.HasSuffix(shell, "nologin") {
		return false
	}
	return true
}

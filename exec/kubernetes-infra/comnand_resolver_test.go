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
	"github.com/eclipse/che-machine-exec/mocks"
	"github.com/stretchr/testify/assert"
	"testing"
)

var (
	containerInfo = make(map[string]string)
)

func TestShoudBeLaunchedShellProcessWithCwd(t *testing.T) {
	exec := model.MachineExec{
		Type: "shell",
		Cmd:  []string{"sh", "-c", "sleep 5 && echo 'ABC' && ls -a -li && pwd"},
		Cwd:  "/projects/testprj",
	}

	cmdResolver := NewCmdResolver(&mocks.ContainerShellDetector{})
	resolvedCmd := cmdResolver.ResolveCmd(exec, containerInfo)

	assert.Equal(t, []string{"sh", "-c", "cd /projects/testprj; sleep 5 && echo 'ABC' && ls -a -li && pwd"}, resolvedCmd)
}

func TestShoudBeLaunchedShellProcessWithoutCwd(t *testing.T) {
	exec := model.MachineExec{
		Type: "shell",
		Cmd:  []string{"sh", "-c", "sleep 5 && echo 'ABC' && ls -a -li && pwd"},
	}

	cmdResolver := NewCmdResolver(&mocks.ContainerShellDetector{})
	resolvedCmd := cmdResolver.ResolveCmd(exec, containerInfo)

	assert.Equal(t, []string{"sh", "-c", "sleep 5 && echo 'ABC' && ls -a -li && pwd"}, resolvedCmd)
}

func TestShouldLaunchTerminalProcessWithCwd(t *testing.T) {
	exec := model.MachineExec{
		Type: "terminal",
		Cmd:  []string{"sh", "-l"},
		Cwd:  "/projects/testprj",
	}

	cmdResolver := NewCmdResolver(&mocks.ContainerShellDetector{})
	resolvedCmd := cmdResolver.ResolveCmd(exec, containerInfo)

	assert.Equal(t, []string{"sh", "-c", "cd /projects/testprj; sh -l"}, resolvedCmd)
}

func TestShouldLaunchTerminalProcessWithoutCwd(t *testing.T) {
	exec := model.MachineExec{
		Type: "terminal",
		Cmd:  []string{"sh", "-l"},
	}

	cmdResolver := NewCmdResolver(&mocks.ContainerShellDetector{})
	resolvedCmd := cmdResolver.ResolveCmd(exec, containerInfo)

	assert.Equal(t, []string{"sh", "-c", "sh -l"}, resolvedCmd)
}

func TestShouldAutoDetectShellForTerminalCommandWithCwd(t *testing.T) {
	shellDetectorMock := &mocks.ContainerShellDetector{}
	shellDetectorMock.On("DetectShell", containerInfo).Return("bash", nil)
	exec := model.MachineExec{
		Type: "terminal",
		Cwd:  "/projects/testprj",
	}

	cmdResolver := NewCmdResolver(shellDetectorMock)
	resolvedCmd := cmdResolver.ResolveCmd(exec, containerInfo)

	assert.Equal(t, []string{"bash", "-c", "cd /projects/testprj; bash"}, resolvedCmd)
}

func TestShouldAutoDetectShellForTerminalCommandWithoutCwd(t *testing.T) {
	shellDetectorMock := &mocks.ContainerShellDetector{}
	shellDetectorMock.On("DetectShell", containerInfo).Return("bash", nil)

	exec := model.MachineExec{
		Type: "terminal",
	}

	cmdResolver := NewCmdResolver(shellDetectorMock)
	resolvedCmd := cmdResolver.ResolveCmd(exec, containerInfo)

	assert.Equal(t, []string{"bash", "-c", "bash"}, resolvedCmd)
}

func TestShouldBeLaunedShellCommandWithAnEmptyCmd(t *testing.T) {
	shellDetectorMock := &mocks.ContainerShellDetector{}
	shellDetectorMock.On("DetectShell", containerInfo).Return("fish", nil)
	exec := model.MachineExec{
		Type: "shell",
	}

	cmdResolver := NewCmdResolver(shellDetectorMock)
	resolvedCmd := cmdResolver.ResolveCmd(exec, containerInfo)

	assert.Equal(t, []string{"fish", "-c", ""}, resolvedCmd)
}

func TestShouldBeResolveCwdLikeUriForShellCommand(t *testing.T) {
	exec := model.MachineExec{
		Type: "shell",
		Cmd:  []string{"sh", "-c", "mvn clean install"},
		Cwd:  "file:///projects/testprj",
	}

	cmdResolver := NewCmdResolver(&mocks.ContainerShellDetector{})
	resolvedCmd := cmdResolver.ResolveCmd(exec, containerInfo)

	assert.Equal(t, []string{"sh", "-c", "cd /projects/testprj; mvn clean install"}, resolvedCmd)
}

func TestShouldBeResolveCwdLikeUriForTerminalCommand(t *testing.T) {
	exec := model.MachineExec{
		Type: "terminal",
		Cmd:  []string{"sh", "-l"},
		Cwd:  "file:///projects/testprj",
	}

	cmdResolver := NewCmdResolver(&mocks.ContainerShellDetector{})
	resolvedCmd := cmdResolver.ResolveCmd(exec, containerInfo)

	assert.Equal(t, []string{"sh", "-c", "cd /projects/testprj; sh -l"}, resolvedCmd)
}

func TestShouldAutoDetectShellForNonEmptyShellCommand(t *testing.T) {
	shellDetectorMock := &mocks.ContainerShellDetector{}
	shellDetectorMock.On("DetectShell", containerInfo).Return("zsh", nil)
	exec := model.MachineExec{
		Type: "shell",
		Cmd:  []string{"", "-c", "top"},
		Cwd:  "/projects/testprj",
	}

	cmdResolver := NewCmdResolver(shellDetectorMock)
	resolvedCmd := cmdResolver.ResolveCmd(exec, containerInfo)

	assert.Equal(t, []string{"zsh", "-c", "cd /projects/testprj; top"}, resolvedCmd)
}

func TestShouldAutoDetectShellForNonEmptyShellCommandWithoutMinusCArgument(t *testing.T) {
	shellDetectorMock := &mocks.ContainerShellDetector{}
	shellDetectorMock.On("DetectShell", containerInfo).Return("zsh", nil)
	exec := model.MachineExec{
		Type: "shell",
		Cmd:  []string{"", "top"},
		Cwd:  "/projects/testprj",
	}

	cmdResolver := NewCmdResolver(shellDetectorMock)
	resolvedCmd := cmdResolver.ResolveCmd(exec, containerInfo)

	assert.Equal(t, []string{"zsh", "-c", "cd /projects/testprj;  top"}, resolvedCmd)
}

func TestShouldAutoDetectShellForEmptyShellCommand(t *testing.T) {
	shellDetectorMock := &mocks.ContainerShellDetector{}
	shellDetectorMock.On("DetectShell", containerInfo).Return("zsh", nil)
	exec := model.MachineExec{
		Type: "shell",
		Cmd:  []string{},
		Cwd:  "/projects/testprj",
	}

	cmdResolver := NewCmdResolver(shellDetectorMock)
	resolvedCmd := cmdResolver.ResolveCmd(exec, containerInfo)

	assert.Equal(t, []string{"zsh", "-c", "cd /projects/testprj; "}, resolvedCmd)
}

func TestShouldLaunchCommandWithoutAnyType(t *testing.T) {
	shellDetectorMock := &mocks.ContainerShellDetector{}
	shellDetectorMock.On("DetectShell", containerInfo).Return("zsh", nil)
	exec := model.MachineExec{
		Type: "",
		Cmd:  []string{"yarn", "run", "build"},
		Cwd:  "/projects/testprj",
	}

	cmdResolver := NewCmdResolver(shellDetectorMock)
	resolvedCmd := cmdResolver.ResolveCmd(exec, containerInfo)

	assert.Equal(t, []string{"zsh", "-c", "cd /projects/testprj; yarn run build"}, resolvedCmd)
}

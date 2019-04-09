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

package exec_info

import (
	"bytes"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	Pods = "pods"
	Exec = "exec"
	Post = "POST"

	ContainerName = "containerName"
	PodName       = "podName"
)

// Exec to get some information from container.
// Command for such exec should be
// "endless" and simple(For example: "whoami", "arch", "env").
// It should not be shell based command.
// This exec is always not "tty" and doesn't provide sending input to the command.
type KubernetesInfoExec struct {
	// command with arguments
	command []string

	// Information to find container.
	namespace     string
	containerName string
	podName       string

	// stdOut/stdErr buffers
	stdOut *bytes.Buffer
	stdErr *bytes.Buffer

	// Api to spawn exec.
	core   v1.CoreV1Interface
	config *rest.Config
}

// Create new kubernetes exec.
func NewKubernetesInfoExec(
	command []string,
	containerName string,
	podName string,
	namespace string,
	core v1.CoreV1Interface,
	config *rest.Config) *KubernetesInfoExec {
	var stdOut, stdErr bytes.Buffer
	return &KubernetesInfoExec{
		command:       command,
		containerName: containerName,
		podName:       podName,
		namespace:     namespace,
		stdOut:        &stdOut,
		stdErr:        &stdErr,
		core:          core,
		config:        config,
	}
}

// Start new kubernetes info exec.
func (exec *KubernetesInfoExec) Start() (err error) {
	req := exec.core.RESTClient().
		Post().
		Namespace(exec.namespace).
		Resource(Pods).
		Name(exec.podName).
		SubResource(Exec).
		// set up params
		VersionedParams(&corev1.PodExecOptions{
			Container: exec.containerName,
			Command:   exec.command,
			Stdout:    true,
			Stderr:    true,
			// no input reader, spawns exec only to get some info from container
			Stdin: false,
			// no tty, exec should launch simple no terminal command
			TTY: false,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(exec.config, Post, req.URL())
	if err != nil {
		return err
	}

	err = executor.Stream(remotecommand.StreamOptions{
		Stdout: exec.stdOut,
		Stderr: exec.stdErr,
		Tty:    false,
	})
	if err != nil {
		return err
	}

	if len(exec.stdErr.Bytes()) != 0 {
		return errors.New(string(exec.stdErr.Bytes()))
	}

	return nil
}

// Get exec output content.
func (exec *KubernetesInfoExec) GetOutput() string {
	return string(exec.stdOut.Bytes())
}

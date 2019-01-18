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
	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/eclipse/che-machine-exec/exec-info"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

const (
	WsIdLabel         = "che.workspace_id"
	MachineNameEnvVar = "CHE_MACHINE_NAME"
)

// Kubernetes specific implementation of the container information filter.
// Eclipse CHE workspace pod could be located in the same
// namespace like Eclipse CHE master or in the separated namespace.
// Kubernetes container filter search workspace pod by workspaceId and
// then search container information inside pod by machine name.
type KubernetesContainerFilter struct {
	ContainerFilter

	podGetterApi corev1.PodsGetter
	namespace    string
}

// Create new kubernetes container filter.
func NewKubernetesContainerFilter(namespace string, podGetterApi corev1.PodsGetter) *KubernetesContainerFilter {
	return &KubernetesContainerFilter{
		namespace:    namespace,
		podGetterApi: podGetterApi,
	}
}

// Find container information by pod label: "wsId" and container environment variables "machineName".
func (filter *KubernetesContainerFilter) FindContainerInfo(identifier *model.MachineIdentifier) (containerInfo map[string]string, err error) {
	filterOptions := metav1.ListOptions{LabelSelector: WsIdLabel + "=" + identifier.WsId}

	wsPods, err := filter.podGetterApi.Pods(filter.namespace).List(filterOptions)
	if err != nil {
		return nil, err
	}

	if len(wsPods.Items) == 0 {
		return nil, errors.New("pod was not found for workspace: " + identifier.WsId)
	}

	for _, pod := range wsPods.Items {
		containerName := findContainerName(pod, identifier.MachineName)
		if containerName != "" {
			containerInfo := make(map[string]string)
			containerInfo[exec_info.ContainerName] = containerName
			containerInfo[exec_info.PodName] = pod.Name

			return containerInfo, nil
		}
	}

	return nil, errors.New("container with name " + identifier.MachineName + " was not found. For workspace: " + identifier.WsId)
}

func findContainerName(pod v1.Pod, machineName string) string {
	containers := pod.Spec.Containers

	for _, container := range containers {
		for _, env := range container.Env {
			if env.Name == MachineNameEnvVar && env.Value == machineName {
				return container.Name
			}
		}
	}
	return ""
}

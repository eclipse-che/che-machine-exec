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
	"errors"
	"github.com/eclipse/che-machine-exec/api/model"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"os"
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

func (filter *KubernetesContainerFilter) GetContainerList() (containersInfo []*model.ContainerInfo, err error) {
	pods, err := filter.getWorkspacePods()
	if err != nil {
		return containersInfo, err
	}

	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			for _, env := range container.Env {
				if env.Name == MachineNameEnvVar {
					containersInfo = append(containersInfo, &model.ContainerInfo{ContainerName: env.Value, PodName: pod.Name})
				}
			}
		}
	}

	return containersInfo, nil
}

// Find container information by pod label: "wsId" and container environment variables "machineName".
func (filter *KubernetesContainerFilter) FindContainerInfo(identifier *model.MachineIdentifier) (containerInfo *model.ContainerInfo, err error) {
	wsPods, err := filter.getWorkspacePods()
	if err != nil {
		return nil, err
	}

	for _, pod := range wsPods.Items {
		containerName := findContainerName(pod, identifier.MachineName)
		if containerName != "" {
			return &model.ContainerInfo{ContainerName: containerName, PodName: pod.Name}, nil
		}
	}

	return nil, errors.New("container with name " + identifier.MachineName + " was not found.")
}

func (filter *KubernetesContainerFilter) getWorkspacePods() (*v1.PodList, error) {
	workspaceID := os.Getenv("CHE_WORKSPACE_ID")
	if workspaceID == "" {
		return nil, errors.New("unable to get current workspace id")
	}

	filterOptions := metav1.ListOptions{LabelSelector: WsIdLabel + "=" + workspaceID}
	wsPods, err := filter.podGetterApi.Pods(filter.namespace).List(filterOptions)
	if err != nil {
		return nil, err
	}

	if len(wsPods.Items) == 0 {
		return nil, errors.New("pods was not found for workspace: " + workspaceID)
	}

	return wsPods, nil
}

func findContainerName(pod v1.Pod, machineName string) string {
	for _, container := range pod.Spec.Containers {
		for _, env := range container.Env {
			if env.Name == MachineNameEnvVar && env.Value == machineName {
				return container.Name
			}
		}
	}
	return ""
}

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

package exec

import (
	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/eclipse/che-machine-exec/client"
	"github.com/eclipse/che-machine-exec/exec-info"
	"github.com/eclipse/che-machine-exec/filter"
	"github.com/eclipse/che-machine-exec/shell"
	"github.com/gorilla/websocket"
	"log"
	"os"
)

var execManager ExecManager

// ExecManager to manage exec life cycle.
type ExecManager interface {
	// Create new Exec defined by machine exec model object.
	Create(machineExec *model.MachineExec) (int, error)

	// Remove information about exec by ExecId.
	// It's can be useful in case exec error or exec exit.
	Remove(execId int)

	// Check if exec with current id is exists
	Check(id int) (int, error)

	// Attach simple websocket connection to the exec stdIn/stdOut by unique exec id.
	Attach(id int, conn *websocket.Conn) error

	// Resize exec by unique id.
	Resize(id int, cols uint, rows uint) error
}

// CreateExecManager creates and returns new instance ExecManager.
// Fail with panic if it is impossible.
func CreateExecManager() (exeManager ExecManager) {
	if isValidKubernetesInfra() {
		infoParser := shell.NewExecInfoParser()
		nameSpace := GetNameSpace()
		clientProvider := client.NewKubernetesClientProvider()
		k8sClient := clientProvider.GetKubernetesClient()
		config := clientProvider.GetKubernetesConfig()

		kubernetesInfoExecCreator := exec_info.NewKubernetesInfoExecCreator(nameSpace, k8sClient.CoreV1(), config)
		shellDetector := shell.NewShellDetector(kubernetesInfoExecCreator, infoParser)
		cmdResolver := NewCmdResolver(shellDetector, kubernetesInfoExecCreator)
		containerFilter := filter.NewKubernetesContainerFilter(nameSpace, k8sClient.CoreV1())

		return Newk8sExecManager(nameSpace, k8sClient.CoreV1(), config, containerFilter, *cmdResolver)
	}

	log.Panic("Error: Unable to create manager. Unable to get service account info.")

	return nil
}

// GetExecManager returns instance exec manager
func GetExecManager() ExecManager {
	if execManager == nil {
		execManager = CreateExecManager()
	}
	return execManager
}

func isValidKubernetesInfra() bool {
	stat, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount")
	if err == nil && stat.IsDir() {
		return true
	}

	return false
}

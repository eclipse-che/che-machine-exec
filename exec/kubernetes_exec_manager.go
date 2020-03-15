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
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/eclipse/che-machine-exec/client"
	exec_info "github.com/eclipse/che-machine-exec/exec-info"
	"github.com/eclipse/che-machine-exec/filter"
	line_buffer "github.com/eclipse/che-machine-exec/output/line-buffer"
	"github.com/eclipse/che-machine-exec/output/utf8stream"
	ws "github.com/eclipse/che-machine-exec/ws-conn"
	"github.com/gorilla/websocket"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

type machineExecs struct {
	mutex   *sync.Mutex
	execMap map[int]*model.MachineExec
}

// KubernetesExecManager manipulates kubernetes container execs.
type KubernetesExecManager struct {
	k8sAPIProvider client.K8sAPIProvider

	nameSpace string
}

var (
	execs = machineExecs{
		mutex:   &sync.Mutex{},
		execMap: make(map[int]*model.MachineExec),
	}
	prevExecID uint64 = 0
)

// Newk8sExecManager create new instance of the kubernetes exec manager.
func Newk8sExecManager(
	namespace string,
	clientProvider client.K8sAPIProvider,
) *KubernetesExecManager {
	return &KubernetesExecManager{
		nameSpace:      namespace,
		k8sAPIProvider: clientProvider,
	}
}

// Create new exec request object
func (manager *KubernetesExecManager) Create(machineExec *model.MachineExec) (int, error) {
	k8sAPI, err := manager.getK8sAPI(machineExec.UserToken)
	if err != nil {
		return -1, err
	}

	containerFilter := filter.NewKubernetesContainerFilter(manager.nameSpace, k8sAPI.GetClient().CoreV1())

	if machineExec.Identifier.MachineName != "" {
		containerInfo, err := containerFilter.FindContainerInfo(&machineExec.Identifier)
		if err != nil {
			return -1, err
		}
		if err = manager.doCreate(machineExec, containerInfo, k8sAPI); err != nil {
			return -1, err
		}
		logrus.Printf("%s is successfully initialized in user specified container %s/%s", machineExec.Cmd,
			containerInfo.PodName, containerInfo.ContainerName)
		return machineExec.ID, nil
	}
	// connect to the first available container. Workaround for Cloud Shell https://github.com/eclipse/che/issues/15434
	containersInfo, err := containerFilter.GetContainerList()
	if err != nil {
		return -1, err
	}
	for _, containerInfo := range containersInfo {
		err = manager.doCreate(machineExec, containerInfo, k8sAPI)
		if err != nil {
			//attempt to initialize terminal in this container failed
			//proceed to next one
			continue
		}
		logrus.Printf("%s is successfully initialized in auto discovered container %s/%s", machineExec.Cmd,
			containerInfo.PodName, containerInfo.ContainerName)
		return machineExec.ID, nil
	}

	var containers []string
	for _, c := range containersInfo {
		containers = append(containers, c.PodName+"\\"+c.ContainerName)
	}
	return -1, fmt.Errorf("failed to initialize terminal in any of {%s}", strings.Join(containers, ", "))
}

func (manager *KubernetesExecManager) doCreate(machineExec *model.MachineExec, containerInfo *model.ContainerInfo, k8sAPI *client.K8sAPI) error {
	cmdResolver := NewCmdResolver(k8sAPI, manager.nameSpace)
	resolvedCmd, err := cmdResolver.ResolveCmd(*machineExec, containerInfo)
	if err != nil {
		return err
	}

	req := k8sAPI.GetClient().CoreV1().RESTClient().
		Post().
		Namespace(manager.nameSpace).
		Resource(exec_info.Pods).
		Name(containerInfo.PodName).
		SubResource(exec_info.Exec).
		// set up params
		VersionedParams(&v1.PodExecOptions{
			Container: containerInfo.ContainerName,
			Command:   resolvedCmd,
			Stdout:    true,
			Stderr:    true,
			Stdin:     true,
			TTY:       machineExec.Tty,
		}, scheme.ParameterCodec)

	logrus.Debugf("Do create %+v ", k8sAPI.GetConfig())
	executor, err := remotecommand.NewSPDYExecutor(k8sAPI.GetConfig(), exec_info.Post, req.URL())
	if err != nil {
		return err
	}
	machineExec.Cmd = resolvedCmd

	defer execs.mutex.Unlock()
	execs.mutex.Lock()

	machineExec.Executor = executor
	machineExec.ID = int(atomic.AddUint64(&prevExecID, 1))
	machineExec.MsgChan = make(chan []byte)
	machineExec.SizeChan = make(chan remotecommand.TerminalSize)
	machineExec.ExitChan = make(chan bool)
	machineExec.ErrorChan = make(chan error)
	machineExec.ConnectionHandler = ws.NewConnHandler()

	execs.execMap[machineExec.ID] = machineExec

	return nil
}

// Remove information about exec
func (*KubernetesExecManager) Remove(execID int) {
	defer execs.mutex.Unlock()

	execs.mutex.Lock()
	delete(execs.execMap, execID)
}

// Check if exec with id exists
func (*KubernetesExecManager) Check(id int) (int, error) {
	machineExec := getByID(id)
	if machineExec == nil {
		return -1, errors.New("Exec '" + strconv.Itoa(id) + "' was not found")
	}
	return machineExec.ID, nil
}

// Attach websoket connnection to the exec by id.
func (*KubernetesExecManager) Attach(id int, conn *websocket.Conn) error {
	machineExec := getByID(id)
	if machineExec == nil {
		return errors.New("Exec '" + strconv.Itoa(id) + "' to attach was not found")
	}
	logrus.Debugf("Attach to exec %s", id)

	machineExec.ReadConnection(conn, machineExec.MsgChan)

	if machineExec.Buffer != nil {
		// restore previous output.
		restoreContent := machineExec.Buffer.GetContent()
		return conn.WriteMessage(websocket.TextMessage, []byte(restoreContent))
	}

	go saveActivity(machineExec)

	ptyHandler := PtyHandlerImpl{machineExec: machineExec, filter: &utf8stream.Utf8StreamFilter{}}
	machineExec.Buffer = line_buffer.New()

	err := machineExec.Executor.Stream(remotecommand.StreamOptions{
		Stdin:             ptyHandler,
		Stdout:            ptyHandler,
		Stderr:            ptyHandler,
		TerminalSizeQueue: ptyHandler,
		Tty:               machineExec.Tty,
	})

	if err != nil {
		machineExec.ErrorChan <- err
	} else {
		machineExec.ExitChan <- true
	}

	return err
}

// Resize exec output frame.
func (*KubernetesExecManager) Resize(id int, cols uint, rows uint) error {
	machineExec := getByID(id)
	if machineExec == nil {
		return errors.New("Exec to resize '" + strconv.Itoa(id) + "' was not found")
	}

	machineExec.SizeChan <- remotecommand.TerminalSize{Width: uint16(cols), Height: uint16(rows)}
	return nil
}

// getK8sAPI returns k8s api.
func (manager *KubernetesExecManager) getK8sAPI(userToken string) (k8s8API *client.K8sAPI, err error) {
	if client.UseUserToken {
		logrus.Info("Create k8s api object with user token")
		k8s8API, err = manager.k8sAPIProvider.Getk8sAPIWithUserToken(userToken)
	} else {
		logrus.Info("Create k8s api object without user token")
		k8s8API, err = manager.k8sAPIProvider.Getk8sAPI()
		logrus.Debugf("Config %+v and client %+v", k8s8API.GetConfig(), k8s8API.GetClient())
		logrus.Debugf("Token %s", k8s8API.GetConfig().BearerToken)
	}
	return
}

// getByID return exec by id.
func getByID(id int) *model.MachineExec {
	defer execs.mutex.Unlock()

	execs.mutex.Lock()
	return execs.execMap[id]
}

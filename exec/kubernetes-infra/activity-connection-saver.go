package kubernetes_infra

import (
	"github.com/eclipse/che-machine-exec/api/model"
	"time"
)

const (
	ActivityTimeOut = 30
)

// To prevent close exec connection
// (https://blog.openshift.com/executing-commands-in-pods-using-k8s-api/ - Connection lifecycle)
// let's send empty byte array each 30 sec.
func saveActivity(machineExec *model.MachineExec) {
	ticker := time.NewTicker(ActivityTimeOut * time.Second)
	for range ticker.C {
		machineExec.MsgChan <- make([]byte, 0)
	}
}

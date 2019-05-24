package main

import (
	"fmt"
	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/eclipse/che-machine-exec/exec"
	infra "github.com/eclipse/che-machine-exec/exec/kubernetes-infra"
	line_buffer "github.com/eclipse/che-machine-exec/output/line-buffer"
	"github.com/eclipse/che-machine-exec/output/utf8stream"
	ws "github.com/eclipse/che-machine-exec/ws-conn"
	"k8s.io/client-go/tools/remotecommand"
)

func main() {
	execManager := exec.GetExecManager()
	identifier := model.MachineIdentifier{
		MachineName: "dev",
		WsId:        "workspace98qa21fh2shz4b6t",
	}
	machineExec := &model.MachineExec{
		// Cmd:        []string{"sleep 2 && echo 'ABC' && ls -a -li && pwd"},

		// Single quotes
		Cmd:     []string{"sh", "-c", "echo Start && { kill 6105 && echo '>>Done'; } || echo '>>Fail'"},
		// Cmd: []string{"sh", "-c", "echo A && { kill $(echo -e '2639 \n 100000' | tr '\n' ' ') && echo \"Webpack dev server's processes are killed\"; } || echo \"Webpack dev server is not running\""},
		IsShell: true,

		// Cmd:        []string{"sh", "-c", "sleep 5 && echo 'ABC' && ls -a -li && pwd"},
		// IsShell: true,

		Identifier: identifier,
		Cwd:        "/projects",
	}
	machineExec.ConnectionHandler = ws.NewConnHandler()
	machineExec.Buffer = line_buffer.New()

	execManager.Create(machineExec)
	ptyHandler := infra.CreatePtyHandlerImpl(machineExec, &utf8stream.Utf8StreamFilter{})

	fmt.Println("Try to start")
	err := machineExec.Executor.Stream(remotecommand.StreamOptions{
		Stdin:             ptyHandler,
		Stdout:            ptyHandler,
		Stderr:            ptyHandler,
		TerminalSizeQueue: ptyHandler,
		Tty:               true,
	})
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("Work completed")
}

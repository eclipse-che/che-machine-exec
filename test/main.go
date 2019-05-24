package main

import (
	"fmt"
	"github.com/eclipse/che-machine-exec/api/model"
	"github.com/eclipse/che-machine-exec/exec"
	infra "github.com/eclipse/che-machine-exec/exec/kubernetes-infra"
	line_buffer "github.com/eclipse/che-machine-exec/output/line-buffer"
	"github.com/eclipse/che-machine-exec/output/utf8stream"
	"k8s.io/client-go/tools/remotecommand"
)

func main() {
	execManager := exec.GetExecManager()
	identifier := model.MachineIdentifier{
		MachineName: "dev",
		WsId:        "workspace98qa21fh2shz4b6t",
	}
	machineExec := &model.MachineExec{
		// Cmd:        []string{"sleep 5 && echo 'ABC' && ls -a -li && pwd"},

		// Single quotes
		Cmd:        []string{"sh", "-c", "kill $(echo -e \"hello\nworld\" | tr '\n' ' ')"},
		IsShell: true,

		// Cmd:        []string{"sh", "-c", "sleep 5 && echo 'ABC' && ls -a -li && pwd"},
		// IsShell: true,

		Identifier: identifier,
		Cwd:        "/projects",
	}
	machineExec.Buffer = line_buffer.New()

	execManager.Create(machineExec)
	ptyHandler := infra.CreatePtyHandlerImpl(machineExec, &utf8stream.Utf8StreamFilter{})

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

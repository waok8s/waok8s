package main

import (
	"os"

	"k8s.io/component-base/logs"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"

	"github.com/waok8s/wao-scheduler-v2/plugins/podspread"
)

func main() {
	command := app.NewSchedulerCommand(
		app.WithPlugin(podspread.Name, podspread.New),
	)

	logs.InitLogs()
	defer logs.FlushLogs()

	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}

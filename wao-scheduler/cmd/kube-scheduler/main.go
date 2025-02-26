package main

import (
	"os"

	"k8s.io/component-base/cli"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"

	"github.com/waok8s/wao-scheduler/pkg/plugins/minimizepower"
	"github.com/waok8s/wao-scheduler/pkg/plugins/podspread"

	_ "github.com/waok8s/wao-scheduler/pkg/scheme" // ensure scheme package is initialized
)

func main() {
	command := app.NewSchedulerCommand(
		app.WithPlugin(minimizepower.Name, minimizepower.New),
		app.WithPlugin(podspread.Name, podspread.New),
	)

	os.Exit(cli.Run(command))
}

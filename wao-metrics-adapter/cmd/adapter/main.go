package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/component-base/logs"
	"k8s.io/klog/v2"
	basecmd "sigs.k8s.io/custom-metrics-apiserver/pkg/cmd"

	waocollector "github.com/Nedopro2022/wao-metrics-adapter/pkg/collector"
	waometric "github.com/Nedopro2022/wao-metrics-adapter/pkg/metric"
	waoprovider "github.com/Nedopro2022/wao-metrics-adapter/pkg/provider"
)

type Adapter struct {
	basecmd.AdapterBase

	// the message printed on startup
	Message string
}

func main() {
	logs.InitLogs()
	defer logs.FlushLogs()

	// adapter
	cmd := &Adapter{
		Message: "WAO Metrics Adapter",
	}

	// init flags
	cmd.Flags().StringVar(&cmd.Message, "msg", "starting adapter...", "startup message")
	logs.AddGoFlags(flag.CommandLine)          // register klog flags
	cmd.Flags().AddGoFlagSet(flag.CommandLine) // register adapter flags
	cmd.Flags().Parse(os.Args)

	// init metricstore
	metricStore := waometric.NewStore()

	// init collector
	cfg, err := cmd.ClientConfig()
	if err != nil {
		klog.Fatalf("unable to construct rest.Config: %v", err)
	}
	collector, err := waocollector.New(cfg, metricStore)
	if err != nil {
		klog.Fatalf("unable to construct collector: %v", err)
	}
	go collector.Run(wait.NeverStop)

	// init provider
	client, err := cmd.DynamicClient()
	if err != nil {
		klog.Fatalf("unable to construct dynamic client: %v", err)
	}
	mapper, err := cmd.RESTMapper()
	if err != nil {
		klog.Fatalf("unable to construct discovery REST mapper: %v", err)
	}
	provider := waoprovider.New(client, mapper, metricStore)
	cmd.WithCustomMetrics(provider)
	// cmd.WithExternalMetrics(provider) // waoprovider.Provider don't support external metrics

	klog.Infof(cmd.Message)
	if err := cmd.Run(wait.NeverStop); err != nil {
		klog.Fatalf("unable to run custom metrics adapter: %v", err)
	}
}

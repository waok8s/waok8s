package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/component-base/logs"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	basecmd "sigs.k8s.io/custom-metrics-apiserver/pkg/cmd"

	waocontroller "github.com/waok8s/waok8s/wao-metrics-adapter/pkg/controller"
	waometrics "github.com/waok8s/waok8s/wao-metrics-adapter/pkg/metrics"
	waoprovider "github.com/waok8s/waok8s/wao-metrics-adapter/pkg/provider"
)

type Adapter struct {
	basecmd.AdapterBase

	// the message printed on startup
	Message string
}

func main() {
	logs.InitLogs()
	defer logs.FlushLogs()

	// init components
	metricsCollector := &waometrics.Collector{}
	metricsStore := &waometrics.Store{}

	// init adapter
	cmd := &Adapter{
		Message: "WAO Metrics Adapter",
	}
	// init flags
	cmd.Flags().StringVar(&cmd.Message, "msg", "starting adapter...", "startup message")
	logs.AddGoFlags(flag.CommandLine)          // register klog flags
	cmd.Flags().AddGoFlagSet(flag.CommandLine) // register adapter flags
	cmd.Flags().Parse(os.Args)

	// init provider
	client, err := cmd.DynamicClient()
	if err != nil {
		klog.Fatalf("unable to construct dynamic client: %v", err)
	}
	mapper, err := cmd.RESTMapper()
	if err != nil {
		klog.Fatalf("unable to construct discovery REST mapper: %v", err)
	}
	provider := waoprovider.New(client, mapper, metricsStore)
	cmd.WithCustomMetrics(provider)
	// cmd.WithExternalMetrics(provider) // waoprovider.Provider don't support external metrics

	klog.Infof(cmd.Message)
	go func() {
		if err := cmd.Run(wait.NeverStop); err != nil {
			klog.Fatalf("unable to run custom metrics adapter: %v", err)
		}
	}()

	// init controller
	// TODO: merge flags, merge logs, use LeaderElection
	setupLog := ctrl.Log.WithName("setup")
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{Development: true})))
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: waocontroller.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable metrics server to avoid port conflict
		},
		HealthProbeBindAddress: "",
		LeaderElection:         false,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	if err := (&waocontroller.NodeConfigReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		SecretClient:     kubernetes.NewForConfigOrDie(mgr.GetConfig()),
		MetricsCollector: metricsCollector,
		MetricsStore:     metricsStore,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Operator")
		os.Exit(1)
	}
	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

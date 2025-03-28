package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/waok8s/waok8s/wao-metrics-adapter/pkg/metrics/redfish"
	"github.com/waok8s/waok8s/wao-metrics-adapter/pkg/util"
)

func main() {
	var address string
	flag.StringVar(&address, "address", "http://localhost:5000", "Redfish server address")
	var serverType string
	var serverTypeUsage strings.Builder
	serverTypeUsage.WriteString("Options:")
	for k := range redfish.GetInletTempFns {
		serverTypeUsage.WriteString(" " + string(k))
	}
	flag.StringVar(&serverType, "serverType", "", serverTypeUsage.String())
	var basicAuth string
	flag.StringVar(&basicAuth, "basicAuth", "", "Basic auth in username@password format")
	var timeout time.Duration
	flag.DurationVar(&timeout, "timeout", 5*time.Second, "Timeout for the request")
	var logLevel int
	flag.IntVar(&logLevel, "v", 3, "klog-style log level")
	flag.Parse()

	var slogLevel slog.Level
	switch {
	case logLevel < 0:
		slogLevel = 100 // silent
	case logLevel == 0:
		slogLevel = slog.LevelError
	case logLevel == 1:
		slogLevel = slog.LevelWarn
	case logLevel == 2:
		slogLevel = slog.LevelInfo
	case logLevel == 3:
		slogLevel = slog.LevelDebug
	case logLevel > 3:
		slogLevel = -100 // verbose
	}

	lg := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
		Level:     slogLevel,
	}))
	slog.SetDefault(lg.With("component", "InletTempClient (Redfish)"))

	requestEditorFns := []util.RequestEditorFn{}
	ss := strings.Split(basicAuth, ":")
	if len(ss) == 2 {
		requestEditorFns = append(requestEditorFns, util.WithBasicAuth(ss[0], ss[1]))
	}
	requestEditorFns = append(requestEditorFns, util.WithCurlLogger(lg.With("func", "WithCurlLogger(RedfishClient.Fetch)")))

	c := redfish.NewInletTempAgent(address, redfish.ServerType(serverType), true, timeout, requestEditorFns...)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	v, err := c.Fetch(ctx)
	cancel()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(v)
}

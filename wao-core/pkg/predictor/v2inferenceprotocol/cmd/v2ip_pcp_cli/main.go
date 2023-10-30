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

	"github.com/waok8s/wao-core/pkg/predictor/v2inferenceprotocol"
	"github.com/waok8s/wao-core/pkg/util"
)

func main() {
	var address string
	flag.StringVar(&address, "address", "http://localhost:5000", "Redfish server address")
	var model string
	flag.StringVar(&model, "model", "modelName", "Model name")
	var modelVersion string
	flag.StringVar(&modelVersion, "modelVersion", "v1.0.0", "Model version")
	var cpuUsage float64
	flag.Float64Var(&cpuUsage, "cpuUsage", 0.0, "CPU usage")
	var inletTemp float64
	flag.Float64Var(&inletTemp, "inletTemp", 0.0, "Inlet temperature")
	var deltaP float64
	flag.Float64Var(&deltaP, "deltaP", 0.0, "Delta P")
	var basicAuth string
	flag.StringVar(&basicAuth, "basicAuth", "", "Basic auth in username@password format")
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
	slog.SetDefault(lg.With("component", "PowerConsumptionPredictor (V2InferenceProtocol)"))

	requestEditorFns := []util.RequestEditorFn{}
	ss := strings.Split(basicAuth, ":")
	if len(ss) == 2 {
		requestEditorFns = append(requestEditorFns, util.WithBasicAuth(ss[0], ss[1]))
	}
	requestEditorFns = append(requestEditorFns, util.WithCurlLogger(lg.With("func", "WithCurlLogger(v2inferenceprotocol.PowerConsumptionPredictor.Predict)")))

	c := v2inferenceprotocol.NewPowerConsumptionPredictor(address, model, modelVersion, true, 2*time.Second, requestEditorFns...)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	v, err := c.Predict(ctx, cpuUsage, inletTemp, deltaP)
	cancel()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", v)
}

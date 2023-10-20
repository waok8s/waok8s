package predictor

import (
	"context"

	waocollector "github.com/waok8s/wao-metrics-adapter/pkg/metriccollector"
)

type PowerConsumptionPredictor interface {
	Predict(ctx context.Context, cpuUsage, inletTemp, deltaP float64) (watt float64, err error)
}

type RequestEditorFn waocollector.RequestEditorFn

var (
	WithRequestHeader = waocollector.WithRequestHeader
	WithCurlLogger    = waocollector.WithCurlLogger
	WithBasicAuth     = waocollector.WithBasicAuth
)

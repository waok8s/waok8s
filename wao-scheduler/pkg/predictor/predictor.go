package predictor

import (
	"context"
)

type PowerConsumptionPredictor interface {
	Endpoint() (string, error)
	Predict(ctx context.Context, cpuUsage, inletTemp, deltaP float64) (watt float64, err error)
}

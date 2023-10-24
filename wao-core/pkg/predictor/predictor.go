package predictor

import (
	"context"

	waov1beta1 "github.com/waok8s/wao-core/api/wao/v1beta1"
)

type PredictorType string

const (
	TypePowerConsumption PredictorType = "PowerConsumption"
	TypeResponseTime     PredictorType = "ResponseTime"
)

type EndpointProvider interface {
	Endpoint() (string, error)
	Get(ctx context.Context, predictorType PredictorType) (*waov1beta1.EndpointTerm, error)
}

type PowerConsumptionPredictor interface {
	Endpoint() (string, error)
	Predict(ctx context.Context, cpuUsage, inletTemp, deltaP float64) (watt float64, err error)
}

type ResponseTimePredictor interface {
	Endpoint() (string, error)
	// Predict() // TODO: define this method
}

package predictor

import (
	"context"
	"time"

	waov1beta1 "github.com/waok8s/wao-core/api/wao/v1beta1"
)

type FakeEndpointProvider struct {
	EndpointValue string
	EndpointError error

	GetValue *waov1beta1.EndpointTerm
	GetError error
	GetDelay time.Duration
}

var _ EndpointProvider = (*FakeEndpointProvider)(nil)

func (p *FakeEndpointProvider) Endpoint() (string, error) {
	if p.EndpointError != nil {
		return "", p.EndpointError
	}
	return p.EndpointValue, nil
}

func (p *FakeEndpointProvider) Get(ctx context.Context, predictorType PredictorType) (*waov1beta1.EndpointTerm, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(p.GetDelay):
		break
	}

	if p.GetError != nil {
		return nil, p.GetError
	}
	return p.GetValue, nil
}

type FakePowerConsumptionPredictor struct {
	EndpointValue string
	EndpointError error

	PredictValue float64
	PredictError error
	PredictDelay time.Duration
}

var _ PowerConsumptionPredictor = (*FakePowerConsumptionPredictor)(nil)

func (p *FakePowerConsumptionPredictor) Endpoint() (string, error) {
	if p.EndpointError != nil {
		return "", p.EndpointError
	}
	return p.EndpointValue, nil
}

func (p *FakePowerConsumptionPredictor) Predict(ctx context.Context, cpuUsage, inletTemp, deltaP float64) (watt float64, err error) {
	select {
	case <-ctx.Done():
		return 0.0, ctx.Err()
	case <-time.After(p.PredictDelay):
		break
	}

	if p.PredictError != nil {
		return 0.0, p.PredictError
	}
	return p.PredictValue, nil
}

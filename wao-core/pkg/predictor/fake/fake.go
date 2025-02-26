package fake

import (
	"context"
	"time"

	waov1beta1 "github.com/waok8s/wao-core/api/wao/v1beta1"
	"github.com/waok8s/wao-core/pkg/predictor"
)

type FakeEndpointProvider struct {
	EndpointValue string
	EndpointError error

	GetValue *waov1beta1.EndpointTerm
	GetError error
	GetDelay time.Duration
}

var _ predictor.EndpointProvider = (*FakeEndpointProvider)(nil)

func NewEndpointProvider(endpointValue string, endpointError error, getValue *waov1beta1.EndpointTerm, getError error, getDelay time.Duration) *FakeEndpointProvider {
	return &FakeEndpointProvider{
		EndpointValue: endpointValue,
		EndpointError: endpointError,
		GetValue:      getValue,
		GetError:      getError,
		GetDelay:      getDelay,
	}
}

func (p *FakeEndpointProvider) Endpoint() (string, error) {
	if p.EndpointError != nil {
		return "", p.EndpointError
	}
	return p.EndpointValue, nil
}

func (p *FakeEndpointProvider) Get(ctx context.Context, predictorType predictor.PredictorType) (*waov1beta1.EndpointTerm, error) {
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

var _ predictor.PowerConsumptionPredictor = (*FakePowerConsumptionPredictor)(nil)

func NewPowerConsumptionPredictor(endpointValue string, endpointError error, predictValue float64, predictError error, predictDelay time.Duration) *FakePowerConsumptionPredictor {
	return &FakePowerConsumptionPredictor{
		EndpointValue: endpointValue,
		EndpointError: endpointError,
		PredictValue:  predictValue,
		PredictError:  predictError,
		PredictDelay:  predictDelay,
	}
}

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

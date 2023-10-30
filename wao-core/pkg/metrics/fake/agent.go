package fake

import (
	"context"
	"time"

	"github.com/waok8s/wao-core/pkg/metrics"
)

type FakeAgent struct {
	Type  metrics.ValueType
	Value float64
	Error error
	Delay time.Duration
}

var _ metrics.Agent = (*FakeAgent)(nil)

func NewInletTempAgent(value float64, err error, delay time.Duration) *FakeAgent {
	return &FakeAgent{Type: metrics.ValueInletTemperature, Value: value, Error: err, Delay: delay}
}

func NewDeltaPAgent(value float64, err error, delay time.Duration) *FakeAgent {
	return &FakeAgent{Type: metrics.ValueDeltaPressure, Value: value, Error: err, Delay: delay}
}

func (a *FakeAgent) ValueType() metrics.ValueType { return a.Type }

func (a *FakeAgent) Fetch(ctx context.Context) (float64, error) {
	select {
	case <-ctx.Done():
		return 0.0, ctx.Err()
	case <-time.After(a.Delay):
		break
	}

	if a.Error != nil {
		return 0.0, a.Error
	}
	return a.Value, nil
}

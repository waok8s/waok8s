package metriccollector

import (
	"context"
	"time"

	"github.com/Nedopro2022/wao-metrics-adapter/pkg/metric"
)

type FakeClient struct {
	Type  metric.ValueType
	Value float64
	Error error
	Delay time.Duration
}

var _ MetricCollector = (*FakeClient)(nil)

func (c *FakeClient) Fetch(ctx context.Context) (float64, error) {
	// TODO: log
	<-time.After(c.Delay)
	if c.Error != nil {
		return 0.0, c.Error
	}
	return c.Value, nil
}

func (c *FakeClient) ValueType() metric.ValueType {
	return c.Type
}

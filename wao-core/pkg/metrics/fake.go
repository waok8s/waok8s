package metrics

import (
	"context"
	"time"
)

type FakeClient struct {
	Type  ValueType
	Value float64
	Error error
	Delay time.Duration
}

var _ Agent = (*FakeClient)(nil)

func (c *FakeClient) ValueType() ValueType { return c.Type }

func (c *FakeClient) Fetch(ctx context.Context) (float64, error) {
	select {
	case <-ctx.Done():
		return 0.0, ctx.Err()
	case <-time.After(c.Delay):
		break
	}

	if c.Error != nil {
		return 0.0, c.Error
	}
	return c.Value, nil
}

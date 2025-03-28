package metrics

import (
	"context"
)

type Agent interface {
	ValueType() ValueType
	Fetch(ctx context.Context) (value float64, err error)
}

package predictor

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type CachedPowerConsumptionPredictor struct {
	predictor PowerConsumptionPredictor

	ttl   time.Duration
	cache sync.Map
}

var _ PowerConsumptionPredictor = (*CachedPowerConsumptionPredictor)(nil)

func NewCachedPowerConsumptionPredictor(predictor PowerConsumptionPredictor, expiration time.Duration) *CachedPowerConsumptionPredictor {
	return &CachedPowerConsumptionPredictor{
		predictor: predictor,
		ttl:       expiration,
	}
}

func cacheKey(endpoint string, cpuUsage, inletTemp, deltaP float64) string {
	return fmt.Sprintf("%s#%f#%f#%f", endpoint, cpuUsage, inletTemp, deltaP)
}

type cachedObject struct {
	Watt float64

	ExpiredAt time.Time
}

func (p *CachedPowerConsumptionPredictor) Predict(ctx context.Context, cpuUsage, inletTemp, deltaP float64) (watt float64, err error) {
	endpoint, err := p.predictor.Endpoint()
	if err != nil {
		return 0.0, err
	}

	key := cacheKey(endpoint, cpuUsage, inletTemp, deltaP)

	if v, ok1 := p.cache.Load(key); ok1 {
		if co, ok2 := v.(*cachedObject); ok2 {
			if co.ExpiredAt.After(time.Now()) {
				return co.Watt, nil
			}
		}
	}

	watt, err = p.predictor.Predict(ctx, cpuUsage, inletTemp, deltaP)
	if err != nil {
		return 0.0, err
	}

	co := &cachedObject{
		Watt:      watt,
		ExpiredAt: time.Now().Add(p.ttl),
	}

	p.cache.Store(key, co)

	return watt, nil
}

func (p *CachedPowerConsumptionPredictor) Endpoint() (string, error) {
	return p.predictor.Endpoint()
}

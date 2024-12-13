package client

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"

	waov1beta1 "github.com/waok8s/wao-core/api/wao/v1beta1"
	"github.com/waok8s/wao-core/pkg/predictor"
	"github.com/waok8s/wao-core/pkg/predictor/fromnodeconfig"
)

type CachedPredictorClient struct {
	client kubernetes.Interface

	ttl   time.Duration
	cache sync.Map
}

func NewCachedPredictorClient(client kubernetes.Interface, ttl time.Duration) *CachedPredictorClient {
	return &CachedPredictorClient{
		client: client,
		ttl:    ttl,
	}
}

func predictorCacheKey(valueType string,
	namespace string, endpointTerm *waov1beta1.EndpointTerm, // common
	predictorType predictor.PredictorType, // GetPredictorEndpoint
	cpuUsage, inletTemp, deltaP float64, // PredictPowerConsumption
) string {
	secretName := ""
	if endpointTerm.BasicAuthSecret != nil {
		secretName = endpointTerm.BasicAuthSecret.Name
	}
	ep := fmt.Sprintf("%s|%s|%s", endpointTerm.Type, endpointTerm.Endpoint, secretName)
	return fmt.Sprintf("%s#%s#%s#%s#%f#%f#%f", valueType, namespace, ep, predictorType, cpuUsage, inletTemp, deltaP)
}

const (
	valueTypePowerConsumptionEndpoint = "PowerConsumptionEndpoint"
	valueTypeWatt                     = "Watt"

	// valueTypeResponseTimeEndpoint = "ResponseTimeEndpoint"
	// valueTypeResponseTime         = "ResponseTime"
)

type predictionCache struct {
	PowerConsumptionEndpoint *waov1beta1.EndpointTerm
	Watt                     float64

	// ResponseTimeEndpoint *waov1beta1.EndpointTerm
	// ResponseTime         float64

	ExpiredAt time.Time

	// mu is used to avoid concurrent requests for the same key (which would result in multiple requests to origin servers)
	mu sync.Mutex
}

func (c *CachedPredictorClient) do(ctx context.Context, valueType string,
	namespace string, endpointTerm *waov1beta1.EndpointTerm, // common
	predictorType predictor.PredictorType, // GetPredictorEndpoint
	cpuUsage, inletTemp, deltaP float64, // PredictPowerConsumption
) (*predictionCache, error) {

	key := predictorCacheKey(valueType, namespace, endpointTerm, predictorType, cpuUsage, inletTemp, deltaP)
	lg := slog.With("func", "CachedPredictorClient.do", "key", key)

	if v, ok1 := c.cache.Load(key); ok1 {
		if cv, ok2 := v.(*predictionCache); ok2 {

			// Wait until the cache is ready
			cv.mu.Lock()
			lg.Debug("predictor cache is available")
			cv.mu.Unlock() // NOTE: any better way to do this?

			// Check if the cache is expired
			if cv.ExpiredAt.After(time.Now()) {
				lg.Debug("predictor cache hit")
				return cv, nil
			}
		}
	}
	lg.Debug("predictor cache missed")

	// Push an empty cache and lock it to avoid concurrent requests
	cv := &predictionCache{
		ExpiredAt: time.Now().Add(c.ttl),
	}
	cv.mu.Lock()
	c.cache.Store(key, cv)

	switch valueType {
	case valueTypePowerConsumptionEndpoint:
		prov, err := fromnodeconfig.NewEndpointProvider(c.client, namespace, endpointTerm)
		if err != nil {
			cv.mu.Unlock()
			c.cache.Delete(key)
			return nil, err
		}
		ep, err := prov.Get(ctx, predictorType)
		if err != nil {
			cv.mu.Unlock()
			c.cache.Delete(key)
			return nil, err
		}
		cv.PowerConsumptionEndpoint = ep
	case valueTypeWatt:
		pred, err := fromnodeconfig.NewPowerConsumptionPredictor(c.client, namespace, endpointTerm)
		if err != nil {
			cv.mu.Unlock()
			c.cache.Delete(key)
			return nil, err
		}
		watt, err := pred.Predict(ctx, cpuUsage, inletTemp, deltaP)
		if err != nil {
			cv.mu.Unlock()
			c.cache.Delete(key)
			return nil, err
		}
		cv.Watt = watt
	default:
		cv.mu.Unlock()
		c.cache.Delete(cv)
		return nil, fmt.Errorf("unknown valueType=%s", valueType)
	}

	c.cache.Store(key, cv)
	cv.mu.Unlock()

	return cv, nil
}

func (c *CachedPredictorClient) GetPredictorEndpoint(ctx context.Context, namespace string, ep *waov1beta1.EndpointTerm, predictorType predictor.PredictorType) (*waov1beta1.EndpointTerm, error) {
	cv, err := c.do(ctx, valueTypePowerConsumptionEndpoint, namespace, ep, predictorType, 0.0, 0.0, 0.0)
	if err != nil {
		return nil, err
	}
	return cv.PowerConsumptionEndpoint, nil
}

func (c *CachedPredictorClient) PredictPowerConsumption(ctx context.Context, namespace string, ep *waov1beta1.EndpointTerm, cpuUsage, inletTemp, deltaP float64) (watt float64, err error) {
	cv, err := c.do(ctx, valueTypeWatt, namespace, ep, "", cpuUsage, inletTemp, deltaP)
	if err != nil {
		return 0.0, err
	}
	return cv.Watt, nil
}

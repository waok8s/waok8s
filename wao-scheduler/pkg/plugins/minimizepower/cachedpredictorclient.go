package minimizepower

import (
	"context"
	"fmt"
	"sync"
	"time"

	waov1beta1 "github.com/waok8s/wao-nodeconfig/api/v1beta1"
	"github.com/waok8s/wao-scheduler/pkg/predictor"
	"github.com/waok8s/wao-scheduler/pkg/predictor/fromnodeconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CachedPredictorClient struct {
	client client.Client

	ttl   time.Duration
	cache sync.Map
}

func NewCachedPredictorClient(client client.Client, ttl time.Duration) *CachedPredictorClient {
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
	ep := fmt.Sprintf("%s|%s|%s", endpointTerm.Type, endpointTerm.Endpoint, endpointTerm.BasicAuthSecret.Name)
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
}

func (c *CachedPredictorClient) do(ctx context.Context, valueType string,
	namespace string, endpointTerm *waov1beta1.EndpointTerm, // common
	predictorType predictor.PredictorType, // GetPredictorEndpoint
	cpuUsage, inletTemp, deltaP float64, // PredictPowerConsumption
) (*predictionCache, error) {

	key := predictorCacheKey(valueType, namespace, endpointTerm, predictorType, cpuUsage, inletTemp, deltaP)

	if v, ok1 := c.cache.Load(key); ok1 {
		if cv, ok2 := v.(*predictionCache); ok2 {
			if cv.ExpiredAt.After(time.Now()) {
				return cv, nil
			}
		}
	}

	cv := &predictionCache{
		ExpiredAt: time.Now().Add(c.ttl),
	}

	switch valueType {
	case valueTypePowerConsumptionEndpoint:
		prov, err := fromnodeconfig.NewEndpointProvider(c.client, namespace, endpointTerm)
		if err != nil {
			return nil, err
		}
		ep, err := prov.Get(ctx, predictorType)
		if err != nil {
			return nil, err
		}
		cv.PowerConsumptionEndpoint = ep
	case valueTypeWatt:
		pred, err := fromnodeconfig.NewPowerConsumptionPredictor(c.client, namespace, endpointTerm)
		if err != nil {
			return nil, err
		}
		watt, err := pred.Predict(ctx, cpuUsage, inletTemp, deltaP)
		if err != nil {
			return nil, err
		}
		cv.Watt = watt
	default:
		return nil, fmt.Errorf("unknown valueType=%s", valueType)
	}

	c.cache.Store(key, cv)

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

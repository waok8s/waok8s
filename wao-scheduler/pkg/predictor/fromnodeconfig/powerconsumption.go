package fromnodeconfig

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	waocollector "github.com/waok8s/wao-metrics-adapter/pkg/metriccollector"
	waov1beta1 "github.com/waok8s/wao-nodeconfig/api/v1beta1"
	"github.com/waok8s/wao-scheduler/pkg/predictor"
	"github.com/waok8s/wao-scheduler/pkg/predictor/v2inferenceprotocol"
)

func NewPowerConsumptionPredictor(
	endpointType, endpoint string,
	basicAuthUsername, basicAuthPassword string,
	insecureSkipVerify bool, requestTimeout time.Duration,
	cacheTTL time.Duration, // 0: no cache
) (predictor.PowerConsumptionPredictor, error) {

	switch endpointType {
	case waov1beta1.TypeFake:

	case waov1beta1.TypeV2InferenceProtocol:
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		address := fmt.Sprintf("%s://%s", u.Scheme, u.Host)

		modelName := ""
		modelVersion := ""
		ss := strings.Split(endpoint, "/")
		for i, s := range ss {
			if s == "models" && len(ss) > i+1 {
				modelName = ss[i+1]
			}
			if s == "versions" && len(ss) > i+1 {
				modelVersion = ss[i+1]
			}
		}
		if modelName == "" {
			return nil, fmt.Errorf("model name is not specified")
		}

		requestEditorFns := []waocollector.RequestEditorFn{
			waocollector.WithBasicAuth(basicAuthUsername, basicAuthPassword),
			// waocollector.WithCurlLogger(nil), // TODO
		}

		var client predictor.PowerConsumptionPredictor
		client = v2inferenceprotocol.NewPowerConsumptionClient(address, modelName, modelVersion, insecureSkipVerify, requestTimeout, requestEditorFns...)

		if cacheTTL != 0 {
			client = predictor.NewCachedPowerConsumptionPredictor(client, cacheTTL)
		}

		return client, nil

	case waov1beta1.TypeRedfish:
		
	}

	return nil, fmt.Errorf("unknown endpoint type: %s", endpointType)
}

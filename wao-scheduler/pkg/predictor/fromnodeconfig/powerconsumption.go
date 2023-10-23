package fromnodeconfig

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	waocollector "github.com/waok8s/wao-metrics-adapter/pkg/metriccollector"
	waov1beta1 "github.com/waok8s/wao-nodeconfig/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/waok8s/wao-scheduler/pkg/predictor"
	"github.com/waok8s/wao-scheduler/pkg/predictor/endpointprovider"
	"github.com/waok8s/wao-scheduler/pkg/predictor/v2inferenceprotocol"
)

func getBasicAuthFromSecret(ctx context.Context, client client.Client, namespace string, ref *corev1.LocalObjectReference) (username, password string) {
	if ref == nil || ref.Name == "" {
		return
	}
	secret := &corev1.Secret{}
	if err := client.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: ref.Name}, secret); err != nil {
		// TODO: log
		return "", ""
	}

	username = string(secret.Data["username"])
	password = string(secret.Data["password"])

	return
}

func NewEndpointProvider(client client.Client, namespace string, endpointTerm *waov1beta1.EndpointTerm) (predictor.EndpointProvider, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	username, password := getBasicAuthFromSecret(ctx, client, namespace, endpointTerm.BasicAuthSecret)

	return newEndpointProvider(endpointTerm.Type, endpointTerm.Endpoint, username, password, true, 3*time.Second)
}

func newEndpointProvider(
	endpointType, endpoint string,
	basicAuthUsername, basicAuthPassword string,
	insecureSkipVerify bool, requestTimeout time.Duration,
) (predictor.EndpointProvider, error) {

	switch endpointType {
	case waov1beta1.TypeFake:
		// TODO
	case waov1beta1.TypeRedfish:

		requestEditorFns := []waocollector.RequestEditorFn{
			waocollector.WithBasicAuth(basicAuthUsername, basicAuthPassword),
			// waocollector.WithCurlLogger(nil), // TODO
		}

		prov, err := endpointprovider.NewRedfishEndpointProvider(endpoint, insecureSkipVerify, requestTimeout, requestEditorFns...)
		if err != nil {
			return nil, err
		}
		return prov, nil
	}

	return nil, fmt.Errorf("unknown endpoint type: %s", endpointType)
}

func NewPowerConsumptionPredictor(client client.Client, namespace string, endpointTerm *waov1beta1.EndpointTerm) (predictor.PowerConsumptionPredictor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	username, password := getBasicAuthFromSecret(ctx, client, namespace, endpointTerm.BasicAuthSecret)

	return newPowerConsumptionPredictor(endpointTerm.Type, endpointTerm.Endpoint, username, password, true, 3*time.Second)
}

func newPowerConsumptionPredictor(
	endpointType, endpoint string,
	basicAuthUsername, basicAuthPassword string,
	insecureSkipVerify bool, requestTimeout time.Duration,
) (predictor.PowerConsumptionPredictor, error) {

	switch endpointType {
	case waov1beta1.TypeFake:
		// TODO
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

		return client, nil
	}

	return nil, fmt.Errorf("unknown endpoint type: %s", endpointType)
}

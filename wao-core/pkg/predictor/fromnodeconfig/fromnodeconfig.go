package fromnodeconfig

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	waov1beta1 "github.com/waok8s/wao-core/api/wao/v1beta1"
	"github.com/waok8s/wao-core/pkg/predictor"
	"github.com/waok8s/wao-core/pkg/predictor/endpointprovider"
	"github.com/waok8s/wao-core/pkg/predictor/v2inferenceprotocol"
	"github.com/waok8s/wao-core/pkg/util"
)

func getBasicAuthFromSecret(ctx context.Context, client client.Client, namespace string, ref *corev1.LocalObjectReference) (username, password string) {
	lg := slog.With("func", "getBasicAuthFromSecret")

	if ref == nil || ref.Name == "" {
		return
	}

	secret := &corev1.Secret{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, secret); err != nil {
		lg.Error("unable to get Secret so skip basic auth", "err", err, "obj", types.NamespacedName{Namespace: namespace, Name: ref.Name})
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

		var prov predictor.EndpointProvider
		prov = &predictor.FakeEndpointProvider{
			EndpointValue: endpoint,
			EndpointError: nil,
			GetValue: &waov1beta1.EndpointTerm{
				Type:     waov1beta1.TypeFake,
				Endpoint: "https://fake-endpoint",
			},
			GetError: nil,
			GetDelay: 50 * time.Millisecond,
		}

		return prov, nil
	case waov1beta1.TypeRedfish:

		requestEditorFns := []util.RequestEditorFn{
			util.WithBasicAuth(basicAuthUsername, basicAuthPassword),
			util.WithCurlLogger(slog.With("func", "WithCurlLogger(RedfishEndpointProvider.Get)")),
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

		var client predictor.PowerConsumptionPredictor
		client = &predictor.FakePowerConsumptionPredictor{
			EndpointValue: endpoint,
			EndpointError: nil,
			PredictValue:  3.14,
			PredictError:  nil,
			PredictDelay:  50 * time.Millisecond,
		}

		return client, nil
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

		requestEditorFns := []util.RequestEditorFn{
			util.WithBasicAuth(basicAuthUsername, basicAuthPassword),
			util.WithCurlLogger(slog.With("func", "WithCurlLogger(v2inferenceprotocol.PowerConsumptionClient.Predict)")),
		}

		var client predictor.PowerConsumptionPredictor
		client = v2inferenceprotocol.NewPowerConsumptionClient(address, modelName, modelVersion, insecureSkipVerify, requestTimeout, requestEditorFns...)

		return client, nil
	}

	return nil, fmt.Errorf("unknown endpoint type: %s", endpointType)
}

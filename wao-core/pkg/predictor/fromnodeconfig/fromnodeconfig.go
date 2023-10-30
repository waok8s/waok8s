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
	"github.com/waok8s/wao-core/pkg/predictor/fake"
	"github.com/waok8s/wao-core/pkg/predictor/redfish"
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

	var prov predictor.EndpointProvider

	switch endpointType {
	case waov1beta1.TypeFake:
		prov = fake.NewEndpointProvider(endpoint, nil, &waov1beta1.EndpointTerm{Type: waov1beta1.TypeFake, Endpoint: "https://fake-endpoint"}, nil, 50*time.Millisecond)
	case waov1beta1.TypeRedfish:
		requestEditorFns := []util.RequestEditorFn{
			util.WithBasicAuth(basicAuthUsername, basicAuthPassword),
			util.WithCurlLogger(slog.With("func", "WithCurlLogger(RedfishEndpointProvider.Get)")),
		}
		p, err := redfish.NewEndpointProvider(endpoint, insecureSkipVerify, requestTimeout, requestEditorFns...)
		if err != nil {
			return nil, err
		}
		prov = p
	default:
		return nil, fmt.Errorf("unknown endpoint type: %s", endpointType)
	}

	return prov, nil
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

	var pred predictor.PowerConsumptionPredictor

	switch endpointType {
	case waov1beta1.TypeFake:
		pred = fake.NewPowerConsumptionPredictor(endpoint, nil, 3.14, nil, 50*time.Millisecond)
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

		pred = v2inferenceprotocol.NewPowerConsumptionPredictor(address, modelName, modelVersion, insecureSkipVerify, requestTimeout, requestEditorFns...)
	default:
		return nil, fmt.Errorf("unknown endpoint type: %s", endpointType)
	}

	return pred, nil
}

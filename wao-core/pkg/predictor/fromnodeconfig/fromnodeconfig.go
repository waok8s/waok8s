package fromnodeconfig

import (
	"context"
	"fmt"
	"io"
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

func getBasicAuthFromSecret(ctx context.Context, client client.Client, namespace string, ref *corev1.LocalObjectReference, logWriter io.Writer) (username, password string) {
	if ref == nil || ref.Name == "" {
		return
	}
	secret := &corev1.Secret{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, secret); err != nil {
		if logWriter != nil {
			fmt.Fprintf(logWriter, "unable to get Secret so skip basic auth obj=%s/%s err=%s\n", namespace, ref.Name, err)
		}
		return "", ""
	}

	username = string(secret.Data["username"])
	password = string(secret.Data["password"])

	return
}

func NewEndpointProvider(client client.Client, namespace string, endpointTerm *waov1beta1.EndpointTerm, logWriter io.Writer) (predictor.EndpointProvider, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	username, password := getBasicAuthFromSecret(ctx, client, namespace, endpointTerm.BasicAuthSecret, logWriter)

	return newEndpointProvider(endpointTerm.Type, endpointTerm.Endpoint, username, password, true, 3*time.Second, logWriter)
}

func newEndpointProvider(
	endpointType, endpoint string,
	basicAuthUsername, basicAuthPassword string,
	insecureSkipVerify bool, requestTimeout time.Duration,
	logWriter io.Writer,
) (predictor.EndpointProvider, error) {

	switch endpointType {
	case waov1beta1.TypeFake:
		// TODO: implement
	case waov1beta1.TypeRedfish:

		requestEditorFns := []util.RequestEditorFn{
			util.WithBasicAuth(basicAuthUsername, basicAuthPassword),
			util.WithCurlLogger(logWriter),
		}

		prov, err := endpointprovider.NewRedfishEndpointProvider(endpoint, insecureSkipVerify, requestTimeout, requestEditorFns...)
		if err != nil {
			return nil, err
		}
		return prov, nil
	}

	return nil, fmt.Errorf("unknown endpoint type: %s", endpointType)
}

func NewPowerConsumptionPredictor(client client.Client, namespace string, endpointTerm *waov1beta1.EndpointTerm, logWriter io.Writer) (predictor.PowerConsumptionPredictor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	username, password := getBasicAuthFromSecret(ctx, client, namespace, endpointTerm.BasicAuthSecret, logWriter)

	return newPowerConsumptionPredictor(endpointTerm.Type, endpointTerm.Endpoint, username, password, true, 3*time.Second, logWriter)
}

func newPowerConsumptionPredictor(
	endpointType, endpoint string,
	basicAuthUsername, basicAuthPassword string,
	insecureSkipVerify bool, requestTimeout time.Duration,
	logWriter io.Writer,
) (predictor.PowerConsumptionPredictor, error) {

	switch endpointType {
	case waov1beta1.TypeFake:
		// TODO: implement
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
			util.WithCurlLogger(logWriter),
		}

		var client predictor.PowerConsumptionPredictor
		client = v2inferenceprotocol.NewPowerConsumptionClient(address, modelName, modelVersion, insecureSkipVerify, requestTimeout, requestEditorFns...)

		return client, nil
	}

	return nil, fmt.Errorf("unknown endpoint type: %s", endpointType)
}

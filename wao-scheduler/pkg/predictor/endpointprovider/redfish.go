package endpointprovider

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/waok8s/wao-metrics-adapter/pkg/metriccollector"
	waov1beta1 "github.com/waok8s/wao-nodeconfig/api/v1beta1"

	"github.com/waok8s/wao-scheduler/pkg/predictor"
	"github.com/waok8s/wao-scheduler/pkg/predictor/endpointprovider/api"
)

type RedfishEndpointProvider struct {
	// address contains scheme, host and port.
	// E.g., "http://10.0.0.1:8080"
	address string

	httpClient    *http.Client
	openAPIClient api.ClientWithResponsesInterface

	editorFns []metriccollector.RequestEditorFn
}

var _ predictor.EndpointProvider = (*RedfishEndpointProvider)(nil)

func NewRedfishEndpointProvider(address string, insecureSkipVerify bool, timeout time.Duration, editorFns ...metriccollector.RequestEditorFn) (*RedfishEndpointProvider, error) {
	c, err := api.NewClientWithResponses(
		address,
		api.WithHTTPClient(&http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipVerify}},
			Timeout:   timeout,
		}),
	)
	if err != nil {
		return nil, err
	}

	return &RedfishEndpointProvider{
		address: address,
		httpClient: &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipVerify}},
			Timeout:   timeout,
		},
		openAPIClient: c,
		editorFns:     editorFns,
	}, nil
}

func (p *RedfishEndpointProvider) getSystemID(ctx context.Context) (string, error) {

	type apiResp struct {
		Members []struct {
			ODataID string `json:"@odata.id"`
		} `json:"members"`
	}

	u, err := url.JoinPath(p.address, "redfish/v1/Systems")
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("unable to create HTTP request: %w", err)
	}
	for i, f := range p.editorFns {
		if err := f(ctx, req); err != nil {
			return "", fmt.Errorf("editorFns[%d] got error: %w", i, err)
		}
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("unable to send HTTP request: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		var apiResp apiResp
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			return "", fmt.Errorf("could not decode resp: %w", err)
		}
		if len(apiResp.Members) == 0 {
			return "", fmt.Errorf("invalid response apiResp=%+v", apiResp)
		}
		ss := strings.Split(apiResp.Members[0].ODataID, "/")
		return ss[len(ss)-1], nil
	default:
		return "", fmt.Errorf("HTTP status=%s", resp.Status)
	}
}

func (p *RedfishEndpointProvider) GetModels(ctx context.Context) (*api.MachineLearningModel, error) {

	systemID, err := p.getSystemID(ctx)
	if err != nil {
		return nil, err
	}

	editorFns := make([]api.RequestEditorFn, len(p.editorFns))
	for i, fn := range p.editorFns {
		editorFns[i] = api.RequestEditorFn(fn)
	}

	resp, err := p.openAPIClient.GetRedfishV1SystemsSystemIdMachineLearningModelWithResponse(ctx, systemID, editorFns...)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode() {
	case http.StatusOK:
		return resp.JSON200, nil
	case http.StatusInternalServerError:
		return nil, fmt.Errorf("%+v", resp.JSON500)
	default:
		return nil, fmt.Errorf("code=%d", resp.StatusCode())
	}

}

func (p *RedfishEndpointProvider) Get(ctx context.Context, predictorType predictor.PredictorType) (*waov1beta1.EndpointTerm, error) {

	modelType := waov1beta1.TypeV2InferenceProtocol
	modelAddress := ""
	modelName := ""
	modelVersion := "v0.1.0"

	models, err := p.GetModels(ctx)
	if err != nil {
		return nil, err
	}

	switch predictorType {
	case predictor.TypePowerConsumption:
		if models.PowerConsumptionModel == nil ||
			models.PowerConsumptionModel.Name == nil || models.PowerConsumptionModel.Url == nil {
			return nil, fmt.Errorf("invalid model: %+v", models.PowerConsumptionModel)
		}
		modelAddress = *models.PowerConsumptionModel.Url
		modelName = *models.PowerConsumptionModel.Name
	case predictor.TypeResponseTime:
		if models.ResponseTimeModel == nil ||
			models.ResponseTimeModel.Name == nil || models.ResponseTimeModel.Url == nil {
			return nil, fmt.Errorf("invalid model: %+v", models.ResponseTimeModel)
		}
		modelAddress = *models.ResponseTimeModel.Url
		modelName = *models.ResponseTimeModel.Name
	default:
		return nil, fmt.Errorf("unknown predictorType=%s", predictorType)
	}

	ep, err := url.JoinPath(modelAddress, "v2/models", modelName, "versions", modelVersion, "infer")
	if err != nil {
		return nil, err
	}

	et := &waov1beta1.EndpointTerm{
		Type:            modelType,
		Endpoint:        ep,
		BasicAuthSecret: nil,
		FetchInterval:   nil,
	}

	return et, nil
}

func (p *RedfishEndpointProvider) Endpoint() (string, error) { return p.address, nil }

package redfish

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	waov1beta1 "github.com/waok8s/wao-core/api/wao/v1beta1"
	"github.com/waok8s/wao-core/pkg/predictor"
	"github.com/waok8s/wao-core/pkg/predictor/redfish/api"
	"github.com/waok8s/wao-core/pkg/util"
)

type EndpointProvider struct {
	// address contains scheme, host and port.
	// E.g., "http://10.0.0.1:8080"
	address string

	httpClient    *http.Client
	openAPIClient api.ClientWithResponsesInterface

	editorFns []util.RequestEditorFn
}

var _ predictor.EndpointProvider = (*EndpointProvider)(nil)

func NewEndpointProvider(address string, insecureSkipVerify bool, timeout time.Duration, editorFns ...util.RequestEditorFn) (*EndpointProvider, error) {
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

	return &EndpointProvider{
		address: address,
		httpClient: &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipVerify}},
			Timeout:   timeout,
		},
		openAPIClient: c,
		editorFns:     editorFns,
	}, nil
}

func (p *EndpointProvider) getSystemID(ctx context.Context) (string, error) {

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

func (p *EndpointProvider) GetModels(ctx context.Context) (*api.MachineLearningModel, error) {

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
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("code=%d err=%+v", resp.StatusCode(), resp.JSON401)
	case http.StatusForbidden:
		return nil, fmt.Errorf("code=%d err=%+v", resp.StatusCode(), resp.JSON403)
	case http.StatusNotFound:
		return nil, fmt.Errorf("code=%d err=%+v", resp.StatusCode(), resp.JSON404)
	case http.StatusMethodNotAllowed:
		return nil, fmt.Errorf("code=%d err=%+v", resp.StatusCode(), resp.JSON405)
	case http.StatusNotAcceptable:
		return nil, fmt.Errorf("code=%d err=%+v", resp.StatusCode(), resp.JSON406)
	case http.StatusInternalServerError:
		return nil, fmt.Errorf("code=%d err=%+v", resp.StatusCode(), resp.JSON500)
	default:
		return nil, fmt.Errorf("code=%d (unexpected)", resp.StatusCode())
	}
}

func (p *EndpointProvider) Get(ctx context.Context, predictorType predictor.PredictorType) (*waov1beta1.EndpointTerm, error) {

	var modelType string
	var modelAddress string
	var modelName string
	var modelVersion string

	models, err := p.GetModels(ctx)
	if err != nil {
		return nil, err
	}

	switch predictorType {
	case predictor.TypePowerConsumption:
		if models.PowerConsumptionModel == nil ||
			models.PowerConsumptionModel.Type == nil || models.PowerConsumptionModel.Url == nil ||
			models.PowerConsumptionModel.Name == nil || models.PowerConsumptionModel.Version == nil {
			return nil, fmt.Errorf("invalid model: %+v", models.PowerConsumptionModel)
		}
		modelType = *models.PowerConsumptionModel.Type
		modelAddress = *models.PowerConsumptionModel.Url
		modelName = *models.PowerConsumptionModel.Name
		modelVersion = *models.PowerConsumptionModel.Version
	case predictor.TypeResponseTime:
		if models.ResponseTimeModel == nil ||
			models.ResponseTimeModel.Type == nil || models.ResponseTimeModel.Url == nil ||
			models.ResponseTimeModel.Name == nil || models.ResponseTimeModel.Version == nil {
			return nil, fmt.Errorf("invalid model: %+v", models.ResponseTimeModel)
		}
		modelType = *models.ResponseTimeModel.Type
		modelAddress = *models.ResponseTimeModel.Url
		modelName = *models.ResponseTimeModel.Name
		modelVersion = *models.ResponseTimeModel.Version
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

func (p *EndpointProvider) Endpoint() (string, error) { return p.address, nil }

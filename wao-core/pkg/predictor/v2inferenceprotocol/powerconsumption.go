package v2inferenceprotocol

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/waok8s/wao-core/pkg/predictor"
	"github.com/waok8s/wao-core/pkg/util"
)

type PowerConsumptionPredictor struct {
	// address contains scheme, host and port.
	// E.g., "http://10.0.0.1:8080"
	address string
	// model name.
	modelName string
	// model version.
	modelVersion string

	client    *http.Client
	editorFns []util.RequestEditorFn
}

var _ predictor.PowerConsumptionPredictor = (*PowerConsumptionPredictor)(nil)

func NewPowerConsumptionPredictor(address, modelName, modelVersion string, insecureSkipVerify bool, timeout time.Duration, editorFns ...util.RequestEditorFn) *PowerConsumptionPredictor {
	return &PowerConsumptionPredictor{
		address:      address,
		modelName:    modelName,
		modelVersion: modelVersion,
		client: &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipVerify}},
			Timeout:   timeout,
		},
		editorFns: editorFns,
	}
}

// Endpoint constructs the API endpoint.
//
// There is 2 types of URL.
//   - With version http://hogefuga:12345/v2/models/piyo/versions/v0.1.0/infer
//   - Without version http://hogefuga:12345/v2/models/piyo/infer
func (p *PowerConsumptionPredictor) Endpoint() (string, error) {
	if p.modelVersion == "" {
		return url.JoinPath(p.address, "v2/models", p.modelName, "infer")
	} else {
		return url.JoinPath(p.address, "v2/models", p.modelName, "versions", p.modelVersion, "infer")
	}
}

func (p *PowerConsumptionPredictor) Predict(ctx context.Context, cpuUsage, inletTemp, deltaP float64) (watt float64, err error) {
	url, err := p.Endpoint()
	if err != nil {
		return 0.0, fmt.Errorf("unable to get endpoint URL: %w", err)
	}

	body, err := json.Marshal(newInferPowerConsumptionRequest(cpuUsage, inletTemp, deltaP))
	if err != nil {
		return 0.0, fmt.Errorf("unable to marshal the request body=%+v err=%w", body, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return 0.0, fmt.Errorf("unable to create HTTP request: %w", err)
	}
	for i, f := range p.editorFns {
		if err := f(ctx, req); err != nil {
			return 0.0, fmt.Errorf("editorFns[%d] got error: %w", i, err)
		}
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return 0.0, fmt.Errorf("unable to send HTTP request: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		var apiResp inferPowerConsumptionResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			return 0.0, fmt.Errorf("could not decode resp: %w", err)
		}
		if len(apiResp.Outputs) == 0 || len(apiResp.Outputs[0].Data) == 0 {
			return 0.0, fmt.Errorf("invalid response apiResp=%+v", apiResp)
		}
		return apiResp.Outputs[0].Data[0], nil
	default:
		return 0.0, fmt.Errorf("HTTP status=%s", resp.Status)
	}
}

// inferPowerConsumptionRequest holds a request.
//
// E.g.,
//
//	{
//	  "inputs": [
//	    {
//	      "name": "predict-prob",
//	      "shape": [ 1, 3 ],
//	      "datatype": "FP32",
//	      "data": [ [10, 22, 0.2 ] ]
//	    }
//	  ]
//	}
type inferPowerConsumptionRequest struct {
	Inputs []struct {
		Name     string      `json:"name"`
		Shape    []int       `json:"shape"`
		Datatype string      `json:"datatype"`
		Data     [][]float32 `json:"data"`
	} `json:"inputs"`
}

var (
	InferPowerConsumptionRequestInputName = "predict-prob"
)

func newInferPowerConsumptionRequest(cpuUsage, ambientTemp, staticPressureDiff float64) *inferPowerConsumptionRequest {
	var (
		name     = InferPowerConsumptionRequestInputName
		datatype = "FP32"
		shapeX   = 1
		shapeY   = 3
	)
	return &inferPowerConsumptionRequest{
		Inputs: []struct {
			Name     string      `json:"name"`
			Shape    []int       `json:"shape"`
			Datatype string      `json:"datatype"`
			Data     [][]float32 `json:"data"`
		}{
			{
				Name:     name,
				Shape:    []int{shapeX, shapeY},
				Datatype: datatype,
				Data:     [][]float32{{float32(cpuUsage), float32(ambientTemp), float32(staticPressureDiff)}},
			},
		},
	}
}

// inferPowerConsumptionResponse holds a response.
// Ignore values except outputs[*].data[]
//
// E.g.,
//
//	{
//	  "model_name": "model1",
//	  "model_version": "v0.1.0",
//	  "id": "0dc429d2-bd02-404b-b624-a0fa628e451e",
//	  "parameters": {
//	    "content_type": null,
//	    "headers": null
//	  },
//	  "outputs": [
//	    {
//	      "name": "predict",
//	      "shape": [ 1, 1 ],
//	      "datatype": "FP64",
//	      "parameters": null,
//	      "data": [ 94.76267448501928 ]
//	    }
//	  ]
//	}
type inferPowerConsumptionResponse struct {
	Outputs []struct {
		Data []float64 `json:"data"`
	} `json:"outputs"`
}

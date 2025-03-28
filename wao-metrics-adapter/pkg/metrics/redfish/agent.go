package redfish

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/waok8s/waok8s/wao-metrics-adapter/pkg/metrics"
	"github.com/waok8s/waok8s/wao-metrics-adapter/pkg/util"
)

type ServerType string

const (
	TypeAutoDetect ServerType = ""

	TypeDelliDRAC      ServerType = "iDRAC"
	TypeLenovoXClarity ServerType = "XClarity"
	TypeSupermicroSSM  ServerType = "SSM"
)

type GetInletTempFunc func(ctx context.Context, server string, client *http.Client, editorFns ...util.RequestEditorFn) (float64, error)

var (
	GetInletTempFns = map[ServerType]GetInletTempFunc{
		TypeDelliDRAC:      GetInletTempForTypeDelliDRAC,
		TypeLenovoXClarity: GetInletTempForTypeLenovoXClarity,
		TypeSupermicroSSM:  GetInletTempForTypeSupermicroSSM,
	}
)

// GetInletTempForTypeDelliDRAC returns inlet temp.
//
//   - URL: https://{SERVER}/redfish/v1/Chassis/System.Embedded.1/Sensors/SystemBoardInletTemp
//   - Key: ["Reading"]
func GetInletTempForTypeDelliDRAC(ctx context.Context, server string, client *http.Client, editorFns ...util.RequestEditorFn) (float64, error) {

	type apiResponse struct {
		Reading float64 `json:"Reading"`
	}

	u, err := url.JoinPath(server, "redfish/v1/Chassis/System.Embedded.1/Sensors/SystemBoardInletTemp")
	if err != nil {
		return 0.0, fmt.Errorf("could not build URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0.0, fmt.Errorf("unable to create HTTP request: %w", err)
	}
	for i, f := range editorFns {
		if err := f(ctx, req); err != nil {
			return 0.0, fmt.Errorf("editorFns[%d] got error: %w", i, err)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0.0, fmt.Errorf("unable to send HTTP request: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		var apiResp apiResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			return 0.0, fmt.Errorf("could not decode resp: %w", err)
		}
		return apiResp.Reading, nil
	default:
		return 0.0, fmt.Errorf("HTTP status=%s", resp.Status)
	}
}

// GetInletTempForTypeLenovoXClarity returns inlet temp.
//
//   - URL: https://{SERVER}/redfish/v1/Chassis/1/Sensors/{SENSOR_ID}
//   - Key: ["Reading"]
//   - SENSOR_ID: ["Name"] == "Ambient Temp" in range [ https://{SERVER}/redfish/v1/Chassis/1/Sensors "Members" ]
//
// NOTE: sensor_id is currently fixed to "128L0" as all our servers have the same ID for the ambient temperature sensor.
// For more flexibility, search for a sensor with `Name: "Ambient Temp"` and cache its ID
// (servers typically have dozens of sensors, so caching is necessary for performance).
// This may require an additional variable (e.g., a sync.Map) in RedfishClient for data sharing.
func GetInletTempForTypeLenovoXClarity(ctx context.Context, server string, client *http.Client, editorFns ...util.RequestEditorFn) (float64, error) {

	const targetSensorName = "Ambient Temp"
	type apiResponse struct {
		Name    string  `json:"Name"`
		Reading float64 `json:"Reading"`
	}

	sensorID := "128L0"

	u, err := url.JoinPath(server, "redfish/v1/Chassis/1/Sensors/", sensorID)
	if err != nil {
		return 0.0, fmt.Errorf("could not build URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0.0, fmt.Errorf("unable to create HTTP request: %w", err)
	}
	for i, f := range editorFns {
		if err := f(ctx, req); err != nil {
			return 0.0, fmt.Errorf("editorFns[%d] got error: %w", i, err)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0.0, fmt.Errorf("unable to send HTTP request: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		var apiResp apiResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			return 0.0, fmt.Errorf("could not decode resp: %w", err)
		}
		if apiResp.Name != targetSensorName {
			return 0.0, fmt.Errorf("sensor name want '%s' but got '%s'", targetSensorName, apiResp.Name)
		}
		return apiResp.Reading, nil
	default:
		return 0.0, fmt.Errorf("HTTP status=%s", resp.Status)
	}
}

// GetInletTempForTypeSupermicroSSM returns inlet temp.
//
//   - URL: https://{SERVER}/redfish/v1/Chassis/1/Thermal
//   - Key: ["Name"] == "System Temp" in range ["Temperatures"] | ["ReadingCelsius"]
func GetInletTempForTypeSupermicroSSM(ctx context.Context, server string, client *http.Client, editorFns ...util.RequestEditorFn) (float64, error) {

	const targetSensorName = "System Temp"
	type apiResponse struct {
		Temperatures []struct {
			Name           string  `json:"Name"`
			ReadingCelsius float64 `json:"ReadingCelsius"`
		} `json:"Temperatures"`
	}

	u, err := url.JoinPath(server, "redfish/v1/Chassis/1/Thermal")
	if err != nil {
		return 0.0, fmt.Errorf("could not build URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0.0, fmt.Errorf("unable to create HTTP request: %w", err)
	}
	for i, f := range editorFns {
		if err := f(ctx, req); err != nil {
			return 0.0, fmt.Errorf("editorFns[%d] got error: %w", i, err)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0.0, fmt.Errorf("unable to send HTTP request: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		var apiResp apiResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			return 0.0, fmt.Errorf("could not decode resp: %w", err)
		}
		for _, sensor := range apiResp.Temperatures {
			if sensor.Name == targetSensorName {
				return sensor.ReadingCelsius, nil
			}
		}
		return 0.0, fmt.Errorf("sensor '%s' not found", targetSensorName)
	default:
		return 0.0, fmt.Errorf("HTTP status=%s", resp.Status)
	}
}

type InletTempAgent struct {
	// address contains scheme, host and port.
	// E.g., "http://10.0.0.1:8080"
	address string
	// serverType contains server type.
	serverType ServerType

	client    *http.Client
	editorFns []util.RequestEditorFn
}

var _ metrics.Agent = (*InletTempAgent)(nil)

// NewInletTempAgent inits the client.
// If serverType is not specified, the client will try all known endpoints.
func NewInletTempAgent(address string, serverType ServerType, insecureSkipVerify bool, timeout time.Duration, editorFns ...util.RequestEditorFn) *InletTempAgent {
	return &InletTempAgent{
		address:    address,
		serverType: serverType,
		client: &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipVerify}},
			Timeout:   timeout,
		},
		editorFns: editorFns,
	}
}

func (a *InletTempAgent) Fetch(ctx context.Context) (float64, error) {
	fn, ok := GetInletTempFns[a.serverType]
	if !ok {
		type result struct {
			ServerType  ServerType
			MetricValue float64
		}
		resultCh := make(chan result, len(GetInletTempFns))
		errCh := make(chan error, len(GetInletTempFns))
		wg := &sync.WaitGroup{}
		for st, fn := range GetInletTempFns {
			st := st
			fn := fn
			wg.Add(1)
			go func() {
				time.Sleep(time.Duration(rand.Intn(500)) * time.Millisecond)

				v, err := fn(ctx, a.address, a.client, a.editorFns...)
				if err != nil {
					errCh <- err
				} else {
					resultCh <- result{ServerType: st, MetricValue: v}
				}

				wg.Done()
			}()
		}
		wg.Wait()
		close(resultCh)
		close(errCh)

		if len(resultCh) > 0 {
			r := <-resultCh
			a.serverType = r.ServerType
			return r.MetricValue, nil
		} else {
			err := errors.New("all GetSensorValueFuncs got error")
			for e := range errCh {
				err = errors.Join(err, e)
			}
			return 0.0, err
		}
	} else {
		return fn(ctx, a.address, a.client, a.editorFns...)
	}
}

func (a *InletTempAgent) ValueType() metrics.ValueType { return metrics.ValueInletTemperature }

package inlettemp

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"time"

	"github.com/Nedopro2022/wao-metrics-adapter/pkg/metriccollector"
)

type ServerType string

const (
	TypeDelliDRAC      ServerType = "Dell-iDRAC"
	TypeLenovoXClarity ServerType = "Lenovo-XClarity"
	TypeSupermicroSSM  ServerType = "Supermicro-SSM"
)

type GetSensorValueFunc func(ctx context.Context, server string, client *http.Client, editorFns ...metriccollector.RequestEditorFn) (float64, error)

var (
	GetSensorValueFn = map[ServerType]GetSensorValueFunc{
		TypeDelliDRAC:      GetSensorValueForTypeDelliDRAC,
		TypeLenovoXClarity: GetSensorValueForTypeLenovoXClarity,
		TypeSupermicroSSM:  GetSensorValueForTypeSupermicroSSM,
	}
)

// GetSensorValueForTypeDelliDRAC returns inlet temp.
//
//   - URL: https://{SERVER}/redfish/v1/Chassis/System.Embedded.1/Sensors/SystemBoardInletTemp
//   - Key: ["Reading"]
func GetSensorValueForTypeDelliDRAC(ctx context.Context, server string, client *http.Client, editorFns ...metriccollector.RequestEditorFn) (float64, error) {
	return 0.0, errors.New("not yet implemented") // TODO
}

// GetSensorValueForTypeLenovoXClarity returns inlet temp.
//
//   - URL: https://{SERVER}/redfish/v1/Chassis/1/Sensors/{SENSOR_ID}
//   - Key: ["Reading"]
//   - SENSOR_ID: https://{SERVER}/redfish/v1/Chassis/1/Sensors
func GetSensorValueForTypeLenovoXClarity(ctx context.Context, server string, client *http.Client, editorFns ...metriccollector.RequestEditorFn) (float64, error) {
	return 0.0, errors.New("not yet implemented") // TODO
}

// GetSensorValueForTypeSupermicroSSM returns inlet temp.
//
//   - URL: https://{SERVER}/redfish/v1/Chassis/1/Thermal
//   - Key: ["Name"] == "System Temp" in range ["Temperatures"] | ["ReadingCelsius"]
func GetSensorValueForTypeSupermicroSSM(ctx context.Context, server string, client *http.Client, editorFns ...metriccollector.RequestEditorFn) (float64, error) {
	return 0.0, errors.New("not yet implemented") // TODO
}

type RedfishClient struct {
	// address contains scheme, host and port.
	// E.g., "http://10.0.0.1:8080"
	address string
	// serverType contains server type.
	serverType ServerType

	client *http.Client
}

var _ metriccollector.MetricCollector = (*RedfishClient)(nil)

// NewRedfishClient inits the client.
// If serverType is not specified, the client will try all known endpoints.
func NewRedfishClient(address string, serverType ServerType, insecureSkipVerify bool, timeout time.Duration) *RedfishClient {
	return &RedfishClient{
		address:    address,
		serverType: serverType,
		client: &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipVerify}},
			Timeout:   timeout,
		},
	}
}

func (c *RedfishClient) Fetch(ctx context.Context, editorFns ...metriccollector.RequestEditorFn) (float64, error) {
	fn, ok := GetSensorValueFn[c.serverType]
	if !ok {
		for st, fn := range GetSensorValueFn {
			v, err := fn(ctx, c.address, c.client)
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			c.serverType = st
			return v, nil
		}
		return 0.0, errors.New("all getSensorValueFn got error")
	} else {
		return fn(ctx, c.address, c.client)
	}
}

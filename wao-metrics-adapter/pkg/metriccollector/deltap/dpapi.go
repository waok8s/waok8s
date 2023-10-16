package deltap

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/Nedopro2022/wao-metrics-adapter/pkg/metriccollector"
)

type DifferentialPressureAPIClient struct {
	// address contains scheme, host and port.
	// E.g., "http://10.0.0.1:8080"
	address string
	// sensorName contains sensorName.
	// E.g., "101037B"
	sensorName string
	// nodeName contains nodeName.
	// E.g., "node0"
	nodeName string
	// nodeIP contains node's IPv4 address.
	// E.g., "10.0.0.2"
	nodeIP string

	client *http.Client
}

var _ metriccollector.MetricCollector = (*DifferentialPressureAPIClient)(nil)

// NewDifferentialPressureAPIClient inits the client.
// At least one of sensorName, nodeName or nodeIP must be specified.
func NewDifferentialPressureAPIClient(address string, sensorName, nodeName, nodeIP string, insecureSkipVerify bool, timeout time.Duration) *DifferentialPressureAPIClient {
	return &DifferentialPressureAPIClient{
		address:    address,
		sensorName: sensorName,
		nodeName:   nodeName,
		nodeIP:     nodeIP,
		client: &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipVerify}},
			Timeout:   timeout,
		},
	}
}

// Endpoint constructs the API endpoint.
//
// There is 3 types of URL.
//   - Get value by sensor name http://hogefuga:12345/api/sensor/2027B30
//   - Get value by node name http://hogefuga:12345/api/sensor/by_nodename/node-0
//   - Get value by node IP http://hogefuga:12345/api/sensor/by_nodeaddress/10.10.0.1
func (c *DifferentialPressureAPIClient) Endpoint() (string, error) {
	switch {
	case c.sensorName != "":
		return url.JoinPath(c.address, "api", "sensor", c.sensorName)
	case c.nodeName != "":
		return url.JoinPath(c.address, "api", "sensor", "by_nodename", c.nodeName)
	case c.nodeIP != "":
		return url.JoinPath(c.address, "api", "sensor", "by_nodeaddress", c.nodeIP)
	default:
		return "", fmt.Errorf("could not construct endpoint from %+v", c)
	}
}

// differentialPressureAPIResponse holds a response.
//
// e.g.
//
//	{
//	  "code": 200,
//	  "sensor": [
//	    {
//	      "pressure": 0.01,
//	      "sensorid": "101037B",
//	      "temperature": 26.02
//	    }
//	  ]
//	}
type differentialPressureAPIResponse struct {
	StatusCode int           `json:"code"`
	Sensors    []sensorValue `json:"sensor"`
}

type sensorValue struct {
	SensorID    string  `json:"sensorid"`
	Pressure    float64 `json:"pressure"`
	Temperature float64 `json:"temperature"`
}

func (c *DifferentialPressureAPIClient) GetSensorValue(ctx context.Context, editorFns ...metriccollector.RequestEditorFn) (sensorValue, error) {
	var v sensorValue

	url, err := c.Endpoint()
	if err != nil {
		return v, fmt.Errorf("unable to get endpoint URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return v, fmt.Errorf("unable to create HTTP request: %w", err)
	}

	for i, f := range editorFns {
		if err := f(ctx, req); err != nil {
			return v, fmt.Errorf("editorFns[%d] got error: %w", i, err)
		}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return v, fmt.Errorf("unable to send HTTP request: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		var apiResp differentialPressureAPIResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			return v, fmt.Errorf("could not decode resp: %w", err)
		}
		if len(apiResp.Sensors) == 0 {
			return v, fmt.Errorf("invalid response apiResp=%+v", apiResp)
		}
		return apiResp.Sensors[0], nil
	default:
		return v, fmt.Errorf("HTTP status=%s", resp.Status)
	}
}

func (c *DifferentialPressureAPIClient) Fetch(ctx context.Context, editorFns ...metriccollector.RequestEditorFn) (float64, error) {
	v, err := c.GetSensorValue(ctx, editorFns...)
	if err != nil {
		return 0.0, err
	}
	return v.Pressure, nil
}

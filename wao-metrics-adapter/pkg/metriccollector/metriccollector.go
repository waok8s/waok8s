package metriccollector

import (
	"context"
	"io"
	"net/http"

	"moul.io/http2curl/v2"

	"github.com/waok8s/wao-metrics-adapter/pkg/metric"
)

type MetricCollector interface {
	ValueType() metric.ValueType
	Fetch(ctx context.Context) (value float64, err error)
}

type RequestEditorFn func(ctx context.Context, req *http.Request) error

func WithRequestHeader(k, v string) RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Add(k, v)
		return nil
	}
}

func WithCurlLogger(w io.Writer) RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		cmd, err := http2curl.GetCurlCommand(req)
		if err != nil {
			w.Write([]byte(err.Error()))
		} else {
			w.Write([]byte(cmd.String()))
		}
		return nil
	}
}

func WithBasicAuth(username, password string) RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		if username != "" && password != "" {
			req.SetBasicAuth(username, password)
		}
		return nil
	}
}

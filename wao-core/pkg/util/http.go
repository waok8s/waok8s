package util

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/go-logr/logr"
	"moul.io/http2curl/v2"
)

type RequestEditorFn func(ctx context.Context, req *http.Request) error

func WithRequestHeader(k, v string) RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Add(k, v)
		return nil
	}
}

func WithCurlLogger(w io.Writer) RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		if w == nil {
			return nil
		}

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

// CurlWriterLogr writes curl command (or any string) in logr.Logger style.
type CurlWriterLogr struct {
	Logger logr.Logger
	Msg    string
}

func (w *CurlWriterLogr) Write(p []byte) (n int, err error) {
	w.Logger.Info(w.Msg, "curl", string(p))
	return len(p), nil
}

// CurlWriter writes curl command (or any string) with # prefix.
type CurlWriter struct{ W io.Writer }

func (w *CurlWriter) Write(p []byte) (n int, err error) {
	return fmt.Fprintf(w.W, "# %s\n", p)
}

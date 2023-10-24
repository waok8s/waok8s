package util

import (
	"context"
	"log/slog"
	"net/http"

	"moul.io/http2curl/v2"
)

type RequestEditorFn func(ctx context.Context, req *http.Request) error

func WithBasicAuth(username, password string) RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		if username != "" && password != "" {
			req.SetBasicAuth(username, password)
		}
		return nil
	}
}

func WithCurlLogger(lg *slog.Logger) RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		if lg == nil {
			lg = slog.Default()
		}

		cmd, err := http2curl.GetCurlCommand(req)
		if err != nil {
			lg.Error("failed to get curl command", "err", err)
		} else {
			lg.Info("sending request", "curl", cmd.String())
		}

		return nil
	}
}

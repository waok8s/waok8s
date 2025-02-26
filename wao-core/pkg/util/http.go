package util

import (
	"context"
	"log/slog"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
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
			lg.Debug("sending request", "curl", cmd.String())
		}

		return nil
	}
}

func GetBasicAuthFromNamespaceScopedSecret(ctx context.Context, client kubernetes.Interface, namespace string, ref *corev1.LocalObjectReference) (username, password string) {
	lg := slog.With("func", "GetBasicAuthFromNamespaceScopedSecret")

	if ref == nil || ref.Name == "" {
		return
	}

	// NOTE: This is a workaround. How to use controller-runtime client to get namespace scoped Secrets?
	secret, err := client.CoreV1().Secrets(namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		lg.Error("unable to get Secret so skip basic auth", "err", err, "obj", types.NamespacedName{Namespace: namespace, Name: ref.Name})
		return "", ""
	}

	// NOTE: RBAC error and crash. Why?
	// secret := &corev1.Secret{}
	// if err := client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, secret); err != nil {
	// 	lg.Error(err, "unable to get Secret so skip basic auth", "namespace", namespace, "name", ref.Name)
	// 	return "", ""
	// }

	username = string(secret.Data["username"])
	password = string(secret.Data["password"])

	return
}

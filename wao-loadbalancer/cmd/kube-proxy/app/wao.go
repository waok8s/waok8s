package app

import "os"

const (
	EnvVarProxyName = "WAO_SERVICE_PROXY_NAME"

	DefaultHealthzBindAddress = "0.0.0.0:10356"
	DefaultMetricsBindAddress = "0.0.0.0:10349"
)

var (
	// WAOProxyName is the service proxy name of this proxy.
	// It uses the value of the environment variable WAO_SERVICE_PROXY_NAME, set in init() function.
	// If not set or empty, this proxy will act as default service proxy.
	// If set, this proxy will act as non-default service proxy, and only proxy services with the same name.
	WAOProxyName = ""
)

func init() {
	WAOProxyName = os.Getenv(EnvVarProxyName)
}

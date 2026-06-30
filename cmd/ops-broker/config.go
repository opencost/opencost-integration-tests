package main

import (
	"fmt"
	"os"
	"strings"
)

// Config is the broker's runtime configuration, all from the environment.
type Config struct {
	// AuthToken is the bearer token callers (tests) must present. This is the
	// broker's OWN auth — separate from the k8s service-account token.
	AuthToken string
	// Addr is the listen address, e.g. ":8080".
	Addr string
	// Namespace is the namespace the broker is allowed to operate in.
	Namespace string
	// OpenCostDeployment is the deployment name the restart op targets.
	OpenCostDeployment string
	// OpenCostSelector is the label selector used to list OpenCost pods.
	OpenCostSelector string
	// PrometheusNamespace is the namespace used by Prometheus chaos scenarios.
	PrometheusNamespace string
	// PrometheusSelector is the label selector used by Prometheus chaos scenarios.
	PrometheusSelector string
	// ChaosNamespace is where broker-owned Chaos Mesh resources are created.
	ChaosNamespace string
	// LogNamespaces is the allowlist of namespaces the /v1/logs endpoint may
	// read from. Any namespace not in this set is rejected.
	LogNamespaces []string
	// Kubeconfig, if set, runs the broker out-of-cluster (local dev). Empty
	// means in-cluster (rest.InClusterConfig).
	Kubeconfig string
}

func LoadConfig() (Config, error) {
	c := Config{
		AuthToken:           os.Getenv("BROKER_AUTH_TOKEN"),
		Addr:                getEnv("BROKER_ADDR", ":8080"),
		Namespace:           getEnv("BROKER_NAMESPACE", "opencost"),
		OpenCostDeployment:  getEnv("BROKER_OPENCOST_DEPLOYMENT", "opencost"),
		OpenCostSelector:    getEnv("BROKER_OPENCOST_SELECTOR", "app.kubernetes.io/name=opencost"),
		PrometheusNamespace: getEnv("BROKER_PROMETHEUS_NAMESPACE", "prometheus"),
		PrometheusSelector:  getEnv("BROKER_PROMETHEUS_SELECTOR", "app.kubernetes.io/name=prometheus"),
		ChaosNamespace:      getEnv("BROKER_CHAOS_NAMESPACE", "opencost"),
		Kubeconfig:          os.Getenv("KUBECONFIG"),
	}
	// Default the log allowlist to the OpenCost and Prometheus namespaces the
	// broker already targets, so restart/chaos panic checks work out of the box.
	c.LogNamespaces = splitList(getEnv("BROKER_LOG_NAMESPACES",
		strings.Join([]string{c.Namespace, c.PrometheusNamespace}, ",")))
	if c.AuthToken == "" {
		return Config{}, fmt.Errorf("BROKER_AUTH_TOKEN is required")
	}
	return c, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// splitList parses a comma-separated env value into a trimmed, non-empty slice.
func splitList(v string) []string {
	out := []string{}
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

package prometheus

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	// DefaultPrometheusURL is the default URL for the Prometheus server
	DefaultPrometheusURL = "https://demo-prometheus.infra.opencost.io"
	// EnvPrometheusURL is the environment variable name for the Prometheus URL
	EnvPrometheusURL = "PROMETHEUS_URL"
)

// Client represents a Prometheus API client
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// PrometheusResponse represents the response from Prometheus API
type PrometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric struct {
				Pod       string `json:"pod"`
				Namespace string `json:"namespace"`
			} `json:"metric"`
		} `json:"result"`
	} `json:"data"`
}

// NewClient creates a new Prometheus client
func NewClient() *Client {
	baseURL := os.Getenv(EnvPrometheusURL)
	if baseURL == "" {
		baseURL = DefaultPrometheusURL
	}

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetPodsByController queries Prometheus for pods of a specific controller type
func (c *Client) GetPodsByController(controllerKind string, window string) (map[string]string, error) {
	// For ReplicaSets, we need to query for Deployment-owned pods
	promQueryKind := controllerKind

	// Query Prometheus for pods of the specified controller type over the specified window
	promQuery := fmt.Sprintf("max_over_time(kube_pod_owner{owner_kind=\"%s\"}[%s])", promQueryKind, window)
	promURL := fmt.Sprintf("%s/api/v1/query?query=%s", c.baseURL, url.QueryEscape(promQuery))

	promResp, err := c.httpClient.Get(promURL)
	if err != nil {
		return nil, fmt.Errorf("failed to query Prometheus for %s with window %s: %v", controllerKind, window, err)
	}
	defer promResp.Body.Close()

	var promData PrometheusResponse
	if err := json.NewDecoder(promResp.Body).Decode(&promData); err != nil {
		return nil, fmt.Errorf("failed to decode Prometheus response for %s with window %s: %v", controllerKind, window, err)
	}

	promPods := make(map[string]string)
	for _, result := range promData.Data.Result {
		promPods[result.Metric.Pod] = result.Metric.Namespace
	}

	if len(promPods) == 0 {
		return nil, fmt.Errorf("no %s pods found in Prometheus metrics for window %s", controllerKind, window)
	}

	return promPods, nil
}

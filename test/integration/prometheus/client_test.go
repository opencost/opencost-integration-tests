package prometheus

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
)

/////////////////////
// Note: Make sure the filters (equalTo and notEqualTo) inside the metric are sorted.
/////////////////////

// TestConstructPromQLQueryURL tests the constructPromQLQueryURL function.
func TestConstructPromQLQueryURL(t *testing.T) {
	// Define a base URL for the test client
	fixedQueryTime := time.Date(2025, time.July, 2, 16, 0, 0, 0, time.UTC).Truncate(time.Hour).Add(time.Hour).Unix()
	tests := []struct {
		name        string
		input       prometheus.PrometheusInput
		expectedURL string
	}{
		{
			name: "Complex Query - All Fields",
			input: prometheus.PrometheusInput{
				Metric:              "kube_pod_container_status_running",
				MetricNotEqualTo:    "0",
				Function:            []string{"avg"},
				AggregateBy:         []string{"container", "pod", "namespace"},
				AggregateWindow:     "24h",
				AggregateResolution: "5m",
			},
			expectedURL: fmt.Sprintf("%s/api/v1/query?query=%s", prometheus.DefaultPrometheusURL, url.QueryEscape(`avg(kube_pod_container_status_running{} != 0) by (container, pod, namespace)[24h:5m]`)),
		},
		{
			name: "Multiple Values after consolidating",
			input: prometheus.PrometheusInput{
				Metric:              "kube_test_metric",
				Filters:             map[string]string{"resource": "memory"},
				IgnoreFilters:       map[string][]string{"container": {"", "POD"}, "node": {""}},
				Function:            []string{"avg_over_time", "avg"},
				AggregateBy:         []string{"container", "pod", "namespace", "node"},
				AggregateWindow:     "24h",
				AggregateResolution: "5m",
			},
			expectedURL: fmt.Sprintf("%s/api/v1/query?query=%s", prometheus.DefaultPrometheusURL, url.QueryEscape(`avg(avg_over_time(kube_test_metric{resource="memory", container!="", container!="POD", node!=""})) by (container, pod, namespace, node)[24h:5m]`)),
		},
		{
			name: "End Time Parameter",
			input: prometheus.PrometheusInput{
				Metric:          "kube_pod_container_resource_requests",
				Filters:         map[string]string{"resource": "memory"},
				IgnoreFilters:   map[string][]string{"container": {"", "POD"}, "node": {""}},
				QueryWindow:     "24h",
				QueryResolution: "5m",
				Function:        []string{"avg_over_time", "avg"},
				AggregateBy:     []string{"container", "pod", "namespace", "node"},
				Time:            &fixedQueryTime,
			},
			expectedURL: fmt.Sprintf("%s/api/v1/query?query=%s&time=%d", prometheus.DefaultPrometheusURL, url.QueryEscape(`avg(avg_over_time(kube_pod_container_resource_requests{resource="memory", container!="", container!="POD", node!=""}[24h:5m])) by (container, pod, namespace, node)`), 1751475600),
		},
		{
			name: "RAM Requested Costs",
			input: prometheus.PrometheusInput{
				Metric:        "kube_pod_container_resource_requests",
				Filters:       map[string]string{"resource": "memory", "unit": "byte"},
				IgnoreFilters: map[string][]string{"container": {"", "POD"}, "node": {""}},
				QueryWindow:   "24h",
				Function:      []string{"avg_over_time", "avg"},
				AggregateBy:   []string{"container", "pod", "node", "namespace"},
				Time:          &fixedQueryTime,
			},
			expectedURL: fmt.Sprintf("%s/api/v1/query?query=%s&time=%d", prometheus.DefaultPrometheusURL, url.QueryEscape(`avg(avg_over_time(kube_pod_container_resource_requests{resource="memory", unit="byte", container!="", container!="POD", node!=""}[24h])) by (container, pod, node, namespace)`), 1751475600),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			client := prometheus.NewClient()
			actualURL := client.ConstructPromQLQueryURL(tc.input)

			// Unescape and normalize for comparison
			expectedQueryUnescaped, err := url.QueryUnescape(strings.TrimPrefix(tc.expectedURL, prometheus.DefaultPrometheusURL+"/api/v1/query?query="))
			if err != nil {
				t.Fatalf("Failed to unescape expected URL for normalization: %v", err)
			}
			actualQueryUnescaped, err := url.QueryUnescape(strings.TrimPrefix(actualURL, prometheus.DefaultPrometheusURL+"/api/v1/query?query="))
			if err != nil {
				t.Fatalf("Failed to unescape actual URL for normalization: %v", err)
			}

			if actualQueryUnescaped != expectedQueryUnescaped {
				t.Errorf("URL Construction is incorrect.\nExpected: %s\nGot: %s", tc.expectedURL, actualURL)
			} else {
				t.Logf("Test Case Passed: %s", tc.name)
			}
		})
	}
}

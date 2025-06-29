package main

import (
	"net/url"
	"strings"
	"fmt"
	"testing"
    "github.com/opencost/opencost-integration-tests/pkg/prometheus"
)

// TestConstructPromQLQueryURL tests the constructPromQLQueryURL function.
func TestConstructPromQLQueryURL(t *testing.T) {
	// Define a base URL for the test client
	tests := []struct {
		name        string
		input       prometheus.PrometheusInput
		expectedURL string
	}{
		{
			name: "Complex Query - All Fields",
			input: prometheus.PrometheusInput{
				Metric:      "kube_test_metric",
				Filters:     map[string]string{"resource": "memory",},
				IgnoreFilters: map[string][]string{"container": {"", "POD"},"node": {""},},
				Window:      "24h",
				Function:    []string{"avg_over_time", "avg"},
				AggregateBy: []string{"container", "pod", "namespace"},
			},
			expectedURL: fmt.Sprintf("%s/api/v1/query?query=%s", prometheus.DefaultPrometheusURL, url.QueryEscape(`avg(avg_over_time(kube_test_metric{resource="memory", container!="", container!="POD", node!=""}[24h])) by (container, pod, namespace)`)),
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
				t.Errorf("URL Construction is incorrect.\nExpected: %s\nGot:      %s", tc.expectedURL, actualURL)
			} else {
				t.Logf("Test Case Passed: %s", tc.name)
			}
		})
	}
}

package allocation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

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

type AllocationPod struct {
	Name       string `json:"name"`
	Properties struct {
		Namespace      string `json:"namespace"`
		ControllerKind string `json:"controllerKind"`
	} `json:"properties"`
}

type AllocationResponse struct {
	Code   int                        `json:"code"`
	Status string                     `json:"status"`
	Data   []map[string]AllocationPod `json:"data"`
}

func validateControllerPods(t *testing.T, controllerKind string, window string) {
	client := &http.Client{Timeout: 10 * time.Second}

	// For ReplicaSets, we need to query for Deployment-owned pods
	promQueryKind := controllerKind

	// Query Prometheus for pods of the specified controller type over the specified window
	promQuery := fmt.Sprintf("max_over_time(kube_pod_owner{owner_kind=\"%s\"}[%s])", promQueryKind, window)
	promURL := fmt.Sprintf("https://demo-prometheus.infra.opencost.io/api/v1/query?query=%s", url.QueryEscape(promQuery))
	promResp, err := client.Get(promURL)
	if err != nil {
		t.Fatalf("Failed to query Prometheus for %s with window %s: %v", controllerKind, window, err)
	}
	defer promResp.Body.Close()

	var promData PrometheusResponse
	if err := json.NewDecoder(promResp.Body).Decode(&promData); err != nil {
		t.Fatalf("Failed to decode Prometheus response for %s with window %s: %v", controllerKind, window, err)
	}

	promPods := make(map[string]string)
	for _, result := range promData.Data.Result {
		promPods[result.Metric.Pod] = result.Metric.Namespace
	}
	if len(promPods) == 0 {
		t.Fatalf("No %s pods found in Prometheus metrics for window %s", controllerKind, window)
	}

	// Convert controllerKind to lowercase for the allocation API query
	lowerControllerKind := strings.ToLower(controllerKind)
	// For ReplicaSets, query OpenCost for Deployment types
	if controllerKind == "ReplicaSet" {
		lowerControllerKind = "deployment"
	}
	allocURL := fmt.Sprintf("http://localhost:9003/allocation?window=%s&aggregate=pod&filter=controllerKind:\"%s\"&accumulate=true", window, lowerControllerKind)
	allocResp, err := client.Get(allocURL)
	if err != nil {
		t.Fatalf("Failed to query OpenCost allocation API for %s with window %s: %v", controllerKind, window, err)
	}
	defer allocResp.Body.Close()

	// Read and print the raw response
	rawBody, err := io.ReadAll(allocResp.Body)
	if err != nil {
		t.Fatalf("Failed to read allocation response body for window %s: %v", window, err)
	}
	t.Logf("Raw allocation response for %s (window: %s): %s", controllerKind, window, string(rawBody))

	// Create a new reader from the raw body for JSON decoding
	allocResp.Body = io.NopCloser(bytes.NewBuffer(rawBody))

	var allocData AllocationResponse
	if err := json.NewDecoder(allocResp.Body).Decode(&allocData); err != nil {
		t.Fatalf("Failed to decode allocation response for %s with window %s: %v", controllerKind, window, err)
	}

	if allocData.Code != 200 || allocData.Status != "success" {
		t.Fatalf("%s allocation API returned error for window %s: code=%d, status=%s", controllerKind, window, allocData.Code, allocData.Status)
	}
	if len(allocData.Data) == 0 {
		t.Fatalf("No data returned from %s allocation API for window %s", controllerKind, window)
	}

	// Collect pod names from allocation data
	allocPods := make(map[string]string)
	for _, podMap := range allocData.Data {
		for podName, pod := range podMap {
			if podName == "" || pod.Name == "" {
				continue
			}

			allocPods[pod.Name] = pod.Properties.Namespace
		}
	}

	// Print all pod names from both sources
	promPodList := make([]string, 0, len(promPods))
	for pod := range promPods {
		promPodList = append(promPodList, pod)
	}
	allocPodList := make([]string, 0, len(allocPods))
	for pod := range allocPods {
		allocPodList = append(allocPodList, pod)
	}
	t.Logf("All Prometheus %s pods (window: %s): %v", controllerKind, window, promPodList)
	t.Logf("All allocation %s pods (window: %s): %v", controllerKind, window, allocPodList)

	// Find mismatches
	missingInProm := []string{}
	for pod := range allocPods {
		if _, exists := promPods[pod]; !exists {
			missingInProm = append(missingInProm, pod)
		}
	}
	missingInAlloc := []string{}
	for pod := range promPods {
		if _, exists := allocPods[pod]; !exists {
			missingInAlloc = append(missingInAlloc, pod)
		}
	}
	t.Logf("%s pods in allocation but not in Prometheus (window: %s): %v", controllerKind, window, missingInProm)
	t.Logf("%s pods in Prometheus but not in allocation (window: %s): %v", controllerKind, window, missingInAlloc)

	// Continue with the original validation
	validAllocPods := 0
	for podName, namespace := range allocPods {
		ns, exists := promPods[podName]
		if !exists {
			t.Errorf("%s pod %s from allocation data not found in Prometheus metrics (window: %s)", controllerKind, podName, window)
			continue
		}
		if ns != namespace {
			t.Errorf("Namespace mismatch for %s pod %s (window: %s): Prometheus=%s, Allocation=%s",
				controllerKind, podName, window, ns, namespace)
		}
		validAllocPods++
	}

	if validAllocPods == 0 {
		t.Errorf("No valid %s pods found in allocation data for window %s", controllerKind, window)
	}
	t.Logf("Found %d %s pods in Prometheus and %d valid pods in allocation data (window: %s)",
		len(promPods), controllerKind, validAllocPods, window)
}

func TestControllerAllocationConsistency(t *testing.T) {
	// Define the time windows to test
	windows := []string{"10m", "1h", "12h", "1d", "2d"}

	// Test DaemonSet pods
	t.Run("DaemonSet", func(t *testing.T) {
		for _, window := range windows {
			t.Run(window, func(t *testing.T) {
				validateControllerPods(t, "DaemonSet", window)
			})
		}
	})

	// Test ReplicaSet pods
	t.Run("ReplicaSet", func(t *testing.T) {
		for _, window := range windows {
			t.Run(window, func(t *testing.T) {
				validateControllerPods(t, "ReplicaSet", window)
			})
		}
	})

	// Test StatefulSet pods
	t.Run("StatefulSet", func(t *testing.T) {
		for _, window := range windows {
			t.Run(window, func(t *testing.T) {
				validateControllerPods(t, "StatefulSet", window)
			})
		}
	})
}

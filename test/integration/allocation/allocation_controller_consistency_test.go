package allocation

import (
	"fmt"
	"strings"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
)

func validateControllerPods(t *testing.T, controllerKind string, window string) {
	// Initialize Prometheus client
	promClient := prometheus.NewClient()

	// Get pods from Prometheus
	promPods, err := promClient.GetPodsByController(controllerKind, window)
	if err != nil {
		t.Fatalf("Failed to get pods from Prometheus: %v", err)
	}

	// Convert controllerKind to lowercase for the allocation API query
	lowerControllerKind := strings.ToLower(controllerKind)
	// For ReplicaSets, query OpenCost for Deployment types
	if controllerKind == "ReplicaSet" {
		lowerControllerKind = "deployment"
	}

	// Create API client and request
	apiClient := api.NewAPI()
	req := api.AllocationRequest{
		Window:     window,
		Aggregate:  "pod",
		Accumulate: "true",
		Filter:     fmt.Sprintf("controllerKind:\"%s\"", lowerControllerKind),
	}

	// Get allocation data
	allocResp, err := apiClient.GetAllocation(req)
	if err != nil {
		t.Fatalf("Failed to query OpenCost allocation API for %s with window %s: %v", controllerKind, window, allocResp.Code)
	}

	if allocResp.Code != 200 {
		t.Fatalf("%s allocation API returned error for window %s: code=%d", controllerKind, window, allocResp.Code)
	}
	if allocResp.Data == nil {
		t.Fatalf("%s allocation API returned nil data for window %s", controllerKind, window)
	}
	if len(allocResp.Data) == 0 {
		t.Fatalf("No data returned from %s allocation API for window %s", controllerKind, window)
	}

	// Collect pod names from allocation data
	allocPods := make(map[string]string)
	for _, podMap := range allocResp.Data {
		for podName, alloc := range podMap {
			if podName == "" || alloc.Properties == nil {
				continue
			}

			allocPods[podName] = alloc.Properties.Namespace
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

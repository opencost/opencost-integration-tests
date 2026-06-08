package autocomplete

// Prometheus-only tests validate ground-truth queries against the demo Prometheus
// instance without requiring autocomplete endpoints on OpenCost.

import (
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
)

func TestPrometheusAllocationGroundTruthQueries(t *testing.T) {
	logTestTargets(t)
	client := prometheus.NewClient()
	endTime := queryEndTime()

	fields := []string{
		"namespace",
		"pod",
		"container",
		"node",
		"cluster",
		"controllerkind",
		"controllername",
		"label",
		"namespacelabel",
	}

	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			values, err := client.AllocationFieldValues(field, defaultWindow, endTime)
			if err != nil {
				t.Fatalf("AllocationFieldValues(%q): %v", field, err)
			}
			if len(values) == 0 {
				t.Fatalf("expected non-empty prometheus ground truth for %q", field)
			}
			t.Logf("field %q: %d distinct values", field, len(values))
		})
	}
}

func TestPrometheusAssetGroundTruthQueries(t *testing.T) {
	logTestTargets(t)
	client := prometheus.NewClient()
	endTime := queryEndTime()

	fields := []string{
		"name",
		"cluster",
		"type",
		"category",
		"providerid",
		"label",
	}

	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			values, err := client.AssetFieldValues(field, defaultWindow, endTime)
			if err != nil {
				t.Fatalf("AssetFieldValues(%q): %v", field, err)
			}
			if len(values) == 0 {
				t.Fatalf("expected non-empty prometheus ground truth for %q", field)
			}
			t.Logf("field %q: %d distinct values", field, len(values))
		})
	}
}

package autocomplete

// Allocation autocomplete integration tests compare OpenCost results against Prometheus
// ground truth from kube-state-metrics on the demo environment.

import (
	"strings"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
)

func TestAllocationAutocompletePrometheusGroundTruth(t *testing.T) {
	logTestTargets(t)
	apiClient := api.NewAPI()
	requireAllocationAutocomplete(t, apiClient)

	promClient := prometheus.NewClient()
	endTime := queryEndTime()

	fields := []struct {
		name  string
		field string
		mode  compareMode
	}{
		{name: "namespace", field: "namespace", mode: compareStrict},
		{name: "pod", field: "pod", mode: compareStrict},
		{name: "container", field: "container", mode: compareGroundTruthInAPI},
		{name: "node", field: "node", mode: compareStrict},
		{name: "cluster", field: "cluster", mode: compareStrict},
		{name: "label keys", field: "label", mode: compareGroundTruthInAPI},
	}

	for _, tc := range fields {
		t.Run(tc.name, func(t *testing.T) {
			promValues, err := promClient.AllocationFieldValues(tc.field, defaultWindow, endTime)
			if err != nil {
				t.Fatalf("prometheus ground truth for %q: %v", tc.field, err)
			}
			if len(promValues) == 0 {
				t.Fatalf("prometheus returned no ground-truth values for field %q", tc.field)
			}

			resp, err := apiClient.GetAllocationAutocomplete(api.AutocompleteRequest{
				Window: defaultWindow,
				Field:  tc.field,
				Limit:  1000,
			})
			if err != nil {
				t.Fatalf("allocation autocomplete API: %v", err)
			}
			if resp.Code != 200 {
				t.Fatalf("allocation autocomplete returned code %d", resp.Code)
			}

			compareAutocompleteResults(t, tc.field, resp.Data.Data, promValues, tc.mode)
			t.Logf("field %q: api=%d prom=%d", tc.field, len(resp.Data.Data), len(promValues))
		})
	}
}

func TestAllocationAutocompleteControllerKindSmoke(t *testing.T) {
	logTestTargets(t)
	apiClient := api.NewAPI()
	requireAllocationAutocomplete(t, apiClient)

	resp, err := apiClient.GetAllocationAutocomplete(api.AutocompleteRequest{
		Window: defaultWindow,
		Field:  "controllerkind",
		Limit:  1000,
	})
	if err != nil {
		t.Fatalf("allocation autocomplete API: %v", err)
	}
	if len(resp.Data.Data) == 0 {
		t.Fatal("expected non-empty controllerkind autocomplete results")
	}

	// OpenCost reports normalized workload controller kinds (not Prometheus owner_kind).
	wantKinds := map[string]struct{}{
		"daemonset":   {},
		"deployment":  {},
		"statefulset": {},
	}
	for _, kind := range resp.Data.Data {
		if _, ok := wantKinds[strings.ToLower(kind)]; !ok {
			t.Errorf("unexpected controllerkind %q", kind)
		}
	}
	t.Logf("controllerkind autocomplete: %+v", resp.Data.Data)
}

func TestAllocationAutocompleteControllerNameSmoke(t *testing.T) {
	logTestTargets(t)
	apiClient := api.NewAPI()
	requireAllocationAutocomplete(t, apiClient)

	resp, err := apiClient.GetAllocationAutocomplete(api.AutocompleteRequest{
		Window: defaultWindow,
		Field:  "controllername",
		Limit:  1000,
	})
	if err != nil {
		t.Fatalf("allocation autocomplete API: %v", err)
	}
	if len(resp.Data.Data) == 0 {
		t.Fatal("expected non-empty controllername autocomplete results")
	}
	t.Logf("controllername autocomplete returned %d values (sample: %q)", len(resp.Data.Data), resp.Data.Data[0])
}

func TestAllocationAutocompleteLabelValue(t *testing.T) {
	logTestTargets(t)
	apiClient := api.NewAPI()
	requireAllocationAutocomplete(t, apiClient)

	promClient := prometheus.NewClient()
	endTime := queryEndTime()

	labelKeys, err := promClient.AllocationFieldValues("label", defaultWindow, endTime)
	if err != nil {
		t.Fatalf("prometheus label keys: %v", err)
	}
	var labelKey string
	for k := range labelKeys {
		labelKey = k
		break
	}
	if labelKey == "" {
		t.Fatal("no pod label keys found in prometheus")
	}

	field := "label:" + labelKey
	promValues, err := promClient.AllocationFieldValues(field, defaultWindow, endTime)
	if err != nil {
		t.Fatalf("prometheus ground truth for %q: %v", field, err)
	}
	if len(promValues) == 0 {
		t.Fatalf("prometheus returned no values for %q", field)
	}

	resp, err := apiClient.GetAllocationAutocomplete(api.AutocompleteRequest{
		Window: defaultWindow,
		Field:  field,
		Limit:  1000,
	})
	if err != nil {
		t.Fatalf("allocation autocomplete API: %v", err)
	}
	compareAutocompleteResults(t, field, resp.Data.Data, promValues, compareGroundTruthInAPI)
}

func TestAllocationAutocompleteNamespaceLabelKeys(t *testing.T) {
	logTestTargets(t)
	apiClient := api.NewAPI()
	requireAllocationAutocomplete(t, apiClient)

	promClient := prometheus.NewClient()
	endTime := queryEndTime()

	promValues, err := promClient.AllocationFieldValues("namespacelabel", defaultWindow, endTime)
	if err != nil {
		t.Fatalf("prometheus ground truth: %v", err)
	}
	if len(promValues) == 0 {
		t.Skip("no namespace labels in prometheus for demo cluster")
	}

	resp, err := apiClient.GetAllocationAutocomplete(api.AutocompleteRequest{
		Window: defaultWindow,
		Field:  "namespacelabel",
		Limit:  1000,
	})
	if err != nil {
		t.Fatalf("allocation autocomplete API: %v", err)
	}
	compareAutocompleteResults(t, "namespacelabel", resp.Data.Data, promValues, compareGroundTruthInAPI)
}

func TestAllocationAutocompleteSearchFilter(t *testing.T) {
	logTestTargets(t)
	apiClient := api.NewAPI()
	requireAllocationAutocomplete(t, apiClient)

	resp, err := apiClient.GetAllocationAutocomplete(api.AutocompleteRequest{
		Window: defaultWindow,
		Field:  "namespace",
		Search: "kube",
		Limit:  100,
	})
	if err != nil {
		t.Fatalf("allocation autocomplete API: %v", err)
	}
	for _, ns := range resp.Data.Data {
		if ns == "" {
			continue
		}
		if !strings.Contains(strings.ToLower(ns), "kube") {
			t.Errorf("search=kube returned value %q that does not match", ns)
		}
	}
}

func TestAllocationAutocompleteLimit(t *testing.T) {
	logTestTargets(t)
	apiClient := api.NewAPI()
	requireAllocationAutocomplete(t, apiClient)

	const limit = 5
	resp, err := apiClient.GetAllocationAutocomplete(api.AutocompleteRequest{
		Window: defaultWindow,
		Field:  "namespace",
		Limit:  limit,
	})
	if err != nil {
		t.Fatalf("allocation autocomplete API: %v", err)
	}
	if len(resp.Data.Data) > limit {
		t.Fatalf("expected at most %d results, got %d", limit, len(resp.Data.Data))
	}
}

package autocomplete

// Assets autocomplete integration tests compare OpenCost results against Prometheus
// or /assets API ground truth on the demo environment.

import (
	"strings"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
)

func TestAssetsAutocompletePrometheusGroundTruth(t *testing.T) {
	logTestTargets(t)
	apiClient := api.NewAPI()
	requireAssetsAutocomplete(t, apiClient)

	promClient := prometheus.NewClient()
	endTime := queryEndTime()

	fields := []struct {
		name  string
		field string
		mode  compareMode
	}{
		{name: "node name", field: "name", mode: compareGroundTruthInAPI},
		{name: "cluster", field: "cluster", mode: compareStrict},
		{name: "provider id", field: "providerid", mode: compareGroundTruthInAPI},
		{name: "label keys", field: "label", mode: compareGroundTruthInAPI},
	}

	for _, tc := range fields {
		t.Run(tc.name, func(t *testing.T) {
			promValues, err := promClient.AssetFieldValues(tc.field, defaultWindow, endTime)
			if err != nil {
				t.Fatalf("prometheus ground truth for %q: %v", tc.field, err)
			}
			if len(promValues) == 0 {
				t.Fatalf("prometheus returned no ground-truth values for field %q", tc.field)
			}

			resp, err := apiClient.GetAssetsAutocomplete(api.AutocompleteRequest{
				Window:   defaultWindow,
				Field:    tc.field,
				TenantID: "opencost",
				Limit:    1000,
			})
			if err != nil {
				t.Fatalf("assets autocomplete API: %v", err)
			}
			if resp.Code != 200 {
				t.Fatalf("assets autocomplete returned code %d", resp.Code)
			}

			compareAutocompleteResults(t, tc.field, resp.Data.Data, promValues, tc.mode)
			t.Logf("field %q: api=%d prom=%d", tc.field, len(resp.Data.Data), len(promValues))
		})
	}
}

func TestAssetsAutocompleteTypeAndCategoryGroundTruth(t *testing.T) {
	logTestTargets(t)
	apiClient := api.NewAPI()
	requireAssetsAutocomplete(t, apiClient)

	t.Run("type", func(t *testing.T) {
		expected, err := assetDistinctProperty(apiClient, "type")
		if err != nil {
			t.Fatalf("assets API ground truth: %v", err)
		}
		if len(expected) == 0 {
			t.Fatalf("no %q values from /assets", "type")
		}

		resp, err := apiClient.GetAssetsAutocomplete(api.AutocompleteRequest{
			Window:   defaultWindow,
			Field:    "type",
			TenantID: "opencost",
			Limit:    1000,
		})
		if err != nil {
			t.Fatalf("assets autocomplete API: %v", err)
		}

		for _, v := range resp.Data.Data {
			if _, ok := canonicalAssetTypes[v]; !ok {
				t.Errorf("field %q: API value %q is not a canonical asset type", "type", v)
			}
		}
		compareAutocompleteResults(t, "type", resp.Data.Data, expected, compareGroundTruthInAPI)
	})

	t.Run("category", func(t *testing.T) {
		expected, err := assetDistinctProperty(apiClient, "category")
		if err != nil {
			t.Fatalf("assets API ground truth: %v", err)
		}
		if len(expected) == 0 {
			t.Fatalf("no %q values from /assets", "category")
		}

		resp, err := apiClient.GetAssetsAutocomplete(api.AutocompleteRequest{
			Window:   defaultWindow,
			Field:    "category",
			TenantID: "opencost",
			Limit:    1000,
		})
		if err != nil {
			t.Fatalf("assets autocomplete API: %v", err)
		}
		compareAutocompleteResults(t, "category", resp.Data.Data, expected, compareStrict)
	})
}

var canonicalAssetTypes = map[string]struct{}{
	"cloud":             {},
	"clustermanagement": {},
	"disk":              {},
	"loadbalancer":      {},
	"network":           {},
	"node":              {},
	"shared":            {},
}

func assetDistinctProperty(apiClient *api.API, field string) (map[string]struct{}, error) {
	resp, err := apiClient.GetAssets(api.AssetsRequest{Window: defaultWindow})
	if err != nil {
		return nil, err
	}

	knownTypes := map[string]struct{}{
		"Node": {}, "Disk": {}, "LoadBalancer": {}, "ClusterManagement": {},
	}

	values := make(map[string]struct{})
	for key, item := range resp.Data {
		switch field {
		case "type":
			for part := range knownTypes {
				if containsAssetKeySegment(key, part) {
					values[strings.ToLower(part)] = struct{}{}
				}
			}
		case "category":
			if item.Properties != nil && item.Properties.Category != "" {
				values[item.Properties.Category] = struct{}{}
			}
		}
	}
	return values, nil
}

func containsAssetKeySegment(key, segment string) bool {
	return strings.Contains(key, "/"+segment+"/")
}

func TestAssetsAutocompleteNodeLabelValue(t *testing.T) {
	logTestTargets(t)
	apiClient := api.NewAPI()
	requireAssetsAutocomplete(t, apiClient)

	promClient := prometheus.NewClient()
	endTime := queryEndTime()

	labelKeys, err := promClient.AssetFieldValues("label", defaultWindow, endTime)
	if err != nil {
		t.Fatalf("prometheus label keys: %v", err)
	}
	var labelKey string
	for k := range labelKeys {
		labelKey = k
		break
	}
	if labelKey == "" {
		t.Skip("no node label keys in prometheus")
	}

	field := "label:" + labelKey
	promValues, err := promClient.AssetFieldValues(field, defaultWindow, endTime)
	if err != nil {
		t.Fatalf("prometheus ground truth for %q: %v", field, err)
	}
	if len(promValues) == 0 {
		t.Skipf("no prometheus values for node label %q", labelKey)
	}

	resp, err := apiClient.GetAssetsAutocomplete(api.AutocompleteRequest{
		Window:   defaultWindow,
		Field:    field,
		TenantID: "opencost",
		Limit:    1000,
	})
	if err != nil {
		t.Fatalf("assets autocomplete API: %v", err)
	}
	compareAutocompleteResults(t, field, resp.Data.Data, promValues, compareGroundTruthInAPI)
}

package autocomplete

import (
	"net/http"
	"strings"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

func TestAllocationAutocompleteValidationErrors(t *testing.T) {
	logTestTargets(t)
	apiClient := api.NewAPI()
	requireAllocationAutocomplete(t, apiClient)

	cases := []struct {
		name string
		req  api.AutocompleteRequest
	}{
		{
			name: "missing field",
			req:  api.AutocompleteRequest{Window: defaultWindow},
		},
		{
			name: "invalid field",
			req:  api.AutocompleteRequest{Window: defaultWindow, Field: "not-a-real-field"},
		},
		{
			name: "excessive limit",
			req:  api.AutocompleteRequest{Window: defaultWindow, Field: "namespace", Limit: 5000},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body, err := apiClient.GetAutocompleteStatus("/allocation/autocomplete", tc.req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if status != http.StatusBadRequest {
				t.Fatalf("expected HTTP 400, got %d: %s", status, strings.TrimSpace(string(body)))
			}
		})
	}
}

func TestAssetsAutocompleteValidationErrors(t *testing.T) {
	logTestTargets(t)
	apiClient := api.NewAPI()
	requireAssetsAutocomplete(t, apiClient)

	status, body, err := apiClient.GetAutocompleteStatus("/assets/autocomplete", api.AutocompleteRequest{
		Window: defaultWindow,
		Field:  "not-a-real-field",
	})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if status != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400, got %d: %s", status, strings.TrimSpace(string(body)))
	}
}

func TestCloudCostAutocompleteValidationErrors(t *testing.T) {
	logTestTargets(t)
	apiClient := api.NewAPI()
	requireCloudCostAutocomplete(t, apiClient)

	cases := []struct {
		name string
		req  api.AutocompleteRequest
	}{
		{
			name: "missing field",
			req:  api.AutocompleteRequest{Window: defaultWindow},
		},
		{
			name: "missing window",
			req:  api.AutocompleteRequest{Field: "service"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body, err := apiClient.GetAutocompleteStatus("/cloudCost/autocomplete", tc.req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if status != http.StatusBadRequest {
				t.Fatalf("expected HTTP 400, got %d: %s", status, strings.TrimSpace(string(body)))
			}
		})
	}
}

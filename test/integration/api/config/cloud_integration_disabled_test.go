package config

// Integration test for OpenCost deployments without cloud billing integration configured.
//
// Documented behavior when cloud billing integration is NOT configured:
//   - GET /cloudCost/status       → HTTP 200, protocol code 200, data: []
//   - GET /cloudCost              → HTTP 200, protocol code 200, empty cloudCosts in all sets
//   - GET /cloudCost/autocomplete → HTTP 200, protocol code 200, data.data: []
//
// These endpoints must never return HTTP 500 when integration is disabled.
//
// The shared CI demo environment has cloud billing enabled, so this test skips when
// /cloudCost/status reports one or more integrations. Run against a local OpenCost
// instance without cloud-integration.json to exercise the assertions:
//
//	export OPENCOST_URL='http://localhost:9003'
//	go test -v ./test/integration/api/config/cloud_integration_disabled_test.go

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/env"
)

const defaultWindow = "24h"

type protocolResponse[T any] struct {
	Code int `json:"code"`
	Data T   `json:"data"`
}

type cloudCostQueryData struct {
	Sets []struct {
		CloudCosts map[string]json.RawMessage `json:"cloudCosts"`
	} `json:"sets"`
}

type cloudCostStatusEntry struct {
	Key      string `json:"key"`
	Provider string `json:"provider"`
	Active   bool   `json:"active"`
}

func TestCloudCostWhenIntegrationDisabled(t *testing.T) {
	apiClient := api.NewAPI()
	t.Logf("OPENCOST_URL=%s", env.GetDefaultURL())

	requireCloudIntegrationDisabled(t, apiClient)

	t.Run("cloudCost query", func(t *testing.T) {
		body := getCloudCostEndpoint(t, apiClient, "/cloudCost", api.AutocompleteRequest{Window: defaultWindow})

		var resp protocolResponse[cloudCostQueryData]
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("decode /cloudCost: %v", err)
		}
		if resp.Code != http.StatusOK {
			t.Fatalf("/cloudCost protocol code %d, want 200", resp.Code)
		}
		for i, set := range resp.Data.Sets {
			if len(set.CloudCosts) > 0 {
				t.Errorf("set %d: expected empty cloudCosts, got %d entries", i, len(set.CloudCosts))
			}
		}
	})

	t.Run("cloudCost autocomplete", func(t *testing.T) {
		body := getCloudCostEndpoint(t, apiClient, "/cloudCost/autocomplete", api.AutocompleteRequest{
			Window: defaultWindow,
			Field:  "service",
			Limit:  1,
		})

		var resp protocolResponse[api.AutocompletePayload]
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("decode /cloudCost/autocomplete: %v", err)
		}
		if resp.Code != http.StatusOK {
			t.Fatalf("/cloudCost/autocomplete protocol code %d, want 200", resp.Code)
		}
		if len(resp.Data.Data) > 0 {
			t.Errorf("expected empty autocomplete data, got %v", resp.Data.Data)
		}
	})

	t.Run("cloudCost status", func(t *testing.T) {
		body := getCloudCostEndpoint(t, apiClient, "/cloudCost/status", api.AutocompleteRequest{})

		var resp protocolResponse[[]cloudCostStatusEntry]
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("decode /cloudCost/status: %v", err)
		}
		if resp.Code != http.StatusOK {
			t.Fatalf("/cloudCost/status protocol code %d, want 200", resp.Code)
		}
		if len(resp.Data) != 0 {
			t.Errorf("expected no cloud cost integrations, got %d", len(resp.Data))
		}
	})
}

func requireCloudIntegrationDisabled(t *testing.T, apiClient *api.API) {
	t.Helper()

	status, body, err := apiClient.GetAutocompleteStatus("/cloudCost/status", api.AutocompleteRequest{})
	if err != nil {
		t.Fatalf("GET /cloudCost/status: %v", err)
	}
	if status == http.StatusNotFound {
		t.Skipf(
			"/cloudCost/status is not deployed on %s (HTTP 404)",
			env.GetDefaultURL(),
		)
	}
	if status == http.StatusInternalServerError {
		t.Fatalf("/cloudCost/status returned HTTP 500: %s", strings.TrimSpace(string(body)))
	}
	if status != http.StatusOK {
		t.Fatalf("/cloudCost/status returned HTTP %d: %s", status, strings.TrimSpace(string(body)))
	}

	var resp protocolResponse[[]cloudCostStatusEntry]
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode /cloudCost/status: %v", err)
	}
	if resp.Code != http.StatusOK {
		t.Fatalf("/cloudCost/status protocol code %d, want 200", resp.Code)
	}
	if len(resp.Data) > 0 {
		t.Skipf(
			"cloud billing integration is configured on %s (%d integration(s)); need instance without cloud integration",
			env.GetDefaultURL(),
			len(resp.Data),
		)
	}
}

func getCloudCostEndpoint(t *testing.T, apiClient *api.API, path string, req api.AutocompleteRequest) []byte {
	t.Helper()

	status, body, err := apiClient.GetAutocompleteStatus(path, req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	if status == http.StatusInternalServerError {
		t.Fatalf("%s returned HTTP 500: %s", path, strings.TrimSpace(string(body)))
	}
	if status != http.StatusOK {
		t.Fatalf("%s returned HTTP %d: %s", path, status, strings.TrimSpace(string(body)))
	}
	return body
}

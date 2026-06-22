package config

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/env"
)

const defaultWindow = "24h" //test the last 24 hours of data

type protocolResponse[T any] struct {
	Code int `json:"code"` //the HTTP status code of the response
	Data T   `json:"data"` //the data of the response
}

type cloudCostQueryData struct {
	Sets []struct { //the sets of the response
		CloudCosts map[string]json.RawMessage `json:"cloudCosts"` //the cloud costs of the response
	} `json:"sets"`
}

type cloudCostStatusEntry struct {
	Key      string `json:"key"`      //the key of the response
	Provider string `json:"provider"` //the provider of the response
	Active   bool   `json:"active"`   //the active status of the response
}

func TestCloudCostWhenIntegrationDisabled(t *testing.T) {
	apiClient := api.NewAPI() //create a new API client
	t.Logf("OPENCOST_URL=%s", env.GetDefaultURL())

	requireCloudIntegrationDisabled(t, apiClient) //check if the cloud integration is disabled

	t.Run("cloudCost query", func(t *testing.T) {
		body := getCloudCostEndpoint(t, apiClient, "/cloudCost", api.AutocompleteRequest{Window: defaultWindow}) //get the cloud cost endpoint

		var resp protocolResponse[cloudCostQueryData] //decode the response
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("decode /cloudCost: %v", err) //if there is an error, fail the test
		}
		if resp.Code != http.StatusOK {
			t.Fatalf("/cloudCost protocol code %d, want 200", resp.Code) //if the response code is not 200, fail the test
		}
		for i, set := range resp.Data.Sets {
			if len(set.CloudCosts) > 0 {
				t.Errorf("set %d: expected empty cloudCosts, got %d entries", i, len(set.CloudCosts)) //if the cloud costs are not empty, fail the test
			}
		}
	})

	t.Run("cloudCost autocomplete", func(t *testing.T) {
		body := getCloudCostEndpoint(t, apiClient, "/cloudCost/autocomplete", api.AutocompleteRequest{ //get the cloud cost autocomplete endpoint
			Window: defaultWindow,
			Field:  "service",
			Limit:  1,
		}) //get the cloud cost autocomplete endpoint

		var resp protocolResponse[api.AutocompletePayload] //decode the response
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("decode /cloudCost/autocomplete: %v", err) //if there is an error, fail the test
		}
		if resp.Code != http.StatusOK {
			t.Fatalf("/cloudCost/autocomplete protocol code %d, want 200", resp.Code) //if the response code is not 200, fail the test
		}
		if len(resp.Data.Data) > 0 {
			t.Errorf("expected empty autocomplete data, got %v", resp.Data.Data) //if the autocomplete data is not empty, fail the test
		}
	})

	t.Run("cloudCost status", func(t *testing.T) {
		body := getCloudCostEndpoint(t, apiClient, "/cloudCost/status", api.AutocompleteRequest{}) //get the cloud cost status endpoint

		var resp protocolResponse[[]cloudCostStatusEntry] //decode the response
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("decode /cloudCost/status: %v", err) //if there is an error, fail the test
		}
		if resp.Code != http.StatusOK {
			t.Fatalf("/cloudCost/status protocol code %d, want 200", resp.Code) //if the response code is not 200, fail the test
		}
		if len(resp.Data) != 0 {
			t.Errorf("expected no cloud cost integrations, got %d", len(resp.Data)) //if the cloud cost integrations are not empty, fail the test
		}
	})
}

func requireCloudIntegrationDisabled(t *testing.T, apiClient *api.API) {
	t.Helper()

	status, body, err := apiClient.GetAutocompleteStatus("/cloudCost/status", api.AutocompleteRequest{}) //get the cloud cost status endpoint
	if err != nil {
		t.Fatalf("GET /cloudCost/status: %v", err) //if there is an error, fail the test
	}
	if status == http.StatusNotFound {
		t.Skipf(
			"/cloudCost/status is not deployed on %s (HTTP 404)", //if the cloud cost status endpoint is not deployed, skip the test
			env.GetDefaultURL(),
		)
	}
	if status == http.StatusInternalServerError {
		t.Fatalf("/cloudCost/status returned HTTP 500: %s", strings.TrimSpace(string(body))) //if the cloud cost status endpoint returned HTTP 500, fail the test
	}
	if status != http.StatusOK {
		t.Fatalf("/cloudCost/status returned HTTP %d: %s", status, strings.TrimSpace(string(body))) //if the cloud cost status endpoint returned HTTP not 200, fail the test
	}

	var resp protocolResponse[[]cloudCostStatusEntry] //decode the response
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode /cloudCost/status: %v", err) //if there is an error, fail the test
	}
	if resp.Code != http.StatusOK {
		t.Fatalf("/cloudCost/status protocol code %d, want 200", resp.Code) //if the response code is not 200, fail the test
	}
	if len(resp.Data) > 0 {
		t.Skipf(
			"cloud billing integration is configured on %s (%d integration(s)); need instance without cloud integration", //if the cloud billing integration is configured, skip the test
			env.GetDefaultURL(),
			len(resp.Data),
		) //if the cloud billing integration is configured, skip the test
	}
}

func getCloudCostEndpoint(t *testing.T, apiClient *api.API, path string, req api.AutocompleteRequest) []byte { //get the cloud cost endpoint
	t.Helper()

	status, body, err := apiClient.GetAutocompleteStatus(path, req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err) //if there is an error, fail the test
	}
	if status == http.StatusInternalServerError {
		t.Fatalf("%s returned HTTP 500: %s", path, strings.TrimSpace(string(body))) //if the cloud cost endpoint returned HTTP 500, fail the test
	}
	if status != http.StatusOK {
		t.Fatalf("%s returned HTTP %d: %s", path, status, strings.TrimSpace(string(body))) //if the cloud cost endpoint returned HTTP not 200, fail the test
	}
	return body
}

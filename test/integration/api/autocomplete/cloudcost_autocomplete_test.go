package autocomplete

// Cloud cost autocomplete tests compare results against the /cloudCost API because
// the demo Prometheus instance does not export cloud cost metrics.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

type cloudCostAPIResponse struct {
	Code int `json:"code"`
	Data struct {
		Sets []struct {
			CloudCosts map[string]cloudCostItem `json:"cloudCosts"`
		} `json:"sets"`
	} `json:"data"`
}

type cloudCostItem struct {
	Properties cloudCostProperties `json:"properties"`
}

type cloudCostProperties struct {
	Provider          string            `json:"provider"`
	Service           string            `json:"service"`
	AccountID         string            `json:"accountID"`
	AccountName       string            `json:"accountName"`
	InvoiceEntityID   string            `json:"invoiceEntityID"`
	InvoiceEntityName string            `json:"invoiceEntityName"`
	Category          string            `json:"category"`
	ProviderID        string            `json:"providerID"`
	Labels            map[string]string `json:"labels"`
}

func TestCloudCostAutocompleteServiceGroundTruth(t *testing.T) {
	logTestTargets(t)
	apiClient := api.NewAPI()
	requireCloudCostAutocomplete(t, apiClient)

	expected, err := cloudCostDistinctProperty(apiClient, "service")
	if err != nil {
		t.Fatalf("cloudCost API ground truth: %v", err)
	}
	if len(expected) == 0 {
		t.Skip("no cloud cost services in demo /cloudCost response")
	}

	resp, err := apiClient.GetCloudCostAutocomplete(api.AutocompleteRequest{
		Window: defaultWindow,
		Field:  "service",
		Limit:  1000,
	})
	if err != nil {
		t.Fatalf("cloudCost autocomplete API: %v", err)
	}
	compareAutocompleteResults(t, "service", resp.Data.Data, expected, compareStrict)
}

func TestCloudCostAutocompleteProviderGroundTruth(t *testing.T) {
	logTestTargets(t)
	apiClient := api.NewAPI()
	requireCloudCostAutocomplete(t, apiClient)

	expected, err := cloudCostDistinctProperty(apiClient, "provider")
	if err != nil {
		t.Fatalf("cloudCost API ground truth: %v", err)
	}
	if len(expected) == 0 {
		t.Skip("no cloud cost providers in demo /cloudCost response")
	}

	resp, err := apiClient.GetCloudCostAutocomplete(api.AutocompleteRequest{
		Window: defaultWindow,
		Field:  "provider",
		Limit:  1000,
	})
	if err != nil {
		t.Fatalf("cloudCost autocomplete API: %v", err)
	}
	compareAutocompleteResults(t, "provider", resp.Data.Data, expected, compareStrict)
}

func TestCloudCostAutocompleteLabelKeys(t *testing.T) {
	logTestTargets(t)
	apiClient := api.NewAPI()
	requireCloudCostAutocomplete(t, apiClient)

	expected, err := cloudCostLabelKeys(apiClient)
	if err != nil {
		t.Fatalf("cloudCost API label keys: %v", err)
	}
	if len(expected) == 0 {
		t.Skip("no cloud cost labels in demo /cloudCost response")
	}

	resp, err := apiClient.GetCloudCostAutocomplete(api.AutocompleteRequest{
		Window: defaultWindow,
		Field:  "label",
		Limit:  1000,
	})
	if err != nil {
		t.Fatalf("cloudCost autocomplete API: %v", err)
	}
	// Autocomplete may surface additional label keys aggregated across windows.
	compareAutocompleteResults(t, "label", resp.Data.Data, expected, compareGroundTruthInAPI)
}

func cloudCostDistinctProperty(apiClient *api.API, property string) (map[string]struct{}, error) {
	parsed, err := fetchCloudCost(apiClient)
	if err != nil {
		return nil, err
	}

	values := make(map[string]struct{})
	for _, set := range parsed.Data.Sets {
		for _, cc := range set.CloudCosts {
			v, ok := cloudCostPropertyValue(cc.Properties, property)
			if !ok || v == "" {
				continue
			}
			values[v] = struct{}{}
		}
	}
	return values, nil
}

func cloudCostLabelKeys(apiClient *api.API) (map[string]struct{}, error) {
	parsed, err := fetchCloudCost(apiClient)
	if err != nil {
		return nil, err
	}

	keys := make(map[string]struct{})
	for _, set := range parsed.Data.Sets {
		for _, cc := range set.CloudCosts {
			for k := range cc.Properties.Labels {
				keys[k] = struct{}{}
			}
		}
	}
	return keys, nil
}

func cloudCostPropertyValue(props cloudCostProperties, property string) (string, bool) {
	switch strings.ToLower(property) {
	case "service":
		return props.Service, props.Service != ""
	case "provider":
		return props.Provider, props.Provider != ""
	case "accountid":
		return props.AccountID, props.AccountID != ""
	case "accountname":
		return props.AccountName, props.AccountName != ""
	case "invoiceentityid":
		return props.InvoiceEntityID, props.InvoiceEntityID != ""
	case "invoiceentityname":
		return props.InvoiceEntityName, props.InvoiceEntityName != ""
	case "category":
		return props.Category, props.Category != ""
	case "providerid":
		return props.ProviderID, props.ProviderID != ""
	default:
		return "", false
	}
}

func fetchCloudCost(apiClient *api.API) (*cloudCostAPIResponse, error) {
	status, body, err := apiClient.GetAutocompleteStatus("/cloudCost", api.AutocompleteRequest{Window: defaultWindow})
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("/cloudCost returned HTTP %d: %s", status, strings.TrimSpace(string(body)))
	}

	parsed := &cloudCostAPIResponse{}
	if err := json.Unmarshal(body, parsed); err != nil {
		return nil, fmt.Errorf("decode /cloudCost: %w", err)
	}
	return parsed, nil
}

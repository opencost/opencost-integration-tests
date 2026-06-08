package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// AutocompleteRequest is the shared query shape for autocomplete endpoints.
type AutocompleteRequest struct {
	Window   string
	Field    string
	Search   string
	Limit    int
	Filter   string
	TenantID string
}

func (r AutocompleteRequest) QueryString() string {
	params := []string{}
	esc := url.QueryEscape

	if r.Window != "" {
		params = append(params, fmt.Sprintf("window=%s", esc(r.Window)))
	}
	if r.Field != "" {
		params = append(params, fmt.Sprintf("field=%s", esc(r.Field)))
	}
	if r.Search != "" {
		params = append(params, fmt.Sprintf("search=%s", esc(r.Search)))
	}
	if r.Limit > 0 {
		params = append(params, fmt.Sprintf("limit=%d", r.Limit))
	}
	if r.Filter != "" {
		params = append(params, fmt.Sprintf("filter=%s", esc(r.Filter)))
	}
	if r.TenantID != "" {
		params = append(params, fmt.Sprintf("tenantId=%s", esc(r.TenantID)))
	}

	if len(params) == 0 {
		return ""
	}
	return fmt.Sprintf("?%s", strings.Join(params, "&"))
}

// AutocompletePayload is the inner data object returned by autocomplete handlers.
type AutocompletePayload struct {
	Data []string `json:"data"`
}

// AutocompleteResponse is the standard OpenCost protocol response for autocomplete.
type AutocompleteResponse struct {
	Code int                 `json:"code"`
	Data AutocompletePayload `json:"data"`
}

// GetAllocationAutocomplete requests GET /allocation/autocomplete.
func (api *API) GetAllocationAutocomplete(req AutocompleteRequest) (*AutocompleteResponse, error) {
	return api.getAutocomplete("/allocation/autocomplete", req)
}

// GetAssetsAutocomplete requests GET /assets/autocomplete.
func (api *API) GetAssetsAutocomplete(req AutocompleteRequest) (*AutocompleteResponse, error) {
	return api.getAutocomplete("/assets/autocomplete", req)
}

// GetCloudCostAutocomplete requests GET /cloudCost/autocomplete.
func (api *API) GetCloudCostAutocomplete(req AutocompleteRequest) (*AutocompleteResponse, error) {
	return api.getAutocomplete("/cloudCost/autocomplete", req)
}

// GetAutocompleteStatus performs a GET and returns the HTTP status without requiring JSON.
func (api *API) GetAutocompleteStatus(path string, req AutocompleteRequest) (int, []byte, error) {
	qs := req.QueryString()
	url := api.URL(path, qs)

	httpResp, err := http.Get(url)
	if err != nil {
		return 0, nil, fmt.Errorf("error getting %s: %w", url, err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return httpResp.StatusCode, nil, fmt.Errorf("error reading %s: %w", url, err)
	}
	return httpResp.StatusCode, body, nil
}

func (api *API) getAutocomplete(path string, req AutocompleteRequest) (*AutocompleteResponse, error) {
	status, body, err := api.GetAutocompleteStatus(path, req)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("autocomplete %s returned HTTP %d: %s", path, status, strings.TrimSpace(string(body)))
	}

	resp := &AutocompleteResponse{}
	if err := json.Unmarshal(body, resp); err != nil {
		return nil, fmt.Errorf("error decoding autocomplete response: %w", err)
	}
	return resp, nil
}

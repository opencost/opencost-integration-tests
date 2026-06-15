package schema

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

var windowRequiredFields = []string{
	"start",
	"end",
}

// fetchRawEndpoint fetches and decodes a raw API response. GetAutocompleteStatus
// is currently the only API helper that exposes both the HTTP status and raw
// response body. AutocompleteRequest supplies the shared window query parameter.
func fetchRawEndpoint(t *testing.T, apiClient *api.API, path string, req api.AutocompleteRequest) map[string]any {
	t.Helper()

	status, body, err := apiClient.GetAutocompleteStatus(path, req)
	if err != nil {
		t.Fatalf("%s request failed: %v", path, err)
	}

	if status != http.StatusOK {
		t.Fatalf("%s returned HTTP %d: %s", path, status, strings.TrimSpace(string(body)))
	}

	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf(
			"%s returned non-JSON or malformed JSON: %v\nbody: %s",
			path,
			err,
			strings.TrimSpace(string(body)),
		)
	}

	return parsed
}

// requireSuccessfulResponse verifies the common successful API response fields.
func requireSuccessfulResponse(t *testing.T, context string, response map[string]any) {
	t.Helper()

	requireFields(t, context, response, []string{"code", "data"})

	code := requireNumber(t, context+" code", response["code"])
	if code != http.StatusOK {
		t.Fatalf("%s code was %.0f, expected %d", context, code, http.StatusOK)
	}
}

// requireFields verifies that all required fields are present.
func requireFields(t *testing.T, context string, obj map[string]any, fields []string) {
	t.Helper()

	for _, field := range fields {
		if _, ok := obj[field]; !ok {
			t.Errorf("%s missing required field %q", context, field)
		}
	}
}

// requireMap verifies that a value is a JSON object.
func requireMap(t *testing.T, context string, value any) map[string]any {
	t.Helper()

	obj, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s expected object, got %T", context, value)
	}

	return obj
}

// requireArray verifies that a value is a JSON array.
func requireArray(t *testing.T, context string, value any) []any {
	t.Helper()

	arr, ok := value.([]any)
	if !ok {
		t.Fatalf("%s expected array, got %T", context, value)
	}

	return arr
}

// requireString verifies that a value is a JSON string.
func requireString(t *testing.T, context string, value any) string {
	t.Helper()

	result, ok := value.(string)
	if !ok {
		t.Fatalf("%s expected string, got %T", context, value)
	}

	return result
}

// requireNumber verifies that a value is a JSON number.
func requireNumber(t *testing.T, context string, value any) float64 {
	t.Helper()

	result, ok := value.(float64)
	if !ok {
		t.Fatalf("%s expected number, got %T", context, value)
	}

	return result
}
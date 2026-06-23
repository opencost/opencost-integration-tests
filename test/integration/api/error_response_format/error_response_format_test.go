package error_response_format

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/env"
)

type apiErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func TestInvalidParamsReturnError(t *testing.T) {
	url := fmt.Sprintf("%s/allocation?window=%s", env.GetDefaultURL(), "invalid-window")

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to send request to %s: %v", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if resp.StatusCode < http.StatusBadRequest || resp.StatusCode > 499 {
		t.Fatalf(
			"expected 4xx client error for invalid params, got %d\nbody=%s",
			resp.StatusCode,
			string(body),
		)
	}

	validateErrorResponse(t, resp, body)
}

func TestAssetsInvalidParamsReturnError(t *testing.T) {
	url := fmt.Sprintf("%s/assets?window=%s", env.GetDefaultURL(), "invalid-window")

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to send request to %s: %v", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if resp.StatusCode < http.StatusBadRequest || resp.StatusCode > 499 {
		t.Fatalf(
			"expected 4xx client error for invalid params, got %d\nbody=%s",
			resp.StatusCode,
			string(body),
		)
	}

	validateErrorResponse(t, resp, body)
}

// TestPrometheusDownReturnsError is intentionally opt-in because it requires
// making Prometheus unavailable in the local Kubernetes stack. It should not run
// against shared/demo environments.

func TestPrometheusDownReturnsError(t *testing.T) {
	if os.Getenv("ENABLE_PROMETHEUS_DOWN_TEST") != "true" {
		t.Skip("set ENABLE_PROMETHEUS_DOWN_TEST=true to run Prometheus-down error response test")
	}

	baseURL := env.GetDefaultURL()
	if !isLocalURL(baseURL) {
		t.Skipf("Prometheus-down test only runs against local OpenCost URLs, got %q", baseURL)
	}

	url := fmt.Sprintf("%s/allocation?window=%s", baseURL, "1h")

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to send request to %s: %v", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if resp.StatusCode < http.StatusInternalServerError || resp.StatusCode > 599 {
		t.Fatalf(
			"expected 5xx server error for Prometheus-down condition, got %d\nbody=%s",
			resp.StatusCode,
			string(body),
		)
	}

	validateErrorResponse(t, resp, body)
}

// validateErrorResponse verifies the current supported OpenCost error contract.
// Error responses may be plain text or JSON, but they must be machine-checkable
// enough for clients and tests: non-empty, expected content type, and free of
// HTML, proxy pages, panics, or stack traces.

func validateErrorResponse(t *testing.T, resp *http.Response, body []byte) {
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	trimmedBody := strings.TrimSpace(string(body))

	if trimmedBody == "" {
		t.Fatalf("expected non-empty error response body, got empty body")
	}

	assertNoUnexpectedErrorBodyContent(t, body)

	if strings.HasPrefix(contentType, "application/json") {
		validateJSONErrorResponseShape(t, body)
		return
	}

	if strings.HasPrefix(contentType, "text/plain") {
		validateTextErrorResponseShape(t, body)
		return
	}

	t.Fatalf(
		"expected error response Content-Type application/json or text/plain, got %q\nstatus=%d\nbody=%s",
		contentType,
		resp.StatusCode,
		string(body),
	)
}

// validateJSONErrorResponseShape applies only when the endpoint returns JSON.
// Plain text error responses are validated separately.

func validateJSONErrorResponseShape(t *testing.T, body []byte) {
	var errResp apiErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf(
			"expected JSON error response, but body could not be decoded: %v\nbody=%s",
			err,
			string(body),
		)
	}

	if errResp.Code == 0 {
		t.Fatalf("expected JSON error response to include non-zero code, got %d", errResp.Code)
	}

	if strings.TrimSpace(errResp.Message) == "" {
		t.Fatalf("expected JSON error response to include non-empty message")
	}
}

func validateTextErrorResponseShape(t *testing.T, body []byte) {
	message := strings.TrimSpace(string(body))
	if message == "" {
		t.Fatalf("expected text error response to include non-empty message")
	}
}

// assertNoUnexpectedErrorBodyContent catches accidental proxy responses,
// HTML error pages, stack traces, and runtime panic output.

func assertNoUnexpectedErrorBodyContent(t *testing.T, body []byte) {
	lowerBody := strings.ToLower(string(body))

	banned := []string{
		"<html",
		"<!doctype",
		"stack trace",
		"panic",
		"traceback",
		"goroutine",
		"bad gateway",
		"service unavailable",
		"upstream",
		"nginx error",
	}

	for _, token := range banned {
		if strings.Contains(lowerBody, token) {
			t.Fatalf("unexpected response body content %q: %s", token, string(body))
		}
	}
}

// isLocalURL prevents destructive Prometheus-down tests from running against
// shared environments such as the public demo instance.

func isLocalURL(rawURL string) bool {
	return strings.HasPrefix(rawURL, "http://127.0.0.1") ||
		strings.HasPrefix(rawURL, "http://localhost") ||
		strings.HasPrefix(rawURL, "https://127.0.0.1") ||
		strings.HasPrefix(rawURL, "https://localhost")
}

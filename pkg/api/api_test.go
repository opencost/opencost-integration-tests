package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONResponseSuccess(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.WriteHeader(http.StatusOK)
	rec.WriteString(`{"code":200,"data":[]}`)

	var resp AllocationResponse
	retryable, err := decodeJSONResponse("http://example/allocation", rec.Result(), &resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if retryable {
		t.Fatal("expected success response to not be retryable")
	}
	if resp.Code != 200 {
		t.Fatalf("expected code 200, got %d", resp.Code)
	}
}

func TestDecodeJSONResponseLogsMalformedBody(t *testing.T) {
	body := "Invalid 'window' parameter: illegal window: 14d:\n{\"code\":500,\"data\":null,\"message\":\"illegal window: [nil, nil)\"}"
	rec := httptest.NewRecorder()
	rec.WriteHeader(http.StatusBadRequest)
	rec.WriteString(body)

	var resp AllocationResponse
	retryable, err := decodeJSONResponse("http://example/allocation?window=14d:", rec.Result(), &resp)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if retryable {
		t.Fatalf("expected client error response to not be retryable, got: %v", err)
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("expected HTTP status in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "response body:") {
		t.Fatalf("expected response body in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("expected json parse error, got: %v", err)
	}
}

func TestDecodeJSONResponseRetryableGatewayTimeout(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.WriteHeader(http.StatusGatewayTimeout)
	rec.WriteString("upstream request timeout")

	var resp AllocationResponse
	retryable, err := decodeJSONResponse("http://example/allocation?window=14d", rec.Result(), &resp)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !retryable {
		t.Fatalf("expected retryable gateway timeout, got: %v", err)
	}
	if !strings.Contains(err.Error(), "upstream request timeout") {
		t.Fatalf("expected response body in error, got: %v", err)
	}
}

func TestIsRetryableHTTPResponse(t *testing.T) {
	if !isRetryableHTTPResponse(http.StatusGatewayTimeout, "upstream request timeout") {
		t.Fatal("expected gateway timeout to be retryable")
	}
	if isRetryableHTTPResponse(http.StatusBadRequest, "Invalid 'window' parameter") {
		t.Fatal("expected bad request to not be retryable")
	}
}

func TestGETLogsMalformedResponseBody(t *testing.T) {
	body := bytes.NewBufferString("not json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		body.WriteTo(w)
	}))
	defer srv.Close()

	client := &API{url: srv.URL}
	var resp AllocationResponse
	err := client.GET("/allocation", AllocationRequest{Window: "14d:"}, &resp)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("expected HTTP status in error, got: %v", err)
	}
}

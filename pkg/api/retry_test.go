package api

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// flakyRoundTripper fails the first failCount requests with a connection-reset
// style error, then returns a 200. It records how many times it was called.
type flakyRoundTripper struct {
	failCount int
	calls     int
}

func (f *flakyRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	f.calls++
	if f.calls <= f.failCount {
		return nil, errors.New("read tcp 10.0.0.1:1->2:443: read: connection reset by peer")
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     make(http.Header),
	}, nil
}

func TestHTTPGetWithRetrySucceedsAfterTransientErrors(t *testing.T) {
	restore := httpGetRetryBackoff
	httpGetRetryBackoff = time.Millisecond
	defer func() { httpGetRetryBackoff = restore }()

	rt := &flakyRoundTripper{failCount: MAX_RETRIES - 1}
	client := &http.Client{Transport: rt}

	resp, err := httpGetWithRetry(client, "http://example.invalid/allocation/autocomplete")
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if rt.calls != MAX_RETRIES {
		t.Fatalf("expected %d attempts, got %d", MAX_RETRIES, rt.calls)
	}
}

func TestHTTPGetWithRetryReturnsErrorAfterMaxRetries(t *testing.T) {
	restore := httpGetRetryBackoff
	httpGetRetryBackoff = time.Millisecond
	defer func() { httpGetRetryBackoff = restore }()

	rt := &flakyRoundTripper{failCount: 100}
	client := &http.Client{Transport: rt}

	if _, err := httpGetWithRetry(client, "http://example.invalid/x"); err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	if rt.calls != MAX_RETRIES {
		t.Fatalf("expected %d attempts, got %d", MAX_RETRIES, rt.calls)
	}
}

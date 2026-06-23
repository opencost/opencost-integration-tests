package prometheus

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"
)

// flakyTransport fails the first failCount requests with failErr, then returns
// a 200 response. It records how many times it was called.
type flakyTransport struct {
	failCount int
	failErr   error
	calls     int
}

func (f *flakyTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.calls++
	if f.calls <= f.failCount {
		return nil, f.failErr
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     make(http.Header),
	}, nil
}

func connReset() error {
	return &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
}

// timeoutError satisfies net.Error and reports itself as a timeout.
type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return false }

func TestIsRetryableError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"conn reset", connReset(), true},
		{"broken pipe", &net.OpError{Op: "write", Err: syscall.EPIPE}, true},
		{"eof", io.EOF, true},
		{"unexpected eof", io.ErrUnexpectedEOF, true},
		{"timeout", timeoutError{}, true},
		{"wrapped conn reset", &url.Error{Op: "Get", URL: "x", Err: connReset()}, true},
		{"plain error", errors.New("some other failure"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryableError(tc.err); got != tc.want {
				t.Fatalf("isRetryableError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestGetRetriesTransientThenSucceeds(t *testing.T) {
	restore := promQueryRetryBackoff
	promQueryRetryBackoff = time.Millisecond
	defer func() { promQueryRetryBackoff = restore }()

	ft := &flakyTransport{failCount: 2, failErr: connReset()}
	c := &Client{httpClient: &http.Client{Transport: ft}}

	resp, err := c.get("http://prometheus.invalid/api/v1/query")
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ft.calls != 3 {
		t.Fatalf("expected 3 attempts (2 failures + 1 success), got %d", ft.calls)
	}
}

func TestGetExhaustsRetriesAndReturnsError(t *testing.T) {
	restore := promQueryRetryBackoff
	promQueryRetryBackoff = time.Millisecond
	defer func() { promQueryRetryBackoff = restore }()

	ft := &flakyTransport{failCount: 100, failErr: connReset()}
	c := &Client{httpClient: &http.Client{Transport: ft}}

	if _, err := c.get("http://prometheus.invalid/api/v1/query"); err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	if ft.calls != promQueryMaxAttempts {
		t.Fatalf("expected %d attempts, got %d", promQueryMaxAttempts, ft.calls)
	}
}

func TestGetDoesNotRetryNonTransientError(t *testing.T) {
	ft := &flakyTransport{failCount: 100, failErr: errors.New("malformed response")}
	c := &Client{httpClient: &http.Client{Transport: ft}}

	if _, err := c.get("http://prometheus.invalid/api/v1/query"); err == nil {
		t.Fatal("expected an error")
	}
	if ft.calls != 1 {
		t.Fatalf("expected a single attempt for a non-transient error, got %d", ft.calls)
	}
}

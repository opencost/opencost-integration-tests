package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/env"
	"github.com/opencost/opencost-integration-tests/pkg/log"
)

const MAX_RETRIES = 3
const defaultHTTPTimeout = 120 * time.Second

var sharedHTTPClient = &http.Client{Timeout: defaultHTTPTimeout}

type API struct {
	url string
}

func NewAPI() *API {
	return &API{
		url: strings.TrimRight(env.GetDefaultURL(), "/"),
	}
}

func NewComparisonAPI() *API {
	return &API{
		url: strings.TrimRight(env.GetComparisonURL(), "/"),
	}
}

// URL constructs a full URL from the API's base URL, the given relative URL,
// and optionally the included query string.
func (api *API) URL(relativeURL string, queryString string) string {
	url := fmt.Sprintf("%s/%s", api.url, strings.TrimLeft(relativeURL, "/"))

	if queryString != "" {
		url = fmt.Sprintf("%s?%s", url, strings.TrimLeft(queryString, "?"))
	}

	return url
}

func decodeJSONResponse(url string, httpResp *http.Response, response interface{}) (retryable bool, err error) {
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return false, fmt.Errorf("error reading response body from %s: %w", url, err)
	}

	bodyStr := strings.TrimSpace(string(body))
	if err := json.Unmarshal(body, response); err != nil {
		retryable = isRetryableHTTPResponse(httpResp.StatusCode, bodyStr)
		log.Errorf(
			"error decoding %s (HTTP %d): %v\nresponse body: %s",
			url,
			httpResp.StatusCode,
			err,
			bodyStr,
		)
		return retryable, fmt.Errorf(
			"error decoding %s (HTTP %d): %w\nresponse body: %s",
			url,
			httpResp.StatusCode,
			err,
			bodyStr,
		)
	}

	return false, nil
}

func isRetryableHTTPResponse(status int, body string) bool {
	switch status {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	lowerBody := strings.ToLower(body)
	return strings.Contains(lowerBody, "upstream request timeout") ||
		strings.Contains(lowerBody, "gateway timeout")
}

// GET submits a GET request to the given URL, with the query string from the
// given QueryStringer, and unmarshals data into the given response struct.
func (api *API) GET(relativeURL string, queryStringer QueryStringer, response interface{}) error {
	qs := ""
	if queryStringer != nil {
		qs = queryStringer.QueryString()
	}

	url := api.URL(relativeURL, qs)

	for try := 0; try < MAX_RETRIES; try++ {

		httpResp, err := sharedHTTPClient.Get(url)
		if err != nil {
			if try == MAX_RETRIES-1 {
				return fmt.Errorf("error getting %s: %w", url, err)
			}
			fmt.Printf("error getting %s: %v, retrying... (%d/%d)\n", url, err, try+1, MAX_RETRIES)
			time.Sleep(5 * time.Second)
			continue
		}
		retryable, err := decodeJSONResponse(url, httpResp, response)
		httpResp.Body.Close()
		if err != nil {
			if retryable && try < MAX_RETRIES-1 {
				fmt.Printf("%v, retrying... (%d/%d)\n", err, try+1, MAX_RETRIES)
				time.Sleep(5 * time.Second)
				continue
			}
			return err
		}
		return nil
	}

	return nil
}

// POST submits a POST request to the given URL, with the query string from the
// given QueryStringer, as well as a body, and unmarshals response data into
// the given response struct.
func (api *API) POST(relativeURL string, queryStringer QueryStringer, body io.Reader, response interface{}) error {
	qs := ""
	if queryStringer != nil {
		qs = queryStringer.QueryString()
	}

	url := api.URL(relativeURL, qs)

	httpResp, err := sharedHTTPClient.Post(url, "application/json", body)
	if err != nil {
		return fmt.Errorf("error getting %s: %w", url, err)
	}

	_, err = decodeJSONResponse(url, httpResp, response)
	httpResp.Body.Close()
	if err != nil {
		return err
	}

	return nil
}

// PUT submits a PUT request to the given URL, with the query string from the
// given QueryStringer, as well as a body, and unmarshals response data into
// the given response struct.
func (api *API) PUT(relativeURL string, queryStringer QueryStringer, body io.Reader, response interface{}) error {
	qs := ""
	if queryStringer != nil {
		qs = queryStringer.QueryString()
	}

	url := api.URL(relativeURL, qs)

	req, err := http.NewRequest(http.MethodPut, url, body)
	if err != nil {
		return fmt.Errorf("error creating PUT request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := sharedHTTPClient
	httpResp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error getting %s: %w", url, err)
	}

	_, err = decodeJSONResponse(url, httpResp, response)
	httpResp.Body.Close()
	if err != nil {
		return err
	}

	return nil
}

// DELETE submits a DELETE request to the given URL, with the query string from
// the given QueryStringer, as well as a body, and unmarshals response data
// into the given response struct.
func (api *API) DELETE(relativeURL string, queryStringer QueryStringer, response interface{}) error {
	qs := ""
	if queryStringer != nil {
		qs = queryStringer.QueryString()
	}

	url := api.URL(relativeURL, qs)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("error creating DELETE request: %w", err)
	}

	client := sharedHTTPClient
	httpResp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error getting %s: %w", url, err)
	}

	if response != nil {
		_, err = decodeJSONResponse(url, httpResp, response)
		httpResp.Body.Close()
		if err != nil {
			return err
		}
	} else {
		httpResp.Body.Close()
	}

	return nil
}

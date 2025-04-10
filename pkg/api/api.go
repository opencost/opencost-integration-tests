package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/env"
)

const MAX_RETRIES = 1

type API struct {
	url string
}

func NewAPI() *API {
	return &API{
		url: strings.TrimRight(env.GetDefaultURL(), "/"),
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

// GET submits a GET request to the given URL, with the query string from the
// given QueryStringer, and unmarshals data into the given response struct.
func (api *API) GET(relativeURL string, queryStringer QueryStringer, response interface{}) error {
	qs := ""
	if queryStringer != nil {
		qs = queryStringer.QueryString()
	}

	url := api.URL(relativeURL, qs)

	for try := 0; try < MAX_RETRIES; try++ {

		httpResp, err := http.Get(url)
		if err != nil {
			if try == MAX_RETRIES-1 {
				return fmt.Errorf("error getting %s: %w", url, err)
			}
			fmt.Printf("error getting %s: %v, retrying... (%d/%d)\n", url, err, try+1, MAX_RETRIES)
			time.Sleep(30 * time.Second)
			continue
		}
		defer httpResp.Body.Close()

		err = json.NewDecoder(httpResp.Body).Decode(response)
		if err != nil {
			if try == MAX_RETRIES-1 {
				return fmt.Errorf("error decoding %s: %w", url, err)
			}
			fmt.Printf("error decoding %s: %v, retrying... (%d/%d)\n", url, err, try+1, MAX_RETRIES)
			time.Sleep(30 * time.Second)
			continue
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

	httpResp, err := http.Post(url, "application/json", body)
	if err != nil {
		return fmt.Errorf("error getting %s: %w", url, err)
	}
	defer httpResp.Body.Close()

	err = json.NewDecoder(httpResp.Body).Decode(response)
	if err != nil {
		return fmt.Errorf("error decoding %s: %w", url, err)
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

	client := &http.Client{}
	httpResp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error getting %s: %w", url, err)
	}
	defer httpResp.Body.Close()

	err = json.NewDecoder(httpResp.Body).Decode(response)
	if err != nil {
		return fmt.Errorf("error decoding %s: %w", url, err)
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

	client := &http.Client{}
	httpResp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error getting %s: %w", url, err)
	}
	defer httpResp.Body.Close()

	if response != nil {
		err = json.NewDecoder(httpResp.Body).Decode(response)
		if err != nil {
			return fmt.Errorf("error decoding %s: %w", url, err)
		}
	}

	return nil
}

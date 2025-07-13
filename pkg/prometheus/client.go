package prometheus

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultPrometheusURL is the default URL for the Prometheus server
	DefaultPrometheusURL = "https://demo-prometheus.infra.opencost.io"
	// EnvPrometheusURL is the environment variable name for the Prometheus URL
	EnvPrometheusURL = "PROMETHEUS_URL"
)

// Client represents a Prometheus API client
type Client struct {
	baseURL    string
	httpClient *http.Client
}
type PrometheusInput struct {
	Function            []string
	Metric              string
	Filters             map[string]string
	IgnoreFilters       map[string][]string
	MetricNotEqualTo    string
	MetricEqualTo       string
	QueryWindow         string
	QueryResolution     string
	Offset              string
	AggregateBy         []string
	AggregateWindow     string
	AggregateResolution string
	Time                *int64
}

type DataPoint struct {
	Timestamp float64
	Value     float64
}

// PrometheusResponse represents the response from Prometheus API
type PrometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric struct {
				Pod       string `json:"pod"`
				Namespace string `json:"namespace"`
				Container string `json:"container"`
				// GPU Specific Fields (Optional Result)
				Device    string `json:"device`
				ModelName string `json:"modelName`
				UUID 	  string `json:UUID`
			} `json:"metric"`
			Value  DataPoint   `json:"value"`
			Values []DataPoint `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// NewClient creates a new Prometheus client
func NewClient() *Client {
	baseURL := os.Getenv(EnvPrometheusURL)
	if baseURL == "" {
		baseURL = DefaultPrometheusURL
	}

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// UnmarshalJSON implements the json.Unmarshaler interface for DataPoint.
// This method is called automatically by json.Unmarshal when it encounters a DataPoint.
func (dp *DataPoint) UnmarshalJSON(b []byte) error {
	// We expect the JSON to be an array, e.g., [1751296800, "1"]
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("DataPoint: expected JSON array for unmarshaling, got %s: %w", string(b), err)
	}

	// --- Unmarshal the first element as the Timestamp (float64) ---
	if err := json.Unmarshal(raw[0], &dp.Timestamp); err != nil {
		return fmt.Errorf("DataPoint: failed to unmarshal first element (timestamp) to float64 from %s: %w", raw[0], err)
	}

	// --- Unmarshal the second element as the Value (string, then convert to float64) ---
	if len(raw) > 1 {
		var valueStr string
		if err := json.Unmarshal(raw[1], &valueStr); err != nil {
			return fmt.Errorf("DataPoint: failed to unmarshal second element (value) to string from %s: %w", raw[1], err)
		}

		parsedValue, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			return fmt.Errorf("DataPoint: failed to parse value string '%s' to float64: %w", valueStr, err)
		}
		dp.Value = parsedValue
	} else {
		// If the value string is missing (e.g., [timestamp]), set Value to its zero value (0.0)
		// or any other default you deem appropriate.
		dp.Value = 0.0
	}

	return nil
}

// GetPodsByController queries Prometheus for pods of a specific controller type
func (c *Client) GetPodsByController(controllerKind string, window string) (map[string]string, error) {
	// For ReplicaSets, we need to query for Deployment-owned pods
	promQueryKind := controllerKind

	// Query Prometheus for pods of the specified controller type over the specified window
	promQuery := fmt.Sprintf("max_over_time(kube_pod_owner{owner_kind=\"%s\"}[%s])", promQueryKind, window)
	promURL := fmt.Sprintf("%s/api/v1/query?query=%s", c.baseURL, url.QueryEscape(promQuery))

	promResp, err := c.httpClient.Get(promURL)
	if err != nil {
		return nil, fmt.Errorf("failed to query Prometheus for %s with window %s: %v", controllerKind, window, err)
	}
	defer promResp.Body.Close()

	var promData PrometheusResponse
	if err := json.NewDecoder(promResp.Body).Decode(&promData); err != nil {
		return nil, fmt.Errorf("failed to decode Prometheus response for %s with window %s: %v", controllerKind, window, err)
	}

	promPods := make(map[string]string)
	for _, result := range promData.Data.Result {
		promPods[result.Metric.Pod] = result.Metric.Namespace
	}

	if len(promPods) == 0 {
		return nil, fmt.Errorf("no %s pods found in Prometheus metrics for window %s", controllerKind, window)
	}

	return promPods, nil
}

func (c *Client) ConstructPromQLQueryURL(promQLArgs PrometheusInput) string {

	filterParts := make([]string, 0, len(promQLArgs.Filters))
	for key, value := range promQLArgs.Filters {
		// PromQL label values should be double-quoted.
		// Using a raw string literal (backticks) for the format string is clean.
		filterPart := fmt.Sprintf(`%s="%s"`, key, value)
		filterParts = append(filterParts, filterPart)
	}
	sort.Slice(filterParts, func(i, j int) bool {
		return strings.ToLower(filterParts[i]) < strings.ToLower(filterParts[j])
	})
	filtersString := strings.Join(filterParts, ", ")

	ignoreFilterParts := []string{}
	for key, values := range promQLArgs.IgnoreFilters {
		// not equal to conditions
		for _, value := range values {
			ignoreFilterPart := fmt.Sprintf(`%s!="%s"`, key, value)
			ignoreFilterParts = append(ignoreFilterParts, ignoreFilterPart)
		}
	}
	sort.Slice(ignoreFilterParts, func(i, j int) bool {
		return strings.ToLower(ignoreFilterParts[i]) < strings.ToLower(ignoreFilterParts[j])
	})
	ignoreFiltersString := strings.Join(ignoreFilterParts, ", ")
	
	allFilters := ""
	if filtersString != "" {
		allFilters = filtersString
		if ignoreFiltersString != "" {
			allFilters = allFilters + ", " + ignoreFiltersString
		}
	}

	var finalPromQLSelector string
	if allFilters == "" {
		finalPromQLSelector = "{}" // Selects all metrics
	} else {
		finalPromQLSelector = "{" + allFilters + "}"
	}

	//promQLQuery := fmt.Sprintf("%s%s offset %s", metric, finalPromQLSelector, window)
	queryWindow := ""
	if promQLArgs.QueryWindow != "" {
		if promQLArgs.QueryResolution != "" {
			queryWindow = fmt.Sprintf("[%s:%s]", promQLArgs.QueryWindow, promQLArgs.QueryResolution)
		} else {
			queryWindow = fmt.Sprintf("[%s]", promQLArgs.QueryWindow)
		}
	}

	promQLQuery := fmt.Sprintf("%s%s%s", promQLArgs.Metric, finalPromQLSelector, queryWindow)

	if promQLArgs.MetricNotEqualTo != "" {
		promQLQuery = fmt.Sprintf("%s != %s", promQLQuery, promQLArgs.MetricNotEqualTo)
	}

	for _, fun := range promQLArgs.Function {
		promQLQuery = fmt.Sprintf("%s(%s)", fun, promQLQuery)
	}

	if len(promQLArgs.AggregateBy) != 0 {
		aggregateWindow := ""
		if promQLArgs.AggregateWindow != "" {
			if promQLArgs.AggregateResolution != "" {
				aggregateWindow = fmt.Sprintf("[%s:%s]", promQLArgs.AggregateWindow, promQLArgs.AggregateResolution)
			} else {
				aggregateWindow = fmt.Sprintf("[%s]", promQLArgs.AggregateWindow)
			}
		}
		aggregateBy := strings.Join(promQLArgs.AggregateBy, ", ")
		promQLQuery = fmt.Sprintf(`%s by (%s)%s`, promQLQuery, aggregateBy, aggregateWindow)
	}

	promURL := fmt.Sprintf("%s/api/v1/query?query=%s", c.baseURL, url.QueryEscape(promQLQuery))

	// Time should be unesca[ed]
	if promQLArgs.Time != nil {
		promURL = fmt.Sprintf("%s&time=%d", promURL, *promQLArgs.Time)
	}

	return promURL
}

func (c *Client) RunPromQLQuery(promQLArgs PrometheusInput) (PrometheusResponse, error) {

	promURL := c.ConstructPromQLQueryURL(promQLArgs)
	promResp, err := c.httpClient.Get(promURL)

	var promData PrometheusResponse

	if err != nil {
		return promData, fmt.Errorf("failed to query Prometheus: %v", err)
	}
	defer promResp.Body.Close()

	if err := json.NewDecoder(promResp.Body).Decode(&promData); err != nil {
		return promData, fmt.Errorf("failed to query Prometheus: %v", err)
	}

	return promData, nil
}

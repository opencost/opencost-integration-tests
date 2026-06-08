package autocomplete

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/env"
)

const defaultWindow = "24h"

// minimumPromCoverage is the fraction of ground-truth values that must appear in API results.
const minimumPromCoverage = 0.85

// minCoverageForSet returns the required ground-truth coverage for a set of the given size.
// Small sets allow at most one missing value to avoid flaky failures on demo drift.
func minCoverageForSet(size int) float64 {
	if size <= 1 {
		return 1.0
	}
	if size <= 10 {
		return float64(size-1) / float64(size)
	}
	return minimumPromCoverage
}

type compareMode int

const (
	// compareStrict requires bidirectional overlap: API values must exist in ground truth
	// (except allowlisted OpenCost-only values) and ground truth must be mostly present in API.
	compareStrict compareMode = iota
	// compareGroundTruthInAPI only checks that ground-truth values appear in API results.
	// OpenCost may return additional enriched values (labels, controllers, asset types, etc.).
	compareGroundTruthInAPI
	// compareGroundTruthInAPINormalized lowercases values before comparing.
	compareGroundTruthInAPINormalized
)

func queryEndTime() int64 {
	return time.Now().UTC().Unix()
}

func requireAllocationAutocomplete(t *testing.T, apiClient *api.API) {
	t.Helper()
	requireAutocompleteEndpoint(t, apiClient, "/allocation/autocomplete", api.AutocompleteRequest{
		Window: defaultWindow,
		Field:  "namespace",
		Limit:  1,
	})
}

func requireAssetsAutocomplete(t *testing.T, apiClient *api.API) {
	t.Helper()
	requireAutocompleteEndpoint(t, apiClient, "/assets/autocomplete", api.AutocompleteRequest{
		Window:   defaultWindow,
		Field:    "name",
		TenantID: "opencost",
		Limit:    1,
	})
}

func requireCloudCostAutocomplete(t *testing.T, apiClient *api.API) {
	t.Helper()
	requireAutocompleteEndpoint(t, apiClient, "/cloudCost/autocomplete", api.AutocompleteRequest{
		Window: defaultWindow,
		Field:  "service",
		Limit:  1,
	})
}

func requireAutocompleteEndpoint(t *testing.T, apiClient *api.API, path string, req api.AutocompleteRequest) {
	t.Helper()
	status, body, err := apiClient.GetAutocompleteStatus(path, req)
	if err != nil {
		t.Fatalf("failed to probe %s: %v", path, err)
	}
	if status == http.StatusNotFound {
		t.Skipf(
			"autocomplete endpoint %s is not deployed on %s (HTTP 404); deploy OpenCost with autocomplete support",
			path,
			env.GetDefaultURL(),
		)
	}
	if status != http.StatusOK {
		t.Fatalf("unexpected HTTP %d probing %s: %s", status, path, strings.TrimSpace(string(body)))
	}
}

func toSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			set[v] = struct{}{}
		}
	}
	return set
}

func isAllocationAPIOnlyValue(field, value string) bool {
	switch field {
	case "pod":
		return value == "prometheus-system-unmounted-pvcs" ||
			value == "network-load-gen-unmounted-pvcs"
	case "container":
		return value == "__unmounted__"
	}
	return false
}

func assertAPISubsetOfGroundTruth(t *testing.T, field string, apiValues []string, groundTruth map[string]struct{}, allowAPIOnly func(string) bool) {
	t.Helper()
	for _, v := range apiValues {
		if v == "" {
			continue
		}
		if allowAPIOnly != nil && allowAPIOnly(v) {
			continue
		}
		if _, ok := groundTruth[v]; !ok {
			t.Errorf("field %q: API value %q not found in ground truth (%d values)", field, v, len(groundTruth))
		}
	}
}

func assertGroundTruthCoverageInAPI(t *testing.T, field string, apiValues []string, groundTruth map[string]struct{}, normalize func(string) string) {
	t.Helper()
	if len(groundTruth) == 0 {
		t.Logf("field %q: no ground-truth values to compare", field)
		return
	}

	apiSet := toSet(apiValues)
	if normalize != nil {
		normalized := make(map[string]struct{}, len(apiSet))
		for v := range apiSet {
			normalized[normalize(v)] = struct{}{}
		}
		apiSet = normalized
	}

	missing := 0
	for v := range groundTruth {
		key := v
		if normalize != nil {
			key = normalize(v)
		}
		if _, ok := apiSet[key]; !ok {
			missing++
		}
	}
	coverage := 1.0 - float64(missing)/float64(len(groundTruth))
	requiredCoverage := minCoverageForSet(len(groundTruth))
	if coverage < requiredCoverage {
		t.Errorf(
			"field %q: ground-truth coverage in API results is %.1f%% (missing %d/%d); want >= %.0f%%",
			field,
			coverage*100,
			missing,
			len(groundTruth),
			requiredCoverage*100,
		)
	}
}

func compareAutocompleteResults(t *testing.T, field string, apiValues []string, groundTruth map[string]struct{}, mode compareMode) {
	t.Helper()
	switch mode {
	case compareStrict:
		assertAPISubsetOfGroundTruth(t, field, apiValues, groundTruth, func(v string) bool {
			return isAllocationAPIOnlyValue(field, v)
		})
		assertGroundTruthCoverageInAPI(t, field, apiValues, groundTruth, nil)
	case compareGroundTruthInAPI:
		assertGroundTruthCoverageInAPI(t, field, apiValues, groundTruth, nil)
	case compareGroundTruthInAPINormalized:
		assertGroundTruthCoverageInAPI(t, field, apiValues, groundTruth, strings.ToLower)
	}
}

func logTestTargets(t *testing.T) {
	t.Helper()
	t.Logf("OPENCOST_URL=%s", env.GetDefaultURL())
	if u := os.Getenv("PROMETHEUS_URL"); u != "" {
		t.Logf("PROMETHEUS_URL=%s", u)
	} else {
		t.Logf("PROMETHEUS_URL=%s (default)", "https://demo-prometheus.infra.opencost.io")
	}
}

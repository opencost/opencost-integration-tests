package preflight

// Description - Precondition guard. Verifies the OpenCost API target and the
// Prometheus target observe the SAME cluster before the rest of the suite runs.
// Refuses the silent demo/localhost fallbacks (fails fast if either URL env var
// is unset), then compares cluster-wide running-pod sets from both sources over a
// shared, hour-aligned historical window. Coarse Jaccard overlap (~0.50): same
// cluster is ~0.90+, different cluster is ~0.0, so churn can't cross the boundary.
// Shares window-alignment/churn-tolerance machinery with the #94 pod-count test.

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
)

const (
	// envPrometheusURL / envOpenCostURL are read RAW (not via prometheus.NewClient
	// or env.GetDefaultURL) precisely because those helpers silently substitute the
	// demo / localhost fallback this guard exists to refuse.
	envPrometheusURL = "PROMETHEUS_URL"
	envOpenCostURL   = "OPENCOST_URL"

	// coherenceThreshold is the minimum cluster-wide Jaccard overlap
	// (intersection/union) of the running-pod-name sets required to pass.
	// Calibration: same-cluster overlap sits ~0.90+ (churn only nibbles the window
	// edge; a live demo check measured 253 vs 254 pods, overlap ~0.99); a
	// wrong-target comparison sits ~0.0. 0.50 sits in the empty middle so neither
	// churn nor a stray scrape can push a mismatched pair over, nor a noisy but
	// correct pair under.
	coherenceThreshold = 0.50

	// primaryWindow / primaryWindowStr are the historical, completed window both
	// sources sample. The duration drives the absolute API window; the string is
	// the PromQL range literal (kept as a literal to avoid time.Duration.String()
	// emitting "24h0m0s", which PromQL rejects).
	primaryWindow    = 24 * time.Hour
	primaryWindowStr = "24h"

	// freshWindow / freshWindowStr are the short recent fallback for clusters with
	// no scrape history over the primary window (fresh installs). Not hour-aligned.
	freshWindow    = 10 * time.Minute
	freshWindowStr = "10m"

	// maxResampleAttempts / resampleBackoff tolerate a pod churning at the window
	// edge, mirroring the #94 pod-count test. Repeated failures on fresh anchors
	// indicate a genuinely incoherent pair, not churn.
	maxResampleAttempts = 3
	resampleBackoff     = 3 * time.Second

	// samplePods bounds how many example pods we print on failure per side.
	samplePods = 5
)

func TestClusterCoherence(t *testing.T) {
	// --- Step 1: fail fast on the silent fallback -------------------------------
	promURL := os.Getenv(envPrometheusURL)
	ocURL := os.Getenv(envOpenCostURL)
	if promURL == "" {
		t.Fatalf("%s is unset — refusing the silent demo-Prometheus fallback. "+
			"Set %s so this guard compares the same cluster OpenCost queries.",
			envPrometheusURL, envPrometheusURL)
	}
	if ocURL == "" {
		t.Fatalf("%s is unset — refusing the silent localhost OpenCost fallback. "+
			"Set %s so this guard compares the same cluster Prometheus scrapes.",
			envOpenCostURL, envOpenCostURL)
	}
	t.Logf("coherence targets: opencost=%s prometheus=%s", ocURL, promURL)

	apiObj := api.NewAPI()
	client := prometheus.NewClient()

	// --- Step 2: primary historical window, with churn resampling ---------------
	// Each attempt re-anchors so both sources sample one clock (see #94).
	for attempt := 1; attempt <= maxResampleAttempts; attempt++ {
		if attempt > 1 {
			time.Sleep(resampleBackoff)
		}

		endTime := time.Now().UTC().Truncate(time.Hour).Unix()
		startTime := endTime - int64(primaryWindow.Seconds())
		t.Logf("attempt %d/%d window=[%d,%d]", attempt, maxResampleAttempts, startTime, endTime)

		apiPods, err := fetchAllocationPods(apiObj, startTime, endTime)
		if err != nil {
			t.Fatalf("allocation query failed: %v", err)
		}
		promPods, err := fetchPrometheusPods(client, primaryWindowStr, endTime, t)
		if err != nil {
			t.Fatalf("prometheus query failed: %v", err)
		}

		// --- Step 3: fresh-cluster handling -------------------------------------
		// No scrape history over 24h: don't declare incoherence off empty sets.
		// Fall back to a short recent window; the helper decides (skip / pass /
		// fatal) and the guard is done either way.
		if len(apiPods) == 0 && len(promPods) == 0 {
			t.Log("both sources empty over primary window; retrying short recent window")
			checkFreshWindow(t, apiObj, client, ocURL, promURL)
			return
		}

		// --- Step 4: cluster-wide overlap ---------------------------------------
		ratio, inter, union := jaccard(apiPods, promPods)
		t.Logf("overlap=%.3f api=%d prom=%d intersection=%d union=%d",
			ratio, len(apiPods), len(promPods), inter, union)

		if ratio >= coherenceThreshold {
			return // coherent — guard passes
		}
		// One-empty / low-overlap that survives every resample is a real mismatch.
		if attempt == maxResampleAttempts {
			failIncoherent(t, ocURL, promURL, ratio, apiPods, promPods)
		}
	}
}

// fetchAllocationPods queries /allocation aggregated by pod over the absolute
// [start,end] window and returns the set of real running pod names. Filters the
// synthetic idle entry and "<ns>-unmounted-pvcs" pods, which never exist in
// Prometheus.
func fetchAllocationPods(apiObj *api.API, start, end int64) (map[string]struct{}, error) {
	resp, err := apiObj.GetAllocation(api.AllocationRequest{
		Window:     fmt.Sprintf("%d,%d", start, end),
		Aggregate:  "pod",
		Accumulate: "false",
	})
	if err != nil {
		return nil, err
	}
	if resp.Code != 200 {
		return nil, fmt.Errorf("/allocation returned code %d", resp.Code)
	}

	pods := make(map[string]struct{})
	for _, step := range resp.Data {
		for name, item := range step {
			if isSyntheticPod(name) { // "*-unmounted-pvcs", "__idle__"
				continue
			}
			if item.Properties == nil || item.Properties.Pod == "" {
				continue
			}
			pods[item.Properties.Pod] = struct{}{}
		}
	}
	return pods, nil
}

// fetchPrometheusPods queries kube_pod_container_status_running over the window
// ending at endTime and returns the set of pods that were up (Value != 0) for at
// least part of the window. Mirrors the #94 PrometheusInput shape.
func fetchPrometheusPods(client *prometheus.Client, windowStr string, endTime int64, t *testing.T) (map[string]struct{}, error) {
	in := prometheus.PrometheusInput{
		Metric:      "kube_pod_container_status_running",
		Function:    []string{"avg_over_time", "avg"},
		QueryWindow: windowStr,
		AggregateBy: []string{"pod", "namespace"},
		Time:        &endTime,
	}
	resp, err := client.RunPromQLQuery(in, t)
	if err != nil {
		return nil, err
	}
	pods := make(map[string]struct{})
	for _, r := range resp.Data.Result {
		if r.Value.Value == 0 { // pod down across the whole window
			continue
		}
		if r.Metric.Pod == "" {
			continue
		}
		pods[r.Metric.Pod] = struct{}{}
	}
	return pods, nil
}

// checkFreshWindow re-runs the comparison over a short recent window for fresh
// clusters. Both sources still empty -> nothing to compare, skip. Exactly one side
// populated -> real incoherence, fatal. Both populated -> assert overlap.
func checkFreshWindow(t *testing.T, apiObj *api.API, client *prometheus.Client, ocURL, promURL string) {
	end := time.Now().UTC().Unix()
	start := end - int64(freshWindow.Seconds())

	apiPods, err := fetchAllocationPods(apiObj, start, end)
	if err != nil {
		t.Fatalf("fresh-window allocation query failed: %v", err)
	}
	promPods, err := fetchPrometheusPods(client, freshWindowStr, end, t)
	if err != nil {
		t.Fatalf("fresh-window prometheus query failed: %v", err)
	}

	if len(apiPods) == 0 && len(promPods) == 0 {
		t.Skip("no pod data in either source over primary or recent window; " +
			"fresh cluster, coherence not yet assessable")
		return
	}
	ratio, _, _ := jaccard(apiPods, promPods)
	t.Logf("fresh-window overlap=%.3f api=%d prom=%d", ratio, len(apiPods), len(promPods))
	if ratio < coherenceThreshold {
		failIncoherent(t, ocURL, promURL, ratio, apiPods, promPods)
	}
}

// --- shared helpers (kept inline this PR; candidate for extraction later) -------

// isSyntheticPod reports names that exist only in OpenCost output, never in the
// Prometheus running-pod metric, and must be excluded before comparing sets.
func isSyntheticPod(name string) bool {
	if strings.HasSuffix(name, "-unmounted-pvcs") {
		return true
	}
	// Idle is not emitted under aggregate=pod on the demo target, but filter it
	// defensively so a future idle-bearing response can't skew the overlap.
	return name == "__idle__"
}

// jaccard returns intersection/union of two name sets plus the raw counts.
// union==0 yields ratio 1.0 (both empty -> trivially coherent; callers handle the
// empty case earlier for the fresh-cluster path).
func jaccard(a, b map[string]struct{}) (ratio float64, intersection, union int) {
	for name := range a {
		if _, ok := b[name]; ok {
			intersection++
		}
	}
	union = len(a) + len(b) - intersection
	if union == 0 {
		return 1.0, 0, 0
	}
	return float64(intersection) / float64(union), intersection, union
}

// failIncoherent emits the diagnostic the ticket requires: both URLs, the overlap,
// and a bounded sample of pods unique to each side, then aborts the suite.
func failIncoherent(t *testing.T, ocURL, promURL string, ratio float64, apiPods, promPods map[string]struct{}) {
	apiOnly := sampleDiff(apiPods, promPods, samplePods)
	promOnly := sampleDiff(promPods, apiPods, samplePods)
	t.Fatalf("INCOHERENT ENVIRONMENT: overlap %.3f < %.2f — OpenCost and Prometheus "+
		"appear to observe different clusters; all downstream ground-truth results "+
		"are meaningless.\n  opencost   = %s\n  prometheus = %s\n  api-only pods (sample):  %v\n  prom-only pods (sample): %v",
		ratio, coherenceThreshold, ocURL, promURL, apiOnly, promOnly)
}

// sampleDiff returns up to n names present in a but not b, sorted for stable output.
func sampleDiff(a, b map[string]struct{}, n int) []string {
	var out []string
	for name := range a {
		if _, ok := b[name]; !ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	if len(out) > n {
		out = out[:n]
	}
	return out
}

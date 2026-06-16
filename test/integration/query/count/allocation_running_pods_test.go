package count

// Description - Verifies per-namespace pod sets from the Allocation API and Prometheus
// agree over a shared, absolute 24h window. Compares via Jaccard overlap (exact-match
// below a size floor), iterates the union of namespaces so single-source namespaces
// fail, filters synthetic "<ns>-unmounted-pvcs" pods, and re-anchors the window on
// transient divergence; on exhaustion, replays failures prefixed with the last window.


import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
)
const (
	// overlapThreshold is the minimum Jaccard overlap (intersection/union) of the
	// per-namespace pod sets required to pass, for namespaces large enough for the
	// ratio to be meaningful (see minNamespacePods). Calibration: against the demo
	// cluster's churning multi-tenant environment, settled-window overlap was 1.000
	// across all namespaces >= minNamespacePods over N runs; 0.80 leaves margin for
	// a pod straddling the window edge while staying far above a wrong-target ~0.
	overlapThreshold = 0.80

	// minNamespacePods is the size floor below which the overlap ratio is not
	// meaningful: in a 1-2 pod namespace a single churned pod swings the ratio to 0,
	// so the ratio cannot distinguish churn from breakage. Such namespaces are
	// checked for exact set equality instead.
	minNamespacePods = 3

	// maxResampleAttempts bounds how many times we re-anchor the window. A single
	// retry handles an attempt that straddles a churn event; repeated failures on
	// freshly-anchored windows indicate real divergence.
	maxResampleAttempts = 3

	// resampleBackoff gives any in-flight scrape/ingest cycle time to settle before
	// the next anchor.
	resampleBackoff = 3 * time.Second
)

func TestQueryAllocation(t *testing.T) {
	apiObj := api.NewAPI()

	testCases := []struct {
		name       string
		window     string
		aggregate  string
		accumulate string
	}{
		{
			name:       "Yesterday",
			window:     "24h",
			aggregate:  "pod",
			accumulate: "false",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var failures []string
			var startTime, endTime int64

			for attempt := 1; attempt <= maxResampleAttempts; attempt++ {
				if attempt > 1 {
					time.Sleep(resampleBackoff)
				}

				// Fresh anchor every attempt so both sources sample one clock.
				endTime = time.Now().UTC().Truncate(time.Hour).Unix()
				startTime = endTime - 86400

				t.Logf("attempt %d/%d API window=%s",
					attempt, maxResampleAttempts, fmt.Sprintf("%d,%d", startTime, endTime))

				// Absolute "start,end" window — the relative "24h" form drifts to
				// the request-processing instant.
				apiResponse, err := apiObj.GetAllocation(api.AllocationRequest{
					Window:     fmt.Sprintf("%d,%d", startTime, endTime),
					Aggregate:  tc.aggregate,
					Accumulate: tc.accumulate,
				})
				if err != nil {
					t.Fatalf("Error while calling Allocation API %v", err)
				}
				if apiResponse.Code != 200 {
					t.Fatalf("API returned non-200 code")
				}

				client := prometheus.NewClient()
				promInput := prometheus.PrometheusInput{
					Metric:      "kube_pod_container_status_running",
					Function:    []string{"avg_over_time", "avg"},
					QueryWindow: tc.window,
					AggregateBy: []string{"container", "pod", "namespace"},
					Time:        &endTime,
				}

				promResponse, err := client.RunPromQLQuery(promInput, t)
				if err != nil {
					t.Fatalf("Error while calling Prometheus API %v", err)
				}

				type podAggregation struct {
					Pods []string
				}
				var apiAggregateCount = make(map[string]*podAggregation)

				for pod, allocationResponeItem := range apiResponse.Data[0] {
					// Synthetic per-namespace "<ns>-unmounted-pvcs" pods never exist
					// in Prometheus.
					if strings.HasSuffix(pod, "-unmounted-pvcs") {
						continue
					}
					if allocationResponeItem.Properties == nil || allocationResponeItem.Properties.Pod == "" {
						continue
					}
					podNamespace := allocationResponeItem.Properties.Namespace
					apiAggregateItem, namespacePresent := apiAggregateCount[podNamespace]
					if !namespacePresent {
						apiAggregateCount[podNamespace] = &podAggregation{
							Pods: []string{pod},
						}
						continue
					}
					if !slices.Contains(apiAggregateItem.Pods, pod) {
						apiAggregateItem.Pods = append(apiAggregateItem.Pods, pod)
					}
				}

				var promAggregateCount = make(map[string]*podAggregation)

				for _, metric := range promResponse.Data.Result {
					podNamespace := metric.Metric.Namespace
					pod := metric.Metric.Pod

					// Pod was down across the window — skip it.
					if metric.Value.Value == 0 {
						continue
					}

					promAggregateItem, namespacePresent := promAggregateCount[podNamespace]
					if !namespacePresent {
						promAggregateCount[podNamespace] = &podAggregation{
							Pods: []string{pod},
						}
						continue
					}

					if !slices.Contains(promAggregateItem.Pods, pod) {
						promAggregateItem.Pods = append(promAggregateItem.Pods, pod)
					}
				}

				// Iterate the union so a namespace present in only one source is
				// reported. Findings are buffered (not t.Errorf'd) so the loop can
				// re-sample on transient divergence; only the last attempt's
				// findings are replayed if all attempts are exhausted.
				failures = failures[:0]
				namespaces := make(map[string]bool)
				for ns := range apiAggregateCount {
					namespaces[ns] = true
				}
				for ns := range promAggregateCount {
					namespaces[ns] = true
				}

				for namespace := range namespaces {
					apiAgg, apiPresent := apiAggregateCount[namespace]
					promAgg, promPresent := promAggregateCount[namespace]
					if !apiPresent || !promPresent {
						failures = append(failures, fmt.Sprintf(
							"[Fail] ns=%s missing from one source (api=%t prom=%t)",
							namespace, apiPresent, promPresent))
						continue
					}
					intersection := 0
					for _, p := range apiAgg.Pods {
						if slices.Contains(promAgg.Pods, p) {
							intersection++
						}
					}
					union := len(apiAgg.Pods) + len(promAgg.Pods) - intersection
					// Defensive: presence guard plus seeded namespace key guarantee
					// union >= 1; kept so a future refactor can't divide by zero.
					ratio := 1.0
					if union > 0 {
						ratio = float64(intersection) / float64(union)
					}

					t.Logf("ns=%s ratio=%.3f api=%d prom=%d intersection=%d union=%d",
						namespace, ratio, len(apiAgg.Pods), len(promAgg.Pods), intersection, union)

					// && (not ||) so a size disagreement between sources falls
					// through to the ratio check and is caught.
					if len(apiAgg.Pods) < minNamespacePods && len(promAgg.Pods) < minNamespacePods {
						if intersection != len(apiAgg.Pods) || intersection != len(promAgg.Pods) {
							failures = append(failures, fmt.Sprintf(
								"[Fail] ns=%s (small) exact-match failed: api=%v prom=%v",
								namespace, apiAgg.Pods, promAgg.Pods))
						}
						continue
					}

					if ratio < overlapThreshold {
						var apiOnly, promOnly []string
						for _, p := range apiAgg.Pods {
							if !slices.Contains(promAgg.Pods, p) {
								apiOnly = append(apiOnly, p)
							}
						}
						for _, p := range promAgg.Pods {
							if !slices.Contains(apiAgg.Pods, p) {
								promOnly = append(promOnly, p)
							}
						}
						failures = append(failures, fmt.Sprintf(
							"[Fail] ns=%s overlap %.3f < %.2f (api=%d prom=%d intersection=%d union=%d)\n  api-only: %v\n  prom-only: %v",
							namespace, ratio, overlapThreshold,
							len(apiAgg.Pods), len(promAgg.Pods), intersection, union,
							apiOnly, promOnly))
					}
				}

				if len(failures) == 0 {
					return
				}
			}

			t.Errorf("all %d attempts failed; last window unix=[%d,%d] utc=[%s, %s]",
				maxResampleAttempts, startTime, endTime,
				time.Unix(startTime, 0).UTC().Format(time.RFC3339),
				time.Unix(endTime, 0).UTC().Format(time.RFC3339))
			for _, f := range failures {
				t.Errorf("%s", f)
			}
		})
	}
}

package prometheus

// Description - Checks for Pod Restarts

import (
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"testing"
	"time"
)

const Resolution = "1m"
const Tolerance = 0.07
const NegligibleUsage = 0.01

func TestNoPodRestart(t *testing.T) {

	testCases := []struct {
		name       string
		window     string
		aggregate  string
		accumulate string
	}{
		{
			name:       "Yesterday",
			window:     "24h",
			aggregate:  "namespace",
			accumulate: "false",
		},
		{
			name:       "Last 7 days",
			window:     "168h",
			aggregate:  "namespace",
			accumulate: "false",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// Use this information to find start and end time of pod
			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)

			endTime := queryEnd.Unix()

			client := prometheus.NewClient()
			// Query all running pod information
			// avg(kube_pod_container_status_running{} != 0)
			// by
			// ("uid", pod, namespace)[24h:5m]

			// Q) != 0 is not necessary I suppose?
			promPodInfoInput := prometheus.PrometheusInput{}

			promPodInfoInput.Metric = "kube_pod_container_status_running"
			promPodInfoInput.MetricNotEqualTo = "0"
			promPodInfoInput.AggregateBy = []string{"uid", "pod", "namespace"}
			promPodInfoInput.Function = []string{"avg"}
			promPodInfoInput.AggregateWindow = tc.window
			promPodInfoInput.AggregateResolution = Resolution
			promPodInfoInput.Time = &endTime

			podInfo, err := client.RunPromQLQuery(promPodInfoInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			type PodKey struct {
				Namespace string
				Pod       string
			}

			// Number of Pod Duplicates (includes replicas and restarts)
			podMap := make(map[PodKey]int)

			for _, podInfoResponseItem := range podInfo.Data.Result {
				pod := podInfoResponseItem.Metric.Pod
				namespace := podInfoResponseItem.Metric.Namespace
				uid := podInfoResponseItem.Metric.UID

				if uid == "" {
					continue
				}

				podKey := PodKey{
					Namespace: namespace,
					Pod: pod,
				}

				_, ok := podMap[podKey]
				t.Logf("%v", podKey)
				if !ok {
					podMap[podKey] = 1
				} else {
					podMap[podKey] += 1
				}
			}

			// Windows are not accurate for prometheus and allocation
			for pod, count := range podMap {
				if count > 1 {
					t.Errorf("[Fail] %v: Pod Restarted. %v Duplicates Found.", pod, count)
				} else {
					t.Logf("[Pass] %v", pod)
				}
			}
		})
	}
}

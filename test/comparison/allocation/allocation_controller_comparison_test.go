package allocation

import (
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

// Each pod must report the same controller from the Prometheus and the collector data sources.
func TestAllocationControllerComparison(t *testing.T) {
	req := api.AllocationRequest{Window: "10m", Aggregate: "pod", Accumulate: "true"}

	promResp, err := api.NewAPI().GetAllocation(req)
	if err != nil {
		t.Fatalf("prometheus allocation: %v", err)
	}
	collectorResp, err := api.NewComparisonAPI().GetAllocation(req)
	if err != nil {
		t.Fatalf("collector allocation: %v", err)
	}
	if len(promResp.Data) == 0 || len(collectorResp.Data) == 0 {
		t.Fatalf("empty allocation response: prometheus=%d sets, collector=%d sets", len(promResp.Data), len(collectorResp.Data))
	}

	compared := map[string]int{} // controllerKind -> pods compared
	for pod, promAlloc := range promResp.Data[0] {
		collectorAlloc, ok := collectorResp.Data[0][pod]
		// Skip pods that only one source has seen so far
		if !ok || promAlloc.Properties == nil || collectorAlloc.Properties == nil {
			continue
		}

		promController := promAlloc.Properties.ControllerKind + "/" + promAlloc.Properties.Controller
		collectorController := collectorAlloc.Properties.ControllerKind + "/" + collectorAlloc.Properties.Controller
		if promController != collectorController {
			t.Errorf("pod %s: prometheus controller %q, collector controller %q", pod, promController, collectorController)
		}
		compared[promAlloc.Properties.ControllerKind]++
	}

	t.Logf("pods compared by controllerKind: %v", compared)
	if compared["statefulset"] == 0 {
		t.Errorf("no StatefulSet pods compared, so this test proved nothing")
	}
}

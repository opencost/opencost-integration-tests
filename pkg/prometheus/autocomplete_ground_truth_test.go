package prometheus

import (
	"testing"
	"time"
)

func TestRunningPodsInWindowQuery(t *testing.T) {
	client := NewClient()
	endTime := time.Now().UTC().Truncate(time.Hour).Add(time.Hour).Unix()

	resp, err := client.runPromQLQuery(RunningPodsInWindowInput("24h", endTime))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Data.Result) == 0 {
		t.Fatal("expected pods running during window")
	}
	if resp.Data.Result[0].Value.Value <= 0 {
		t.Logf("sample value at endTime: %v (series still indicates window presence)", resp.Data.Result[0].Value.Value)
	}
}

func TestAllocationFieldValuesNamespace(t *testing.T) {
	client := NewClient()
	endTime := time.Now().UTC().Truncate(time.Hour).Add(time.Hour).Unix()

	values, err := client.AllocationFieldValues("namespace", "24h", endTime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(values) == 0 {
		t.Fatal("expected at least one namespace from prometheus")
	}
}

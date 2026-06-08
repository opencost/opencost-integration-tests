package prometheus

import (
	"testing"
	"time"
)

func TestRunningPodKeysAliveAtTime(t *testing.T) {
	client := NewClient()
	endTime := time.Now().UTC().Truncate(time.Hour).Add(time.Hour).Unix()

	keys, err := client.RunningPodKeysAliveAtTime("24h", endTime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("expected running pods at endTime")
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

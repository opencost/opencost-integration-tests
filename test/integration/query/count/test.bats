setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "Query: Verify Number of Pods per Namespace Allocations" {
    go test ./test/integration/query/count/allocation_running_pods_test.go
}

@test "Query: Number of Pods per Namespace Allocations Summary" {
    go test ./test/integration/query/count/allocations_summary_running_pods_test.go
}


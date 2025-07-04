setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "Query: Number of Allocations per Aggregate" {
    go test ./test/integration/query/count/allocations_running_pods_test.go
}

@test "Query: Number of Allocations Summary" {
    go test ./test/integration/query/count/allocations_summary_running_pods_test.go
}


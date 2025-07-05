setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "Assert Cost Information: /allocation and /allocation/summary" {
    go test ./test/integration/query/assert/allocation_allocationSummary_namespace_aggregation_comparison_test.go
}



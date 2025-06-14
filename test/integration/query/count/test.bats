setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "Query: Number of allocations per aggregate" {
    go test ./test/integration/query/count/allocations_per_aggregate_test.go
}



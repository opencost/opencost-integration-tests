# test/integration/query/validation/test.bats
#!/usr/bin/env bats

setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "check for negative idle costs" {
    go test ./test/integration/query/validation/negative_idle_test.go ./test/integration/query/validation/types.go -v
}

@test "validate cost aggregation consistency" {
    go test ./test/integration/query/validation/cost_aggregation_test.go ./test/integration/query/validation/types.go -v
}

@test "validate efficiency metrics" {
    go test ./test/integration/query/validation/efficiency_metrics_test.go ./test/integration/query/validation/types.go -v
}

@test "validate idle resource allocation" {
    go test ./test/integration/query/validation/idle_allocation_test.go ./test/integration/query/validation/types.go -v
}
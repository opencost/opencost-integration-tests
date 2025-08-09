setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "Assert Cost Information: /allocation and /allocation/summary" {
    go test ./test/integration/query/assert/allocation_allocationSummary_namespace_aggregation_comparison_test.go
}

@test "Assert Node Pricing: /asset" {
    go test ./test/integration/query/assert/pricing/node_oracle_pricing_test.go
}

@test "Assert PV Pricing: /asset" {
    go test ./test/integration/query/assert/pricing/pv_oracle_pricing_test.go
}



setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "allocation: controller kind consistency" {
    go test ./test/integration/allocation/allocation_controller_consistency_test.go
}

@test "allocation: Idle allocation Cost Validation" {
    go test test/integration/allocation/idle_allocation_cost_validation_test.go
}

@test "allocation: Non negative Cost Validation" {
    go test test/integration/allocation/non-negative_cost_validation_test.go
}

@test "allocation: TotalCost equal to sum of Costs" {
    go test test/integration/allocation/total_cost_equal_sum_of_costs_test.go
}
setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "allocation: controller kind consistency" {
    go test ./test/integration/allocation/allocation_controller_consistency_test.go
    go test ./test/integration/allocation/idle_cost_negative_test.go
}

@test "allocation: negative idle cost values" {
    go test ./test/integration/allocation/idle_cost_negative_test.go
}
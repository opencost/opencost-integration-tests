setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "allocation: controller kind consistency" {
    go test ./test/integration/allocation/allocation_controller_consistency_test.go
}

@test "allocation: negative cost idle allocation" {
    go test ./test/integration/allocation/cost_validation_test.go
}
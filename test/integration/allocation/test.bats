setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "allocation: controller kind consistency" {
    go test ./test/integration/allocation/allocation_controller_consistency_test.go
}
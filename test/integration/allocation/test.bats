setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "prometheus: start and end time of a resource" {
    go test ./test/integration/allocation/allocation_controller_consistency_test.go
}
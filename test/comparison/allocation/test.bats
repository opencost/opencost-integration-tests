setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "allocation: comparison" {
    go test ./test/comparison/allocation/allocation_comparison_test.go
}
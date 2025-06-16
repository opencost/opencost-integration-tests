setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "Assert: /allocation and /allocation/summary" {
    go test ./test/integration/query/assert/allocation_allocationsummary_test.go
}



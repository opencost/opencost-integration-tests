setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "query allocation non-negativity sanity checks" {
    go test ./test/integration/query/sanity -run TestAllocationNonNegativitySanity -count=1 -v
}
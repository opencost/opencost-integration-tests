setup() {
    DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")" >/dev/null 2>&1 && pwd)"
    cd "$DIR"
}

teardown() {
    : # Nothing to tear down.
}

@test "api: Allocation Response Schema Stability" {
    go test -count=1 . -run '^TestAllocationResponseSchemaStability$'
}

@test "api: Assets Response Schema Stability" {
    go test -count=1 . -run '^TestAssetsResponseSchemaStability$'
}

@test "api: CloudCost Response Schema Stability" {
    go test -count=1 . -run '^TestCloudCostResponseSchemaStability$'
}
setup() {
    DIR="$( cd "$( dirname "$BATS_TEST_FILENAME" )" >/dev/null 2>&1 && pwd )"
    cd "$DIR"
}

teardown() {
    : # nothing to tear down
}

@test "autocomplete: prometheus ground truth queries (no OpenCost autocomplete required)" {
    go test -count=1 -run 'TestPrometheus(Allocation|Asset)GroundTruthQueries'
}

@test "autocomplete: allocation prometheus ground truth" {
    go test -count=1 -run TestAllocationAutocompletePrometheusGroundTruth
}

@test "autocomplete: allocation controller kind and name smoke" {
    go test -count=1 -run 'TestAllocationAutocompleteController(Kind|Name)Smoke'
}

@test "autocomplete: allocation label value" {
    go test -count=1 -run TestAllocationAutocompleteLabelValue
}

@test "autocomplete: allocation namespace labels" {
    go test -count=1 -run TestAllocationAutocompleteNamespaceLabelKeys
}

@test "autocomplete: allocation search and limit" {
    go test -count=1 -run 'TestAllocationAutocomplete(SearchFilter|Limit)'
}

@test "autocomplete: assets prometheus ground truth" {
    go test -count=1 -run 'TestAssetsAutocomplete(Prometheus|TypeAndCategory)'
}

@test "autocomplete: assets node label value" {
    go test -count=1 -run TestAssetsAutocompleteNodeLabelValue
}

@test "autocomplete: cloud cost ground truth" {
    go test -count=1 -run 'TestCloudCostAutocomplete'
}

@test "autocomplete: validation errors" {
    go test -count=1 -run 'ValidationErrors'
}

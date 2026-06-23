setup() {
    DIR="$( cd "$( dirname "$BATS_TEST_FILENAME" )" >/dev/null 2>&1 && pwd )"
    cd "$DIR"
}

teardown() {
    : # nothing to tear down
}

@test "reliability: no OpenCost pod restarts" {
    go test -count=1 no_pod_restarts_test.go
}

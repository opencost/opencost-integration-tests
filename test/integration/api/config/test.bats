setup() {
    DIR="$( cd "$( dirname "$BATS_TEST_FILENAME" )" >/dev/null 2>&1 && pwd )"
    cd "$DIR"
}

teardown() {
    : # nothing to tear down
}

@test "config: cloud integration disabled" {
    go test -count=1 cloud_integration_disabled_test.go
}
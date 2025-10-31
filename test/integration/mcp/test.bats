setup() {
    DIR="$( cd "$( dirname "$BATS_TEST_FILENAME" )" >/dev/null 2>&1 && pwd )"
    cd $DIR
}

teardown() {
    : # nothing to tear down
}

@test "mcp: allocation costs vs HTTP API" {
    go test allocation_mcp_vs_http_test.go helpers.go
}

@test "mcp: asset costs vs HTTP API" {
    go test asset_mcp_vs_http_test.go helpers.go
}


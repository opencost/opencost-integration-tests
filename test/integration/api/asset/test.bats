setup() {
    DIR="$( cd "$( dirname "$BATS_TEST_FILENAME" )" >/dev/null 2>&1 && pwd )"
    cd "$DIR"
}

teardown() {
    : # nothing to tear down
}

@test "asset: Smoke Test" {
    #60sec cap bc SDK http.Get has no timeout, avoids go test's default 10 min
    go test -timeout 60s assets_smoke_test.go 
}

@test "asset: Node Labels" {
    go test node_labels_test.go
}

@test "asset: Spot Node" {
    go test spot_nodes_test.go
}

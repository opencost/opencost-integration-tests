setup() {
    DIR="$( cd "$( dirname "$BATS_TEST_FILENAME" )" >/dev/null 2>&1 && pwd )"
    cd $DIR
}

teardown() {
    : # nothing to tear down
}

@test "asset: Node Labels" {
    go test node_labels_test.go
}

@test "asset: Spot Node" {
    go test spot_nodes_test.go
}

setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "prometheus: start and end time of a resource" {
    go test ./test/integration/prometheus/prometheus_test.go
}
setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "prometheus: start and end time of a resource" {
    go test ./test/integration/prometheus/prometheus_test.go
}
@test "prometheus: RAM Bytes" {
    go test ./test/integration/prometheus/ram_bytehours_ground_truth_test.go
}

@test "prometheus: CPU Core Hours" {
    go test ./test/integration/prometheus/cpu_allocation_ground_truth_test.go
}
setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "prometheus: start and end time of a resource" {
    go test ./test/integration/prometheus/prometheus_test.go
}

@test "prometheus: RAMBytes, RAMBytesHours and RAMRequestAverage Costs" {
    go test ./test/integration/prometheus/consolidated_ram_costs_analysis_test.go
}

@test "prometheus: Max RAM Usage Costs" {
    go test ./test/integration/prometheus/ram_maxtime_ground_truth_test.go
}

@test "prometheus: PromQL URL Constructor Test" {
    go test ./test/integration/prometheus/client_test.go
}
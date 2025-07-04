setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

# ------------------------------------------------------
# Prometheus Package 
@test "prometheus: start and end time of a resource" {
    go test ./test/integration/prometheus/prometheus_test.go
}

@test "prometheus: PromQL URL Constructor Test" {
    go test ./test/integration/prometheus/client_test.go
}
# ------------------------------------------------------


# ------------------------------------------------------
# RAM Costs
@test "prometheus: RAMBytes, RAMBytesHours and RAMByteRequestAverage Costs" {
    go test ./test/integration/prometheus/ram_bytehours__ram_request_average_costs_analysis_test.go
}

@test "prometheus: Max RAM Usage Costs" {
    go test ./test/integration/prometheus/ram_maxtime_ground_truth_test.go
}
# ------------------------------------------------------

# ------------------------------------------------------
# CPU Costs
@test "prometheus: CPUCores, CPUCoreHours and CPUCoreRequestAverage Costs" {
    go test ./test/integration/prometheus/cpu_corehours__cpu_request_average_costs_analysis_test.go
}

# ------------------------------------------------------


# ------------------------------------------------------
# GPU Costs
@test "prometheus: CPUCores, CPUCoreHours and CPUCoreRequestAverage Costs" {
    go test ./test/integration/prometheus/gpu__gpu_request_average_costs_analysis_test.go
}

# ------------------------------------------------------
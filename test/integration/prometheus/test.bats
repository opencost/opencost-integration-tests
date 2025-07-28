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

@test "prometheus: Average RAM Usage Costs" {
    go test ./test/integration/prometheus/ram_average_usage_test.go
}

# ------------------------------------------------------

# ------------------------------------------------------
# CPU Costs
@test "prometheus: CPUCores, CPUCoreHours and CPUCoreRequestAverage Costs" {
    go test ./test/integration/prometheus/cpu_corehours__cpu_request_average_costs_analysis_test.go
}

@test "prometheus: Average CPU Usage Costs" {
    go test ./test/integration/prometheus/cpu_average_usage_test.go
}

# ------------------------------------------------------


# ------------------------------------------------------
# GPU Costs
@test "prometheus: GPUHours and GPURequestAverage Costs" {
    go test ./test/integration/prometheus/gpu__gpu_request_average_costs_analysis_test.go
}

@test "prometheus: Max GPU Usage Costs" {
    go test ./test/integration/prometheus/gpu_maxtime_ground_truth_test.go
}

@test "prometheus: Average GPU Usage Costs" {
    go test ./test/integration/prometheus/gpu_average_usage_test.go
}

@test "prometheus: GPU Info" {
    go test ./test/integration/prometheus/gpu_info_test.go
}
# ------------------------------------------------------


# ------------------------------------------------------
# Network Costs
@test "prometheus: Network Transfer and Receive Bytes" {
    go test ./test/integration/prometheus/network_costs_test.go
}

@test "prometheus: Network Internet Cost" {
    go test ./test/integration/prometheus/network_internet_costs_test.go
}

@test "prometheus: Network Zone Cost" {
    go test ./test/integration/prometheus/network_zone_costs_test.go
}

@test "prometheus: Network Region Cost" {
    go test ./test/integration/prometheus/network_region_costs_test.go
}
# ------------------------------------------------------

# ------------------------------------------------------
# Load Balancer Costs
@test "prometheus: Load Balancer Cost" {
    go test ./test/integration/prometheus/load_balancer_costs_test.go
}
# ------------------------------------------------------
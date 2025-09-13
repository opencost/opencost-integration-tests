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

# ------------------------------------------------------

# ------------------------------------------------------
# Persistent Volume Costs
    
@test "prometheus: Mounted Persistent Volume Costs" {
    go test ./test/integration/prometheus/pv_bytehours_pv_request_costs_analysis_test.go
}

@test "prometheus: GPU Info" {
    go test ./test/integration/prometheus/gpu_info_test.go
}

@test "prometheus: GPU Count" {
    go test ./test/integration/prometheus/gpu_count_test.go
}
# ------------------------------------------------------

# ------------------------------------------------------
# Load Balancer Costs
@test "prometheus: Load Balancer Cost" {
    go test ./test/integration/prometheus/load_balancer_costs_test.go
}
# ------------------------------------------------------


# ------------------------------------------------------
# Node Costs
@test "prometheus: Node Hourly Cost" {
    go test ./test/integration/prometheus/node_costs_test.go
}

# ------------------------------------------------------

# ------------------------------------------------------
# UID Verification Tests
@test "prometheus: Pod UID Verification" {
    go test ./test/integration/prometheus/uid_verification_test.go -run TestPodUIDVerification
}

@test "prometheus: Deployment UID Verification" {
    go test ./test/integration/prometheus/uid_verification_test.go -run TestDeploymentUIDVerification
}

@test "prometheus: StatefulSet UID Verification" {
    go test ./test/integration/prometheus/uid_verification_test.go -run TestStatefulSetUIDVerification
}

@test "prometheus: Service UID Verification" {
    go test ./test/integration/prometheus/uid_verification_test.go -run TestServiceUIDVerification
}

@test "prometheus: Namespace UID Verification" {
    go test ./test/integration/prometheus/uid_verification_test.go -run TestNamespaceUIDVerification
}

@test "prometheus: Node UID Verification" {
    go test ./test/integration/prometheus/uid_verification_test.go -run TestNodeUIDVerification
}

@test "prometheus: PersistentVolume UID Verification" {
    go test ./test/integration/prometheus/uid_verification_test.go -run TestPersistentVolumeUIDVerification
}

@test "prometheus: PersistentVolumeClaim UID Verification" {
    go test ./test/integration/prometheus/uid_verification_test.go -run TestPersistentVolumeClaimUIDVerification
}

@test "prometheus: Job UID Verification" {
    go test ./test/integration/prometheus/uid_verification_test.go -run TestJobUIDVerification
}


# ------------------------------------------------------
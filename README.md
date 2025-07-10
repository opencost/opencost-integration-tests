# OpenCost Integration Tests

## Setup

Clone submodules and download Go dependencies

```sh
git submodule update --init --recursive
go mod download
```

## Running Tests Locally

First, you'll need to configure access to a running instance of OpenCost.

```sh
# Example 1: Local run
export OPENCOST_URL='http://localhost:9003'

# Example 2: Remote run
export OPENCOST_URL='https://demo.infra.opencost.io/model'
```

Running tests with [bats](https://bats-core.readthedocs.io/en/stable/index.html) is simple. The bats binary is already included in a git submodule within the test suite. Invoke that binary from the command line, giving as an argument a file or directory containing tests.

```sh
# Example 1: Run a single test file
./test/bats/bin/bats ./test/integration/prometheus/test.bats

# Example 2: Run all tests in a directory
./test/bats/bin/bats ./test/integration/prometheus/ -r
```

## Writing Tests

OpenCost's integration tests are a fusion of bash and Go.

All bats tests originate in bash. As such, they are designed to provide access to anything suite of tools you might want from the command line, including `kubectl`, `curl`, `grep`, etc.

There is also a Go SDK in the `pkg` directory, which provides helper types and functions for querying the APIs and asserting various properties about the results. The SDK is intentionally written without access to the `opencost` package, so that it's not possible to cheat. All tests will take the perspective of a pure consumer of OpenCost APIs.

First, decide where your tests belongs. All tests should start in `test/integration/`. Try to be specific about what kind of test you are writing. For example, testing query accuracy goes under `test/integration/query/accuracy/`. If there is already a `test.bats` file, then you should add to that file. If there is not, then create it.

A `test.bats` file should look something like this:

```sh
setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "query allocation consistency" {
    go test ./test/integration/query/consistency/allocation_test.go
}
```

There is a `setup` function and a `teardown` function, which may be used to perform actions before and after _each individual test_. Then there are the tests, themselves, which begin with `@test` and a name. Within each test should be a series of `bash` commands. If they exit with `0` the test passes. If they exit with `1` then they fail. For more details and tutorials on writing bash-native bats tests, please read the docs here: https://bats-core.readthedocs.io/en/stable/writing-tests.html

# LFX Mentorship 

WIP TESTS 

| Complete | Description | Prometheus Query | API Endpoint | Description | Difficulty |
|:----------:|-------------|--------------------|--------------|------------|------------|
| ✅       | Query Number of Allocations | kube_pod_container_status_running | /allocation | For each aggregate, query Prometheus for the expected number of aggregated results. Query the API. Then confirm number of results match expected | Low |
| ✅        | Query Number of Summary Allocations | kube_pod_container_status_running | /allocation/summary | For each aggregate, query Prometheus for the expected number of aggregated results. Query the API. Then confirm number of results match expected | Low |
| [ ]        | Query Number of Assets | pv_hourly_cost/node_hourly_cost/kubecost_load_balancer_cost | /assets | For each aggregate, query Prometheus for the expected number of un aggregated results. Query the assets API. Then confirm number of results match expected, and costs match | Low |
| ✅        | Ensure Summary Allocation Matches Allocation | N/A | /allocation and /allocation/summary | For each aggregate, query summary allocations and allocations. ensure all fields are equal for a variety of aggregates and time frames | Medium |
| ✅        | Ground Truth: CPU Allocation | container_cpu_allocation | /allocation | For each aggregate, query prometheus to determine the CPU Core hours. Then query the allocation API to confirm that the sum of the CPU core hours for each namespace sum to the expected amounts from prometheus. ensure the allocation is the max or requested and usage. Perform math to ensure the GPU cost = usage * price | Medium |
| ✅       | Ground Truth: Memory Requests | kube_pod_container_resource_requests{resource="memory" | /allocation | For each namespace, query prometheus to determine the requested RAM byte hours. Then query the allocation API to confirm that the sum of the RAM requested Byte hours for each namespace sum to the expected amounts from prometheus | Low |
| ✅        | Ground Truth: Average Memory Usage | avg_over_time(container_memory_working_set_bytes | /allocation | For each POD and NAMESPACE, query prometheus to determine the average RAM byte hours. Then query the allocation API to confirm that the sum of the RAM requested Byte hours for each namespace and POD average to the expected amounts from prometheus | Low |
| ✅        | Ground Truth: Max Memory Usage | max_over_time(container_memory_working_set_bytes | /allocation | For each POD and NAMESPACE, query prometheus to determine the max RAM byte hours. Then query the allocation API to confirm that the sum of the RAM requested Byte hours for each namespace and POD max to the expected amounts from prometheus | Low |
| ✅        | Ground Truth: Average CPU Usage | rate(container_cpu_usage_seconds_total{container!="", | /allocation | For each aggregate, query prometheus to get the average CPU usage. then, query the allocation API and for each result, confirm that the CPU usage matches what comes from prometheus. | Medium |
| ✅       | Ground Truth: GPUs requested | avg_over_time(kube_pod_container_resource_requests{resource="nvidia_com_gpu", | /allocation | For each aggregate, query prometheus to get the average GPU request. then, query the allocation API and for each result, confirm that the GPU request matches what comes from prometheus | Low |
| [ ]        | Ground Truth: Node resource Hourly Cost | avg_over_time(node_cpu_hourly_cost/avg_over_time(node_ram_hourly_cost/avg_over_time(node_gpu_hourly_cost | /allocation and /assets | For each aggregate, query prometheus to get the CPU hourly cost. then, query allocation to confirm that idle + allocated CPU cost is equal to the prom result. Also, confirm that node_cpu_hourly_cost + node_ram_hourly_cost + node_gpu_hourly_cost equals the total node cost for assets | Medium |
| [ ]        | Ground Truth: Spot Nodes | kubecost_node_is_spot | /assets/ | For each aggregate, query prometheus to get the number of spot nodes. then, query the assets API and confirm that nodes showing as spot in prom match to the assets API response | Low |
| [ ]        | PVC info vs allocations | kube_persistentvolumeclaim_info | /allocations | For each aggregate, query prometheus to get the PVC info. then, ensure every PVC is accounted for in the response and that their numbers match. | Low |
| [ ]        | PVC info vs allocations | kube_persistentvolumeclaim_info | /assets | For each aggregate, query prometheus to get the PVC info. then, query assets and ensure every disk is accounted for between the two calls | Low |
| [ ]        | Ground Truth: PVC allocations | avg_over_time(pod_pvc_allocation | /allocations | For each aggregate, query prometheus to get the PVC allocations. Ensure in the result, that all PVC allocations match to pods and that they sum to the totals of the PVC info from prometheus | Low |
| [ ]        | Ground Truth: storage byte requests | kube_persistentvolumeclaim_resource_requests_storage_bytes| /allocations | For each aggregate, query prometheus to get the PVC requested storage bytes. Ensure in the result, that all PVC allocations match to pods and that their requested bytes match the prom result | Low |
| [ ]        | Ground Truth: storage byte hours|kube_persistentvolume_capacity_bytes| /allocations | For each aggregate, query prometheus to get the PVC capacity bytes. in the corresponding allocation ensure bytes match | Low |
| [ ]        | Ground Truth: PV costs | pv_hourly_cost | /allocations | Query cloud provider pricing to get expected costs for a given PV. ensure that the prom results match expected costs. ensure allocations API call returns PVs that match the costs. fail the tests if no PVs are in the returned allocations data | Medium |
| [ ]        | Ground Truth: PV info | kubecost_pv_info | /allocation | For each aggregate, query prometheus to get the PV info for each PV. Then, query allocations and confirm each PV is present in every aggregation and that the info matches. | Low |
| [ ]        | Ground Truth: Network Bytes | container_network_receive_bytes_total/container_network_transmit_bytes_total | /allocation | For each aggregate, query prometheus to get thenetwork transmit/receive byte for every container. THen, query allocations and confirm each allocation matches the pod entry, and that for other aggregations, that the sums match. | Low |
| [ ]        | Ground Truth: Node Labels | kube_node_labels | /asset | For each aggregate, query prometheus to get the labels for each node. then, query the assets API and confirm the labels match for all the different aggregations | Low |
| [ ]        | Ground Truth: Node annotations | kube_node_annotations | /asset | For each aggregate, query prometheus to get the annotations for each node. then, query the assets API and confirm the annotations match for all the different aggregations | Low |
| [ ]        | Ground Truth: Labels | kube_pod_labels/kube_namespace_labels | /allocation | For each aggregate, query prometheus to get the labels for each pod and namespace. then, query the allocation API and for each result, confirm the labels in each aggregate contain the expected labels | Medium |
| [ ]        | Ground Truth: annotations | kube_pod_annotations/kube_namespace_annotations | /allocation | For each aggregate, query prometheus to get the annotation for each pod and namespace. then, query the allocation API and for each result, confirm the annotations in each aggregate contain the expected labels | Low |
| [ ]        | Ground Truth: load balancer cost | kubecost_load_balancer_cost | /allocation | Query the cloud provider API to get expected pricing. compare that to the prometheus results, and compare to the allocation costs for load balancers for multiple allocation. | Medium |
| [ ]        | Ground Truth: GPU Info | DCGM_FI_DEV_DEC_UTIL{ | /allocation | query the gpu info prometheus endpoint, then query allocations for multiple aggregations and confirm the GPU info matches what is coming out of prometheus. | Low |
| ✅        | Ground Truth: GPUs usage | avg_over_time(DCGM_FI_PROF_GR_ENGINE_ACTIVE | /allocation | For each aggregate, query prometheus to get the average GPU usage. then, query the allocation API and for each result, confirm that the GPU usage matches what comes from prometheus. | Medium |
| ✅        | Ground Truth: GPUs MAX | max_over_time(DCGM_FI_PROF_GR_ENGINE_ACTIVE | /allocation | For each aggregate, query prometheus to get the max GPU usage. then, query the allocation API and for each result, confirm that the GPU max matches what comes from prometheus. | Medium |
| [ ]        | Ground Truth: Container requests | kube_pod_container_resource_requests{resource="cpu", unit="core", | /allocation | For each aggregate, query prometheus to determine the cpu requests. then, query the allocations API and confirm that the CPU requests average to approximately the amount returned from prometheus | Low |
| ✅        | Ground Truth: Memory Allocation | container_memory_allocation_bytes | /allocation | For each namespace, query prometheus to determine the RAM byte hours. Then query the allocation API to confirm that the sum of the RAM Byte hours for each namespace sum to the expected amounts from prometheus. for each response, confirm that the allocation/prices matches the maximum of requests and usage | Low |
| ✅        | Ground Truth: GPU Allocation | avg_over_time(container_gpu_allocation{ | /allocation | For each namespace, query prometheus to determine the GPU Hours. Then query the allocation API to confirm that the sum of the GPU hours for each namespace sum to the expected amounts (max of gpu request and usage) from prometheus. Fail the tests if no namespace has more than 0 GPU core hours allocated. perform math to ensure that GPU cost = allocation * price | Medium |
| [ ]        | Ground Truth: Assets | node_total_hourly_cost / pv_hourly_cost | /assets | Use kubectl to get the lists of nodes and disks. query prometheus to ensure those inventories match what kubectl returns. Then, query the Assets API to confirm those returned results for nodes and disk match the kubectl and prometheus results. verify IDs, CPU/RAM/GPU and size (for disks) | Medium |
| [ ]        | Ground Truth: Node Pricing | node_total_hourly_cost | /assets | Query pricing API (for oracle: https://docs.oracle.com/en-us/iaas/Content/Billing/Tasks/signingup_topic-Estimating_Costs.htm#accessing_list_pricing). then query node_total_hourly_cost and confirm accuracy. | Medium |
| [ ]        | Ground Truth: PV Pricing | pv_hourly_cost | /assets | Query pricing API (for oracle: https://docs.oracle.com/en-us/iaas/Content/Billing/Tasks/signingup_topic-Estimating_Costs.htm#accessing_list_pricing). get disk list prices. then query pv_hourly_cost and confirm accuracy. | Medium |
| [ ]        | Ground Truth: DCGM Metrics vs allocations | node_gpu_count | /allocation | Query DCGM directly for expected GPUs counts and IDs. Compare those to the results returned from allocations. ensure the GPU ids are all present, as well as the amounts | Medium |
| ✅        | Data Quality: Non-zero GPU Costs | node_gpu_hourly_cost | /allocation | Query the API directly and ensure that there are >0 GPU costs for >0 allocations | Low |
| [ ]        | Exported Cloud Costs Vs Ground Truth | N/A - creating these is in project scope | /cloudCost | Configure a prometheus exporter for cloud costs. with cloud costs enabled, query the upstream for cloud costs. then, query /cloudCosts on the OpenCost install. Verify returned results against the expected items. | High |
| [ ]        | Ensure no pod restarts on OpenCost | kube_pod_container_status_running | N/A | use a kubernetes client to ensure the OpenCost pod has 0 restarts and no errors in the logs. also, query prometheus to ensure the opencost pod never left the running state after its initial boot | Low |
| [ ]        | Implement Dev Stack for OpenCost | N/A | N/A | Deploy a dev stack for opencost. the integration tests should be run on a loop on that. Images that pass testing should get promoted to the dev env. | Medium |
| [ ]        | Implement Chaos testing on OpenCost Dev Stack | N/A| N/A | Deploy a chaos monkey to the opencost dev stack. Integration tests should be updated to allow for temporary outages when monkey killed a pod, but should expect both prom backed and promless data to be intact.| Medium |
| [ ]        | Log Inspector Test | N/A| N/A | Implement a test that scans opencost logs inside k8s. If there is a panic or an ERROR log, fail the test | Medium |
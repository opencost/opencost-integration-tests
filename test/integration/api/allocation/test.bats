setup() {
    DIR="$( cd "$( dirname "$BATS_TEST_FILENAME" )" >/dev/null 2>&1 && pwd )"
    cd $DIR
}

teardown() {
    : # nothing to tear down
}
@test "allocation: controller kind consistency" {
    go test allocation_controller_consistency_test.go
}

@test "allocation: Pod Labels" {
    go test pod_labels_test.go
}

@test "allocation: Namespace Labels" {
    go test namespace_labels_test.go
}

@test "allocation: Pod Annotations" {
    go test pod_annotations_test.go
}

@test "allocation: Namespace Annotations" {
    go test namespace_annotations_test.go
}

@test "validate_api: negative idle cost values" {
    go test idle_cost_negative_test.go
}

@test "validate_api: negative total efficiency" {
   go test negative_total_efficiency_test.go
}

@test "validate_api: validate Total cost vs individual cost" {
   go test individual_costs_sum_vs_totalcost_test.go
}

@test "validate_api: validate Window Start and End" {
    go test correct_window_values_test.go
}

@test "validate_api: validate if all of idle costs are spread" {
    go test share_idle_shares_test.go
}

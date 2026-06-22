#run go test files
@test "observability: OpenCost logs have no unexpected errors" { 
    go test ./test/integration/observability/log_inspector_test.go
}

# test/integration/query/validation/test.bats
#!/usr/bin/env bats

setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

@test "check for negative idle costs" {
    go test ./test/integration/query/validation/negative_idle_test.go -v
}
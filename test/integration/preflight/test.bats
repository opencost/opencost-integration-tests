setup() {
    : # nothing to set up
}

teardown() {
    : # nothing to tear down
}

# bats test_tags=preflight
@test "preflight: opencost and prometheus observe the same cluster" {
    # -count=1 defeats Go's test cache so the guard re-queries live targets every
    # run; a cached pass from a differently-configured run would defeat the purpose.
    go test -v -count=1 ./test/integration/preflight/...
}

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

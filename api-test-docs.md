# openCost Allocation API Integration Tests

this test suite validates the integrity of OpenCost's `/allocation/compute` API rseponse, focusing on idle resource behavior and cost accurayc.

## 📁 test Ovrview

| file | desription |
|------|-------------|
| `idle_allocation_test.go` | verifies correct handling of `__idle__` allocations — present only when `includeIdle=true` and costs are within expected bounds. |
| `negative_idle_cost_test.go` | ensures no negative cost values (`cpuCost`, `ramCost`, `gpuCost`, `totalCost`) are returned in any allocation. |
| `types.go` | defines Go data structures to parse the API response and provides a helper function `FetchAllocationData()` to query the API. |
| `test.bats` | a bats test script that runs the Go-based validation tests using the `go test` command. |
| `bats` | the local BATS exectuable used to run shell-based `.bats` test scripts. |

---

## ▶️ running the Tests

1. set the OpenCost endpoint enviromnent variable:

```bash
export OPENCOST_URL='https://demo.infra.opencost.io/model'
```
run the integration test via BATS:

```bash
./test/bats/bin/bats ./test/integration/query/validation/test.bats
```
# Asset Ground-Truth Tests

Verifies OpenCost's **per-unit pricing** against a known fixed-pricing fixture
applied through the broker. With pricing pinned cluster-wide, OpenCost's
instantaneous pricing metrics must reflect the fixture.

It asserts on OpenCost's `/metrics` gauges — `node_cpu_hourly_cost`,
`node_ram_hourly_cost`, `pv_hourly_cost` — **not** the windowed `/assets` cost.
A pricing change is not retroactive, so `/assets?window=24h` integrates the
*historically recorded* hourly cost and lags a freshly-applied fixture; the
instantaneous metrics reflect it immediately.

Each test does two checks:

1. **Uniformity (formula-independent)** — every node (or PV) reports the same
   per-unit price, because pricing is fixed across the cluster.
2. **Expected price** — that price equals the fixture's **monthly** value ÷ 730.
   Verified empirically: OpenCost interprets custom `CPU`/`RAM`/`storage` as
   monthly prices and emits the hourly rate as `value/730` (e.g. fixture
   `CPU: 1.0` → `node_cpu_hourly_cost = 0.00137`).

The broker's `/v1/nodes` and `/v1/disks` lists are used as an independent count
cross-check.

> ⚠️ **This suite mutates the cluster.** Applying the fixture rewrites OpenCost's
> pricing ConfigMap and restarts OpenCost (cleanup restores the default). It must
> never run against a shared/demo instance — that is why it lives here (gated,
> manual) and not under `test/integration/...` (which CI runs against the demo).

Like the chaos and restart suites, it reaches the cluster only through the trusted
[ops-broker](../../cmd/ops-broker) via [`pkg/cluster`](../../pkg/cluster), using the
Live contract operations `POST`/`DELETE /v1/config`, `GET /v1/nodes`, `GET /v1/disks`.

## Running

Skipped unless `ASSETS_ENABLED` is set, and needs broker + OpenCost API config:

```bash
export ASSETS_ENABLED=1
export OPENCOST_BROKER_URL=http://ops-broker.opencost.svc:8080
export OPENCOST_BROKER_TOKEN=<broker token>
export OPENCOST_URL=http://opencost.opencost.svc:9003   # OpenCost model API

go test ./test/assets/ -run TestAssetNodePricing -v
go test ./test/assets/ -run TestAssetDiskPricing -v
```

Set `ASSETS_DRY_RUN=1` to exercise the wiring without applying the fixture.

## Configuration

| Env var | Default | Purpose |
| --- | --- | --- |
| `ASSETS_ENABLED` | _(unset)_ | Gate; tests skip unless set |
| `ASSETS_DRY_RUN` | _(unset)_ | Log intended actions, take no effect |
| `OPENCOST_BROKER_URL` | _(required)_ | Broker base URL |
| `OPENCOST_BROKER_TOKEN` | _(required)_ | Broker bearer token |
| `OPENCOST_URL` | _(required)_ | OpenCost model API; `<OPENCOST_URL>/metrics` must be reachable |
| `OPENCOST_NAMESPACE` | `opencost` | Pinned OpenCost namespace |
| `OPENCOST_DEPLOYMENT` | `opencost` | Pinned OpenCost deployment |

## Prerequisites (verified against the opencost-helm-chart)

- **Custom pricing must be enabled** on the target OpenCost
  (`opencost.customPricing.enabled=true`, `provider=custom`). When disabled,
  OpenCost ignores the pricing ConfigMap and the fixture has no effect — the
  expected-price check would (correctly) fail while uniformity may still pass.
- OpenCost reads the ConfigMap named by `PRICING_CONFIGMAP_NAME` (default
  `custom-pricing-model`); the fixture targets that name with **flat** field keys
  (CPU, RAM, storage, provider, …) — not a `default.json` wrapper.

## Cleanup is snapshot-based (safe to re-run)

Applying the fixture snapshots any pre-existing `custom-pricing-model` into the
`opencost.io/fixture-snapshot` annotation before overwriting it. Cleanup
(`DeleteConfig`, run via `t.Cleanup`) then **restores the original** ConfigMap if
one existed, or deletes the ConfigMap if the fixture created it. Re-applying does
not clobber the saved original. You should still prefer a dedicated/test OpenCost,
but a mis-timed failure no longer leaves the cluster without its pricing config.

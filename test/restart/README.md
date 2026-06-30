# Restart Recovery Test

Verifies that OpenCost survives a rolling restart: the deployment and pods come
back ready, the restarted process logs no panic, the allocation API serves
traffic again, and a fixed historical window reports the same cost before and
after the restart (proving Prometheus history was not lost).

Like the chaos suite, this test never touches the cluster directly. It drives
everything through the trusted [ops-broker](../../cmd/ops-broker) via
[`pkg/cluster`](../../pkg/cluster), using these Live contract operations:

- `POST /v1/restart` — trigger the rolling restart
- `GET /v1/deployments/{name}` — wait for rollout (rollout status analogue)
- `GET /v1/pods` — confirm new pods report ready
- `GET /v1/logs` — assert the restarted process did not panic

The history check additionally queries OpenCost's `/allocation` API (via
`OPENCOST_URL`) for a fixed past window before and after the restart.

## Running

The test is skipped unless `RESTART_ENABLED` is set, and needs broker
credentials:

```bash
export RESTART_ENABLED=1
export OPENCOST_BROKER_URL=http://ops-broker.opencost.svc:8080
export OPENCOST_BROKER_TOKEN=<broker token>

go test ./test/restart/ -run TestRestartRecovery -v
```

Set `RESTART_DRY_RUN=1` to exercise the test wiring without actually restarting
OpenCost.

## Configuration

| Env var | Default | Purpose |
| --- | --- | --- |
| `RESTART_ENABLED` | _(unset)_ | Gate; test skips unless set |
| `RESTART_DRY_RUN` | _(unset)_ | Log intended actions, take no effect |
| `OPENCOST_BROKER_URL` | _(required)_ | Broker base URL |
| `OPENCOST_BROKER_TOKEN` | _(required)_ | Broker bearer token |
| `OPENCOST_NAMESPACE` | `opencost` | Pinned OpenCost namespace |
| `OPENCOST_DEPLOYMENT` | `opencost` | Pinned OpenCost deployment |
| `OPENCOST_SELECTOR` | `app.kubernetes.io/name=opencost` | Pod selector for log reads |

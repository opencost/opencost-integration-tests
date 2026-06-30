# Running the Broker-Driven Test Suites

How to run `test/assets`, `test/restart`, and `test/chaos`.

> **These suites mutate a live cluster** (restart OpenCost, kill pods, inject
> chaos, rewrite the pricing ConfigMap). Use a dedicated/test cluster only —
> never the demo or a shared instance.

## 1. Two test tiers

| Tier | Location | Runner | Target | Broker? |
| --- | --- | --- | --- | --- |
| Standard integration | `test/integration/...` | `bats` (CI) | remote demo | No |
| **Broker-driven (this doc)** | `test/{assets,restart,chaos}` | `go test`, env-gated | **your** cluster | **Yes** |

## 2. Why a broker

Test runners are untrusted, so they never hold cluster creds. They call the
in-cluster **ops-broker** (`cmd/ops-broker`) over authenticated HTTP via the
**`pkg/cluster`** SDK; the broker runs a fixed allowlist of ops under scoped RBAC.
Contract: [`docs/broker-api-contract.md`](./broker-api-contract.md).

## 3. Prerequisites

- **Tools:** `go`, `kubectl`, `docker`; `k3d`/`kind`/`minikube` for local; `openssl`.
- **Cluster:** healthy OpenCost + Prometheus. Chaos suite also needs **Chaos Mesh**
  (`podchaos`/`networkchaos` CRDs). Assets suite also needs **custom pricing** (§5).
- **Know your real values** — broker defaults assume `opencost`/`prometheus-system`;
  override if different (e.g. everything in `default`):
  ```bash
  kubectl get deploy -A | grep -iE 'opencost|prometheus'
  kubectl get deploy <opencost> -n <ns> -o jsonpath='{.spec.selector.matchLabels}{"\n"}'
  kubectl get svc -n <ns> | grep -i opencost      # model API port, usually 9003
  ```

## 4. Deploy the broker (one-time)

Manifests in `deploy/ops-broker/` default to the `opencost` namespace; retarget if
yours differs. **Skip `networkpolicy.yaml`** (placeholder default-deny that breaks
the kubelet probe locally) and `prometheus-chaos-rbac.yaml` (only needed if
Prometheus is in a different namespace than the broker).

```bash
# 1. Build + load image
docker build -f cmd/ops-broker/Dockerfile -t ops-broker:dev .
k3d image import ops-broker:dev -c <cluster>     # kind: kind load docker-image; remote: docker push

# 2. Auth token
TOKEN="$(openssl rand -base64 32)"; printf '%s' "$TOKEN" > /tmp/ob-token
kubectl create secret generic ops-broker-auth -n <ns> --from-literal=token="$TOKEN"

# 3. Apply, retargeting namespace + image
{ for f in serviceaccount role rolebinding clusterrole clusterrolebinding service deployment; do
    echo '---'; cat "deploy/ops-broker/$f.yaml"; done; } \
  | sed -E 's/^([[:space:]]*)namespace: opencost$/\1namespace: <ns>/' \
  | sed 's#REPLACE_ME/ops-broker:latest#ops-broker:dev#' \
  | kubectl apply -f -

# 4. Point the broker at your namespaces/selectors
kubectl set env deployment/ops-broker -n <ns> \
  BROKER_NAMESPACE=<ns> BROKER_PROMETHEUS_NAMESPACE=<prom-ns> \
  BROKER_CHAOS_NAMESPACE=<ns> BROKER_LOG_NAMESPACES=<ns>,<prom-ns>
kubectl rollout status deployment/ops-broker -n <ns> --timeout=120s
```

> If the OpenCost deployment isn't named `opencost`, also update `resourceNames:
> ["opencost"]` in `role.yaml` (it scopes the restart permission) before applying.

Smoke-test (leave the forward running):
```bash
kubectl port-forward -n <ns> svc/ops-broker 8080:80 &
OPENCOST_BROKER_URL=http://localhost:8080 OPENCOST_BROKER_TOKEN="$(cat /tmp/ob-token)" \
  OPENCOST_NAMESPACE=<ns> OPENCOST_DEPLOYMENT=<opencost> \
  ./scripts/smoke-ops-broker.sh        # expect: "ops-broker smoke checks passed"
```

## 5. Enable custom pricing (assets suite only)

Without this, the fixture has no effect and the assets price check fails by design.
Add two env vars to the OpenCost **model** container:
```bash
kubectl get deploy <opencost> -n <ns> \
  -o jsonpath='{.spec.template.spec.containers[*].name}'; echo   # find model container
kubectl set env deployment/<opencost> -n <ns> -c <model-container> \
  PRICING_CONFIGMAP_NAME=custom-pricing-model CONFIG_PATH=/tmp/custom-config
kubectl rollout status deployment/<opencost> -n <ns> --timeout=150s
# verify: logs show "Starting *v1.ConfigMap controller" (CM may be absent — test supplies it)
```

> **Unit note (already handled by the test):** OpenCost reads custom CPU/RAM/storage
> as **monthly** and emits hourly = `value/730`; the suite asserts on the
> `*_hourly_cost` metrics against `fixtureValue/730`.

## 6. Port-forwards

The suites restart/kill OpenCost, which kills an ordinary `:9003` forward —
make it self-healing:
```bash
while true; do kubectl port-forward -n <ns> svc/<opencost> 9003:9003 >/dev/null 2>&1; sleep 1; done &
kubectl port-forward -n <ns> svc/ops-broker 8080:80 &     # broker isn't restarted; plain is fine
```

## 7. Run the suites

Shared env (set once):
```bash
export OPENCOST_URL=http://localhost:9003            # <URL>/metrics must resolve (assets)
export OPENCOST_BROKER_URL=http://localhost:8080 OPENCOST_BROKER_TOKEN="$(cat /tmp/ob-token)"
export OPENCOST_NAMESPACE=<ns> OPENCOST_DEPLOYMENT=<opencost> \
       OPENCOST_SELECTOR='app.kubernetes.io/name=opencost'
```

| Var | Required | Notes |
| --- | --- | --- |
| `OPENCOST_BROKER_URL` / `_TOKEN` | yes | broker URL + bearer token |
| `OPENCOST_URL` | yes | model API; `/metrics` reachable for assets |
| `OPENCOST_NAMESPACE` / `_DEPLOYMENT` / `_SELECTOR` | restart, assets | default `opencost` |
| `<SUITE>_ENABLED` | yes | gate; suite skips without it |
| `<SUITE>_DRY_RUN` | no | wire-through, no mutation |

```bash
RESTART_ENABLED=1 go test ./test/restart/ -v -timeout 300s   # restart + history-survival
ASSETS_ENABLED=1  go test ./test/assets/  -v -timeout 300s   # needs §5
CHAOS_ENABLED=1   go test ./test/chaos/   -v -timeout 420s   # needs Chaos Mesh

# all three — sequential (-p 1) so they don't fight over OpenCost
ASSETS_ENABLED=1 RESTART_ENABLED=1 CHAOS_ENABLED=1 \
  go test -p 1 ./test/assets/ ./test/restart/ ./test/chaos/ -v -timeout 600s
```

## 8. Caveats

- **Dedicated cluster only.** Assets snapshots+restores the pricing CM on cleanup,
  but a run killed mid-suite may leave the fixture applied.
- **Namespace defaults are `opencost`/`prometheus-system`** — override broker env (§4)
  and `OPENCOST_NAMESPACE`/`_DEPLOYMENT` if different.
- **Assets needs custom pricing enabled** (§5).
- **Use the self-healing `:9003` forward** (§6) or runs fail on a dropped connection.
- **Run serially.** Concurrent runs collide on the same OpenCost.
- **Chaos:** if Prometheus is in another namespace, apply `prometheus-chaos-rbac.yaml`
  (edit its namespace first). Soft `warning:` lines (cache-served requests) are not failures.

## 9. Troubleshooting

| Symptom | Cause / fix |
| --- | --- |
| Broker `CrashLoopBackOff` after deploy | NetworkPolicy blocks the kubelet probe — don't apply `networkpolicy.yaml`. |
| Broker RBAC/403 errors | OpenCost deployment not named `opencost` → fix `resourceNames` in `role.yaml`. |
| Assets shows default prices, not fixture | Custom pricing not enabled (§5). |
| Assets metric never appears | `<OPENCOST_URL>/metrics` unreachable (wrong URL or `:9003` forward down). |
| Failure on dropped connection mid-run | `:9003` forward died on restart — use the self-healing loop (§6). |
| Suite "passes" instantly | `*_ENABLED` not set → it skipped (look for `--- SKIP`). |

## 10. Standard integration tests (CI)

Separate; against the remote demo, no broker:
```bash
go vet ./...
PROMETHEUS_URL=https://demo-prometheus.infra.opencost.io \
OPENCOST_URL=https://demo.infra.opencost.io/model \
OPENCOST_MCP_URL=https://mcp-demo.infra.opencost.io/ \
  ./test/bats/bin/bats -T --no-parallelize-within-files --jobs 4 -r test/integration
```
Data-dependent on the live demo — confirm a failure against `main` before assuming
you caused it.

## Teardown

```bash
pkill -f 'port-forward.*ops-broker'; pkill -f 'port-forward.*9003'
kubectl delete -n <ns> deploy/ops-broker svc/ops-broker sa/ops-broker secret/ops-broker-auth
kubectl delete clusterrole ops-broker clusterrolebinding ops-broker
kubectl set env deployment/<opencost> -n <ns> -c <model-container> \
  PRICING_CONFIGMAP_NAME- CONFIG_PATH-       # optional: revert custom pricing
```

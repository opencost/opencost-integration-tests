setup() {
    : # nothing to set up
}

teardown() {
    if [ "${ENABLE_PROMETHEUS_DOWN_TEST:-}" = "true" ]; then
        case "${OPENCOST_URL:-}" in
            http://127.0.0.1*|http://localhost*|https://127.0.0.1*|https://localhost*)
                PROM_NS="${PROMETHEUS_NAMESPACE:-default}"
                PROM_DEPLOY="${PROMETHEUS_DEPLOYMENT:-prometheus-server}"

                kubectl scale deployment "$PROM_DEPLOY" -n "$PROM_NS" --replicas=1 >/dev/null
                kubectl rollout status deployment "$PROM_DEPLOY" -n "$PROM_NS" --timeout=120s >/dev/null
                ;;
        esac
    fi
}

@test "api error responses use expected format" {
    go test ./test/integration/api/error_response_format -run 'Test.*InvalidParamsReturnError' -count=1 -v
}
# This test intentionally mutates the local Kubernetes stack by scaling
# Prometheus down. It is opt-in and local-only so it cannot affect demo/shared
# environments.

@test "api returns expected error format when Prometheus is unavailable" {
    if [ "${ENABLE_PROMETHEUS_DOWN_TEST:-}" != "true" ]; then
        skip "set ENABLE_PROMETHEUS_DOWN_TEST=true to run Prometheus-down test"
    fi

    case "${OPENCOST_URL:-}" in
        http://127.0.0.1*|http://localhost*|https://127.0.0.1*|https://localhost*)
            ;;
        *)
            skip "Prometheus-down test only runs against local OPENCOST_URL, got ${OPENCOST_URL:-unset}"
            ;;
    esac

    PROM_NS="${PROMETHEUS_NAMESPACE:-default}"
    PROM_DEPLOY="${PROMETHEUS_DEPLOYMENT:-prometheus-server}"

    kubectl scale deployment "$PROM_DEPLOY" -n "$PROM_NS" --replicas=0
    sleep 15

    go test ./test/integration/api/error_response_format -run TestPrometheusDownReturnsError -count=1 -v
}
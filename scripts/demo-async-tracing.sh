#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
demo_root="${1:-$(mktemp -d /tmp/goforj-async-grafana.XXXXXX)}"
case "$demo_root" in
  /tmp/*|/private/tmp/*) ;;
  *) echo "Choose a new demo directory under /tmp." >&2; exit 2 ;;
esac
mkdir -p "$demo_root"
demo_root="$(cd "$demo_root" && pwd -P)"
case "$demo_root" in
  /tmp/*|/private/tmp/*) ;;
  *) echo "The demo directory must resolve under /tmp." >&2; exit 2 ;;
esac
case "$demo_root" in
  "$repo_root"|"$repo_root"/*) echo "Choose a demo directory outside the checkout." >&2; exit 2 ;;
esac
if [[ -n "$(ls -A "$demo_root")" ]]; then
  echo "The demo directory must be empty: $demo_root" >&2
  exit 2
fi
command -v jq >/dev/null
command -v curl >/dev/null
docker compose version >/dev/null
compose_project="async-demo-$(basename "$demo_root" | tr '[:upper:]' '[:lower:]' | tr -cs '[:alnum:]_-' '-')"
GRAFANA_PORT="${GRAFANA_PORT:-23001}"
TEMPO_HTTP_PORT="${TEMPO_HTTP_PORT:-23200}"
TEMPO_OTLP_HTTP_PORT="${TEMPO_OTLP_HTTP_PORT:-24318}"
TEMPO_OTLP_GRPC_PORT="${TEMPO_OTLP_GRPC_PORT:-24317}"
LOKI_HTTP_PORT="${LOKI_HTTP_PORT:-23100}"
OBSERVABILITY_VM_PORT="${OBSERVABILITY_VM_PORT:-28428}"

cat > "$demo_root/.goforj.yml" <<'CONFIG'
project_name: Async Tracing Demo
module_name: example.com/asyncdemo
render:
  components: [cli, metrics, observability, grafana, docker]
  starter_kit: none
  agent_guidance: none
CONFIG
cat > "$demo_root/.env" <<ENV
APP_ENV=local
COMPOSE_PROFILES=victoriametrics,grafana
IP_ADDRESS=127.0.0.1
GRAFANA_PORT=$GRAFANA_PORT
TEMPO_HTTP_PORT=$TEMPO_HTTP_PORT
TEMPO_OTLP_HTTP_PORT=$TEMPO_OTLP_HTTP_PORT
TEMPO_OTLP_GRPC_PORT=$TEMPO_OTLP_GRPC_PORT
LOKI_HTTP_PORT=$LOKI_HTTP_PORT
OBSERVABILITY_VM_PORT=$OBSERVABILITY_VM_PORT
ENV
chmod 600 "$demo_root/.env"
cat > "$demo_root/demo-storage.yml" <<'STORAGE'
services:
  loki:
    volumes:
      - type: tmpfs
        target: /loki
        tmpfs:
          size: 134217728
          mode: 01777
STORAGE
cd "$repo_root"
go build -o "$demo_root/forj" ./cmd/forj
(cd "$demo_root" && ./forj render) > "$demo_root/render.log" 2>&1
compose=(docker compose --project-name "$compose_project" --file "$demo_root/docker-compose.yml" --file "$demo_root/demo-storage.yml" --env-file "$demo_root/.env")
succeeded=0
# Preserve evidence on failure, but release only this demo's containers and volumes.
finish() {
  if [[ "$succeeded" == 0 ]]; then
    "${compose[@]}" logs --no-color > "$demo_root/stack.log" 2>&1 || true
    "${compose[@]}" down --volumes >/dev/null 2>&1 || true
    echo "Demo failed. Evidence: $demo_root" >&2
  fi
}
trap finish EXIT
"${compose[@]}" --profile grafana up --build -d grafana > "$demo_root/compose.log" 2>&1

# Wait for observed readiness instead of guessing startup or export delays.
wait_for() {
  local attempt
  for attempt in {1..90}; do
    if "$@"; then return 0; fi
    sleep 1
  done
  echo "Timed out: $*" >&2
  return 1
}
wait_for curl -fs -o /dev/null "http://127.0.0.1:$GRAFANA_PORT/api/health"
wait_for curl -fs -o /dev/null "http://127.0.0.1:$TEMPO_HTTP_PORT/ready"
wait_for curl -fs -o /dev/null "http://127.0.0.1:$LOKI_HTTP_PORT/ready"
wait_for curl -fs -o /dev/null -X POST -H 'Content-Type: application/x-protobuf' --data-binary '' "http://127.0.0.1:$TEMPO_OTLP_HTTP_PORT/v1/traces"

FORJ_ASYNC_DEMO_OTLP_ENDPOINT="http://127.0.0.1:$TEMPO_OTLP_HTTP_PORT" \
FORJ_ASYNC_DEMO_REPORT="$demo_root/report.json" \
go test -tags=integration ./internal/forj -run '^TestRenderedAsyncTraceSeparateProcesses$' -count=1 -v -timeout=6m > "$demo_root/acceptance.log" 2>&1
trace_id="$(jq -er '.trace_id' "$demo_root/report.json")"
handler_span_id="$(jq -er '.handler_span_id' "$demo_root/report.json")"

trace_ready() {
  curl -fsS -H 'Accept: application/json' "http://127.0.0.1:$TEMPO_HTTP_PORT/api/traces/$trace_id" > "$demo_root/tempo.json" || return 1
  jq -e --slurpfile proof "$demo_root/report.json" '
    [.. | objects | select(has("spanId") and has("name"))] as $spans |
    ($spans | length == 6) and
    all($proof[0].span_relationships[]; . as $expected |
      any($spans[]; .spanId == $expected.span_id and .traceId == $expected.trace_id and
        (.parentSpanId // "") == ($expected.parent_span_id // "") and .name == $expected.name and
        (.status.code // "STATUS_CODE_UNSET") == $expected.status))
  ' "$demo_root/tempo.json" >/dev/null
}
log_ready() {
  curl -fsS --get "http://127.0.0.1:$LOKI_HTTP_PORT/loki/api/v1/query_range" \
    --data-urlencode "query={service_name=\"async-eventconsumer\"} | trace_id=\"$trace_id\"" \
    --data-urlencode 'limit=100' > "$demo_root/loki.json" || return 1
  jq -e --arg span "$handler_span_id" '([.. | strings | select(. == "order finished")] | length > 0) and ([.. | strings | select(. == $span)] | length > 0)' "$demo_root/loki.json" >/dev/null
}
wait_for trace_ready
wait_for log_ready
succeeded=1
{
  echo '#!/usr/bin/env bash'
  echo 'set -euo pipefail'
  printf '%q ' "${compose[@]}" down --volumes
  echo
} > "$demo_root/cleanup.sh"
chmod +x "$demo_root/cleanup.sh"
echo "Grafana: http://127.0.0.1:$GRAFANA_PORT"
echo "Trace ID: $trace_id"
echo "Evidence: $demo_root"
echo "Cleanup: $demo_root/cleanup.sh"

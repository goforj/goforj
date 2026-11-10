# Upgrade Notes

Review the notes for the changes you are adopting before upgrading an existing project.

## Unreleased

### Contextual application logs in Grafana

Grafana projects now include Loki, Alloy, and a provisioned Logs Overview. Updating the CLI, running `forj render`, and rebuilding the app and generated stack enables local log export when `APP_ENV=local` and the `grafana` profile is active. Review scaffold ownership before a full render. Application logging APIs stay available, and existing console and Lighthouse deduplication remain in place.

Use `appLogger.WithContext(ctx)` to associate log records with the active OTel trace and span. HTTP access logs bind their request context automatically. External export happens before presentation deduplication, preserving individual events across executions.

Alloy now receives both signals on the existing host ports `14318` and `14317`, using the same `TEMPO_OTLP_HTTP_PORT` and `TEMPO_OTLP_GRPC_PORT` overrides. Generated container apps use `alloy` as their local receiver. Tempo retains its query API and internal OTLP receiver. Loki adds query port `13100` (`LOKI_HTTP_PORT`) and a separate named volume with seven-day retention.

Explicit `OTEL_LOGS_EXPORTER=none` keeps logs local to existing outputs. Remote logs require `OTEL_LOGS_EXPORTER=otlp` or `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT`; an existing generic trace endpoint does not silently enable remote log export. Production, staging, and tests have no automatic log destination. `OTEL_SDK_DISABLED=true` disables both signals.

OpenTelemetry API and SDK pins move from 1.45 to 1.47 for the stable Logs API and SDK. Their minimum Go requirement remains compatible with GoForj's Go 1.26 minimum. The generated logging README documents bounded batching, dropped-record behavior, flush deadlines, and the application's responsibility for exported log contents.

### Grafana receives local traces automatically

Projects with Grafana, Observability, and Docker now render Tempo, a provisioned trace datasource, and Traces Overview. After updating the CLI and reviewing scaffold ownership, run `forj render`, rebuild the app, and start the generated stack with `docker-compose up -d --build`. The existing `grafana` Compose profile includes Tempo. No datasource setup is needed.

With `APP_ENV=local` and that profile active, apps send traces to Tempo automatically. Explicit OTLP endpoint settings keep their existing destination, and `OTEL_TRACES_EXPORTER=none` keeps export disabled. Staging, production, and test processes have no automatic destination. This is a local runtime behavior change for existing Grafana projects after regeneration, not an API or persisted-data migration.

Tempo adds host ports `13200` (query), `14317` (OTLP gRPC), and `14318` (OTLP HTTP). Override them with `TEMPO_HTTP_PORT`, `TEMPO_OTLP_GRPC_PORT`, and `TEMPO_OTLP_HTTP_PORT`; host apps follow the ingestion overrides automatically. Trace data lives in a separate named Docker volume. Existing metrics and Lighthouse storage are unchanged.

### Lighthouse tracing uses OpenTelemetry

Framework execution boundaries now create standard OTel spans. Lighthouse builds inspect records from those spans and retains its bounded, richer debugging payloads. Existing inspect lookup IDs and `LIGHTHOUSE_INSPECT_*` retention settings remain supported; records add `otel_trace_id`, `span_id`, and `parent_span_id` for correlation.

After updating the CLI, review starter-kit ownership before running a full `forj render`, then rebuild the app. This refreshes framework-managed inspect, HTTP, and wiring code and synchronizes OTel dependencies. No minimum Go version change is required. Keep customized starter-kit source committed because a full render can refresh its scaffold.

Export remains off until configured outside the local Grafana default described above. Set `OTEL_TRACES_EXPORTER=otlp`, `OTEL_EXPORTER_OTLP_ENDPOINT`, and optionally `OTEL_SERVICE_NAME` to send traces independently from Lighthouse. HTTP/protobuf is the default; select gRPC with `OTEL_EXPORTER_OTLP_TRACES_PROTOCOL=grpc`. The generated `internal/inspects/README.md` lists supported settings, outbound HTTP instrumentation, application spans, and transport limits.

Malformed OTLP endpoint URLs, header names, or percent-encoding now fail construction without logging their values. App-owned exporter failures also use generic diagnostic messages while preserving the original cause through standard error unwrapping. Framework-generated exceptions export their type and a generic message; original error text stays in Lighthouse, alongside explicit application exception events. CLI span names use parsed command paths and argument placeholders, excluding flag and positional values. The instrumented HTTP client omits URL user information, query strings, and fragments from automatic span attributes, and uses generic failure descriptions without changing requests or caller-visible errors.

OTel head sampling now governs whether an execution is recorded, including inherited remote sampling decisions. Lighthouse's sample rate, capacity, and connection gate only reduce its local enrichment and retention. Nested `Begin` calls create independently finished child executions; retain the returned context and pair each call with `Finish`. Standard custom spans use normal OTel APIs.

This change adds HTTP propagation and tracing for existing runtime boundaries. Queue and event transports do not yet carry standard trace metadata across brokers. Prometheus metrics and existing logging contracts continue independently.


### Swagger terminology is now OpenAPI

The generated API reference uses Scalar to display OpenAPI documents. Framework labels, generated filenames, and canonical routes now use OpenAPI terminology.

After installing a CLI version that includes this change, run the targeted upgrade from your project root:

```sh
forj render --only openapi
```

This updates only the two generated OpenAPI files and the documentation route call in `internal/http/server.go`, then removes the old generated filenames. Dependencies, Wire output, environment files, and other framework code remain untouched. The command supports existing WebAPI and WebUI Apps, including named Apps. Unrecognized server wiring is reported before files change. Repeating the command is safe.

| Previous name | Canonical name |
| --- | --- |
| `internal/http/swagger.go` | `internal/http/openapi.go` |
| `internal/http/swagger_test.go` | `internal/http/openapi_test.go` |
| `/swagger` and `/swagger/` | `/openapi` and `/openapi/` |
| `/swagger/doc.json` | `/openapi/doc.json` |
| `API_SWAGGER_ENABLED` | `API_OPENAPI_ENABLED` |
| CLI resource ID `swagger` | CLI resource ID `openapi` |

Existing `/swagger` URLs remain working aliases. Settings resolve in this order: `API_OPENAPI_ENABLED`, `API_SWAGGER_ENABLED`, then `SWAGGER_ENABLED`. Existing disabled settings stay effective when the canonical setting is absent. There is no need to rename external deployment settings at the same time; use the canonical name for new configuration.

Update any scripts that select the CLI documentation resource by its old `swagger` ID to use `openapi`. The OpenAPI document artifacts remain at `build/openapi.json` and `build/<app>/openapi.json`.

Review the resulting diff and rebuild your App binaries using your normal build workflow. A full `forj render` includes this migration too, when a complete framework refresh is intended.

## Earlier beta project layouts

Projects created before the App composition layout should also review the [single-app structure migration guide](migration.md).

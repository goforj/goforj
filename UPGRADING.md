# Upgrade Notes

Review the notes for the changes you are adopting before upgrading an existing project.

## Unreleased

### Lighthouse tracing uses OpenTelemetry

Framework execution boundaries now create standard OTel spans. Lighthouse builds inspect records from those spans and retains its bounded, richer debugging payloads. Existing inspect lookup IDs and `LIGHTHOUSE_INSPECT_*` retention settings remain supported; records add `otel_trace_id`, `span_id`, and `parent_span_id` for correlation.

After updating the CLI, review starter-kit ownership before running a full `forj render`, then rebuild the app. This refreshes framework-managed inspect, HTTP, and wiring code and synchronizes OTel dependencies. No minimum Go version change is required. Keep customized starter-kit source committed because a full render can refresh its scaffold.

Export remains off until configured. Set `OTEL_TRACES_EXPORTER=otlp`, `OTEL_EXPORTER_OTLP_ENDPOINT`, and optionally `OTEL_SERVICE_NAME` to send traces independently from Lighthouse. HTTP/protobuf is the default; select gRPC with `OTEL_EXPORTER_OTLP_TRACES_PROTOCOL=grpc`. The generated `internal/inspects/README.md` lists supported settings, outbound HTTP instrumentation, application spans, and transport limits.

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

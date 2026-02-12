# Forj API Index Design

## Feature Name

- Product name: **Forj API Index**
- CLI commands:
  - **`forj build:api-index`** (runs API indexing)
  - **`forj build`** (runs all `build:*` pipelines; currently includes `build:api-index`)
- Optional shorthand/branding: **`apix`**

## Goal

Build a source-driven API metadata layer that discovers HTTP routes and handler contracts and emits machine-readable artifacts for:

- DevConsole API exploration
- OpenAPI generation
- Future build/codegen workflows

## Implementation Status (Current, Accurate to Code)

This section describes what is implemented today in `internal/apix` and wired via `internal/forj/api_index_cmd.go`.

### Parsing strategy

- Uses **Go AST parsing** (`go/parser`, `go/ast`) over `.go` files.
- Does **not** currently use `go/packages` or full type-checking.
- Skips `_test.go`, `vendor`, `.git`, `node_modules`, `.cache`, `tmp`, and `templates/`.

### Route discovery

- Discovers routes from:
  - `http.NewRoute(method, path, handler)`
  - `http.NewRouteGroup(prefix, routes)`
- Tracks handler expression and source location.
- Builds controller-to-group prefix mapping from router patterns:
  - `ProvideAppRoutes(...)`
  - `ProvideRoutes(...)`
- Falls back to unprefixed route paths when owner/group mapping is unknown.

### Handler analysis (AST pattern-based)

- Extracted inputs:
  - Path params via `ctx.Param("...")`
  - Query params via `ctx.QueryParam("...")`
  - Headers via `ctx.Request().Header.Get("...")` pattern
  - Request body via `ctx.Bind(...)` with inferred type name
- Extracted outputs:
  - Response status/type from Echo calls:
    - `ctx.JSON(...)`
    - `ctx.String(...)`
    - `ctx.NoContent(...)`
    - `ctx.XML(...)`
    - `ctx.Blob(...)`
    - `ctx.HTML(...)`
- Supports integer statuses and common `http.Status*` constants.

### Normalization and diagnostics

- Builds stable operation IDs as `<METHOD>:<path>`.
- Resolves handlers by function name, with package/receiver hints where available.
- Emits diagnostics for:
  - `handler_not_found`
  - `handler_ambiguous`

### Artifacts emitted

`forj build:api-index` writes:

- Manifest JSON (`build/api_index.json` by default)
- Diagnostics JSON (`build/api_index.diagnostics.json` by default)
- Minimal OpenAPI JSON (`build/openapi.json` by default)

## Canonical Manifest (Current Schema)

Top-level object (`apix.Manifest`):

- `version` (currently `"1"`)
- `operations[]`
- `schemas[]`
- `diagnostics[]`

Operation fields (`apix.Operation`):

- `id`
- `method`
- `path`
- `handler`:
  - `expression`
  - `package`
  - `receiver`
  - `function`
  - `file`
  - `line`
- `middleware[]` (present in schema; not populated yet)
- `inputs`:
  - `path_params[]`
  - `query_params[]`
  - `headers[]`
  - `body` (`type_name`, `source`, `confidence`)
- `outputs`:
  - `responses[]` (`status_code`, `type_name`, `source`, `confidence`)

Diagnostic fields (`apix.Diagnostic`):

- `severity`
- `code`
- `message`
- `file`
- `line`
- `operation`

Schema entries (`apix.Schema`):

- `name`
- `kind` (`object|array|map|unknown`)
- `confidence`

## OpenAPI Output (Current)

- Generates a minimal OpenAPI 3.0.3 document.
- Includes path + method operations and response codes.
- Response payload type names are carried in `x-forj-type`.
- Does not yet generate full parameter/body component schemas.

## Known Gaps

1. Middleware discovery is not implemented.
2. Confidence/provenance is field-level only in selected places, not universal.
3. No annotation/override layer yet.
4. No strict/fail threshold mode yet.
5. No package include/exclude filters yet.
6. Type resolution is AST-based and heuristic, not full semantic typing.

## Planned Next Phases

### Phase 2

- Improve request/response schema extraction.
- Expand response inference coverage.
- Start middleware capture.

### Phase 3

- Annotation override layer.
- Security/auth hints.
- Richer OpenAPI emission from manifest schemas.

### Phase 4

- Strict mode and CI enforcement thresholds.
- Incremental indexing/caching for larger projects.

## Package Layout (Flat)

- `internal/apix/indexer.go` (entrypoint orchestration)
- `internal/apix/discovery.go` (route discovery)
- `internal/apix/router_mapping.go` (group-owner mapping)
- `internal/apix/analyze.go` (handler AST analysis)
- `internal/apix/normalize.go` (route+handler assembly)
- `internal/apix/schema.go` (schema collection)
- `internal/apix/emit_json.go` (artifact writing)
- `internal/apix/emit_openapi.go` (OpenAPI projection)
- `internal/apix/diagnostics.go` (diagnostic model)
- `internal/apix/types.go` (manifest types)

CLI wiring:

- `internal/forj/api_index_cmd.go`
- `internal/forj/build_cmd.go`

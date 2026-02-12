# Forj API Index Design

## Feature Name

- Product name: **Forj API Index**
- CLI command: **`forj build:api-index`**
- Optional shorthand/branding: **`apix`**

## Goal

Build a Forj build layer that parses source code, inspects all HTTP routes and handlers, and automatically produces comprehensive API metadata for downstream consumers such as:

- DevConsole route explorer and request tooling
- OpenAPI/Swagger generation
- Future SDK/codegen features

## Scope (Design Only)

This document defines architecture, data model, extraction strategy, and rollout phases. It does not define implementation details beyond design-level decisions.

## Core Principles

1. **Single source of truth**
   - Generate one canonical API manifest.
   - DevConsole and Swagger read from that manifest.

2. **Static-first, pragmatic**
   - Use static analysis for broad coverage.
   - Accept that 100% inference is not always possible in Go.

3. **Confidence-aware metadata**
   - Every inferred value should carry confidence and provenance.
   - Unknowns must be explicit.

4. **Composable output**
   - Keep extraction and emitters separated.
   - Add new consumers without re-parsing source.

## High-Level Flow

1. Run `forj build:api-index`
2. Parse project packages with `go/packages` (`NeedSyntax`, `NeedTypes`, `NeedTypesInfo`)
3. Discover route registrations and route groups
4. Resolve handler symbols and inspect AST/type info
5. Infer request/response contracts and middleware metadata
6. Emit canonical manifest + diagnostics
7. Optional: emit OpenAPI from manifest

## Proposed Architecture

Implementation note: package layout is flat. Keep all indexer code in a single package (`internal/apix`) and split by files, not subpackages.

### 1) Discovery Layer

Responsibilities:

- Find route definitions from known Forj/Echo patterns (`http.NewRoute`, `RouteGroup`, Echo `Add`, group prefixes)
- Build full route table:
  - method
  - path
  - effective prefix
  - handler symbol
  - middleware symbols
  - source location

Output:

- `[]RouteRef`

### 2) Handler Analyzer

Responsibilities:

- Resolve handler function/method definitions
- Inspect handler bodies to infer:
  - request inputs
  - status code outputs
  - response body types (where possible)
  - common error paths

Signals to inspect:

- Echo APIs (`c.Param`, `c.QueryParam`, `c.Bind`, `c.JSON`, `c.String`, `c.NoContent`)
- `c.Request().Header.Get(...)`
- JSON decode patterns (`json.Decoder.Decode`)
- Form/multipart access patterns

Output:

- `HandlerContract`

### 3) Schema Resolver

Responsibilities:

- Expand Go struct types into JSON-like schema metadata
- Respect tags (`json`, validation tags where useful)
- Track optional/nullable signals conservatively

Output:

- reusable `SchemaRef` map used by operations

### 4) Normalizer

Responsibilities:

- Merge route + handler + schema facts
- Resolve duplicates, conflicts, and precedence
- Attach confidence/provenance metadata

Output:

- canonical `ApiIndexManifest`

### 5) Emitters

Responsibilities:

- Convert manifest to consumer-specific formats:
  - DevConsole payload
  - OpenAPI document
  - (future) SDK config / test scaffolding

## Canonical Manifest (IR) Proposal

`ApiIndexManifest`:

- `version` (schema version)
- `generated_at`
- `operations[]`
- `schemas{}`
- `diagnostics[]`

`Operation` fields:

- `id`
- `method`
- `path`
- `group_prefix`
- `handler`:
  - package
  - receiver
  - function
  - source file/line
- `middlewares[]`
- `inputs`:
  - `path_params[]`
  - `query_params[]`
  - `headers[]`
  - `body` (schema ref or unknown)
- `outputs`:
  - `responses[]` (status + schema ref + content type where known)
- `security_hints[]`
- `confidence`
- `provenance[]`

`Diagnostic` fields:

- severity (`info|warn|error`)
- code
- operation id
- message
- location

## Inference Confidence Model

Each inferred field should include:

- `source`: `explicit|inferred|unknown`
- `confidence`: `high|medium|low`

Examples:

- `c.QueryParam("page")` => query param `page`, high confidence
- dynamic key from variable => query param unknown key, low confidence diagnostic

## Annotation / Override Layer (Optional but Recommended)

Static analysis alone will miss some dynamic/abstracted patterns. Add optional handler/DTO annotations for override-only cases.

Use cases:

- explicitly declare undocumented query params
- add non-obvious response codes
- declare auth/security requirements
- resolve wrapper/helper abstractions

Design requirement:

- annotations should augment or override inferred values, not replace inference entirely

## Non-Goals (V1)

- Perfect reflection-aware inference
- Runtime tracing-based schema discovery
- Full semantic validation of all business rules

## Rollout Plan

### Phase 1 (MVP)

- Route discovery
- Handler identity
- Path/query extraction for direct Echo calls
- Basic response status extraction
- Canonical manifest + diagnostics

### Phase 2

- Request body and response body schema extraction
- Better status code mapping
- Middleware mapping improvements

### Phase 3

- Security/auth inference
- Annotation support for overrides
- OpenAPI emitter parity with manifest

### Phase 4

- CI enforcement mode:
  - fail on unresolved critical metadata
  - configurable thresholds for unknowns

## CLI Proposal

- `forj build` (runs all build pipelines)
- `forj build:api-index`
  - Generates canonical manifest and diagnostics

Suggested flags:

- `--out build/api_index.json`
- `--diag-out build/api_index.diagnostics.json`
- `--openapi-out build/openapi.json`
- `--strict` (enables failure thresholds)
- `--include`/`--exclude` package globs

## Package Layout (Flat)

Core implementation package:

- `internal/apix`

Suggested file layout:

- `internal/apix/indexer.go` (public entrypoints)
- `internal/apix/discovery.go` (route extraction)
- `internal/apix/analyze.go` (handler AST/type analysis)
- `internal/apix/schema.go` (DTO/schema extraction)
- `internal/apix/normalize.go` (manifest assembly)
- `internal/apix/emit_json.go` (canonical manifest output)
- `internal/apix/emit_openapi.go` (OpenAPI adapter)
- `internal/apix/diagnostics.go` (diagnostics/confidence tracking)
- `internal/apix/types.go` (IR structs)

CLI wiring:

- `internal/forj/api_index_cmd.go`

## Integration Targets

### DevConsole

- Consume `ApiIndexManifest.operations`
- Display route contracts with inferred params/body/response
- Show diagnostics inline (unknown/low-confidence parts)

### OpenAPI/Swagger

- Generate OpenAPI paths/operations from manifest
- Map reusable schemas from `schemas{}`
- Carry confidence notes as extension fields (e.g. `x-forj-confidence`)

## Risks and Mitigations

1. **Incomplete inference due to indirection**
   - Mitigation: annotation layer + diagnostics

2. **False confidence**
   - Mitigation: strict provenance/confidence model

3. **Emitter drift**
   - Mitigation: all emitters consume same canonical manifest

4. **Performance on large projects**
   - Mitigation: package filtering + incremental caching (future)

## Success Criteria

- Generated manifest covers all registered routes
- DevConsole and OpenAPI can be generated from manifest only
- Unknowns are explicitly reported, not silently omitted
- Teams can improve metadata quality iteratively without hand-maintaining specs

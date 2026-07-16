# Go MCP Server Design

## Purpose

This design defines how GoForj should approach a Go-native MCP server integration.

The immediate goal is not just "support MCP somehow." The goal is to give GoForj a clear, reusable, production-minded way to expose developer-assistance capabilities for teams building applications on the GoForj framework.

In practice, this MCP server should help developers:

- inspect and understand their generated app
- look up GoForj documentation and library usage
- inspect routes, schedules, queues, caches, storage, and config
- run safe validation and diagnostics
- inspect database connections and run bounded read-oriented queries
- understand how GoForj APIs are intended to be used

This is not a generic MCP server for arbitrary automation. It is a framework-aware developer companion for GoForj applications.

This should be treated as framework infrastructure, not as a one-off integration.

## Working Name

The working product name for this MCP server is `GoForj Atlas`.

`Atlas` fits the product direction well because this server is meant to help developers navigate and understand both:

- the GoForj framework
- the application's runtime wiring
- the project's operational surfaces

It complements `Lighthouse` without overlapping with its control-plane metaphor.

## Why This Matters

MCP is becoming a standard way for AI tools and local agents to discover:

- tools
- resources
- prompts
- structured capabilities
- execution surfaces

For GoForj, that creates several important opportunities:

- generated apps can expose safe, explicit operational capabilities
- generated apps can expose framework-aware developer tooling without inventing custom ad hoc CLIs for every concern
- Lighthouse can eventually act as an MCP-aware control plane
- local development workflows can become more agent-native
- framework primitives can be surfaced to AI tooling through stable contracts instead of ad hoc shell commands

If GoForj does this well, MCP becomes another framework boundary:

- explicit
- typed
- testable
- safe by default

## Reference Implementation Assessment

`github.com/mark3labs/mcp-go` looks like a strong fit for what GoForj needs.

It already covers the categories we would otherwise spend time rebuilding:

- server construction
- tool, resource, and prompt registration
- stdio support
- additional transport options
- handler-oriented APIs
- hooks and middleware patterns
- examples and testability

That changes the recommendation in this design:

- GoForj should not build a new general-purpose MCP runtime by default
- GoForj should adopt `mark3labs/mcp-go`
- GoForj should focus its own effort on framework integration, capability design, policy, and generated wiring

## Design Goals

The Go MCP server approach should aim for:

- a Go-native implementation
- explicit typed registration of tools/resources/prompts
- transport support for local stdio first
- room for HTTP/SSE or streamable HTTP later
- strong handler boundaries
- framework-friendly dependency injection
- safe-by-default capability exposure
- easy testing of handlers without spinning up full transports
- support for both framework-owned and app-owned MCP modules

## Non-Goals

This design does not aim to:

- implement every MCP transport on day one
- turn MCP into arbitrary remote shell access
- auto-expose the whole app/runtime with no curation
- make MCP the only automation interface
- couple MCP tightly to Lighthouse UI concerns

## Product Direction

GoForj should support MCP at two levels:

### 1. MCP runtime adoption

GoForj should use `mark3labs/mcp-go` as the underlying MCP server implementation unless a concrete gap proves it is insufficient.

That means GoForj does not need to own:

- the generic MCP runtime
- transport protocol handling
- baseline tool/resource/prompt primitives
- core request routing

### 2. GoForj integration layer

GoForj should own:

- component selection
- generated wiring
- framework-owned tool/resource definitions
- capability pack registration
- framework policy boundaries
- integration with runtime, config, storage, scheduler, jobs, and later observability

Suggested generated app integration:

```text
internal/mcp/
  server.go
  tools.go
  resources.go
  prompts.go
```

That gives generated apps a clear extension point while keeping GoForj focused on framework value instead of generic protocol ownership.

## Package Boundary

The boundary should follow the same pattern as other GoForj primitives, but with clearer ownership.

### Upstream MCP runtime

`mark3labs/mcp-go` should own:

- server runtime
- protocol transport mechanics
- core registration primitives
- baseline MCP lifecycle concerns

### Generated app integration

Suggested shape:

```text
internal/mcp/
  server.go
  registry.go
  tool_project.go
  tool_routes.go
  tool_schedules.go
  tool_jobs.go
  resource_project_config.go
  resource_env.go
```

GoForj should own:

- capability pack composition
- generated app wiring
- framework-aware handlers
- policy decisions
- app extension points

The generated app layer should decide what to expose.

If GoForj eventually needs a reusable helper package, it should be a thin integration-oriented package, not a full replacement MCP runtime.

## Core Concepts

The MCP server model should center around four things:

- server
- transports
- registries
- handlers

### Server

The server coordinates:

- transport lifecycle
- request routing
- capability advertisement
- handler execution

Suggested shape:

```go
type Server struct {
    info       ServerInfo
    tools      *ToolRegistry
    resources  *ResourceRegistry
    prompts    *PromptRegistry
    logger     Logger
}
```

### Registry

`Registry` is still the right word for GoForj's capability composition layer even if the underlying implementation comes from `mark3labs/mcp-go`.

Inside GoForj, registry can still mean:

- explicit capability registration
- discoverability
- stable lookup
- server-owned capability composition

### Handler

Handlers should be plain Go functions behind typed interfaces.

Example:

```go
type ToolHandler interface {
    Invoke(context.Context, ToolCall) (ToolResult, error)
}
```

This makes testing straightforward and keeps protocol plumbing separate from business logic.

### Transport

Transport should still be treated as an explicit architectural concern, but GoForj should prefer the transport abstractions already provided by `mark3labs/mcp-go` instead of inventing new ones up front.

That allows GoForj to stay focused on:

- stdio for local agent integration
- future HTTP/SSE support
- future streamable HTTP support
- capability behavior rather than protocol plumbing

## Transport Strategy

### v1: stdio first

GoForj should start with stdio transport.

Why:

- lowest friction for local AI tools
- most common first MCP integration path
- easiest to reason about
- enough to validate the server model

Good initial use cases:

- local coding agents
- CLI-driven assistants
- editor integrations

### v2: HTTP/SSE or streamable HTTP

Once the integration model is proven, GoForj can enable additional upstream-supported transports.

Those become useful for:

- Lighthouse integration
- remote process boundaries
- multi-service tool exposure

This should be a later step, not a blocker for the initial architecture.

## Registration Model

Registration should be explicit and typed.

Avoid:

- giant generic maps of `string -> any`
- reflection-driven magical registration
- package-global auto-registration

Prefer:

The same principle should apply whether GoForj uses upstream registration directly or builds a thin internal adapter around it.

## Tool Model

Tools are active operations.

Good GoForj tool examples:

- list routes
- list schedules
- inspect queue status
- inspect cache/store metadata
- inspect configured database connections
- run bounded read-only database queries
- validate storage, cache, and database connectivity
- look up framework docs for a package, concept, or API
- explain how to use a GoForj library surface
- validate project wiring or environment configuration

Bad examples:

- unrestricted shell execution
- raw filesystem mutation with no policy
- "do anything" catch-all handlers

Tool handlers should:

- validate inputs
- execute one bounded responsibility
- return structured results
- be policy-aware

## Resource Model

Resources are read-oriented, addressable data surfaces.

Good GoForj resource examples:

- project config
- about/runtime info
- route inventory
- schedule inventory
- selected env file metadata
- generated app component inventory
- cache inventory
- storage inventory
- database inventory
- framework documentation indexes
- package and API reference surfaces

Resource handlers should be:

- stable
- predictable
- explicit about shape and pagination if needed

## Prompt Model

Prompt support is useful, but not essential for v1.

Prompt use cases in GoForj could include:

- "summarize project wiring"
- "explain route surface"
- "review queue configuration"
- "inspect scheduler setup"

Prompts should come after:

- server
- tools
- resources

That ordering matters because tools/resources are the more foundational capability.

## Execution and Safety Model

This is the most important architectural constraint.

GoForj must not treat MCP as a hidden privileged backdoor.

Safe-by-default principles:

- no arbitrary shell execution by default
- no arbitrary file write tools by default
- no mutating database queries by default
- explicit registration only
- capability exposure should be opt-in
- dangerous operations should be clearly separated from read-only ones
- transport exposure should not imply trust

If GoForj later adds mutating tools, they should be modeled intentionally:

- scoped
- named clearly
- policy-gated
- auditable

## Configuration Model

Atlas support should eventually be a first-class framework capability.

Suggested future component:

- `Atlas`

This component should control:

- whether MCP server wiring is rendered
- which transport(s) are enabled
- which framework-owned capability packs are enabled

Suggested future config shape:

```yaml
atlas:
  enabled: true
  transport: stdio
  tools:
    routes: true
    schedules: true
    jobs: true
  resources:
    project_config: true
    about: true
```

Important:

The user config should represent enabled capability packs, not internal dependency graphs or protocol mechanics.

Internally this still remains MCP-based. `Atlas` is the product and framework-facing name layered on top of the MCP protocol.

## Capability Packs

Capability packs are a useful abstraction for GoForj integration.

Instead of toggling every individual tool separately, GoForj can group framework-owned MCP surfaces by concern.

Examples:

- project
- routes
- schedules
- jobs
- config
- docs
- storage
- cache
- database
- validation
- observability

This matches how users think better than dozens of raw individual toggles.

## Suggested v1 Surface Area

The first useful GoForj MCP surface should stay small, obviously valuable, and directly useful during app development.

Suggested v1:

### Tools

- `project.routes.list`
- `project.schedules.list`
- `project.jobs.list`
- `project.about.inspect`
- `project.database.connections.list`
- `project.database.query.read`
- `project.cache.inspect`
- `project.storage.inspect`
- `project.validate.runtime`
- `goforj.docs.search`
- `goforj.docs.api.explain`

### Resources

- `project://config`
- `project://about`
- `project://routes`
- `project://schedules`
- `project://caches`
- `project://storages`
- `project://databases`
- `goforj://docs/index`
- `goforj://docs/packages`

This is enough to prove:

- typed registration
- transport handling
- resource/tool distinction
- framework integration value
- developer-assistance value

## Suggested Go API Shape

### Direct upstream usage

```go
server := mcp.NewServer("goforj-app", "0.1.0")
```

### GoForj-owned registration layer

```go
func RegisterFrameworkTools(server *mcp.Server, deps Deps) error {
    return registerProjectRoutes(server, deps)
}
```

### Run

```go
if err := server.ServeStdio(ctx); err != nil {
    return err
}
```

The exact API shape should follow the upstream package closely enough that GoForj is not fighting the library.

If GoForj introduces its own abstraction, it should be very thin and exist mainly to:

- group capability packs
- inject framework dependencies
- centralize policy and naming conventions
- avoid scattering upstream MCP wiring throughout the codebase

## Request Handling Direction

The server should not scatter protocol routing logic across transports.

Instead:

- transport parses inbound frames/messages
- server handles protocol method dispatch
- handler adapters invoke registered capability handlers

That keeps transports thin and preserves one protocol execution model.

## Error Model

Errors should be classified clearly.

At minimum:

- invalid request
- unknown capability
- invalid input
- execution failure
- internal server failure

The Go API should expose typed errors where useful, but avoid overengineering.

Example:

```go
var ErrToolNotFound = errors.New("tool not found")
```

The protocol layer can translate internal errors into MCP-compatible response errors.

## Testing Strategy

This should be testable at three levels.

### Unit tests

Test:

- capability pack registration
- handler invocation
- input validation
- capability advertisement

### Integration tests

Test:

- stdio transport end-to-end
- generated app MCP startup
- framework capability packs
- policy behavior for exposed tools/resources
- editor or agent compatibility smoke tests where practical

The design should make it easy to test handlers without requiring a subprocess transport harness for every case.

Upstream library examples and test helpers should be treated as an advantage to lean on, not something to replace.

## Relationship To Lighthouse

Lighthouse should not be the only consumer of this work.

But Lighthouse is a very natural consumer.

Possible future roles:

- Lighthouse as an MCP client
- Lighthouse as an MCP capability browser
- Lighthouse as an execution UI for safe tool invocations

That is useful, but it should sit on top of the MCP contract, not define it.

This is the correct layering:

- upstream MCP runtime
- GoForj-generated server integration
- optional Lighthouse UX on top

## Relationship To Future AI Workflows

This design gives GoForj a principled way to expose framework-aware developer assistance to agents.

That matters because AI interactions are much better when they are:

- typed
- bounded
- discoverable
- safe

instead of:

- fragile shell commands
- undocumented REST endpoints
- hidden internal knowledge

MCP should become one of the framework’s official machine-facing surfaces for helping developers build and operate GoForj applications.

## Open Questions

These should be decided before implementation broadens too much:

- should GoForj use `mark3labs/mcp-go` directly inside generated integration code, or wrap it in a thin internal adapter first?
- should prompts ship in v1 or wait until tools/resources are stable?
- should GoForj render MCP into apps as an explicit component or as a later optional integration?
- which upstream-supported transport should come second after stdio?
- what policy model should govern mutating tools if they are added later?
- what concrete gap would justify building more GoForj-owned MCP infrastructure later?

## Recommended First Implementation Slice

The first implementation should stay narrow and integration-first.

Recommended slice:

1. adopt `mark3labs/mcp-go` for the underlying server runtime
2. build a small GoForj registration layer for framework capability packs
3. render minimal MCP server wiring into apps
4. expose a small set of read-oriented project/routes/schedules/docs capabilities
5. add tests for registration, capability behavior, and stdio end-to-end
6. verify the integration shape before introducing any deeper abstraction

This gives GoForj a credible MCP foundation without spending time reimplementing solved protocol concerns.

## Recommendation

GoForj should build MCP support as a reusable framework integration on top of `mark3labs/mcp-go`, not as a brand new general-purpose MCP runtime.

The architecture should be:

- stdio-first
- explicit registration
- registry-centered
- handler-driven
- safe by default
- transport-flexible over time through upstream support

GoForj's ownership should stay concentrated on:

- capability design
- framework-aware handlers
- developer-assistance workflows
- policy boundaries
- generated wiring
- future Lighthouse and agent integrations

That gives GoForj a durable machine-facing capability layer for teams building GoForj apps without taking on unnecessary protocol maintenance.

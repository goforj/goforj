# GoForj Atlas Agent Integration Design

## Purpose

This design defines how GoForj should integrate with local AI agents through
Atlas.

Atlas should be more than an MCP server. It should be a project installer for
agent-native framework assistance. It should detect local AI agents, write
framework-aware guidelines, sync skills, configure MCP, and expose curated
project tools such as docs search, app info, logs, routes, and read-only
database access.

The working GoForj name remains `Atlas`.

## Reference material

- Local GoForj MCP design: `docs/designs/go-mcp-server-design.md`

## Core recommendation

GoForj should split Atlas into two layers:

```text
GoForj Atlas
  Agent integration layer
    install/update commands
    agent detection
    guideline writing
    skill writing
    MCP config writing

  MCP runtime layer
    project/app inspection
    documentation search
    route and schedule inspection
    read-only database tools
    logs and browser diagnostics
```

The existing Go MCP server design covers the runtime layer. This design covers
the product and installation layer that makes that runtime useful in real local
agent workflows.

## Product shape

Atlas should make GoForj projects understandable to local agents without asking
users to manually copy prompt text, hand-edit MCP config, or remember framework
conventions.

The user-facing flow should be:

```bash
forj atlas:install
```

That should detect supported agents, ask which integration surfaces to install,
and write project-local files.

Suggested install surfaces:

- guidelines
- skills
- MCP server config

Suggested follow-up commands:

```bash
forj atlas:update
forj atlas:list-skills
forj atlas:add-skill <path-or-repository>
forj atlas:mcp
```

`atlas:mcp` should start the project-local Atlas MCP server over stdio.

Use `atlas` as the CLI namespace. `assist` is friendly, but `atlas` already
connects to the existing MCP design and gives GoForj one durable name for
agent-facing project navigation.

## Why this fits GoForj

GoForj now has a stronger project model:

```text
project
  app
    runtime
    runtime
  app
    runtime
```

That is exactly the kind of structure agents need help navigating. Atlas should
give agents stable answers to questions like:

- Which apps exist in this project?
- Which runtime is active?
- Where are routes registered?
- Where should a generated controller be wired?
- Which app owns this migration?
- Which command should inspect routes for a named app?
- What docs apply to this exact GoForj version?

Without Atlas, agents will infer these answers from file search and often get
them wrong. With Atlas, the framework provides the answers directly.

## Agent integration model

### Install command

Atlas should provide an install command that discovers the environment, asks
which surfaces to configure, detects agents, writes files, and persists the
selected configuration.

```bash
forj atlas:install
```

Suggested flags:

```bash
forj atlas:install --guidelines
forj atlas:install --skills
forj atlas:install --mcp
forj atlas:install --agent codex
forj atlas:install --all-agents
forj atlas:install --no-interaction
```

The default interactive path should install all three surfaces for detected
project agents.

### Update command

Atlas should provide an update command to re-run discovery and refresh generated
guidelines, skills, and MCP config:

```bash
forj atlas:update
forj atlas:update --discover
forj atlas:update --ignore-skills
```

This is important because GoForj projects change. A project may add a named app,
Vue starter kit, queues, schedules, observability, or a new database driver after
Atlas was first installed.

### Agent adapters

Atlas should model each supported agent as an adapter with:

- system detection
- project detection
- guideline path
- skills path
- MCP config path
- MCP install strategy

Suggested package:

```text
internal/forj/atlas/
  agents/
    agent.go
    detector.go
    codex.go
    claude.go
    copilot.go
    cursor.go
  guidelines/
  skills/
  mcpconfig/
```

Suggested interface:

```go
type Agent interface {
    Name() string
    DisplayName() string
    DetectSystem(context.Context) bool
    DetectProject(root string) bool
    GuidelinesPath(root string) string
    SkillsPath(root string) string
    MCPConfigPath(root string) string
    WriteMCPConfig(root string, server MCPServerConfig) error
}
```

Codex should be first-class:

```text
guidelines: AGENTS.md
skills:     .agents/skills
MCP config: .codex/config.toml
MCP key:    mcp_servers
```

Claude Code should be next:

```text
guidelines: CLAUDE.md
skills:     .claude/skills
MCP config: .mcp.json
```

GitHub Copilot should also be part of the initial adapter set because it has
stable repository instruction files and VS Code workspace MCP config:

```text
guidelines: .github/copilot-instructions.md
skills:     .github/instructions/*.instructions.md
prompts:    .github/prompts/*.prompt.md
MCP config: .vscode/mcp.json
MCP key:    servers
```

Cursor can follow once the core installer is stable.

### Guideline composition

Atlas should compose guidelines from:

- core GoForj conventions
- current GoForj version
- Go version
- selected components
- detected apps and runtimes
- database/queue/cache/storage choices
- frontend starter kit
- local project conventions from `.ai/guidelines`

Core guidelines should teach agents to prefer GoForj `make:*` commands for
scaffolding whenever a matching command exists. Raw file creation should be the
fallback for custom code or unsupported shapes, not the default path for
framework-owned artifacts.

Suggested source layout:

```text
templates/atlas/guidelines/
  foundation.md.tmpl
  goforj.md.tmpl
  multi_app.md.tmpl
  database.md.tmpl
  queues.md.tmpl
  schedules.md.tmpl
  observability.md.tmpl
  vue.md.tmpl
```

Generated guideline files should use a marker block so Atlas can replace its own
content without damaging user-authored notes.

```md
<!-- goforj-atlas:start -->
...
<!-- goforj-atlas:end -->
```

The guidelines should stay short. They should point agents to Atlas tools and
skills instead of trying to encode the entire framework in one giant prompt.

### Skill sync

Atlas should install reusable agent guidance and keep it synchronized per agent.
The source format can stay GoForj-owned, but each adapter should write the
native format expected by that agent.

Suggested built-in skills:

- `goforj-app-architecture`
- `goforj-app-registration`
- `goforj-make-commands`
- `goforj-go-package-design`
- `goforj-migrations`
- `goforj-runtime-workflows`
- `goforj-database-and-data-access`
- `goforj-observability`
- `goforj-vue-starter-kit`
- `goforj-testing-and-validation`

Initial skill catalog:

```text
goforj-app-architecture
  Teaches the project/app/runtime model, default app, named apps, cmd/<app>
  binaries, shared internal packages, and app composition directories.

goforj-app-registration
  Teaches where routes, commands, schedules, lifecycle hooks, jobs, and app
  composition files live. This should prevent agents from writing registration
  code into shared internals when the selected app owns the registration point.

goforj-make-commands
  Teaches agents to prefer forj make:* commands for generated framework code,
  including how app prefixes route output into app/<name> and app-specific Wire
  files. This skill should explicitly discourage manually creating controllers,
  jobs, schedules, commands, models, repositories, or migrations when a matching
  make command exists.

goforj-go-package-design
  Teaches agents GoForj's Go package design style. Code should usually be
  cohesive within package scope instead of deeply nested into Java/PHP-style
  class folders. This skill should cover package-local interfaces, constructors,
  services, controllers, repositories, jobs, schedules, tests, and how grouped
  make-command names map to package boundaries. It should also teach that
  package-scoped implementation code still registers through the selected app
  composition files. Example workflows should include forj make:command
  billing:reports:sync and forj marketplace make:command billing:reports:sync.

goforj-migrations
  Teaches raw DDL migration conventions, app-scoped migration paths, connection
  names, rollback expectations, and the difference between default and named
  app migration ownership.

goforj-runtime-workflows
  Teaches build, run, dev, route:list, scheduler, jobs, and app-prefixed command
  workflows. This should include when to use forj <app> <command> versus the
  built ./bin/<app> binary.

goforj-database-and-data-access
  Teaches repository/service boundaries, generated database config, query
  access patterns, and how to inspect schema safely through Atlas tools.

goforj-observability
  Teaches logs, metrics labels, app/runtime identity, Lighthouse boundaries,
  Grafana dashboards, and local development diagnostics.

goforj-vue-starter-kit
  Teaches generated Vue layout, local auth defaults, frontend env files,
  embedded frontend assets, and app-specific frontend paths.

goforj-testing-and-validation
  Teaches expected validation commands, test render rules, Go cache env vars,
  integration-test boundaries, and how to verify framework changes without
  rendering inside the GoForj repo.
```

Suggested source layout:

```text
templates/atlas/skills/
  goforj-app-architecture/SKILL.md
  goforj-app-registration/SKILL.md
  goforj-make-commands/SKILL.md
  goforj-go-package-design/SKILL.md
  goforj-migrations/SKILL.md
  goforj-runtime-workflows/SKILL.md
  goforj-database-and-data-access/SKILL.md
  goforj-observability/SKILL.md
  goforj-vue-starter-kit/SKILL.md
  goforj-testing-and-validation/SKILL.md
```

User-defined skills should live under:

```text
.ai/skills/
```

Atlas should copy or symlink those into each agent's skill directory, depending
on what the agent supports and what is safest on the current platform.

Agents that do not support `SKILL.md` directly should get equivalent native
files. For GitHub Copilot, Atlas should map reusable skills into
`.github/instructions/*.instructions.md` and workflow prompts into
`.github/prompts/*.prompt.md`.

Suggested Copilot output:

```text
.github/instructions/goforj-app-architecture.instructions.md
.github/instructions/goforj-app-registration.instructions.md
.github/instructions/goforj-make-commands.instructions.md
.github/instructions/goforj-go-package-design.instructions.md
.github/instructions/goforj-migrations.instructions.md
.github/instructions/goforj-runtime-workflows.instructions.md
.github/instructions/goforj-database-and-data-access.instructions.md
.github/instructions/goforj-observability.instructions.md
.github/instructions/goforj-vue-starter-kit.instructions.md
.github/instructions/goforj-testing-and-validation.instructions.md

.github/prompts/goforj-create-app.prompt.md
.github/prompts/goforj-add-route.prompt.md
.github/prompts/goforj-add-job.prompt.md
.github/prompts/goforj-add-schedule.prompt.md
.github/prompts/goforj-review-package-design.prompt.md
.github/prompts/goforj-debug-runtime.prompt.md
.github/prompts/goforj-review-change.prompt.md
```

Prompt files should be workflow-oriented and short. Instruction files should be
standing guidance that Copilot can apply while editing matching files.

### MCP config writing

Atlas should write agent-specific MCP config instead of asking the user to do it
by hand.

GoForj should install one project-level MCP server by default:

```toml
[mcp_servers.goforj-atlas]
command = "forj"
args = ["atlas:mcp"]
cwd = "/absolute/path/to/project"
```

Adapters should write the same server shape using each agent's native config
format. For VS Code GitHub Copilot, that means workspace JSON:

```json
{
  "servers": {
    "goforj-atlas": {
      "command": "forj",
      "args": ["atlas:mcp"],
      "cwd": "/absolute/path/to/project"
    }
  }
}
```

`forj atlas:mcp` should be a framework-owned command that starts the
project-local MCP server without depending on a prebuilt app binary. The
generated project can still own the app-aware MCP wiring and handlers, but the
agent config should point at the stable framework CLI command.

The server should be app-aware internally. It should not install one MCP server
per app by default.

Tools should accept an `app` argument where app selection matters:

```json
{
  "app": "marketplace"
}
```

When omitted, the default app should be used.

## Product boundaries

Atlas should be GoForj-shaped. It should center on Go modules, GoForj
components, generated project discovery, apps, runtimes, and app registration
points.

The MVP should not include:

- arbitrary shell execution
- arbitrary Go code execution
- write-capable MCP tools
- large generated prompt files that duplicate the documentation
- hard requirements that agents must search docs before every small edit

One MCP server per project is simpler than one MCP server per app because Atlas
tools can accept an `app` argument.

## MCP runtime capabilities

The Go MCP server design already recommends `mark3labs/mcp-go` and a stdio-first
runtime. Atlas should build on that.

The first Atlas MCP tools should be read-only.

### application-info

Returns project, app, runtime, component, and version information.

This should be the first tool agents are told to call in a new chat.

Example shape:

```json
{
  "project": "commerce",
  "goforj_version": "0.12.0",
  "go_version": "1.24",
  "apps": [
    {
      "name": "app",
      "default": true,
      "runtimes": ["http", "jobs", "scheduler"]
    },
    {
      "name": "marketplace",
      "default": false,
      "runtimes": ["http", "jobs"]
    }
  ],
  "components": ["web-api", "jobs", "observability", "vue"]
}
```

### search-docs

Searches GoForj documentation for the current project version.

GoForj should embed the Markdown documentation that ships with the installed
`forj` version. This is a major strength of Atlas: local docs search can be
versioned, deterministic, offline, and tied directly to the CLI that is running
the MCP server.

MVP sources, in priority order:

- embedded Markdown docs from the installed `forj` binary
- checked-out `goforj-docs` index when available
- generated project docs and API index

Development override:

```bash
GOFORJ_DOCS_PATH=/workspace/code/goforj-docs forj atlas:mcp
```

That lets framework and docs development use the live docs repo while normal
projects use the embedded docs that match their installed CLI.

Optional later source:

```text
https://docs.goforj.dev/api/search
```

A hosted endpoint can help with cross-version browsing, richer global search,
or docs that were published after a local CLI install. It should not replace the
embedded docs as the default source.

The important part is local version awareness. Agents should not use docs for a
newer or older GoForj API unless no exact match exists.

### read-doc-section

Returns one bounded section from an embedded Markdown document.

Inputs:

- `path`
- `heading`
- `token_limit`

Atlas should parse Markdown into heading trees and return only the requested
section. It should not return an entire document unless the document is already
small enough for the requested token limit.

### read-doc-neighborhood

Returns a bounded section plus nearby context.

Inputs:

- `path`
- `heading`
- `before`
- `after`
- `token_limit`

This is useful when the target section depends on setup notes immediately before
it or follow-up examples immediately after it.

### list-doc-headings

Returns the heading tree for a Markdown document.

Agents should use this when they know the file they need but not the exact
section. It gives them a cheap way to inspect structure before requesting
content.

### explain-api

Maps a GoForj command, symbol, generated file, or concept to the most relevant
docs sections.

Examples:

- `forj marketplace make:controller checkout`
- `app/marketplace/routes.go`
- `cmd/marketplace/main.go`
- `migrations/marketplace/default`

This should return a small set of doc paths and headings, not long prose.

### Docs retrieval model

Atlas should embed all Markdown docs and make the MCP tools responsible for
token discipline.

Recommended implementation:

- embed Markdown files with `go:embed`
- embed a docs manifest with GoForj version, docs revision, and file checksums
- parse frontmatter, titles, headings, and body sections
- chunk by Markdown section rather than arbitrary character count
- index path, title, headings, tags, components, and app/runtime relevance
- rank title and heading matches above body matches
- cap result counts and token output aggressively
- include related sections instead of dumping larger documents
- lazy-load section bodies if startup time or memory ever becomes noticeable

Embedding all Markdown keeps the implementation simple and deterministic. The
splicing tools keep the agent experience efficient.

The MCP server should report the docs revision in `application-info` so agents
can tell which docs set they are using.

### project-layout

Returns the app-aware file layout and registration points.

This should explain:

- default app composition in `app/`
- named app composition in `app/<name>/`
- default entrypoint in `cmd/app/main.go`
- named app entrypoints in `cmd/<name>/main.go`
- shared business logic in `internal/`

### route-list

Returns routes for the selected app.

This should mirror the app-aware behavior users get from:

```bash
forj route:list
forj marketplace route:list
```

### schedule-list

Returns schedules for the selected app.

### command-list

Returns registered shell commands for the selected app.

### database-connections

Returns safe database connection metadata. It should not expose secrets.

### database-schema

Returns table, column, index, and relationship information for a selected
connection.

### database-query

Allows bounded read-only queries.

This tool should allow read statements only and reject mutation keywords.

Allowed examples:

- `SELECT`
- `EXPLAIN`
- `SHOW`
- `DESCRIBE`
- `WITH` when the final statement is read-only

Rejected examples:

- `INSERT`
- `UPDATE`
- `DELETE`
- `DROP`
- `ALTER`
- `TRUNCATE`
- mutation statements hidden inside CTEs

The tool should enforce:

- query timeout
- row limit
- connection allowlist
- no secret leakage

### read-log-entries

Returns recent framework log entries.

### last-error

Returns the most recent application error with surrounding context.

### browser-logs

Returns browser console logs captured during local development.

This should probably share infrastructure with Lighthouse, but Atlas should not
require the Lighthouse UI to be open.

### get-absolute-url

Converts a relative app path into the local absolute URL for the selected app.

This helps agents and browser tools avoid guessing ports.

## Tool execution model

Some MCP tools may need a fresh application boot to avoid leaking stale
environment state between calls.

GoForj should not use subprocess execution everywhere, but the concern is real.

Recommended approach:

- run lightweight project inspection in-process
- use explicit timeouts on every tool
- reload project config per request
- isolate database and runtime-backed tools behind narrow services
- consider subprocess execution for tools that need a full generated app boot

The MVP should avoid write tools and arbitrary code execution entirely.

Do not add arbitrary runtime code execution in the first version.

## State and configuration

GoForj should avoid bloating runtime project config with agent-install state.

Recommended file:

```text
.goforj/atlas.json
```

Suggested shape:

```json
{
  "version": 1,
  "features": {
    "guidelines": true,
    "skills": true,
    "mcp": true
  },
  "agents": ["codex", "claude"],
  "skills": [
    "goforj-app-architecture",
    "goforj-make-commands",
    "goforj-go-package-design",
    "goforj-migrations"
  ],
  "last_discovered": {
    "apps": ["app", "marketplace"],
    "components": ["web-api", "jobs", "observability", "vue"]
  }
}
```

This file is project-owned and should be committed by default because it
describes which Atlas surfaces the project expects to keep in sync.

Machine-specific state should not go in this file.

## Multi-app behavior

Atlas should treat multi-app support as a first-class part of the product.

The MCP server is project-level. Tools are app-aware.

Default behavior:

```bash
forj atlas:mcp
```

starts one server for the current project.

The agent can then ask:

```json
{
  "tool": "route-list",
  "arguments": {
    "app": "marketplace"
  }
}
```

Guidelines should explicitly teach the agent:

- unprefixed commands target the default app
- `forj <app> <command>` targets a named app
- `forj <app> make:*` writes to that app's registration points
- generated app composition lives in `app/` or `app/<name>/`
- binaries live in `cmd/app` and `cmd/<name>`

This is one of the biggest practical wins for Atlas because agents otherwise
will confuse shared internals with app composition points.

## Relationship to Lighthouse

Lighthouse and Atlas should stay separate but share underlying services where it
makes sense.

```text
Lighthouse
  human-facing local control plane
  runtime inspection UI
  logs, metrics, traces, execution views

Atlas
  agent-facing MCP and guidance layer
  tools, resources, prompts, skills
  docs search and project introspection
```

Shared pieces may include:

- route inspection
- runtime identity
- log access
- browser log capture
- metrics metadata
- app/runtime discovery

Atlas should not depend on the Lighthouse UI process.

## Security model

Atlas should be safe by default.

MVP rules:

- stdio transport only
- local project only
- read-only tools only
- no arbitrary shell tool
- no arbitrary Go execution tool
- no mutation SQL
- no secret values in config output
- explicit timeouts on every tool
- bounded output sizes
- app arguments validated against known apps

Later write tools may be possible, but they should be explicit opt-in capability
packs, not part of the default install.

## Testing and evaluation

Atlas needs normal unit and integration tests, but that is not enough. The
product succeeds only if agents consistently find the right framework context,
write code into the right app, and avoid wasting tokens.

Testing should use a layered approach:

```text
deterministic tests
  fast CI checks for writers, config, docs retrieval, MCP tools, and safety

scenario tests
  rendered GoForj projects with app-aware workflows and expected outputs

agent evaluation
  optional replay/eval runs that measure whether local agents make better
  framework decisions with Atlas installed
```

### Agent adapter tests

Each adapter should have golden tests for generated files.

Test cases:

- Codex writes `AGENTS.md`, `.agents/skills`, and `.codex/config.toml`
- Claude writes `CLAUDE.md`, `.claude/skills`, and `.mcp.json`
- GitHub Copilot writes `.github/copilot-instructions.md`,
  `.github/instructions`, `.github/prompts`, and `.vscode/mcp.json`
- existing user content is preserved outside Atlas marker blocks
- existing Atlas marker blocks are replaced instead of duplicated
- stale generated skills are removed
- user-owned `.ai/skills` are preserved

These tests should compare full file output against fixtures.

### MCP protocol tests

The MCP server should have protocol-level tests that start `forj atlas:mcp` over
stdio and call tools through the MCP client API.

Test cases:

- server starts in a generated project without a prebuilt app binary
- `application-info` returns project, app, runtime, component, and docs revision
- app-aware tools default to the default app
- app-aware tools accept a named app
- unknown apps return clear errors
- tool output is bounded
- tool timeouts are enforced
- secret values are redacted

These tests should run without network access.

### Docs retrieval tests

Embedded docs search should be tested like a retrieval system, not just as file
I/O.

Create a small evaluation set:

```text
query: "make controller for marketplace app"
expected:
  - app registration docs
  - make command docs
  - multi-app command prefix docs

query: "make command inside billing reports package"
expected:
  - Go package design docs
  - make command docs
  - app registration docs

query: "where should billing report service files go"
expected:
  - Go package design docs
  - app architecture docs

query: "where does cmd marketplace main go"
expected:
  - app architecture docs
  - cmd/<app>/main.go docs

query: "read only database MCP query"
expected:
  - Atlas database-query docs
  - security model docs
```

Measure:

- expected section appears in top 3 results
- result snippets stay under the token limit
- `read-doc-section` returns only the requested section
- `read-doc-neighborhood` includes requested before/after context
- `list-doc-headings` returns stable heading trees
- `explain-api` maps common commands and paths to useful docs sections

Docs retrieval tests should use the embedded docs manifest and should also run
against `GOFORJ_DOCS_PATH` fixtures.

### Skill effectiveness tests

Skills should have scenario fixtures that prove they point agents toward the
right workflow.

The deterministic version does not need an LLM. It should assert that each skill
contains the required triggers, commands, paths, and safety constraints.

Examples:

- `goforj-app-registration` mentions `app/`, `app/<name>/`, routes, commands,
  schedules, lifecycle hooks, and app-specific Wire files
- `goforj-make-commands` mentions `forj <app> make:*` and unprefixed default app
  behavior
- `goforj-make-commands` says agents should prefer make-command scaffolding
  over raw file creation when a matching make command exists
- `goforj-go-package-design` mentions Go package boundaries, package-local
  code, avoiding Java/PHP-style nesting, grouped make-command names, and
  selected-app registration
- `goforj-migrations` mentions raw DDL and app-scoped migration paths
- `goforj-testing-and-validation` mentions `/tmp` render locations and Go cache
  env vars

The agent-eval version should run a small prompt suite against supported agents
when credentials or local tooling are available.

Example prompts:

- "Add a checkout controller to the marketplace app."
- "Create a billing reports sync command inside the marketplace app."
- "Create a nightly cleanup schedule for backstage."
- "Explain where the marketplace app's binary entrypoint lives."
- "Find the docs for read-only database inspection."

Score:

- did the agent call Atlas tools before guessing?
- did it choose the correct app?
- did it prefer a matching `forj make:*` command over raw file creation?
- did it preserve Go package scope instead of creating unnecessary nested
  class-style directories?
- did it write to the correct registration point?
- did it avoid stale binary assumptions?
- did it keep output concise?

These evaluations should be optional or nightly, not required for every PR.

### GitHub Actions agent evals

GitHub Actions can run Atlas evaluation, but the workflow should be split into
two lanes.

Default PR lane:

- no AI provider secrets
- no model calls
- no network dependency
- deterministic writer, docs retrieval, MCP protocol, and rendered-project tests
- required for merge

Optional agent-eval lane:

- runs on `workflow_dispatch`
- runs on a scheduled workflow
- may run after merge on the default branch
- uses repository or environment secrets for model providers
- has explicit token, time, and cost budgets
- uploads redacted transcripts and scorecards as artifacts
- reports trends before it becomes a merge blocker

The agent-eval lane should not run with secrets on arbitrary pull requests. It
should use protected environments or manual approval if provider keys are
expensive or sensitive.

Suggested workflow split:

```text
.github/workflows/atlas-ci.yml
  pull_request
  push
  deterministic checks only

.github/workflows/atlas-agent-eval.yml
  workflow_dispatch
  schedule
  optional provider-backed agent evaluations
```

The optional workflow can run a matrix by scenario and agent:

```text
scenario:
  single-app
  multi-app
  vue
  jobs-schedules
  observability

agent:
  codex
  claude
```

GitHub Copilot should be treated differently. CI should deterministically test
the generated Copilot instruction, prompt, and MCP config files. Headless
Copilot behavior should not be a required CI signal unless GitHub provides a
stable CI-friendly execution surface for the exact Copilot mode GoForj wants to
support.

Suggested eval command shape:

```bash
GOCACHE=/tmp/gocache \
GOMODCACHE=/tmp/gomodcache \
FORJ_AGENT_EVAL=1 \
go test ./internal/forj/atlas/eval -run TestAgentEval -tags agent_eval
```

The eval harness should:

- render projects in `/tmp`
- install Atlas for the selected agents
- start `forj atlas:mcp`
- give the agent a scenario prompt
- capture tool calls, file diffs, and final answer
- score the run with deterministic assertions first
- use model-based judgment only as supplemental scoring
- redact secrets before logs or artifacts are uploaded

Recommended scoring dimensions:

- selected the right app
- used the right command shape
- used available scaffold commands instead of hand-creating framework artifacts
- preserved Go package scope instead of unnecessary class-style nesting
- wrote to the right registration point
- used Atlas docs/tools before guessing
- avoided unsafe database or shell behavior
- produced a small, reviewable diff
- passed the relevant project validation command

### Rendered project scenario tests

Atlas should be tested against real rendered projects in `/tmp`.

Minimum scenarios:

- default single-app project
- multi-app project with two named apps
- Vue starter kit project
- project with jobs and schedules
- project with observability enabled

Each scenario should run:

```bash
forj atlas:install --all-agents --no-interaction
forj atlas:mcp
```

Then validate generated agent files, MCP startup, app-aware tools, and docs
retrieval.

### Quality gates

Before Atlas ships, CI should enforce:

- golden file tests for agent output
- unit tests for docs parsing and section chunking
- retrieval evaluation for embedded docs
- MCP protocol tests for read-only tools
- rendered project smoke tests in `/tmp`
- no network dependency for default MCP tests
- no secrets in MCP output fixtures

Agent-in-the-loop evals should report trends, not block normal CI until the eval
harness is stable.

## Implementation plan

### Phase 1: Agent installer

Build:

- `forj atlas:install`
- `forj atlas:update`
- Codex agent adapter
- Claude Code agent adapter
- GitHub Copilot adapter
- guideline composer
- guideline writer with marker blocks
- `.goforj/atlas.json`

This phase can ship value before the MCP runtime is complete.

### Phase 2: Skills

Build:

- built-in skill source templates
- skill composer
- agent-native skill/instruction writer
- stale skill cleanup
- `.ai/skills` support
- Copilot `.github/instructions` output
- Copilot `.github/prompts` output
- `forj atlas:list-skills`

Initial built-in skills should cover app architecture, app registration, make
commands, Go package design, migrations, runtime workflows, database/data
access, observability, Vue starter kit workflows, and testing/validation.

### Phase 3: MCP MVP

Build:

- `forj atlas:mcp`
- MCP config writer for Codex, Claude, and GitHub Copilot
- `application-info`
- `search-docs`
- `read-doc-section`
- `read-doc-neighborhood`
- `list-doc-headings`
- `explain-api`
- `project-layout`
- `route-list`
- `schedule-list`
- `command-list`

This should use the Go MCP runtime design and `mark3labs/mcp-go`.

### Phase 4: Runtime diagnostics

Build:

- `database-connections`
- `database-schema`
- `database-query`
- `read-log-entries`
- `last-error`
- `get-absolute-url`

The database query tool must be read-only and bounded.

### Phase 5: Browser and observability tools

Build:

- `browser-logs`
- app/runtime-aware metrics metadata
- Lighthouse shared inspection services where appropriate

### Phase 6: Optional hosted docs search

Build an optional hosted docs search endpoint for GoForj.

The embedded docs search should remain the default. A hosted endpoint can add
cross-version and newly published docs lookup when the user or agent explicitly
needs it.

The hosted endpoint should understand:

- GoForj version
- component set
- starter kit
- generated project features
- docs package versions

## Decisions

### Use Atlas

GoForj should use `Atlas` as the product and CLI namespace for agent-facing
project navigation. It fits the existing MCP design and complements Lighthouse.

### Install one project MCP server

Atlas should install one project-level MCP server. Do not create one MCP server
per app by default.

The server should expose app-aware tools instead.

### Keep guidelines small

Guidelines should tell agents how to use Atlas tools and where the key GoForj
boundaries are. They should not become a giant static manual.

### Avoid arbitrary execution in MVP

GoForj should start with read-only, framework-aware tools and add write or
execution tools only after the policy model is proven.

### Install Codex when AGENTS.md exists

If `AGENTS.md` already exists, Atlas should treat Codex support as selected by
default. The writer must preserve user-authored content and only replace the
Atlas marker block.

### Keep make commands as CLI mutations

Atlas should teach agents to use `forj make:*` and `forj <app> make:*` through
the normal CLI for source mutations.

The MCP server should not expose write-capable scaffolding tools in the MVP. A
future write-capable MCP pack can be considered later, but it should be explicit
opt-in and should still delegate to the same make-command machinery instead of
duplicating scaffolding behavior.

# Resource Maker Command Design

This design defines a repeatable pattern for `make:*` commands that create or configure named App resources.

The first concrete target is `forj make:queue`, but the pattern should be reusable for later resource makers such as caches, storage disks, event transports, mailers, or other config-backed primitives.

These should remain App-owned commands. The framework CLI can delegate to them through `forj`, but the generated App should own the resource-specific behavior because it owns the App configuration, templates, and wiring.

## Problem

Some generated App primitives are not primarily code files. They are resource declarations backed by environment configuration and generated accessors.

Queues are the clearest example:

```env
QUEUE_NAME=default
QUEUE_WORKERS=30
QUEUE_EMAILS_NAME=emails
QUEUE_EMAILS_WORKERS=6
QUEUE_REPORTS_NAME=production-report-jobs
QUEUE_REPORTS_WORKERS=2
```

Developers should not have to remember every env key shape, naming rule, fallback behavior, or section placement. At the same time, project setup should remain scriptable for docs, teams, CI, and repeatable scaffolding.

The command model should support both:

```bash
forj make:queue
```

and:

```bash
forj make:queue emails --workers 6
forj make:queue reports --workers 2 --name production-report-jobs
```

## Goals

- Provide a consistent wizard mode when required resource details are missing.
- Keep every resource maker fully scriptable with args and flags.
- Update existing `.env` resource sections in place instead of appending random lines.
- Preserve user comments, unrelated env order, and formatting as much as practical.
- Make generated changes easy to review from command output.
- Make commands idempotent: re-running updates existing resource keys rather than duplicating them.
- Share the env-section editing approach across future resource maker commands.

## Non-Goals

- Do not introduce a second configuration model outside `.env` for local generated Apps.
- Do not make resource maker commands depend on running infrastructure.
- Do not force all resource-backed primitives into code generation if env edits are enough.
- Do not make wizard mode mandatory for automation.
- Do not silently rewrite unrelated `.env` sections.

## Command UX Pattern

Each resource maker should support two modes.

### Wizard Mode

If required inputs are missing and the process is interactive, the command should ask only for fields it needs.

Example:

```bash
forj make:queue
```

Prompts:

```text
Queue resource name: emails
Workers: 6
Backend queue name: emails
```

The wizard should present the resulting changes before writing when the command changes existing values.

Wizard mode should only run when stdin and stdout are attached to a TTY. Missing required input in non-interactive contexts should use the clean error behavior below.

### Direct Mode

If required inputs are present, the command should run without prompting:

```bash
forj make:queue emails --workers 6
forj make:queue reports --workers 2 --name production-report-jobs
```

Direct mode should be the mode used in docs, scenarios, scripts, and tests.

### Non-Interactive Missing Input

If required inputs are missing and no TTY is attached, the command should fail with a clean console error and an example:

```text
missing queue name
example: forj make:queue emails --workers 6
```

## Resource Naming

Resource makers should distinguish between:

- resource name: the App-facing resource identifier, such as `emails`
- backend name: the external/backend identifier, such as `production-email-jobs`

For queues:

```env
QUEUE_EMAILS_NAME=production-email-jobs
QUEUE_EMAILS_WORKERS=6
```

If backend name is omitted, it defaults to the resource name.

Resource names should normalize to uppercase snake case for env keys:

```text
emails -> QUEUE_EMAILS_*
billing-reports -> QUEUE_BILLING_REPORTS_*
billing:reports -> QUEUE_BILLING_REPORTS_*
```

## Env Section Editing

Resource makers should update `.env` under the existing resource section.

For queues, the target section is:

```env
# Queue
QUEUE_DRIVER=redis
QUEUE_SUPPORTED_DRIVERS=redis
QUEUE_WORKERS=30
QUEUE_NAME=default
QUEUE_SERVER_LOG_LEVEL=error
QUEUE_SHUTDOWN_TIMEOUT=10s
# Named queues can prioritize work by allocating workers:
# QUEUE_EMAILS_NAME=emails
# QUEUE_EMAILS_WORKERS=6
```

Adding `emails` should produce:

```env
# Queue
QUEUE_DRIVER=redis
QUEUE_SUPPORTED_DRIVERS=redis
QUEUE_WORKERS=30
QUEUE_NAME=default
QUEUE_SERVER_LOG_LEVEL=error
QUEUE_SHUTDOWN_TIMEOUT=10s
# Named queues can prioritize work by allocating workers:
QUEUE_EMAILS_NAME=emails
QUEUE_EMAILS_WORKERS=6
```

Adding `reports` with a backend override should produce:

```env
QUEUE_REPORTS_NAME=production-report-jobs
QUEUE_REPORTS_WORKERS=2
```

### Placement Rules

1. If the target section header exists, update inside that section.
2. If a named-resource comment exists, place named resources after it.
3. If the resource already exists, update its existing keys in place.
4. If some keys exist and some are missing, fill missing keys next to the existing resource keys.
5. If the target section does not exist, append a new section at the end with a blank line before it.
6. Preserve unrelated comments and variables.
7. Preserve newline-at-EOF behavior.

### File Selection

Default local resource makers should update `.env`.

Future flags may allow another env file:

```bash
forj make:queue emails --workers 6 --env-file .env.local
```

That should be explicit. Do not write to multiple env files by default.

## Output Pattern

Use the generated App console package.

Direct mode should print concise changed/skipped lines:

```text
generated queue emails
updated .env: QUEUE_EMAILS_NAME=emails
updated .env: QUEUE_EMAILS_WORKERS=6
```

If values already match:

```text
queue emails already configured
skipped .env: QUEUE_EMAILS_NAME=emails
skipped .env: QUEUE_EMAILS_WORKERS=6
```

If existing values change:

```text
updated .env: QUEUE_REPORTS_WORKERS=2
```

Avoid logger-style output for CLI generation errors.

## `make:queue`

`make:queue` should configure named queues.

### Direct Examples

```bash
forj make:queue emails --workers 6
forj make:queue reports --workers 2 --name production-report-jobs
```

### Wizard Example

```bash
forj make:queue
```

Prompts:

```text
Queue resource name
Workers
Backend queue name
```

### Flags

- `<name>`: optional positional resource name; missing starts wizard when interactive
- `--workers`: worker allocation for this queue
- `--name`: backend queue name; defaults to resource name
- `--env-file`: env file to update; defaults to `.env`

### Env Writes

For `forj make:queue emails --workers 6`:

```env
QUEUE_EMAILS_NAME=emails
QUEUE_EMAILS_WORKERS=6
```

For `forj make:queue reports --workers 2 --name production-report-jobs`:

```env
QUEUE_REPORTS_NAME=production-report-jobs
QUEUE_REPORTS_WORKERS=2
```

Do not write `QUEUE_QUEUES`.
Do not write Redis-specific weighted queue config for normal named queues.

## Reusable Env Editor

Implement an internal helper in generated Apps that can be reused by resource makers.

Desired capabilities:

- parse line-oriented env files without losing comments
- find section by comment header
- find named-resource insertion point inside a section
- upsert key/value pairs
- report changed, created, skipped values
- write files atomically enough for local CLI use

This helper should avoid a heavyweight env parser if that parser cannot preserve comments and ordering.

The helper should be tested independently from `make:queue`.

## Reusable Command Contract

Each resource maker should be thin command code wrapped around a small resource spec.

The important constraint is that wizard prompts and direct command inputs should be backed by the same field definitions. If the queue resource needs a name, worker count, backend name, env section, and env key prefix, those facts should live in one reusable data shape. The command should not separately define flags, prompts, validation, examples, and env writes in different places where they can drift.

The layering should look like this:

```text
ResourceMaker
  FieldSpec[]       shared field schema
  WizardSpec        renders missing FieldSpec values interactively
  CLISpec           maps args/flags into FieldSpec values
  EnvSpec           maps resolved FieldSpec values into env writes
  Command           calls the runner with concrete App dependencies
```

The dependency direction matters:

- `FieldSpec` is the source of truth.
- `WizardSpec` uses `FieldSpec` for prompts, defaults, validation, and help text.
- `CLISpec` uses `FieldSpec` for positional args, flags, defaults, and validation.
- the concrete Kong command binds raw CLI values, then hands them to the reusable runner.
- the reusable runner resolves final field values before env writes or post-write actions run.

Conceptually:

```go
type ResourceMaker struct {
	Command      CommandSpec
	Resource     ResourceSpec
	Fields       []FieldSpec
	Wizard       WizardSpec
	CLI          CLISpec
	Env          EnvSpec
	Examples     []ExampleSpec
	AfterWrite   []PostWriteAction
}

type CommandSpec struct {
	Name        string
	Description string
}

type ResourceSpec struct {
	Kind          string
	DefaultName   string
	NormalizeName func(string) string
}

type FieldSpec struct {
	Name        string
	Label       string
	Flag        string
	Argument    bool
	Prompt      string
	Help        string
	Required    bool
	Default     FieldDefault
	Validate    FieldValidator
}

type WizardSpec struct {
	Enabled      bool
	Fields       []string
	ConfirmWrite bool
}

type CLISpec struct {
	ArgumentFields []string
	FlagFields     []string
}

type EnvSpec struct {
	File          string
	Section       string
	InsertAfter    string
	KeyPrefix      string
	Writes        []EnvWriteSpec
}
```

This does not require dynamic Kong command registration or reflection-heavy command construction. Each concrete command can still have a typed Kong struct for clear CLI parsing, but it should only bind raw args and flags. Its `Run`, `Help`, wizard prompts, validation, output, and env writes should be driven from the shared resource maker spec.

For example, `make:queue` may still expose a concrete command struct:

```go
type QueueCmd struct {
	Name    string `arg:"" optional:"" help:"Queue resource name"`
	Workers int    `help:"Worker allocation for this queue"`
	Backend string `name:"name" help:"Backend queue name"`
}
```

But those struct fields should map into the resource maker by field name:

```go
input := maker.Input{
	"name":    cmd.Name,
	"workers": cmd.Workers,
	"backend": cmd.Backend,
}
```

From there, the runner applies defaults, prompts if interactive, validates, writes env values, and prints output. That keeps the public CLI typed and Kong-native while making wizard and direct mode share one contract.

## Wizard Renderer

Resource maker wizards should use a reusable prompt renderer instead of each command implementing its own Bubble Tea model.

The existing `forj dev` TUI is not the right abstraction to reuse directly. It is a full-screen runtime/log interface with footer state, transcript rendering, filters, and command dispatch. Resource makers need a smaller flow: ask for missing fields, validate them, optionally confirm changes, then return resolved values.

The existing `forj new` wizard is closer in spirit because it already uses Bubble Tea, Bubbles inputs, lists, and shared terminal styling. However, it is currently project-creation-specific. The resource maker work should extract or introduce a small reusable wizard package rather than coupling resource commands to the project wizard model.

The reusable wizard layer should provide:

- terminal detection and no-TTY fallback behavior
- text input prompts from `FieldSpec`
- select prompts for fields with choices
- default value display
- validation error display
- optional change confirmation
- consistent styles with the rest of the CLI

This keeps the implementation aligned with the existing Bubble Tea/Bubbles stack while keeping resource maker commands small and testable.

The output after changes are applied should still use the console package, not the Bubble Tea renderer. Bubble Tea owns transient interactive input; console owns persistent command results and errors.

For `make:queue`, the spec would express:

- command name: `make:queue`
- section: `# Queue`
- env prefix: `QUEUE`
- resource name argument: `emails`
- backend name flag: `--name`
- workers flag: `--workers`
- env writes:
  - `QUEUE_<RESOURCE>_NAME`
  - `QUEUE_<RESOURCE>_WORKERS`

The same spec should drive:

- command name and examples
- resource section header
- env key prefix
- required fields
- default values
- wizard prompts
- validation rules
- env writes
- optional post-write actions

That contract keeps the resource-specific behavior explicit while allowing the section-editing, wizard, idempotency, and console output behavior to be shared.

The command implementation should follow this flow:

1. Bind typed CLI args and flags into a resource input object.
2. Ask missing required fields from the same `FieldSpec` list when interactive.
3. Validate the final input with the same field validators.
4. Build env writes from the same `EnvWriteSpec` list.
5. Apply writes through the reusable env editor.
6. Emit console output based on the editor result.

This gives later commands a repeatable shape without making every resource command feel generic or opaque.

## Future Resource Makers

This pattern may later apply to:

- `make:cache`
- `make:storage`
- `make:event-bus`
- `make:mail`

Each future command should define:

- resource section header
- env key prefix
- required fields
- wizard prompts
- direct flags
- idempotent update rules
- whether any code generation or wiring is also required

## Open Questions

- Should direct mode require `--yes` when it changes existing values?
- Should `make:queue` also run `forj generate --queue` after editing `.env`?
- Should `make:queue` update `.env.host` or only `.env`?
- Should named queue defaults use `workers=6`, ask every time, or require direct flag?
- Should env section editing live under `internal/makecmd` or a small reusable internal package?

## Tasks

- [x] Define the resource maker spec types.
- [ ] Add tests proving one spec drives direct input validation, wizard prompts, and env writes.
- [x] Add or extract a reusable wizard renderer backed by Bubble Tea/Bubbles.
- [x] Ensure wizard rendering is only used when stdin and stdout are TTYs.
- [ ] Add reusable env section editor tests.
- [x] Implement reusable env section editor.
- [x] Add a small reusable resource maker command contract.
- [x] Add `make:queue` command with direct mode.
- [x] Add `make:queue` wizard mode for interactive no-arg usage.
- [x] Register `make:queue` in generated App command wiring when jobs are enabled.
- [x] Add help text and examples for `make:queue`.
- [x] Ensure `make:queue` uses the console package for output and errors.
- [x] Ensure non-interactive missing input fails with a clean example.
- [x] Add tests for inserting into an existing `# Queue` section.
- [x] Add tests for replacing existing queue values without duplicating keys.
- [x] Add tests for creating a missing `# Queue` section.
- [x] Add rendered App integration coverage for `forj make:queue emails --workers 6`.
- [ ] Decide whether `make:queue` should run `forj generate --queue` automatically.
- [ ] Update docs after implementation.

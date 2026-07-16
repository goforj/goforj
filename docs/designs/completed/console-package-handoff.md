# Console Package Handoff

## Status

Completed. GoForj's own CLI now imports `github.com/goforj/console`, its
duplicated `internal/console` package has been removed, and its build and
project-creation progress paths use the extracted loader. The private
`__FORJ_BUILD_PROGRESS__` protocol remains local because it is an integration
contract with the dev TUI rather than public console presentation.

## Current Package Surface

The released package grew beyond the minimum extraction contract below into a
disciplined, line-oriented console toolkit. It now includes:

- package-level helpers and equivalent `*Console` methods for common operations
- semantic messages, coordinated writers, marks, styles, and terminal policy
- ANSI-aware wrapping, truncation, padding, indentation, and visible width
- sections, rules, key/value summaries, lists, boxes, bordered or compact tables, and trees
- questions, defaults, confirmation, choices, and non-echoed secret input
- concurrency-safe loaders and determinate progress with stable redirected output

It remains intentionally smaller than a TUI framework: it does not own an event
loop, alternate screen, raw-key navigation, command parser, subprocess runner,
or structured logging pipeline.

Most operations are available globally and on an instance. Package helpers are
the concise default for applications, while an instance isolates writers,
input, environment policy, terminal hooks, and exit behavior. Loader and
progress constructors are the naming exception: the global forms are
`NewLoader` and `NewProgress`, while instance forms are `Console.Loader` and
`Console.Progress`.

## Original Extraction Scope

The first step extracted the semantic console/ANSI output layer into a
standalone package.

The extraction was intended to own:

- semantic marks (`action`, `info`, `success`, `warn`, `error`, `debug`)
- ANSI colorization policy
- message printing helpers
- runtime configurability (writers/env/TTY hooks)

It was not intended to own app logging pipelines, domain log fields, or TUI
layout state. Those boundaries remain in force as the package has expanded.

## Previous State (in goforj)

The extraction began from:

- `internal/console/console.go`
- `internal/console/console_runtime.go`
- `internal/console/console_runtime_test.go`

### Public-ish behavior currently used

- Top-level helpers:
  - `ActionMark`, `InfoMark`, `SuccessMark`, `WarnMark`, `ErrorMark`, `DebugMark`
  - `Actionf`, `Infof`, `Successf`, `Warnf`, `Errorf`, `Fatalf`, `Debugf`
  - `Colorize`
- ANSI constants:
  - `ColorReset`, `ColorBoldWhite`, `ColorGray`, `ColorGreen`, `ColorYellow`, `ColorRed`, `ColorCyan`

### Runtime type extracted

- `type Console`
- `func New(Config) *Console`
- `func SetDefault(*Console)` for package-level helper wiring
- `Config` supports writer/env/TTY/exit overrides for testability and embedding

## Extraction Prerequisites

- No hard dependency on global `os.Stdout`/`os.Stderr` in core runtime logic
- Color/debug behavior is testable via injected env/TTY hooks
- Backward-compatible top-level helpers remain intact

## Initial Extraction API

The first extraction preserved this compatibility surface; the current package
adds the broader layout, prompt, loader, progress, and utility APIs described
above.

```go
type Config struct {
  Stdout io.Writer
  Stderr io.Writer
  ColorEnabled *bool
  DebugEnabled *bool
  Getenv func(string) string
  IsTerminal func(int) bool
  Exit func(int)
}

type Console struct {}

func New(cfg Config) *Console
func SetDefault(c *Console)

func (c *Console) ActionMark() string
func (c *Console) InfoMark() string
func (c *Console) SuccessMark() string
func (c *Console) WarnMark() string
func (c *Console) ErrorMark() string
func (c *Console) DebugMark() string

func (c *Console) Actionf(format string, args ...any)
func (c *Console) Infof(format string, args ...any)
func (c *Console) Successf(format string, args ...any)
func (c *Console) Warnf(format string, args ...any)
func (c *Console) Errorf(format string, args ...any)
func (c *Console) Fatalf(format string, args ...any)
func (c *Console) Debugf(format string, args ...any)
func (c *Console) Colorize(color, value string) string
```

Top-level wrappers should continue to exist for ergonomics.

## Initial Behavior Contract

- `NO_COLOR` disables ANSI unless overridden by `ColorEnabled=true`.
- `CLICOLOR_FORCE=1` enables ANSI when auto mode is used.
- Debug prints enabled when any of `FORJ_DEBUG`, `APP_DEBUG`, `DEBUG` is set and not `0`.
- `Errorf` writes stderr; others write stdout.
- `Fatalf` writes error then exits code `1`.

## Initial Test Coverage

The extraction established coverage for:

1. Debug output toggles from env.
2. `NO_COLOR` disables color.
3. `CLICOLOR_FORCE` forces color.
4. Top-level helpers route through default console.
5. `Fatalf` exit function is injectable/testable (no real process exit in tests).

Additional coverage included:

- TTY auto-detection path with fake terminal hook.
- Unicode and ASCII fallbacks for constrained terminals.

## Package Release Notes

- Module: `github.com/goforj/console`
- Keep the dependency footprint focused: `golang.org/x/term` owns terminal integration and `github.com/rivo/uniseg` owns grapheme/cell handling.
- GoForj consumes the hardened package through the tagged version recorded in
  `go.mod`.
- The package README and API index are generated from source-comment examples,
  including output for default usage, custom writers, layout, prompts, loaders,
  and progress.
- The [package README](https://github.com/goforj/console#readme) is the
  authoritative evolving API reference; this handoff records the framework
  boundary and initial compatibility contract.

## Completed GoForj Migration

1. [x] Replace framework imports from `internal/console` with the external module.
2. [x] Keep function names identical to avoid mass churn.
3. [x] Remove the framework's internal package once all callers are migrated.
4. [x] Keep one smoke test in `internal/forj` validating expected semantic marks are still rendered.
5. [x] Replace the build and project-creation spinners with the extracted loader.

Generated application templates deliberately retain their own `internal/console`
source. They are standalone application assets, not callers of GoForj's removed
framework package, and changing that dependency contract is outside this extraction.

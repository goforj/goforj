# Console Package Handoff

## Status

Completed. GoForj's own CLI now imports `github.com/goforj/console`, its duplicated
`internal/console` package has been removed, and its two interactive spinner paths
use the extracted loader. The private `__FORJ_BUILD_PROGRESS__` protocol remains
local because it is an integration contract with the dev TUI rather than public
console presentation.

## Scope
Extract the semantic console/ANSI output layer into a standalone package.

This package should own:
- semantic marks (`action`, `info`, `success`, `warn`, `error`, `debug`)
- ANSI colorization policy
- message printing helpers
- runtime configurability (writers/env/TTY hooks)

It should not own app logging pipelines, domain log fields, or TUI layout state.

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

## Why it is now extraction-ready
- No hard dependency on global `os.Stdout`/`os.Stderr` in core runtime logic
- Color/debug behavior is testable via injected env/TTY hooks
- Backward-compatible top-level helpers remain intact

## Required API in extracted package
Keep this stable first:

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

## Behavior contract
- `NO_COLOR` disables ANSI unless overridden by `ColorEnabled=true`.
- `CLICOLOR_FORCE=1` enables ANSI when auto mode is used.
- Debug prints enabled when any of `FORJ_DEBUG`, `APP_DEBUG`, `DEBUG` is set and not `0`.
- `Errorf` writes stderr; others write stdout.
- `Fatalf` writes error then exits code `1`.

## Test matrix
Unit tests required:
1. Debug output toggles from env.
2. `NO_COLOR` disables color.
3. `CLICOLOR_FORCE` forces color.
4. Top-level helpers route through default console.
5. `Fatalf` exit function is injectable/testable (no real process exit in tests).

Optional but useful:
- TTY auto-detection path with fake terminal hook.
- Unicode marks fallback strategy if needed for constrained terminals.

## Packaging notes
- Module: `github.com/goforj/console`
- Keep the dependency footprint focused: `golang.org/x/term` owns terminal integration and `github.com/rivo/uniseg` owns grapheme/cell handling.
- GoForj consumes the hardened package through the published `v0.1.0` semantic release.
- Add README examples for:
  - default usage
  - custom writers (tests/CLI embedding)
  - forced color/no color

## Migration back into goforj
1. [x] Replace framework imports from `internal/console` with the external module.
2. [x] Keep function names identical to avoid mass churn.
3. [x] Remove the framework's internal package once all callers are migrated.
4. [x] Keep one smoke test in `internal/forj` validating expected semantic marks are still rendered.
5. [x] Replace the build and project-creation spinners with the extracted loader.

Generated application templates deliberately retain their own `internal/console`
source. They are standalone application assets, not callers of GoForj's removed
framework package, and changing that dependency contract is outside this extraction.

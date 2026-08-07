package project

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestDevWatchJSONSeparatesLegacyAndNativeMatchers verifies JSON uses one watch key whose shape preserves both watcher modes.
func TestDevWatchJSONSeparatesLegacyAndNativeMatchers(t *testing.T) {
	input := `[
  {
    "name": "Legacy",
    "watch": "-file .go -postpone",
    "exec": "forj build"
  },
  {
    "name": "Native",
    "watch": [".go", ".env", "re:^schemas/.+\\.json$"],
    "ignore": ["_test.go", "generated"],
    "roots": ["./schemas"],
    "workdir": "./tools",
    "files": {"exclude": ["generated.go"]},
    "dirs": {"include": ["schemas"], "exclude": ["vendor"]},
    "exec": "forj api-index",
    "env": {"INDEX_MODE": "fast"},
    "debounce": "125ms",
    "poll": "2s",
    "postpone": true,
    "restart": true,
    "exit": true,
    "stdin": true
  }
]`

	var watches []DevWatch
	if err := json.Unmarshal([]byte(input), &watches); err != nil {
		t.Fatalf("unmarshal dev watches: %v", err)
	}
	if len(watches) != 2 {
		t.Fatalf("watch count = %d, want 2", len(watches))
	}
	legacy := watches[0]
	if !legacy.IsLegacy() || legacy.Watch != "-file .go -postpone" || len(legacy.Include) != 0 {
		t.Fatalf("legacy watch was not preserved: %#v", legacy)
	}
	native := watches[1]
	if native.IsLegacy() {
		t.Fatalf("native watch was classified as legacy: %#v", native)
	}
	if want := []string{".go", ".env", `re:^schemas/.+\.json$`}; !reflect.DeepEqual(native.Include, want) {
		t.Fatalf("native watch = %#v, want %#v", native.Include, want)
	}
	if !reflect.DeepEqual(native.Files.Exclude, []string{"generated.go"}) ||
		!reflect.DeepEqual(native.Dirs.Include, []string{"schemas"}) ||
		!reflect.DeepEqual(native.Dirs.Exclude, []string{"vendor"}) {
		t.Fatalf("scoped native matchers were not decoded: %#v", native)
	}

	encoded, err := json.Marshal(watches)
	if err != nil {
		t.Fatalf("marshal dev watches: %v", err)
	}
	var documents []map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &documents); err != nil {
		t.Fatalf("inspect marshaled dev watches: %v", err)
	}
	if got := string(documents[0]["watch"]); got != `"-file .go -postpone"` {
		t.Fatalf("legacy watch JSON = %s, want a scalar", got)
	}
	if got := string(documents[1]["watch"]); got != `[".go",".env","re:^schemas/.+\\.json$"]` {
		t.Fatalf("native watch JSON = %s, want a matcher list", got)
	}
	for index, document := range documents {
		if _, ok := document["include"]; ok {
			t.Fatalf("watch %d exposed competing include field: %s", index, encoded)
		}
	}

	var roundTripped []DevWatch
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal round-tripped dev watches: %v", err)
	}
	if !reflect.DeepEqual(roundTripped, watches) {
		t.Fatalf("round-tripped watches = %#v, want %#v", roundTripped, watches)
	}
}

// TestDevWatchJSONPreservesExplicitEmptyWatchShapes keeps empty legacy and native values distinct across JSON round trips.
func TestDevWatchJSONPreservesExplicitEmptyWatchShapes(t *testing.T) {
	original := []DevWatch{
		{Name: "Legacy", Legacy: true, Exec: "make legacy"},
		{Name: "Native", Include: []string{}, Exec: "make native"},
	}

	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal empty dev watch shapes: %v", err)
	}
	if got := string(encoded); !strings.Contains(got, `"watch":""`) || !strings.Contains(got, `"watch":[]`) {
		t.Fatalf("empty dev watch shapes were not preserved: %s", got)
	}
	var roundTripped []DevWatch
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal empty dev watch shapes: %v", err)
	}
	if !reflect.DeepEqual(roundTripped, original) {
		t.Fatalf("round-tripped empty watches = %#v, want %#v", roundTripped, original)
	}
}

// TestDevWatchJSONRejectsNonCanonicalMatcherFields prevents ambiguous or lossy JSON watcher contracts.
func TestDevWatchJSONRejectsNonCanonicalMatcherFields(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "competing include", input: `{"name":"Native","include":[".go"],"exec":"make"}`, want: "include is invalid"},
		{name: "ambiguous include", input: `{"name":"Native","watch":[".go"],"include":[".md"],"exec":"make"}`, want: "include is invalid"},
		{name: "null watch", input: `{"name":"Invalid","watch":null,"exec":"make"}`, want: "watch must be a legacy string or matcher list"},
		{name: "boolean watch", input: `{"name":"Invalid","watch":true,"exec":"make"}`, want: "watch must be a legacy string or matcher list"},
		{name: "object watch", input: `{"name":"Invalid","watch":{"suffix":".go"},"exec":"make"}`, want: "watch must be a legacy string or matcher list"},
		{name: "numeric matcher", input: `{"name":"Invalid","watch":[".go",42],"exec":"make"}`, want: "native watch matcher 1: expected a string"},
		{name: "null matcher", input: `{"name":"Invalid","watch":[null],"exec":"make"}`, want: "native watch matcher 0: expected a string"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var watch DevWatch
			err := json.Unmarshal([]byte(test.input), &watch)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("unmarshal error = %v, want %q", err, test.want)
			}
		})
	}
}

// TestDevWatchJSONRejectsAmbiguousInternalMatchers prevents encoding a watcher that cannot round-trip without losing one matcher mode.
func TestDevWatchJSONRejectsAmbiguousInternalMatchers(t *testing.T) {
	_, err := json.Marshal(DevWatch{
		Name:    "Ambiguous",
		Watch:   "-file .go",
		Include: []string{".md"},
		Exec:    "make",
	})
	if err == nil || !strings.Contains(err.Error(), "legacy watch and native matchers cannot both be set") {
		t.Fatalf("marshal error = %v, want matcher mode conflict", err)
	}
}

// TestDevWatchYAMLSeparatesLegacyAndNativeMatchers verifies that the watch
// node's shape selects compatibility or native behavior without changing old input.
func TestDevWatchYAMLSeparatesLegacyAndNativeMatchers(t *testing.T) {
	input := `dev:
  watches:
    - name: Legacy
      watch: -file .go -postpone
      exec: forj build
    - name: Native
      watch: [.go, .env, "re:^schemas/.+\\.json$"]
      ignore: [_test.go, generated]
      root: ./schemas
      workdir: ./tools
      files:
        exclude: [generated.go]
      dirs:
        include: [schemas]
        exclude: [vendor]
      exec: forj api-index
      env:
        INDEX_MODE: fast
      debounce: 125ms
      poll: 2s
      postpone: true
      restart: true
      exit: true
      stdin: true
`

	var config Config
	if err := yaml.Unmarshal([]byte(input), &config); err != nil {
		t.Fatalf("unmarshal dev watches: %v", err)
	}
	if len(config.Dev.Watches) != 2 {
		t.Fatalf("watch count = %d, want 2", len(config.Dev.Watches))
	}
	legacy := config.Dev.Watches[0]
	if !legacy.IsLegacy() || legacy.Watch != "-file .go -postpone" || len(legacy.Include) != 0 {
		t.Fatalf("legacy watch was not preserved: %#v", legacy)
	}

	native := config.Dev.Watches[1]
	if native.IsLegacy() {
		t.Fatalf("native watch was classified as legacy: %#v", native)
	}
	if want := []string{".go", ".env", `re:^schemas/.+\.json$`}; !reflect.DeepEqual(native.Include, want) {
		t.Fatalf("native include = %#v, want %#v", native.Include, want)
	}
	if want := []string{"_test.go", "generated"}; !reflect.DeepEqual(native.Ignore, want) {
		t.Fatalf("native ignore = %#v, want %#v", native.Ignore, want)
	}
	if want := []string{"./schemas"}; !reflect.DeepEqual(native.Roots, want) {
		t.Fatalf("native roots = %#v, want %#v", native.Roots, want)
	}
	if native.WorkDir != "./tools" || native.Debounce != "125ms" || native.Poll != "2s" {
		t.Fatalf("native timing and path controls were not decoded: %#v", native)
	}
	if !native.Postpone || !native.Restart || !native.Exit || !native.Stdin {
		t.Fatalf("native process controls were not decoded: %#v", native)
	}
	if native.Env["INDEX_MODE"] != "fast" {
		t.Fatalf("native env = %#v, want INDEX_MODE=fast", native.Env)
	}
	if !reflect.DeepEqual(native.Files.Exclude, []string{"generated.go"}) ||
		!reflect.DeepEqual(native.Dirs.Include, []string{"schemas"}) ||
		!reflect.DeepEqual(native.Dirs.Exclude, []string{"vendor"}) {
		t.Fatalf("scoped native matchers were not decoded: %#v", native)
	}
}

// TestDevWatchYAMLRoundTripPreservesBothModes verifies that renderer writes do
// not turn legacy flags into native matchers or discard native controls.
func TestDevWatchYAMLRoundTripPreservesBothModes(t *testing.T) {
	original := Config{Dev: DevConfig{Watches: []DevWatch{
		{Name: "Legacy", Watch: "-file .go -postpone", Exec: "forj build"},
		{
			Name:     "Native",
			Include:  []string{".md"},
			Ignore:   []string{"_data"},
			Roots:    []string{"docs", "examples"},
			WorkDir:  "docs",
			Exec:     "make docs",
			Debounce: "200ms",
			Poll:     "1s",
			Postpone: true,
		},
	}}}

	encoded, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal dev watches: %v", err)
	}
	if !strings.Contains(string(encoded), "watch: -file .go -postpone") {
		t.Fatalf("legacy watch was not emitted as a scalar:\n%s", encoded)
	}
	if !strings.Contains(string(encoded), "watch:\n") || !strings.Contains(string(encoded), "- .md") {
		t.Fatalf("native watch was not emitted as a list:\n%s", encoded)
	}

	var roundTripped Config
	if err := yaml.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal round-tripped dev watches: %v", err)
	}
	if !reflect.DeepEqual(roundTripped.Dev.Watches, original.Dev.Watches) {
		t.Fatalf("round-tripped watches = %#v, want %#v", roundTripped.Dev.Watches, original.Dev.Watches)
	}
}

// TestDevAppsYAMLSupportsConciseLifecycleConfig verifies the generated config's
// boolean, command suffix, and SPA path shorthands.
func TestDevAppsYAMLSupportsConciseLifecycleConfig(t *testing.T) {
	input := `dev:
  apps:
    app:
      run: run
      spas:
        portal: ./cmd/app/frontend
        admin:
          path: ./cmd/app/admin
          build: npm run build:admin
          watch: [.ts, .css]
          ignore: [node_modules, dist]
    reporting: true
`

	var config Config
	if err := yaml.Unmarshal([]byte(input), &config); err != nil {
		t.Fatalf("unmarshal dev apps: %v", err)
	}
	app := config.Dev.Apps["app"]
	if app.Run == nil || app.Run.Exec != "run" {
		t.Fatalf("default app run shorthand was not decoded: %#v", app)
	}
	if app.Build != nil {
		t.Fatalf("omitted app build should use defaults, got %#v", app.Build)
	}
	if got := app.SPAs["portal"].Path; got != "./cmd/app/frontend" {
		t.Fatalf("portal path = %q, want conventional scalar path", got)
	}
	admin := app.SPAs["admin"]
	if admin.Path != "./cmd/app/admin" || admin.Build != "npm run build:admin" {
		t.Fatalf("admin SPA overrides were not decoded: %#v", admin)
	}
	if reporting := config.Dev.Apps["reporting"]; !reflect.DeepEqual(reporting, DevApp{}) {
		t.Fatalf("reporting app true shorthand = %#v, want empty defaults", reporting)
	}

	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal concise dev apps: %v", err)
	}
	for _, expected := range []string{"watch: [.ts, .css]", "ignore: [node_modules, dist]"} {
		if !strings.Contains(string(encoded), expected) {
			t.Fatalf("round-trip YAML omitted compact matcher list %q:\n%s", expected, encoded)
		}
	}
	var roundTripped Config
	if err := yaml.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal round-tripped dev apps: %v", err)
	}
	if !reflect.DeepEqual(roundTripped.Dev.Apps, config.Dev.Apps) {
		t.Fatalf("round-tripped dev apps = %#v, want %#v", roundTripped.Dev.Apps, config.Dev.Apps)
	}
}

// TestDevAppsYAMLRejectsFalseEntry keeps app inclusion equivalent to map presence.
func TestDevAppsYAMLRejectsFalseEntry(t *testing.T) {
	input := `dev:
  apps:
    worker: false
`

	var config Config
	err := yaml.Unmarshal([]byte(input), &config)
	if err == nil || !strings.Contains(err.Error(), "remove the app from dev.apps to exclude it") {
		t.Fatalf("false dev app error = %v, want presence-based inclusion guidance", err)
	}
}

// TestDevAppsYAMLPreservesEmptyAllowlistPresence keeps native-none distinct from legacy discovery.
func TestDevAppsYAMLPreservesEmptyAllowlistPresence(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		structured bool
	}{
		{name: "absent legacy model", input: "dev: {}\n"},
		{name: "explicit empty native model", input: "dev:\n  apps: {}\n", structured: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var config Config
			if err := yaml.Unmarshal([]byte(test.input), &config); err != nil {
				t.Fatalf("unmarshal config: %v", err)
			}
			if got := config.Dev.UsesStructuredApps(); got != test.structured {
				t.Fatalf("UsesStructuredApps() = %t, want %t", got, test.structured)
			}
			encoded, err := yaml.Marshal(config)
			if err != nil {
				t.Fatalf("marshal config: %v", err)
			}
			hasEmptyApps := strings.Contains(string(encoded), "apps: {}")
			if hasEmptyApps != test.structured {
				t.Fatalf("encoded empty apps presence = %t, want %t:\n%s", hasEmptyApps, test.structured, encoded)
			}
			var roundTripped Config
			if err := yaml.Unmarshal(encoded, &roundTripped); err != nil {
				t.Fatalf("unmarshal round trip: %v", err)
			}
			if got := roundTripped.Dev.UsesStructuredApps(); got != test.structured {
				t.Fatalf("round-trip UsesStructuredApps() = %t, want %t", got, test.structured)
			}
		})
	}
}

// TestDevConfigSetAppsPreservesEmptyAllowlistPresence verifies API callers can disable legacy App discovery explicitly.
func TestDevConfigSetAppsPreservesEmptyAllowlistPresence(t *testing.T) {
	var config DevConfig
	config.SetApps(map[string]DevApp{})
	if !config.UsesStructuredApps() {
		t.Fatal("SetApps did not retain explicit empty App configuration")
	}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if !strings.Contains(string(encoded), "apps: {}") {
		t.Fatalf("SetApps empty allowlist was erased:\n%s", encoded)
	}
}

// TestDevRunYAMLPreservesEmptyAllowlistPresence keeps explicit legacy exclusion distinct from the pre-allowlist model.
func TestDevRunYAMLPreservesEmptyAllowlistPresence(t *testing.T) {
	t.Parallel()
	var config Config
	if err := yaml.Unmarshal([]byte("dev:\n  run: {}\n"), &config); err != nil {
		t.Fatalf("unmarshal empty dev.run: %v", err)
	}
	if config.Dev.Run == nil || len(config.Dev.Run) != 0 {
		t.Fatalf("empty dev.run was not retained in memory: %#v", config.Dev.Run)
	}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal empty dev.run: %v", err)
	}
	if !strings.Contains(string(encoded), "run: {}") {
		t.Fatalf("empty dev.run was omitted:\n%s", encoded)
	}
	var roundTripped Config
	if err := yaml.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal empty dev.run round trip: %v", err)
	}
	if roundTripped.Dev.Run == nil {
		t.Fatal("round-trip empty dev.run became the absent pre-allowlist model")
	}
}

// TestDevAppCommandYAMLRoundTripsLifecycleStates verifies omission, disabling, shorthand, and full overrides remain distinct.
func TestDevAppCommandYAMLRoundTripsLifecycleStates(t *testing.T) {
	input := `dev:
  apps:
    omitted: true
    disabled:
      run: false
    shorthand:
      run: jobs --once
    override:
      run:
        exec: forj jobs run
        watch: [.go]
        env:
          JOB_MODE: eager
        postpone: false
`

	var config Config
	if err := yaml.Unmarshal([]byte(input), &config); err != nil {
		t.Fatalf("unmarshal dev app command states: %v", err)
	}
	if config.Dev.Apps["omitted"].Run != nil {
		t.Fatalf("omitted run = %#v, want nil default", config.Dev.Apps["omitted"].Run)
	}
	disabled := config.Dev.Apps["disabled"].Run
	if disabled == nil || !disabled.Disabled || disabled.Shorthand {
		t.Fatalf("disabled run was not preserved: %#v", disabled)
	}
	shorthand := config.Dev.Apps["shorthand"].Run
	if shorthand == nil || shorthand.Disabled || !shorthand.Shorthand || shorthand.Exec != "jobs --once" {
		t.Fatalf("shorthand run was not preserved: %#v", shorthand)
	}
	override := config.Dev.Apps["override"].Run
	if override == nil || override.Disabled || override.Shorthand || override.Exec != "forj jobs run" {
		t.Fatalf("mapping run was not preserved: %#v", override)
	}
	if !reflect.DeepEqual(override.Watch, []string{".go"}) || override.Env["JOB_MODE"] != "eager" || !override.PostponeSet || override.Postpone {
		t.Fatalf("mapping run controls were not preserved: %#v", override)
	}

	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal dev app command states: %v", err)
	}
	encodedText := string(encoded)
	for _, expected := range []string{"omitted: true", "run: false", "run: jobs --once", "exec: forj jobs run", "watch: [.go]", "postpone: false"} {
		if !strings.Contains(encodedText, expected) {
			t.Fatalf("round-trip YAML omitted %q:\n%s", expected, encoded)
		}
	}
	var roundTripped Config
	if err := yaml.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal round-tripped dev app command states: %v", err)
	}
	if !reflect.DeepEqual(roundTripped.Dev.Apps, config.Dev.Apps) {
		t.Fatalf("round-tripped dev app commands = %#v, want %#v", roundTripped.Dev.Apps, config.Dev.Apps)
	}
}

// TestDevAppCommandYAMLPreservesTrueCompatibilityAlias keeps older explicit defaults readable without making them canonical.
func TestDevAppCommandYAMLPreservesTrueCompatibilityAlias(t *testing.T) {
	input := `dev:
  apps:
    legacy:
      run: true
`

	var config Config
	if err := yaml.Unmarshal([]byte(input), &config); err != nil {
		t.Fatalf("unmarshal true command compatibility alias: %v", err)
	}
	run := config.Dev.Apps["legacy"].Run
	if run == nil || run.Disabled || run.Shorthand || run.IsMapping() || run.Exec != "" {
		t.Fatalf("true command compatibility alias = %#v, want conventional default", run)
	}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal true command compatibility alias: %v", err)
	}
	if !strings.Contains(string(encoded), "run: true") {
		t.Fatalf("true command compatibility alias did not round trip:\n%s", encoded)
	}
}

// TestDevAppCommandYAMLPreservesEmptyMappingShape keeps a full override distinct from the compatibility-only true alias.
func TestDevAppCommandYAMLPreservesEmptyMappingShape(t *testing.T) {
	input := `dev:
  apps:
    invalid:
      run: {}
`

	var config Config
	if err := yaml.Unmarshal([]byte(input), &config); err != nil {
		t.Fatalf("unmarshal empty run mapping: %v", err)
	}
	run := config.Dev.Apps["invalid"].Run
	if run == nil || !run.IsMapping() {
		t.Fatalf("empty run mapping = %#v, want explicit mapping state", run)
	}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal empty run mapping: %v", err)
	}
	if !strings.Contains(string(encoded), "run: {}") {
		t.Fatalf("empty run mapping did not retain its shape:\n%s", encoded)
	}
}

// TestDevAppsYAMLSupportsExplicitCommandOverrides verifies that app build and
// runtime maps expose the native watcher controls needed beyond defaults.
func TestDevAppsYAMLSupportsExplicitCommandOverrides(t *testing.T) {
	input := `dev:
  apps:
    billing:
      build:
        exec: forj billing build -o ./bin/billing
        watch: [.go, .env]
        ignore: [wire_gen.go]
        root: ./cmd/billing
        workdir: .
        env:
          CGO_ENABLED: "0"
        debounce: 175ms
        poll: 3s
        postpone: true
      run: false
`

	var config Config
	if err := yaml.Unmarshal([]byte(input), &config); err != nil {
		t.Fatalf("unmarshal explicit dev app: %v", err)
	}
	app := config.Dev.Apps["billing"]
	if app.Build == nil {
		t.Fatalf("billing app build was not decoded: %#v", app)
	}
	build := app.Build
	if build.Exec != "forj billing build -o ./bin/billing" || build.Root != "./cmd/billing" || build.WorkDir != "." {
		t.Fatalf("billing build path and command controls were not decoded: %#v", build)
	}
	if !reflect.DeepEqual(build.Watch, []string{".go", ".env"}) || !reflect.DeepEqual(build.Ignore, []string{"wire_gen.go"}) {
		t.Fatalf("billing build matchers were not decoded: %#v", build)
	}
	if build.Env["CGO_ENABLED"] != "0" || build.Debounce != "175ms" || build.Poll != "3s" || !build.Postpone {
		t.Fatalf("billing build execution controls were not decoded: %#v", build)
	}
	if app.Run == nil || !app.Run.Disabled {
		t.Fatalf("billing run false should disable only the runtime: %#v", app.Run)
	}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal explicit dev app: %v", err)
	}
	for _, expected := range []string{"watch: [.go, .env]", "ignore: [wire_gen.go]"} {
		if !strings.Contains(string(encoded), expected) {
			t.Fatalf("explicit dev app YAML omitted compact matcher list %q:\n%s", expected, encoded)
		}
	}
}

// TestDevWatchYAMLRejectsAmbiguousIncludes keeps each watcher on one explicit
// input grammar so native and legacy matching cannot be combined accidentally.
func TestDevWatchYAMLRejectsAmbiguousIncludes(t *testing.T) {
	input := `dev:
  watches:
    - name: Ambiguous
      watch: [.go]
      include: [.md]
      exec: make
`

	var config Config
	err := yaml.Unmarshal([]byte(input), &config)
	if err == nil || !strings.Contains(err.Error(), "watch and include cannot both be set") {
		t.Fatalf("ambiguous watcher error = %v, want watch/include conflict", err)
	}
}

// TestDevWatchYAMLPreservesEmptyLegacyScalar keeps an all-files wgo watcher distinct from a native watcher with no watch key.
func TestDevWatchYAMLPreservesEmptyLegacyScalar(t *testing.T) {
	input := `dev:
  watches:
    - name: Everything
      watch: ""
      exec: make refresh
`
	var config Config
	if err := yaml.Unmarshal([]byte(input), &config); err != nil {
		t.Fatalf("unmarshal empty legacy watch: %v", err)
	}
	if len(config.Dev.Watches) != 1 || !config.Dev.Watches[0].IsLegacy() {
		t.Fatalf("empty scalar watch lost legacy mode: %#v", config.Dev.Watches)
	}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal empty legacy watch: %v", err)
	}
	if !strings.Contains(string(encoded), `watch: ""`) {
		t.Fatalf("empty scalar watch was not preserved:\n%s", encoded)
	}
}

// TestDevFlowMatcherYAMLRoundTripPreservesSyntaxSensitiveStrings verifies compact lists remain valid YAML without changing matcher types or values.
func TestDevFlowMatcherYAMLRoundTripPreservesSyntaxSensitiveStrings(t *testing.T) {
	matchers := []string{
		"",
		"true",
		"null",
		"42",
		"2026-07-13",
		"#comment",
		"!tag",
		"&anchor",
		"*alias",
		"%directive",
		"@reserved",
		"`reserved`",
		"- list item",
		"? mapping key",
		": mapping value",
		"path,with,commas",
		"[literal brackets]",
		"{literal braces}",
		"colon: value",
		"hash # value",
		`re:^schemas/.+\.json$`,
		`re:^[a,b]{1,2}$`,
		`quote"and'slash\`,
		" leading space",
		"trailing space ",
		"line\nbreak",
		"tab\tvalue",
	}
	original := Config{Dev: DevConfig{Apps: map[string]DevApp{
		"app": {
			Build: &DevAppCommand{
				Exec:   "forj build -o ./bin/app",
				Watch:  append([]string(nil), matchers...),
				Ignore: append([]string(nil), matchers...),
			},
			SPAs: map[string]DevSPA{
				"frontend": {
					Path:   "./cmd/app/frontend",
					Build:  "npm run build",
					Watch:  append([]string(nil), matchers...),
					Ignore: append([]string(nil), matchers...),
				},
			},
		},
	}}}

	encoded, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal syntax-sensitive matcher lists: %v", err)
	}
	if strings.Count(string(encoded), "watch: [") != 2 || strings.Count(string(encoded), "ignore: [") != 2 {
		t.Fatalf("matcher lists were not emitted as compact flow sequences:\n%s", encoded)
	}

	var document yaml.Node
	if err := yaml.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("parse marshaled syntax-sensitive matcher lists: %v\n%s", err, encoded)
	}
	flowSequences := 0
	var visit func(*yaml.Node)
	visit = func(node *yaml.Node) {
		if node.Kind == yaml.MappingNode {
			for index := 0; index+1 < len(node.Content); index += 2 {
				key := node.Content[index]
				value := node.Content[index+1]
				if key.Value == "watch" || key.Value == "ignore" {
					if value.Kind != yaml.SequenceNode || value.Style&yaml.FlowStyle == 0 {
						t.Fatalf("%s matcher was not a flow sequence:\n%s", key.Value, encoded)
					}
					flowSequences++
					for _, scalar := range value.Content {
						if scalar.Kind != yaml.ScalarNode || scalar.Tag != "!!str" {
							t.Fatalf("flow matcher decoded as kind %d tag %q value %q:\n%s", scalar.Kind, scalar.Tag, scalar.Value, encoded)
						}
					}
				}
				visit(value)
			}
			return
		}
		for _, child := range node.Content {
			visit(child)
		}
	}
	visit(&document)
	if flowSequences != 4 {
		t.Fatalf("flow matcher sequence count = %d, want 4:\n%s", flowSequences, encoded)
	}

	var roundTripped Config
	if err := yaml.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal syntax-sensitive matcher lists: %v\n%s", err, encoded)
	}
	app := roundTripped.Dev.Apps["app"]
	if app.Build == nil || !reflect.DeepEqual(app.Build.Watch, matchers) || !reflect.DeepEqual(app.Build.Ignore, matchers) {
		t.Fatalf("build matchers changed across YAML round trip: %#v", app.Build)
	}
	frontend := app.SPAs["frontend"]
	if !reflect.DeepEqual(frontend.Watch, matchers) || !reflect.DeepEqual(frontend.Ignore, matchers) {
		t.Fatalf("SPA matchers changed across YAML round trip: %#v", frontend)
	}
}

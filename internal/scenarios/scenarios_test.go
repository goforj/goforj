package scenarios

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestMain turns the package test binary into a deterministic subprocess for scenario command tests.
func TestMain(m *testing.M) {
	if os.Getenv("GOFORJ_SCENARIO_HELPER") == "1" {
		failArgument := os.Getenv("GOFORJ_SCENARIO_FAIL_ARGUMENT")
		for _, argument := range os.Args[1:] {
			if failArgument != "" && argument == failArgument {
				os.Exit(9)
			}
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestLoadEmbeddedScenarioSpecs verifies the shipped catalog is non-empty and includes the golden-path entry point.
func TestLoadEmbeddedScenarioSpecs(t *testing.T) {
	specs, err := List("")
	if err != nil {
		t.Fatalf("load specs: %v", err)
	}
	if len(specs) == 0 {
		t.Fatal("expected embedded specs")
	}
	found := false
	for _, spec := range specs {
		if spec.ID == "json-api-route" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected json-api-route spec")
	}
}

// TestValidateScenarioCatalog verifies malformed IDs and dependency graphs fail before execution can consume the catalog.
func TestValidateScenarioCatalog(t *testing.T) {
	tests := []struct {
		name    string
		specs   []ScenarioSpec
		wantErr string
	}{
		{
			name:    "empty catalog",
			wantErr: "scenario catalog is empty",
		},
		{
			name:    "absolute ID",
			specs:   []ScenarioSpec{{ID: "/tmp/escape"}},
			wantErr: `scenario ID "/tmp/escape" must be a safe slug`,
		},
		{
			name:    "traversal ID",
			specs:   []ScenarioSpec{{ID: "../escape"}},
			wantErr: `scenario ID "../escape" must be a safe slug`,
		},
		{
			name:    "forward path separator",
			specs:   []ScenarioSpec{{ID: "group/scenario"}},
			wantErr: `scenario ID "group/scenario" must be a safe slug`,
		},
		{
			name:    "backward path separator",
			specs:   []ScenarioSpec{{ID: `group\scenario`}},
			wantErr: `scenario ID "group\\scenario" must be a safe slug`,
		},
		{
			name:    "duplicate ID",
			specs:   []ScenarioSpec{{ID: "first"}, {ID: "first"}},
			wantErr: `duplicate scenario ID "first"`,
		},
		{
			name:    "unknown dependency",
			specs:   []ScenarioSpec{{ID: "first", DependsOn: []string{"missing"}}},
			wantErr: `scenario "first" depends on unknown scenario "missing"`,
		},
		{
			name: "dependency cycle",
			specs: []ScenarioSpec{
				{ID: "first", DependsOn: []string{"second"}},
				{ID: "second", DependsOn: []string{"third"}},
				{ID: "third", DependsOn: []string{"first"}},
			},
			wantErr: "scenario dependency cycle: first -> second -> third -> first",
		},
		{
			name: "valid DAG",
			specs: []ScenarioSpec{
				{ID: "base"},
				{ID: "cache", DependsOn: []string{"base"}},
				{ID: "events", DependsOn: []string{"base"}},
				{ID: "app", DependsOn: []string{"cache", "events"}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateScenarioCatalog(test.specs)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("validate catalog: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validate catalog succeeded, want error containing %q", test.wantErr)
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validate catalog error = %q, want substring %q", err, test.wantErr)
			}
		})
	}
}

// TestDecodeScenarioSpecRejectsUnknownFields prevents misspelled spec keys from silently disappearing from published scenarios.
func TestDecodeScenarioSpecRejectsUnknownFields(t *testing.T) {
	_, err := decodeScenarioSpec([]byte("id: example\ntitle: Example\nmarkdown:\n  intros: typo\n"))
	if err == nil || !strings.Contains(err.Error(), "field intros not found") {
		t.Fatalf("decodeScenarioSpec() error = %v, want unknown-field failure", err)
	}
}

// TestDecodeScenarioSpecRejectsTrailingDocuments keeps one file from smuggling an unvalidated second scenario document.
func TestDecodeScenarioSpecRejectsTrailingDocuments(t *testing.T) {
	_, err := decodeScenarioSpec([]byte("id: first\ntitle: First\n---\nid: second\ntitle: Second\n"))
	if err == nil || !strings.Contains(err.Error(), "multiple YAML documents") {
		t.Fatalf("decodeScenarioSpec() error = %v, want trailing-document failure", err)
	}
}

// TestDecodeScenarioSpecV2NormalizesPreparation verifies the live schema compiles into the execution model shared with legacy specs.
func TestDecodeScenarioSpecV2NormalizesPreparation(t *testing.T) {
	spec, err := decodeScenarioSpec([]byte(`schema_version: 2
id: invoice-http-route
title: Invoice HTTP Route
prepare:
  steps:
    - id: seed-invoices
      title: Seed invoices
      command: [go, run, ./internal/testseed]
  checks:
    - command: [go, test, ./internal/invoices]
      contains: [PASS]
steps:
  - id: scaffold-controller
    title: Scaffold the Controller
    command: [forj, "make:controller", invoices]
checks:
  - command: [forj, build]
`))
	if err != nil {
		t.Fatalf("decode v2 scenario: %v", err)
	}
	if spec.SchemaVersion != 2 {
		t.Fatalf("schema version = %d, want 2", spec.SchemaVersion)
	}
	if got := spec.Prepare.Steps[0].Run.Run; strings.Join(got, " ") != "go run ./internal/testseed" {
		t.Fatalf("preparation command = %q", got)
	}
	if got := spec.Prepare.Checks[0].Contains; len(got) != 1 || got[0] != "PASS" {
		t.Fatalf("preparation check evidence = %q", got)
	}
	if got := spec.Steps[0].ID; got != "scaffold-controller" {
		t.Fatalf("target step ID = %q", got)
	}
	if got := spec.Verify.Commands[0].Run; strings.Join(got, " ") != "forj build" {
		t.Fatalf("final check = %q", got)
	}
}

// TestDecodeScenarioSpecV2RejectsInvalidContracts proves malformed live recipes fail before catalog execution.
func TestDecodeScenarioSpecV2RejectsInvalidContracts(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "unsupported version", body: "schema_version: 3\nid: example\ntitle: Example\n", wantErr: "unsupported schema_version 3"},
		{name: "unknown field", body: "schema_version: 2\nid: example\ntitle: Example\nprepares: {}\n", wantErr: "field prepares not found"},
		{name: "missing action", body: "schema_version: 2\nid: example\ntitle: Example\nsteps:\n  - id: add-route\n    title: Add route\n", wantErr: "must declare exactly one"},
		{name: "multiple actions", body: "schema_version: 2\nid: example\ntitle: Example\nsteps:\n  - id: add-route\n    title: Add route\n    command: [forj, build]\n    write:\n      path: route.go\n      content: package route\n", wantErr: "must declare exactly one"},
		{name: "invalid step ID", body: "schema_version: 2\nid: example\ntitle: Example\nsteps:\n  - id: Add Route\n    title: Add route\n    command: [forj, build]\n", wantErr: "must be a safe slug"},
		{name: "duplicate step ID", body: "schema_version: 2\nid: example\ntitle: Example\nprepare:\n  steps:\n    - id: add-route\n      title: Prepare route\n      command: [forj, build]\nsteps:\n  - id: add-route\n    title: Add route\n    command: [forj, build]\n", wantErr: `duplicate scenario step ID "add-route"`},
		{name: "missing check command", body: "schema_version: 2\nid: example\ntitle: Example\nchecks:\n  - contains: [PASS]\n", wantErr: "checks[0].command is required"},
		{name: "blank executable", body: "schema_version: 2\nid: example\ntitle: Example\nchecks:\n  - command: ['']\n", wantErr: "checks[0].command executable is required"},
		{name: "shell step", body: "schema_version: 2\nid: example\ntitle: Example\nsteps:\n  - id: run-shell\n    title: Run shell\n    command: [/bin/sh, -c, echo hidden]\n", wantErr: "steps[0].command must not invoke a shell interpreter"},
		{name: "shell check", body: "schema_version: 2\nid: example\ntitle: Example\nchecks:\n  - command: [pwsh.exe, -Command, Write-Output hidden]\n", wantErr: "checks[0].command must not invoke a shell interpreter"},
		{name: "command path", body: "schema_version: 2\nid: example\ntitle: Example\nchecks:\n  - command: [/usr/bin/tool]\n", wantErr: "checks[0].command executable must be a tool name, not a path"},
		{name: "file traversal", body: "schema_version: 2\nid: example\ntitle: Example\nsteps:\n  - id: write-file\n    title: Write file\n    write:\n      path: ../escape.txt\n      content: escape\n", wantErr: "steps[0].write.path \"../escape.txt\" must be a relative path"},
		{name: "missing file body", body: "schema_version: 2\nid: example\ntitle: Example\nsteps:\n  - id: write-file\n    title: Write file\n    write:\n      path: example.txt\n      content: ''\n", wantErr: "steps[0].write.content is required"},
		{name: "invalid Go body", body: "schema_version: 2\nid: example\ntitle: Example\nsteps:\n  - id: write-file\n    title: Write file\n    write:\n      path: example.go\n      content: 'package example\\nfunc Broken('\n", wantErr: "steps[0].write.content: invalid Go source"},
		{name: "single backslash traversal", body: "schema_version: 2\nid: example\ntitle: Example\nsteps:\n  - id: write-file\n    title: Write file\n    write:\n      path: ..\\escape.txt\n      content: escape\n", wantErr: "must be a relative path"},
		{name: "windows drive path", body: "schema_version: 2\nid: example\ntitle: Example\nsteps:\n  - id: write-file\n    title: Write file\n    write:\n      path: C:\\escape.txt\n      content: escape\n", wantErr: "must be a relative path"},
		{name: "anchor", body: "schema_version: 2\nid: example\ntitle: Example\nsteps:\n  - &step\n    id: add-route\n    title: Add route\n    command: [forj, build]\n", wantErr: "aliases and anchors are not supported"},
		{name: "alias", body: "schema_version: 2\nid: example\ntitle: Example\nsteps:\n  - &step\n    id: add-route\n    title: Add route\n    command: [forj, build]\n  - *step\n", wantErr: "aliases and anchors are not supported"},
		{name: "merge key", body: "schema_version: 2\nid: example\ntitle: Example\nprepare: &base {}\n<<: *base\n", wantErr: "YAML merge keys are not supported"},
		{name: "duplicate key", body: "schema_version: 2\nid: example\nid: second\ntitle: Example\n", wantErr: "mapping key \"id\" already defined"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeScenarioSpec([]byte(test.body))
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("decodeScenarioSpec() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

// TestDecodeScenarioSpecV2AllowsFragmentGoAppendAndEmptyReplacement keeps real source edits concise without weakening whole-file writes.
func TestDecodeScenarioSpecV2AllowsFragmentGoAppendAndEmptyReplacement(t *testing.T) {
	spec, err := decodeScenarioSpec([]byte("schema_version: 2\nid: example\ntitle: Example\nsteps:\n  - id: append-route\n    title: Append route\n    append:\n      path: route.go\n      content: '\\nfunc Route() {}\\n'\n  - id: remove-marker\n    title: Remove marker\n    replace:\n      path: route.go\n      old: marker\n      new: ''\n"))
	if err != nil {
		t.Fatalf("decodeScenarioSpec(): %v", err)
	}
	if spec.Steps[0].Append == nil || spec.Steps[1].Replace == nil || spec.Steps[1].Replace.New != "" {
		t.Fatalf("normalized source edits = %#v", spec.Steps)
	}
}

// TestDecodeScenarioSpecV1PreservesLegacyActions ensures schema selection does not tighten established external catalogs.
func TestDecodeScenarioSpecV1PreservesLegacyActions(t *testing.T) {
	spec, err := decodeScenarioSpec([]byte("id: example\ntitle: Example\nsteps:\n  - title: Legacy\n    run:\n      run: [forj, build]\n"))
	if err != nil {
		t.Fatalf("decode legacy scenario: %v", err)
	}
	if spec.SchemaVersion != 0 || len(spec.Prepare.Steps) != 0 || spec.Steps[0].ID != "" {
		t.Fatalf("legacy normalization changed: %#v", spec)
	}
}

// TestEmbeddedV1ScenariosRetainDecodeAndDocumentationParity freezes legacy behavior while the live schema migrates incrementally.
func TestEmbeddedV1ScenariosRetainDecodeAndDocumentationParity(t *testing.T) {
	entries, err := embeddedScenarioSpecs.ReadDir("specs")
	if err != nil {
		t.Fatalf("read embedded specs: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			body, err := embeddedScenarioSpecs.ReadFile("specs/" + entry.Name())
			if err != nil {
				t.Fatalf("read embedded spec: %v", err)
			}
			_, version, err := decodeScenarioDocument(body)
			if err != nil {
				t.Fatalf("decode scenario document: %v", err)
			}
			if version != 0 {
				t.Skip("versioned scenario is covered by its schema contract")
			}
			var legacy scenarioSpecV1
			decoder := yaml.NewDecoder(strings.NewReader(string(body)))
			decoder.KnownFields(true)
			if err := decoder.Decode(&legacy); err != nil {
				t.Fatalf("decode legacy wire: %v", err)
			}
			want := normalizeScenarioV1(legacy)
			if strings.TrimSpace(want.App.ModuleName) == "" {
				want.App.ModuleName = "example.com/" + strings.ReplaceAll(want.ID, "-", "")
			}
			got, err := decodeScenarioSpec(body)
			if err != nil {
				t.Fatalf("decode selected schema: %v", err)
			}
			wantYAML, err := yaml.Marshal(want)
			if err != nil {
				t.Fatalf("marshal legacy spec: %v", err)
			}
			gotYAML, err := yaml.Marshal(got)
			if err != nil {
				t.Fatalf("marshal selected spec: %v", err)
			}
			if !bytes.Equal(gotYAML, wantYAML) {
				index := 0
				for index < len(gotYAML) && index < len(wantYAML) && gotYAML[index] == wantYAML[index] {
					index++
				}
				start := index - 80
				if start < 0 {
					start = 0
				}
				gotEnd := index + 160
				if gotEnd > len(gotYAML) {
					gotEnd = len(gotYAML)
				}
				wantEnd := index + 160
				if wantEnd > len(wantYAML) {
					wantEnd = len(wantYAML)
				}
				t.Fatalf("normalized v1 spec changed at byte %d\ngot:  %q\nwant: %q", index, gotYAML[start:gotEnd], wantYAML[start:wantEnd])
			}
			gotMarkdown, err := renderScenarioMarkdown(got)
			if err != nil {
				t.Fatalf("render selected schema: %v", err)
			}
			wantMarkdown, err := renderScenarioMarkdown(want)
			if err != nil {
				t.Fatalf("render legacy schema: %v", err)
			}
			if gotMarkdown != wantMarkdown {
				t.Fatal("legacy generated documentation changed during schema selection")
			}
		})
	}
}

// TestCompileScenarioPlanOrdersDependencyDiamondOnce proves plan compilation owns shared dependency ordering for every execution mode.
func TestCompileScenarioPlanOrdersDependencyDiamondOnce(t *testing.T) {
	step := func(id string) ScenarioStep {
		return ScenarioStep{ID: id, Title: id, Run: &ScenarioCommand{Run: []string{"true"}}}
	}
	specs := []ScenarioSpec{
		{ID: "base", Prepare: ScenarioPreparation{Steps: []ScenarioStep{step("base-prepare")}}, Steps: []ScenarioStep{step("base-target")}},
		{ID: "cache", DependsOn: []string{"base"}, Steps: []ScenarioStep{step("cache-target")}},
		{ID: "events", DependsOn: []string{"base"}, Steps: []ScenarioStep{step("events-target")}},
		{ID: "app", DependsOn: []string{"cache", "events"}, Prepare: ScenarioPreparation{Steps: []ScenarioStep{step("app-prepare")}}, Steps: []ScenarioStep{step("app-target")}},
	}
	byID := make(map[string]ScenarioSpec, len(specs))
	for _, spec := range specs {
		byID[spec.ID] = spec
	}
	plan, err := compileScenarioPlan(byID["app"], byID)
	if err != nil {
		t.Fatalf("compile scenario plan: %v", err)
	}
	var got []string
	for _, planned := range plan.dependencySteps {
		got = append(got, planned.step.ID)
	}
	want := "base-prepare,base-target,cache-target,events-target"
	if strings.Join(got, ",") != want {
		t.Fatalf("dependency steps = %q, want %q", got, want)
	}
	if len(plan.preparationSteps) != 1 || plan.preparationSteps[0].step.ID != "app-prepare" || len(plan.targetSteps) != 1 || plan.targetSteps[0].step.ID != "app-target" {
		t.Fatalf("selected plan stages = %#v / %#v", plan.preparationSteps, plan.targetSteps)
	}
}

// TestUnsafeScenarioIDCannotMutateOutsideRoots verifies generation and execution reject a catalog before ID-derived paths are touched.
func TestUnsafeScenarioIDCannotMutateOutsideRoots(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, specDir, operationRoot string) error
	}{
		{
			name: "generate",
			run: func(t *testing.T, specDir, operationRoot string) error {
				t.Helper()
				return Generate(GenerateOptions{SpecDir: specDir, OutDir: operationRoot})
			},
		},
		{
			name: "validate",
			run: func(t *testing.T, specDir, operationRoot string) error {
				t.Helper()
				return Validate(ValidateOptions{Logger: logger.NewSilentLogger(), SpecDir: specDir, WorkDir: operationRoot, ForjExec: "unused"})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := t.TempDir()
			specDir := filepath.Join(base, "specs")
			operationRoot := filepath.Join(base, "operation")
			outsideRoot := filepath.Join(base, "outside")
			if err := os.MkdirAll(outsideRoot, 0o755); err != nil {
				t.Fatalf("create outside root: %v", err)
			}
			sentinelPath := filepath.Join(outsideRoot, "sentinel.txt")
			if err := os.WriteFile(sentinelPath, []byte("preserve me"), 0o644); err != nil {
				t.Fatalf("write sentinel: %v", err)
			}
			outsideMarkdownPath := filepath.Join(base, "outside.md")
			if err := os.WriteFile(outsideMarkdownPath, []byte("preserve this too"), 0o644); err != nil {
				t.Fatalf("write markdown sentinel: %v", err)
			}
			writeScenarioSpecFixture(t, specDir, "unsafe.yaml", "id: ../outside\ntitle: Unsafe scenario\n")

			err := test.run(t, specDir, operationRoot)
			if err == nil {
				t.Fatal("operation succeeded with an unsafe scenario ID")
			}
			body, readErr := os.ReadFile(sentinelPath)
			if readErr != nil {
				t.Fatalf("read sentinel after rejection: %v", readErr)
			}
			if string(body) != "preserve me" {
				t.Fatalf("sentinel content = %q, want it unchanged", body)
			}
			markdownBody, readErr := os.ReadFile(outsideMarkdownPath)
			if readErr != nil {
				t.Fatalf("read markdown sentinel after rejection: %v", readErr)
			}
			if string(markdownBody) != "preserve this too" {
				t.Fatalf("markdown sentinel content = %q, want it unchanged", markdownBody)
			}
			if _, statErr := os.Stat(operationRoot); !os.IsNotExist(statErr) {
				t.Fatalf("operation root was touched before catalog validation: %v", statErr)
			}
		})
	}
}

// TestValidateRequiresLogger establishes the execution logger before subprocess failures need it.
func TestValidateRequiresLogger(t *testing.T) {
	if err := Validate(ValidateOptions{}); err == nil || !strings.Contains(err.Error(), "logger is required") {
		t.Fatalf("Validate() error = %v, want logger invariant", err)
	}
}

// TestPrepareRejectsLegacyScenarioBeforeMutation keeps live evaluation from guessing at a v1 starting state.
func TestPrepareRejectsLegacyScenarioBeforeMutation(t *testing.T) {
	base := t.TempDir()
	specDir := filepath.Join(base, "specs")
	workDir := filepath.Join(base, "work")
	writeScenarioSpecFixture(t, specDir, "legacy.yaml", "id: legacy\ntitle: Legacy\n")

	_, err := Prepare(context.Background(), PrepareOptions{
		Logger:      logger.NewSilentLogger(),
		SpecDir:     specDir,
		WorkDir:     workDir,
		ScenarioID:  "legacy",
		ForjExec:    os.Args[0],
		Environment: os.Environ(),
	})
	if !errors.Is(err, ErrUnsupportedLiveScenario) {
		t.Fatalf("Prepare() error = %v, want unsupported live scenario", err)
	}
	if _, statErr := os.Stat(workDir); !os.IsNotExist(statErr) {
		t.Fatalf("work root was mutated before rejection: %v", statErr)
	}
}

// TestResolvePreparationReturnsStableLivePlanWithoutMutation separates trusted selection from workspace execution.
func TestResolvePreparationReturnsStableLivePlanWithoutMutation(t *testing.T) {
	specDir := t.TempDir()
	writeScenarioSpecFixture(t, specDir, "live.yaml", "schema_version: 2\nid: live\ntitle: Live\n")
	resolved, err := ResolvePreparation(ResolveOptions{SpecDir: specDir, ScenarioID: "live"})
	if err != nil {
		t.Fatalf("ResolvePreparation(): %v", err)
	}
	if resolved.ScenarioID != "live" || resolved.SchemaVersion != liveScenarioSchemaVersion || resolved.PlanDigest == "" || resolved.CatalogDigest == "" || len(resolved.ScenarioDigests) != 1 || len(resolved.ProjectConfigYAML) == 0 {
		t.Fatalf("resolved preparation = %#v", resolved)
	}
	if resolved.ScenarioDigests[0].ID != "live" {
		t.Fatalf("scenario digests = %#v", resolved.ScenarioDigests)
	}
	entries, err := os.ReadDir(specDir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "live.yaml" {
		t.Fatalf("resolution mutated source catalog: entries=%v error=%v", entries, err)
	}
}

// TestPrepareExecutesOnlyLivePrefix proves dependencies and fixture work cannot leak the golden target into the agent workspace.
func TestPrepareExecutesOnlyLivePrefix(t *testing.T) {
	specDir := t.TempDir()
	writeScenarioSpecFixture(t, specDir, "base.yaml", `schema_version: 2
id: invoice-domain
title: Invoice Domain
steps:
  - id: add-domain
    title: Add domain
    write:
      path: internal/invoices/domain.txt
      content: dependency-state
`)
	writeScenarioSpecFixture(t, specDir, "live.yaml", `schema_version: 2
id: invoice-http-route
title: Invoice HTTP Route
depends_on: [invoice-domain]
prepare:
  steps:
    - id: seed-invoices
      title: Seed invoices
      write:
        path: internal/invoices/seed.txt
        content: prepared-state
  checks:
    - command: [forj, check-start]
steps:
  - id: add-controller
    title: Add controller
    write:
      path: internal/invoices/controller.txt
      content: target-oracle-secret
checks:
  - command: [forj, check-target]
`)
	environment := append(os.Environ(), "GOFORJ_SCENARIO_HELPER=1")
	prepared, err := Prepare(context.Background(), PrepareOptions{
		Logger:      logger.NewSilentLogger(),
		SpecDir:     specDir,
		WorkDir:     t.TempDir(),
		ScenarioID:  "invoice-http-route",
		ForjExec:    os.Args[0],
		Environment: environment,
	})
	if err != nil {
		t.Fatalf("Prepare(): %v", err)
	}
	root := prepared.Root
	t.Cleanup(func() {
		if err := prepared.Close(); err != nil {
			t.Fatalf("close prepared scenario: %v", err)
		}
	})

	for _, path := range []string{"internal/invoices/domain.txt", "internal/invoices/seed.txt"} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("prepared workspace missing %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "internal/invoices/controller.txt")); !os.IsNotExist(err) {
		t.Fatalf("target step reached prepared workspace: %v", err)
	}
	if prepared.SchemaVersion != 2 || prepared.CatalogDigest == "" || len(prepared.ScenarioDigests) != 2 || prepared.ForjExecutable == "" || prepared.ForjDigest == "" || prepared.PlanDigest == "" || prepared.BaselineTree == "" {
		t.Fatalf("prepared metadata = %#v", prepared)
	}
	if prepared.ScenarioDigests[0].ID != "invoice-domain" || prepared.ScenarioDigests[1].ID != "invoice-http-route" {
		t.Fatalf("scenario digest order = %#v", prepared.ScenarioDigests)
	}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(body), "target-oracle-secret") {
			return fmt.Errorf("target oracle leaked into %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestPrepareRejectsChangedResolvedPlanBeforeMutation prevents selection and execution from observing different external catalogs.
func TestPrepareRejectsChangedResolvedPlanBeforeMutation(t *testing.T) {
	specDir := t.TempDir()
	writeScenarioSpecFixture(t, specDir, "live.yaml", `schema_version: 2
id: live
title: Live
steps:
  - id: target
    title: Target
    write:
      path: target.txt
      content: target
`)
	workRoot := filepath.Join(t.TempDir(), "work")
	_, err := Prepare(context.Background(), PrepareOptions{
		Logger:             logger.NewSilentLogger(),
		SpecDir:            specDir,
		WorkDir:            workRoot,
		ScenarioID:         "live",
		ForjExec:           os.Args[0],
		Environment:        os.Environ(),
		ExpectedPlanDigest: "sha256:stale",
	})
	if err == nil || !strings.Contains(err.Error(), "resolved preparation plan changed") {
		t.Fatalf("Prepare() error = %v, want changed-plan rejection", err)
	}
	if _, statErr := os.Stat(workRoot); !os.IsNotExist(statErr) {
		t.Fatalf("changed plan mutated workspace: %v", statErr)
	}
}

// TestPrepareRejectsChangedResolvedToolsBeforeMutation keeps the execution bytes bound to the toolchain identity selected by the caller.
func TestPrepareRejectsChangedResolvedToolsBeforeMutation(t *testing.T) {
	specDir := t.TempDir()
	writeScenarioSpecFixture(t, specDir, "live.yaml", `schema_version: 2
id: live
title: Live
steps:
  - id: target
    title: Target
    write:
      path: target.txt
      content: target
`)
	workRoot := filepath.Join(t.TempDir(), "work")
	_, err := Prepare(context.Background(), PrepareOptions{
		Logger:             logger.NewSilentLogger(),
		SpecDir:            specDir,
		WorkDir:            workRoot,
		ScenarioID:         "live",
		ForjExec:           os.Args[0],
		Environment:        os.Environ(),
		ExpectedToolDigest: "sha256:stale",
	})
	if err == nil || !strings.Contains(err.Error(), "resolved scenario tools changed") {
		t.Fatalf("Prepare() error = %v, want changed-tool rejection", err)
	}
	if _, statErr := os.Stat(workRoot); !os.IsNotExist(statErr) {
		t.Fatalf("changed tools mutated workspace: %v", statErr)
	}
}

// TestClonePreparedCreatesAnIndependentIdenticalTree verifies cached bases never become candidate workspaces.
func TestClonePreparedCreatesAnIndependentIdenticalTree(t *testing.T) {
	baseRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseRoot, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseRoot, "internal", "service.go"), []byte("package internal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest, err := digestScenarioTree(baseRoot)
	if err != nil {
		t.Fatal(err)
	}
	base := &PreparedScenario{Root: baseRoot, ScenarioID: "clone-fixture", BaselineTree: digest}
	clone, err := ClonePrepared(base, t.TempDir())
	if err != nil {
		t.Fatalf("ClonePrepared(): %v", err)
	}
	clonePath := filepath.Join(clone.Root, "internal", "service.go")
	if err := os.WriteFile(clonePath, []byte("package changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(baseRoot, "internal", "service.go"))
	if err != nil || string(body) != "package internal\n" {
		t.Fatalf("base changed through clone: %q, %v", body, err)
	}
	if err := clone.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}
	if _, err := os.Stat(clone.Root); !os.IsNotExist(err) {
		t.Fatalf("clone root survived Close(): %v", err)
	}
}

// TestClonePreparedRejectsSymlinks keeps version-one bases portable and closes link semantics at the trust boundary.
func TestClonePreparedRejectsSymlinks(t *testing.T) {
	baseRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseRoot, "target"), []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(baseRoot, "link")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	digest, err := digestScenarioTree(baseRoot)
	if err != nil {
		t.Fatal(err)
	}
	base := &PreparedScenario{Root: baseRoot, ScenarioID: "linked-clone", BaselineTree: digest}
	_, err = ClonePrepared(base, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "unsupported symlink") {
		t.Fatalf("ClonePrepared() error = %v, want link rejection", err)
	}
}

// TestClonePreparedPreservesReadOnlyDirectories proves post-order mode restoration does not block copying children.
func TestClonePreparedPreservesReadOnlyDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce Unix directory write modes")
	}
	baseRoot := t.TempDir()
	directory := filepath.Join(baseRoot, "readonly")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "fixture.txt"), []byte("fixture"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(directory, 0o700) })
	digest, err := digestScenarioTree(baseRoot)
	if err != nil {
		t.Fatal(err)
	}
	clone, err := ClonePrepared(&PreparedScenario{Root: baseRoot, ScenarioID: "readonly-clone", BaselineTree: digest}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clone.Close() })
	info, err := os.Stat(filepath.Join(clone.Root, "readonly"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o555 {
		t.Fatalf("cloned directory mode = %o, want 555", info.Mode().Perm())
	}
}

// TestInvoiceHTTPRoutePlanKeepsControllerOutOfPreparation protects the real diagnostic fixture from leaking its target implementation.
func TestInvoiceHTTPRoutePlanKeepsControllerOutOfPreparation(t *testing.T) {
	catalog, err := loadScenarioCatalog("")
	if err != nil {
		t.Fatalf("loadScenarioCatalog(): %v", err)
	}
	plan, ok := catalog.plans["invoice-http-route"]
	if !ok {
		t.Fatal("invoice-http-route scenario is not registered")
	}
	if plan.spec.SchemaVersion != liveScenarioSchemaVersion {
		t.Fatalf("schema version = %d, want %d", plan.spec.SchemaVersion, liveScenarioSchemaVersion)
	}
	if !reflect.DeepEqual(plan.dependencyScenarioIDs, []string{"invoice-domain"}) {
		t.Fatalf("dependency closure = %q", plan.dependencyScenarioIDs)
	}
	if len(plan.targetSteps) != 2 || plan.targetSteps[0].step.ID != "scaffold-invoice-controller" {
		t.Fatalf("target steps = %#v", plan.targetSteps)
	}
	for _, step := range append(append([]plannedScenarioStep(nil), plan.dependencySteps...), plan.preparationSteps...) {
		if step.step.Write != nil && step.step.Write.Path == "internal/invoices/controller.go" {
			t.Fatalf("controller target leaked through %s step %q", step.spec.ID, step.step.ID)
		}
		if step.step.Run != nil && reflect.DeepEqual(step.step.Run.Run, []string{"forj", "make:controller", "invoices"}) {
			t.Fatalf("generator target leaked through %s step %q", step.spec.ID, step.step.ID)
		}
	}
}

// TestPrepareFailureNeverAppliesTarget proves a failed starting-state check cannot fall through into golden work.
func TestPrepareFailureNeverAppliesTarget(t *testing.T) {
	specDir := t.TempDir()
	writeScenarioSpecFixture(t, specDir, "failure.yaml", `schema_version: 2
id: failing-start
title: Failing Start
prepare:
  steps:
    - id: prepare-state
      title: Prepare state
      write:
        path: prepared.txt
        content: prepared
  checks:
    - command: [forj, reject-start]
steps:
  - id: apply-target
    title: Apply target
    write:
      path: target.txt
      content: target`)
	workDir := t.TempDir()
	environment := append(os.Environ(), "GOFORJ_SCENARIO_HELPER=1", "GOFORJ_SCENARIO_FAIL_ARGUMENT=reject-start")
	_, err := Prepare(context.Background(), PrepareOptions{
		Logger:      logger.NewSilentLogger(),
		SpecDir:     specDir,
		WorkDir:     workDir,
		Keep:        true,
		ScenarioID:  "failing-start",
		ForjExec:    os.Args[0],
		Environment: environment,
	})
	if err == nil || !strings.Contains(err.Error(), "reject-start") {
		t.Fatalf("Prepare() error = %v, want starting-check failure", err)
	}
	entries, readErr := os.ReadDir(workDir)
	if readErr != nil || len(entries) != 1 {
		t.Fatalf("retained failed workspaces = %v, %v", entries, readErr)
	}
	root := filepath.Join(workDir, entries[0].Name())
	if _, statErr := os.Stat(filepath.Join(root, "prepared.txt")); statErr != nil {
		t.Fatalf("preparation step did not run: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, "target.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("target step ran after failed starting check: %v", statErr)
	}
}

// TestValidateV2ExecutesCompletePlan proves ordinary scenario testing extends the same prefix through target work.
func TestValidateV2ExecutesCompletePlan(t *testing.T) {
	specDir := t.TempDir()
	writeScenarioSpecFixture(t, specDir, "complete.yaml", `schema_version: 2
id: complete-plan
title: Complete Plan
prepare:
  steps:
    - id: prepare-state
      title: Prepare state
      write:
        path: prepared.txt
        content: prepared
  checks:
    - command: [forj, check-start]
steps:
  - id: apply-target
    title: Apply target
    write:
      path: target.txt
      content: target
checks:
  - command: [forj, check-target]`)
	workDir := t.TempDir()
	environment := append(os.Environ(), "GOFORJ_SCENARIO_HELPER=1")
	if err := Validate(ValidateOptions{
		Logger:      logger.NewSilentLogger(),
		SpecDir:     specDir,
		WorkDir:     workDir,
		Keep:        true,
		IDs:         []string{"complete-plan"},
		ForjExec:    os.Args[0],
		Environment: environment,
	}); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	entries, err := os.ReadDir(workDir)
	if err != nil {
		t.Fatalf("read work root: %v", err)
	}
	var completedRoot string
	for _, entry := range entries {
		candidate := filepath.Join(workDir, entry.Name())
		if _, err := os.Stat(filepath.Join(candidate, "target.txt")); err == nil {
			completedRoot = candidate
			break
		}
	}
	if completedRoot == "" {
		t.Fatalf("complete target not found beneath %s", workDir)
	}
	for _, path := range []string{"prepared.txt", "target.txt"} {
		if _, err := os.Stat(filepath.Join(completedRoot, path)); err != nil {
			t.Fatalf("complete plan missing %s: %v", path, err)
		}
	}
}

// TestRenderScenarioMarkdownIncludesPreparationContext keeps fixture work distinct from numbered target steps.
func TestRenderScenarioMarkdownIncludesPreparationContext(t *testing.T) {
	spec, err := decodeScenarioSpec([]byte(`schema_version: 2
id: prepared-doc
title: Prepared Doc
prepare:
  steps:
    - id: seed-data
      title: Seed data
      command: [forj, seed]
  checks:
    - command: [forj, check-start]
steps:
  - id: add-feature
    title: Add feature
    command: [forj, build]
`))
	if err != nil {
		t.Fatalf("decode scenario: %v", err)
	}
	body, err := renderScenarioMarkdown(spec)
	if err != nil {
		t.Fatalf("render scenario: %v", err)
	}
	for _, token := range []string{"## Starting State", "### Seed data", "forj check-start", "## Step 1: Add feature"} {
		if !strings.Contains(body, token) {
			t.Fatalf("rendered v2 documentation missing %q\n%s", token, body)
		}
	}
}

// writeScenarioSpecFixture writes an external catalog fixture so public entry points exercise the same loader as user-provided specs.
func writeScenarioSpecFixture(t *testing.T, specDir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("create spec dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(specDir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write scenario spec: %v", err)
	}
}

// TestScenarioSpecsDeclareCumulativePrimitiveDependencies keeps each independently rendered scenario truthful about the prior steps it applies.
func TestScenarioSpecsDeclareCumulativePrimitiveDependencies(t *testing.T) {
	specs, err := List("")
	if err != nil {
		t.Fatalf("load specs: %v", err)
	}
	byID := make(map[string]ScenarioSpec, len(specs))
	for _, spec := range specs {
		byID[spec.ID] = spec
	}

	tests := []struct {
		id   string
		want project.Components
	}{
		{id: "json-api-route", want: project.Components{}},
		{id: "cached-user-profile", want: project.Components{Cache: true}},
		{id: "file-upload-storage", want: project.Components{Cache: true, Storage: true}},
		{id: "users-created-event", want: project.Components{Cache: true, Events: true, Storage: true}},
		{id: "reports-generate-job", want: project.Components{Cache: true, Events: true, Storage: true, Jobs: true}},
		{id: "reports-daily-schedule", want: project.Components{Cache: true, Events: true, Storage: true, Jobs: true}},
		{id: "runtime-observability", want: project.Components{Cache: true, Events: true, Storage: true, Jobs: true}},
	}
	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			spec, ok := byID[test.id]
			if !ok {
				t.Fatalf("scenario %q was not loaded", test.id)
			}
			got := project.Components{
				Cache:   spec.App.Components.Cache,
				Events:  spec.App.Components.Events,
				Storage: spec.App.Components.Storage,
				Jobs:    spec.App.Components.Jobs,
			}
			if got != test.want {
				t.Fatalf("scenario primitive components = %#v, want %#v", got, test.want)
			}
		})
	}
}

// TestRenderScenarioMarkdownIncludesVerificationBanner verifies generated documentation describes its executable contract.
func TestRenderScenarioMarkdownIncludesVerificationBanner(t *testing.T) {
	catalog, err := loadScenarioCatalog("")
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	specs, err := catalog.selectSpecs([]string{"json-api-route"}, false)
	if err != nil {
		t.Fatalf("select spec: %v", err)
	}
	body, err := renderScenarioMarkdown(specs[0])
	if err != nil {
		t.Fatalf("render markdown: %v", err)
	}
	for _, token := range []string{
		"::: info Verified Scenario",
		"forj make:controller users",
		"forj build",
		"/api/v1/users/:id",
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("generated markdown missing %q\n%s", token, body)
		}
	}
	wantVerification := "```bash\nforj route:list\n```\n\nExpected output includes:\n\n```text\n/api/v1/users/:id\n```"
	if !strings.Contains(body, wantVerification) {
		t.Fatalf("generated markdown does not keep readable expected output beside its command\n%s", body)
	}
}

// TestRenderScenarioMarkdownQuotesFrontMatter keeps punctuation and line breaks from changing YAML structure.
func TestRenderScenarioMarkdownQuotesFrontMatter(t *testing.T) {
	spec := ScenarioSpec{
		Title:       "Reports: daily #1",
		Description: "First line\nsecond line",
	}
	body, err := renderScenarioMarkdown(spec)
	if err != nil {
		t.Fatalf("render markdown: %v", err)
	}

	frontMatter := strings.SplitN(body, "---\n", 3)
	if len(frontMatter) != 3 {
		t.Fatalf("generated markdown has invalid front matter:\n%s", body)
	}
	var metadata struct {
		Title       string `yaml:"title"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(frontMatter[1]), &metadata); err != nil {
		t.Fatalf("decode front matter: %v", err)
	}
	if metadata.Title != spec.Title || metadata.Description != spec.Description {
		t.Fatalf("front matter = %#v, want title %q and description %q", metadata, spec.Title, spec.Description)
	}
}

// TestRenderScenarioMarkdownRejectsInvalidGo keeps documentation generation from publishing malformed complete files.
func TestRenderScenarioMarkdownRejectsInvalidGo(t *testing.T) {
	_, err := renderScenarioMarkdown(ScenarioSpec{
		Steps: []ScenarioStep{{
			Title: "Broken file",
			Write: &ScenarioFileChange{
				Path:     "broken.go",
				Language: "go",
				Content:  "package broken\nfunc Broken(\n",
			},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "format broken.go") {
		t.Fatalf("renderScenarioMarkdown() error = %v, want formatting failure", err)
	}
}

// TestScenarioGenerateCheckDetectsDrift verifies check mode compares rendered content without overwriting it.
func TestScenarioGenerateCheckDetectsDrift(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(GenerateOptions{OutDir: dir, IDs: []string{"json-api-route"}}); err != nil {
		t.Fatalf("generate: %v", err)
	}

	path := filepath.Join(dir, "json-api-route.md")
	if err := os.WriteFile(path, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("write stale doc: %v", err)
	}

	if err := Generate(GenerateOptions{OutDir: dir, Check: true, IDs: []string{"json-api-route"}}); err == nil {
		t.Fatal("expected check to detect drift")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read stale doc after check: %v", err)
	}
	if string(body) != "stale\n" {
		t.Fatalf("check mode rewrote stale doc:\n%s", body)
	}
}

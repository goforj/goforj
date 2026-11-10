package forj

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/goforj/goforj/internal/projectupgrade"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// frameworkUpgradeFixture models an existing app with a private environment and owner additions.
func frameworkUpgradeFixture(t *testing.T, oauth bool) string {
	t.Helper()
	root := t.TempDir()
	config := &project.Config{ProjectName: "Auth upgrade", GoModuleName: "example.com/custom", Render: project.RenderConfig{Components: project.Components{CLI: true, WebAPI: true, Auth: true, OAuth: oauth, Metrics: true, DatabaseSQLite: true}}}
	data, err := yaml.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		".goforj.yml":                      string(data),
		".env":                             "AUTH_PASSWORD_RESET_RETURN_TOKEN=PRIVATE_ENV_CANARY\n",
		"internal/auth/service.go":         "package auth\nfunc CustomLogin() {}\n",
		"internal/auth/impersonation.go":   "package auth\nfunc OperatorPolicy() {}\n",
		"internal/queues/accessors_gen.go": "package queues\nfunc OwnerQueue() {}\n",
		"app/lifecycle.go":                 "package app\n// owner bootstrap\n",
	} {
		writeProjectRendererTestFile(t, filepath.Join(root, name), source)
	}
	return root
}

// TestDiscoveredAppsKeepInheritedRenderSettings prevents discovery from turning inherited starter, showcase, or help settings into defaults.
func TestDiscoveredAppsKeepInheritedRenderSettings(t *testing.T) {
	root := t.TempDir()
	original := &project.Config{ProjectName: "Inherited App", GoModuleName: "example.com/inherited", Render: project.RenderConfig{
		Components: project.Components{CLI: true, WebUI: true, WebAPI: true}, StarterKit: project.StarterKitVue, StarterKitOptions: project.NewStarterKitOptions(false), HelpFormat: project.HelpFormatGuided,
	}}
	source, err := yaml.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	writeProjectRendererTestFile(t, filepath.Join(root, ".goforj.yml"), string(source))
	writeProjectRendererTestFile(t, filepath.Join(root, "app/billing/lifecycle.go"), "package billingapp\n")
	projection, err := frameworkUpgradeConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	app := project.AppForName("billing")
	if appRenderStarterKit(projection, app) != appRenderStarterKit(original, app) || appRenderComponentLibrary(projection, app) != appRenderComponentLibrary(original, app) || appRenderHelpFormat(projection, app) != appRenderHelpFormat(original, app) {
		t.Fatal("discovery changed inherited render settings")
	}
	snapshot, err := frameworkSnapshotForConfig(projection)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, artifact := range snapshot.Artifacts {
		if artifact.Path == "cmd/billing/frontend/package.json" {
			found = true
		}
	}
	if !found {
		t.Fatal("discovered App lost its inherited starter source")
	}
}

// TestFrameworkInputsAreFrozenAndRechecked rejects changed intent while deliberately ignoring owner-only settings.
func TestFrameworkInputsAreFrozenAndRechecked(t *testing.T) {
	root := frameworkUpgradeFixture(t, true)
	projection, err := frameworkUpgradeConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := frameworkSnapshotForConfig(projection)
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(root, ".goforj.yml"))
	if err != nil {
		t.Fatal(err)
	}
	writeProjectRendererTestFile(t, filepath.Join(root, ".goforj.yml"), string(source)+"\nowner_setting: ignored-private-value\n")
	if err := verifyFrameworkInputs(root, snapshot.InputsDigest); err != nil {
		t.Fatal(err)
	}
	writeProjectRendererTestFile(t, filepath.Join(root, "app/newworker/lifecycle.go"), "package newworkerapp\n")
	if err := verifyFrameworkInputs(root, snapshot.InputsDigest); err == nil {
		t.Fatal("accepted App discovery changes during planning")
	}
	frozen, err := frameworkSnapshotForConfig(projection)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot, frozen) {
		t.Fatal("frozen recipe reread changed live project intent")
	}
}

// TestRendererCancellationAndDeadlineRemainObservable bounds subprocess lifecycle without exposing child diagnostics.
func TestRendererCancellationAndDeadlineRemainObservable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixtures")
	}
	projection, err := frameworkUpgradeConfig(frameworkUpgradeFixture(t, false))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "slow-renderer")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexec sleep 5\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := frameworkSnapshotFromRenderer(ctx, projection, binary); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error=%v", err)
	}
	ctx, cancel = context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := frameworkSnapshotFromRenderer(ctx, projection, binary); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline error=%v", err)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("renderer survived its deadline")
	}
}

// TestInactiveResourceOutputsRemainOwnerControlled avoids claiming a generated-looking file when its generator is not selected.
func TestInactiveResourceOutputsRemainOwnerControlled(t *testing.T) {
	root := t.TempDir()
	config := &project.Config{GoModuleName: "example.com/cli", Render: project.RenderConfig{Components: project.Components{CLI: true}}}
	source, err := yaml.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	writeProjectRendererTestFile(t, filepath.Join(root, ".goforj.yml"), string(source))
	writeProjectRendererTestFile(t, filepath.Join(root, "internal/caches/accessors_gen.go"), "owner generated-looking file")
	snapshot, err := frameworkUpgradeSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range snapshot.Artifacts {
		if artifact.Path == "internal/caches/accessors_gen.go" {
			t.Fatal("inactive cache generator claimed owner source")
		}
	}
	plan, err := projectupgrade.Build(root, snapshot, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range plan.Files {
		if file.Path == "internal/caches" && file.Action != "preserve" {
			t.Fatal("owner cache directory was not preserved")
		}
	}
}

// TestSnapshotJSONRejectsAmbiguity bounds the renderer protocol independently of its typed payload validation.
func TestSnapshotJSONRejectsAmbiguity(t *testing.T) {
	if err := validateSnapshotJSON([]byte{'"', 0xff, '"'}); err == nil {
		t.Fatal("accepted invalid UTF-8 protocol text")
	}
	for _, source := range []string{`{"protocol":3,"protocol":2}`, `{"protocol":3,"Protocol":3}`, `{"ſcope":"framework"}`, `{"artifacts":[{"path":"a","path":"b"}]}`, `{} {}`, strings.Repeat("[", 40) + strings.Repeat("]", 40), `{"broken":`, ``} {
		if err := validateSnapshotJSON([]byte(source)); err == nil {
			t.Fatalf("accepted ambiguous or malformed JSON: %q", source)
		}
	}
	if err := validateSnapshotJSON([]byte(`{"artifacts":[{"source":"{\\\"path\\\":1,\\\"path\\\":2}"}]}`)); err != nil {
		t.Fatal("source string parsed as protocol metadata", err)
	}
}

// TestFrameworkPlanningAcceptsLocalModuleNames follows project creation's local Go module grammar rather than publication constraints.
func TestFrameworkPlanningAcceptsLocalModuleNames(t *testing.T) {
	root := t.TempDir()
	writeProjectRendererTestFile(t, filepath.Join(root, ".goforj.yml"), "module_name: forj\nrender:\n  components: [cli]\n")
	if _, err := frameworkUpgradeSnapshot(root); err != nil {
		t.Fatal("valid local module rejected", err)
	}
	for _, module := range []string{"go", "toolchain", "example.com/app/v1", " forj "} {
		config := &project.Config{GoModuleName: module, Render: project.RenderConfig{Components: project.Components{CLI: true}}}
		source, err := yaml.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		writeProjectRendererTestFile(t, filepath.Join(root, ".goforj.yml"), string(source))
		if _, err := frameworkUpgradeConfig(root); err == nil {
			t.Fatalf("accepted invalid module %q", module)
		}
	}
}

// TestFrameworkUpgradeSnapshotIsPureAndComponentGated proves snapshots render current authoritative templates without reading private env values or writing output.
func TestFrameworkUpgradeSnapshotIsPureAndComponentGated(t *testing.T) {
	for _, oauth := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "oauth"}[oauth], func(t *testing.T) {
			root := frameworkUpgradeFixture(t, oauth)
			before := openAPIUpgradeSnapshot(t, root)
			snapshot, err := frameworkUpgradeSnapshot(root)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, openAPIUpgradeSnapshot(t, root)) {
				t.Fatal("snapshot mutated app")
			}
			if len(snapshot.Artifacts) < 80 {
				t.Fatalf("framework inventory is unexpectedly narrow: %d", len(snapshot.Artifacts))
			}

			encoded, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "PRIVATE_ENV_CANARY") || strings.Contains(string(encoded), "OperatorPolicy") {
				t.Fatal("snapshot captured private state or owner source")
			}
			for _, artifact := range snapshot.Artifacts {
				if strings.HasSuffix(artifact.Path, ".go") && artifact.Ownership == "framework-template" && strings.Contains(artifact.Source, "<no value>") {
					t.Fatalf("unresolved template %s", artifact.Path)
				}
				if !oauth && strings.Contains(artifact.Path, "oauth") {
					t.Fatalf("unexpected OAuth artifact %s", artifact.Path)
				}
			}
			second, err := frameworkUpgradeSnapshot(root)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(snapshot, second) {
				t.Fatal("snapshot is nondeterministic")
			}
		})
	}
}

// TestFrameworkUpgradeSnapshotMatchesRenderer checks byte parity against the actual renderer write boundary.
func TestFrameworkUpgradeSnapshotMatchesRenderer(t *testing.T) {
	for _, starter := range []project.StarterKit{project.StarterKitNone, project.StarterKitVue, project.StarterKitReact, project.StarterKitTemplHTMX} {
		t.Run(string(starter), func(t *testing.T) {
			root := frameworkUpgradeFixture(t, true)
			config, err := project.LoadProjectConfigAt(root)
			if err != nil {
				t.Fatal(err)
			}
			config.Render.StarterKit = starter
			config.Render.Components.DatabaseMySQL = true
			config.Render.Components.DatabasePostgres = true
			config.Render.Components.WebUI = starter != project.StarterKitNone
			config.Render.Components.Metrics = starter != project.StarterKitTemplHTMX
			encoded, err := yaml.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			writeProjectRendererTestFile(t, filepath.Join(root, ".goforj.yml"), string(encoded))
			snapshot, err := frameworkUpgradeSnapshot(root)
			if err != nil {
				t.Fatal(err)
			}
			renderer := unitProjectRenderer(t)
			if err := renderer.beginRenderInvocation(t.TempDir()); err != nil {
				t.Fatal(err)
			}
			config, err = project.LoadProjectConfigAt(root)
			if err != nil {
				t.Fatal(err)
			}
			renderer.config = config
			renderer.resources.plan, err = project.DefaultResourcePlan(project.ProjectComponents(config))
			if err != nil {
				t.Fatal(err)
			}
			compared := 0
			for _, artifact := range snapshot.Artifacts {
				if artifact.Ownership != "framework-template" || !strings.HasPrefix(artifact.Path, "internal/") {
					continue
				}
				if artifact.Template == "" {
					t.Fatalf("missing template provenance: %s", artifact.Path)
				}
				if err := renderer.renderTemplateFile(artifact.Path, artifact.Template, config); err != nil {
					t.Fatalf("render %s: %v", artifact.Path, err)
				}
				actual, err := renderer.workspace.readFile(artifact.Path)
				if err != nil {
					t.Fatal(err)
				}
				if string(actual) != artifact.Source {
					t.Fatalf("snapshot differs from normal render: %s", artifact.Path)
				}
				compared++
			}
			if compared < 50 {
				t.Fatalf("insufficient framework parity coverage: %d", compared)
			}
			if starter == project.StarterKitTemplHTMX {
				derivedTempl := false
				for _, artifact := range snapshot.Artifacts {
					if artifact.Ownership == "app-derived" && strings.HasSuffix(artifact.Path, "_templ.go") {
						derivedTempl = true
					}
				}
				if !derivedTempl {
					t.Fatal("templ compiler sidecars were not classified as app-derived")
				}
			}

		})
	}
}

// TestFrameworkUpgradeCommandPreservesApp exercises machine output, discovery, and the CLI parser without injected renderer dependencies.
func TestFrameworkUpgradeCommandPreservesApp(t *testing.T) {
	root := frameworkUpgradeFixture(t, true)
	t.Chdir(root)
	before := openAPIUpgradeSnapshot(t, root)
	command := RenderCmd{Plan: true, JSON: true}
	var output bytes.Buffer
	if err := command.runFrameworkUpgradeInspection(&output); err != nil {
		t.Fatal(err)
	}
	var plan projectupgrade.Plan
	if err := json.Unmarshal(output.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.BaselineProvenance != "unknown" {
		t.Fatal("unversioned app assigned provenance")
	}
	if !reflect.DeepEqual(before, openAPIUpgradeSnapshot(t, root)) {
		t.Fatal("plan mutated app")
	}
	var cli struct {
		Render RenderCmd `cmd:""`
	}
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"render", "--plan", "--json", "--from-renderer", "old-forj"}); err != nil {
		t.Fatal(err)
	}
	if !cli.Render.Plan || cli.Render.FromRenderer != "old-forj" {
		t.Fatal("parser lost upgrade flags")
	}
	command.JSON = false
	output.Reset()
	if err := command.runFrameworkUpgradeInspection(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "review-unversioned") {
		t.Fatal("text plan lacks review actions")
	}
	command.Plan = false
	command.Snapshot = true
	output.Reset()
	if err := command.runFrameworkUpgradeInspection(&output); err != nil {
		t.Fatal(err)
	}
	var snapshot projectupgrade.Snapshot
	if err := json.Unmarshal(output.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
}

// TestFrameworkUpgradeFlagsFailBeforeRender prevents typos and unsupported combinations from falling into mutating rendering.
func TestFrameworkUpgradeFlagsFailBeforeRender(t *testing.T) {
	for _, command := range []RenderCmd{
		{JSON: true}, {FromRenderer: "old"}, {TargetRenderer: "next"},
		{Plan: true, Snapshot: true}, {Only: "auth", Plan: true}, {Only: "openapi", Plan: true},
		{Plan: true, Timings: true}, {Plan: true, Components: []string{"auth"}},
		{Snapshot: true, JSON: true}, {Snapshot: true, FromRenderer: "old"},
		{Snapshot: true, TargetRenderer: "next"},
	} {
		if err := command.Run(); err == nil {
			t.Fatalf("accepted invalid flags: %+v", command)
		}
	}
}

// TestFrameworkUpgradeSnapshotRejectsUnsupportedProjects avoids synthesizing misleading output when auth inputs are missing.
func TestFrameworkUpgradeSnapshotRejectsUnsupportedProjects(t *testing.T) {
	for _, source := range []string{"render:\n  components: [auth, web_api, database_sqlite]\n", "[invalid yaml"} {
		root := t.TempDir()
		writeProjectRendererTestFile(t, filepath.Join(root, ".goforj.yml"), source)
		if _, err := frameworkUpgradeSnapshot(root); err == nil {
			t.Fatal("accepted unsupported config")
		}
	}
	if _, err := frameworkUpgradeSnapshot(t.TempDir()); err == nil {
		t.Fatal("accepted absent config")
	}
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(t.TempDir(), "private"), filepath.Join(root, ".goforj.yml")); err != nil {
		t.Fatal(err)
	}
	if _, err := frameworkUpgradeSnapshot(root); err == nil {
		t.Fatal("followed config symlink")
	}
}

// snapshotRendererScript creates an executable protocol peer without depending on a globally installed forj.
func snapshotRendererScript(t *testing.T, output string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell protocol fixtures require POSIX")
	}
	name := filepath.Join(t.TempDir(), "comparison renderer")
	if err := os.WriteFile(name, []byte("#!/bin/sh\ntest \"$(cat .env)\" = APP_ENV=local || exit 42\ntest -z \"$FORJ_UPGRADE_PRIVATE_CANARY\" || exit 43\nprintf scratch > renderer-side-effect\ncat <<'FORJ_SNAPSHOT'\n"+output+"\nFORJ_SNAPSHOT\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return name
}

// TestFrameworkUpgradeOrchestratesRendererProtocol exercises two binary roles, unknown provenance, and protocol failure paths.
func TestFrameworkUpgradeOrchestratesRendererProtocol(t *testing.T) {
	root := frameworkUpgradeFixture(t, true)
	t.Chdir(root)
	t.Setenv("FORJ_UPGRADE_PRIVATE_CANARY", "must-not-reach-child")
	snapshot, err := frameworkUpgradeSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	binary := snapshotRendererScript(t, string(encoded))
	var output bytes.Buffer
	command := RenderCmd{Plan: true, JSON: true, FromRenderer: binary, TargetRenderer: binary}
	if err := command.runFrameworkUpgradeInspection(&output); err != nil {
		t.Fatal(err)
	}
	var plan projectupgrade.Plan
	if err := json.Unmarshal(output.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Baseline == nil || plan.BaselineProvenance != "supplied-unverified" {
		t.Fatal("missing or falsely authenticated baseline")
	}
	if _, err := os.Stat(filepath.Join(root, "renderer-side-effect")); !os.IsNotExist(err) {
		t.Fatal("child renderer ran in the application directory")
	}
	for _, invalid := range []string{"not json", string(encoded) + "\n{}", strings.Replace(string(encoded), `"protocol":3`, `"protocol":99`, 1), strings.Replace(string(encoded), `"protocol":3`, `"unknown":1,"protocol":3`, 1)} {
		if _, err := externalFrameworkSnapshot(root, snapshotRendererScript(t, invalid)); err == nil {
			t.Fatal("accepted incompatible protocol")
		}
	}
	if _, err := externalFrameworkSnapshot(root, filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("accepted missing renderer")
	}
	if _, err := externalFrameworkSnapshot(filepath.Join(t.TempDir(), "absent"), binary); err == nil {
		t.Fatal("accepted failed renderer")
	}
	snapshot.InputsDigest = projectupgrade.Digest([]byte("other inputs"))
	encoded, err = json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	foreign := snapshotRendererScript(t, string(encoded))
	command.FromRenderer = foreign
	command.TargetRenderer = ""
	if err := command.runFrameworkUpgradeInspection(&bytes.Buffer{}); err == nil {
		t.Fatal("accepted different baseline inputs")
	}
	command.FromRenderer = ""
	command.TargetRenderer = foreign
	if err := command.runFrameworkUpgradeInspection(&bytes.Buffer{}); err == nil {
		t.Fatal("accepted different target inputs")
	}
	command.FromRenderer = filepath.Join(t.TempDir(), "absent")
	command.TargetRenderer = ""
	if err := command.runFrameworkUpgradeInspection(&bytes.Buffer{}); err == nil {
		t.Fatal("accepted unavailable baseline renderer")
	}
	command.FromRenderer = ""
	command.TargetRenderer = filepath.Join(t.TempDir(), "absent")
	if err := command.runFrameworkUpgradeInspection(&bytes.Buffer{}); err == nil {
		t.Fatal("accepted unavailable target renderer")
	}
}

// TestFrameworkUpgradeBoundsProtocolOutput refuses oversized renderer output instead of silently truncating snapshots.
func TestFrameworkUpgradeBoundsProtocolOutput(t *testing.T) {
	var output boundedSnapshotOutput
	if _, err := output.Write(make([]byte, 32<<20)); err != nil {
		t.Fatal(err)
	}
	if _, err := output.Write([]byte("x")); err == nil {
		t.Fatal("accepted oversized output")
	}
	output = boundedSnapshotOutput{}
	if _, err := io.Copy(&output, strings.NewReader(strings.Repeat("x", (32<<20)+1))); err == nil {
		t.Fatal("streaming copy bypassed the output limit")
	}
}

// rejectedPlanWriter models a closed output destination so inspection errors remain observable without publishing partial application changes.
type rejectedPlanWriter struct{}

// Write fails independently of content to exercise the CLI output error contract.
func (rejectedPlanWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

// TestFrameworkUpgradePropagatesOutputFailure preserves actionable errors for both human and machine consumers.
func TestFrameworkUpgradePropagatesOutputFailure(t *testing.T) {
	t.Chdir(frameworkUpgradeFixture(t, true))
	for _, command := range []RenderCmd{{Plan: true}, {Plan: true, JSON: true}, {Snapshot: true}} {
		if err := command.runFrameworkUpgradeInspection(rejectedPlanWriter{}); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("output error = %v", err)
		}
	}
}

// TestFrameworkUpgradeWithoutAuthPlansCoreAndNamedApps proves framework planning does not depend on authentication or a database.
func TestFrameworkUpgradeWithoutAuthPlansCoreAndNamedApps(t *testing.T) {
	root := t.TempDir()
	config := &project.Config{ProjectName: "CLI and worker", GoModuleName: "example.com/framework", Render: project.RenderConfig{Components: project.Components{CLI: true}}, Apps: map[string]project.AppConfig{
		"worker": {Components: project.Components{CLI: true, Jobs: true}},
		"admin":  {Components: project.Components{CLI: true, WebAPI: true}},
	}}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	writeProjectRendererTestFile(t, filepath.Join(root, ".goforj.yml"), string(encoded))
	writeProjectRendererTestFile(t, filepath.Join(root, "internal/logger/app.go"), "package logger\nfunc OwnerLogging() {}\n")
	writeProjectRendererTestFile(t, filepath.Join(root, "app/worker/wire/wire_gen.go"), "package wire\nfunc OwnerWorkerAccessor() {}\n")
	before := openAPIUpgradeSnapshot(t, root)
	snapshot, err := frameworkUpgradeSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := map[string]projectupgrade.Artifact{}
	for _, artifact := range snapshot.Artifacts {
		artifacts[artifact.Path] = artifact
		if strings.HasPrefix(artifact.Path, "internal/auth/") {
			t.Fatalf("non-auth project acquired auth artifact: %s", artifact.Path)
		}
	}
	for _, name := range []string{"internal/logger/app.go", "internal/runtime/runtime.go", "internal/jobs/worker.go", "internal/http/server.go", "app/admin/wire/inject_http.go", "cmd/worker/main.go"} {
		if _, ok := artifacts[name]; !ok {
			t.Fatalf("framework artifact missing: %s", name)
		}
	}
	if artifacts["app/worker/lifecycle.go"].Ownership != "seed" {
		t.Fatal("named App lifecycle is not owner seeded")
	}
	if artifacts["app/worker/wire/wire_gen.go"].Ownership != "app-derived" {
		t.Fatal("named Wire output is not derived")
	}
	plan, err := projectupgrade.Build(root, snapshot, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Scope != "framework" {
		t.Fatal("plan retains component scope")
	}
	if !reflect.DeepEqual(before, openAPIUpgradeSnapshot(t, root)) {
		t.Fatal("framework planning modified the original project")
	}
}

// TestFrameworkUpgradeProjectionStripsOwnerSettings preserves only supported render intent when crossing renderer process boundaries.
func TestFrameworkUpgradeProjectionStripsOwnerSettings(t *testing.T) {
	root := frameworkUpgradeFixture(t, true)
	config, err := project.LoadProjectConfigAt(root)
	if err != nil {
		t.Fatal(err)
	}
	config.Extra = map[string]any{"owner_secret": "PRIVATE_CONFIG_CANARY"}
	config.Render.ModuleReplaces = map[string]string{"example.com/private": "/private/local-path"}
	config.Dev.Pre = []project.DevTask{{Name: "owner", Cmd: "do-not-execute-owner-command"}}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	writeProjectRendererTestFile(t, filepath.Join(root, ".goforj.yml"), string(encoded))
	projection, err := frameworkUpgradeConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = yaml.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, canary := range []string{"PRIVATE_CONFIG_CANARY", "private/local-path", "do-not-execute-owner-command"} {
		if strings.Contains(string(encoded), canary) {
			t.Fatal("owner-only setting entered renderer projection")
		}
	}
}

// TestFrameworkUpgradeMaximalInventoryNeedsNoTools covers the full component composition without running toolchains or application generators.
func TestFrameworkUpgradeMaximalInventoryNeedsNoTools(t *testing.T) {
	root := t.TempDir()
	config := &project.Config{ProjectName: "Maximum framework", GoModuleName: "example.com/maximal", Render: project.RenderConfig{Components: project.Components{
		CLI: true, Docker: true, DemoApp: true, Auth: true, OAuth: true, WebAPI: true, WebUI: true, Metrics: true, Observability: true, Grafana: true,
		DatabaseMySQL: true, Scheduler: true, Cache: true, Events: true, Storage: true, Jobs: true, Mail: true,
	}}}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	writeProjectRendererTestFile(t, filepath.Join(root, ".goforj.yml"), string(encoded))
	t.Setenv("PATH", t.TempDir())
	first, err := frameworkUpgradeSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := frameworkUpgradeSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("maximal framework snapshot is nondeterministic")
	}
	seen := map[string]bool{}
	for _, artifact := range first.Artifacts {
		seen[artifact.Path] = true
		if artifact.Ownership == "private" && (artifact.Digest != "" || artifact.Source != "") {
			t.Fatal("private generated values escaped inventory")
		}
	}
	for _, name := range []string{"internal/auth/service.go", "internal/events/manager_gen.go", "internal/storages/manager_gen.go", "internal/queues/manager_gen.go", "internal/mail/manager_gen.go", "internal/schedules/scheduler.go", "internal/logger/app.go", "docker-compose.yml", "containers/observability/grafana/dashboards/logs-overview.json"} {
		if !seen[name] {
			t.Fatalf("maximal framework artifact missing: %s", name)
		}
	}
}

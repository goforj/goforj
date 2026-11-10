package forj

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/goforj/goforj/internal/projectupgrade"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// authUpgradeFixture models an existing app with a private environment and owner additions.
func authUpgradeFixture(t *testing.T, oauth bool) string {
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

// TestAuthUpgradeSnapshotIsPureAndComponentGated proves snapshots render current authoritative templates without reading private env values or writing output.
func TestAuthUpgradeSnapshotIsPureAndComponentGated(t *testing.T) {
	for _, oauth := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "oauth"}[oauth], func(t *testing.T) {
			root := authUpgradeFixture(t, oauth)
			before := openAPIUpgradeSnapshot(t, root)
			snapshot, err := authUpgradeSnapshot(root)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, openAPIUpgradeSnapshot(t, root)) {
				t.Fatal("snapshot mutated app")
			}
			count := len(authBaseTemplates())
			if oauth {
				count += len(authOAuthTemplates())
			}
			if len(snapshot.Artifacts) != count {
				t.Fatalf("artifacts=%d, want %d", len(snapshot.Artifacts), count)
			}
			encoded, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "PRIVATE_ENV_CANARY") || strings.Contains(string(encoded), "OperatorPolicy") {
				t.Fatal("snapshot captured private state or owner source")
			}
			for _, artifact := range snapshot.Artifacts {
				if strings.Contains(artifact.Source, "{{") || strings.Contains(artifact.Source, "<no value>") {
					t.Fatalf("unresolved template %s", artifact.Path)
				}
				if !oauth && strings.Contains(artifact.Path, "oauth") {
					t.Fatalf("unexpected OAuth artifact %s", artifact.Path)
				}
			}
			second, err := authUpgradeSnapshot(root)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(snapshot, second) {
				t.Fatal("snapshot is nondeterministic")
			}
		})
	}
}

// TestAuthUpgradeSnapshotMatchesRenderer checks byte parity against the actual renderer write boundary.
func TestAuthUpgradeSnapshotMatchesRenderer(t *testing.T) {
	for _, starter := range []project.StarterKit{project.StarterKitNone, project.StarterKitVue, project.StarterKitReact, project.StarterKitTemplHTMX} {
		t.Run(string(starter), func(t *testing.T) {
			root := authUpgradeFixture(t, true)
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
			snapshot, err := authUpgradeSnapshot(root)
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
			for _, artifact := range snapshot.Artifacts {
				if err := renderer.renderTemplateFile(artifact.Path, artifact.Path+".tmpl", config); err != nil {
					t.Fatalf("render %s: %v", artifact.Path, err)
				}
				actual, err := renderer.workspace.readFile(artifact.Path)
				if err != nil {
					t.Fatal(err)
				}
				if string(actual) != artifact.Source {
					t.Fatalf("snapshot differs from normal render: %s", artifact.Path)
				}
			}
		})
	}
}

// TestAuthUpgradeCommandPreservesApp exercises machine output, discovery, and the CLI parser without injected renderer dependencies.
func TestAuthUpgradeCommandPreservesApp(t *testing.T) {
	root := authUpgradeFixture(t, true)
	t.Chdir(root)
	before := openAPIUpgradeSnapshot(t, root)
	command := RenderCmd{Only: "auth", Plan: true, JSON: true}
	var output bytes.Buffer
	if err := command.runAuthUpgradeInspection(&output); err != nil {
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
	if _, err := parser.Parse([]string{"render", "--only", "auth", "--plan", "--json", "--from-renderer", "old-forj"}); err != nil {
		t.Fatal(err)
	}
	if !cli.Render.Plan || cli.Render.FromRenderer != "old-forj" {
		t.Fatal("parser lost upgrade flags")
	}
	command.JSON = false
	output.Reset()
	if err := command.runAuthUpgradeInspection(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "review-unversioned") {
		t.Fatal("text plan lacks review actions")
	}
	command.Plan = false
	command.Snapshot = true
	output.Reset()
	if err := command.runAuthUpgradeInspection(&output); err != nil {
		t.Fatal(err)
	}
	var snapshot projectupgrade.Snapshot
	if err := json.Unmarshal(output.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
}

// TestAuthUpgradeFlagsFailBeforeRender prevents typos and unsupported combinations from falling into mutating rendering.
func TestAuthUpgradeFlagsFailBeforeRender(t *testing.T) {
	for _, command := range []RenderCmd{
		{JSON: true}, {FromRenderer: "old"}, {TargetRenderer: "next"},
		{Only: "auth", Plan: true, Snapshot: true}, {Plan: true}, {Only: "openapi", Plan: true},
		{Only: "auth", Plan: true, Timings: true}, {Only: "auth", Plan: true, Components: []string{"auth"}},
		{Only: "auth", Snapshot: true, JSON: true}, {Only: "auth", Snapshot: true, FromRenderer: "old"},
		{Only: "auth", Snapshot: true, TargetRenderer: "next"},
	} {
		if err := command.Run(); err == nil {
			t.Fatalf("accepted invalid flags: %+v", command)
		}
	}
}

// TestAuthUpgradeSnapshotRejectsUnsupportedProjects avoids synthesizing misleading output when auth inputs are missing.
func TestAuthUpgradeSnapshotRejectsUnsupportedProjects(t *testing.T) {
	for _, source := range []string{"module_name: example.com/app\nrender:\n  components: [cli]\n", "render:\n  components: [auth, web_api, database_sqlite]\n", "[invalid yaml"} {
		root := t.TempDir()
		writeProjectRendererTestFile(t, filepath.Join(root, ".goforj.yml"), source)
		if _, err := authUpgradeSnapshot(root); err == nil {
			t.Fatal("accepted unsupported config")
		}
	}
	if _, err := authUpgradeSnapshot(t.TempDir()); err == nil {
		t.Fatal("accepted absent config")
	}
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(t.TempDir(), "private"), filepath.Join(root, ".goforj.yml")); err != nil {
		t.Fatal(err)
	}
	if _, err := authUpgradeSnapshot(root); err == nil {
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
	if err := os.WriteFile(name, []byte("#!/bin/sh\ntest ! -e .env || exit 42\ntest -z \"$FORJ_UPGRADE_PRIVATE_CANARY\" || exit 43\nprintf scratch > renderer-side-effect\ncat <<'FORJ_SNAPSHOT'\n"+output+"\nFORJ_SNAPSHOT\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return name
}

// TestAuthUpgradeOrchestratesRendererProtocol exercises two binary roles, unknown provenance, and protocol failure paths.
func TestAuthUpgradeOrchestratesRendererProtocol(t *testing.T) {
	root := authUpgradeFixture(t, true)
	t.Chdir(root)
	t.Setenv("FORJ_UPGRADE_PRIVATE_CANARY", "must-not-reach-child")
	snapshot, err := authUpgradeSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	binary := snapshotRendererScript(t, string(encoded))
	var output bytes.Buffer
	command := RenderCmd{Only: "auth", Plan: true, JSON: true, FromRenderer: binary, TargetRenderer: binary}
	if err := command.runAuthUpgradeInspection(&output); err != nil {
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
	for _, invalid := range []string{"not json", string(encoded) + "\n{}", strings.Replace(string(encoded), `"protocol":1`, `"protocol":99`, 1), strings.Replace(string(encoded), `"protocol":1`, `"unknown":1,"protocol":1`, 1)} {
		if _, err := externalAuthSnapshot(root, snapshotRendererScript(t, invalid)); err == nil {
			t.Fatal("accepted incompatible protocol")
		}
	}
	if _, err := externalAuthSnapshot(root, filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("accepted missing renderer")
	}
	if _, err := externalAuthSnapshot(filepath.Join(t.TempDir(), "absent"), binary); err == nil {
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
	if err := command.runAuthUpgradeInspection(&bytes.Buffer{}); err == nil {
		t.Fatal("accepted different baseline inputs")
	}
	command.FromRenderer = ""
	command.TargetRenderer = foreign
	if err := command.runAuthUpgradeInspection(&bytes.Buffer{}); err == nil {
		t.Fatal("accepted different target inputs")
	}
	command.FromRenderer = filepath.Join(t.TempDir(), "absent")
	command.TargetRenderer = ""
	if err := command.runAuthUpgradeInspection(&bytes.Buffer{}); err == nil {
		t.Fatal("accepted unavailable baseline renderer")
	}
	command.FromRenderer = ""
	command.TargetRenderer = filepath.Join(t.TempDir(), "absent")
	if err := command.runAuthUpgradeInspection(&bytes.Buffer{}); err == nil {
		t.Fatal("accepted unavailable target renderer")
	}
}

// TestAuthUpgradeBoundsProtocolOutput refuses oversized renderer output instead of silently truncating snapshots.
func TestAuthUpgradeBoundsProtocolOutput(t *testing.T) {
	var output boundedSnapshotOutput
	if _, err := output.Write(make([]byte, 16<<20)); err != nil {
		t.Fatal(err)
	}
	if _, err := output.Write([]byte("x")); err == nil {
		t.Fatal("accepted oversized output")
	}
	output = boundedSnapshotOutput{}
	if _, err := io.Copy(&output, strings.NewReader(strings.Repeat("x", (16<<20)+1))); err == nil {
		t.Fatal("streaming copy bypassed the output limit")
	}
}

// rejectedPlanWriter models a closed output destination so inspection errors remain observable without publishing partial application changes.
type rejectedPlanWriter struct{}

// Write fails independently of content to exercise the CLI output error contract.
func (rejectedPlanWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

// TestAuthUpgradePropagatesOutputFailure preserves actionable errors for both human and machine consumers.
func TestAuthUpgradePropagatesOutputFailure(t *testing.T) {
	t.Chdir(authUpgradeFixture(t, true))
	for _, command := range []RenderCmd{{Only: "auth", Plan: true}, {Only: "auth", Plan: true, JSON: true}, {Only: "auth", Snapshot: true}} {
		if err := command.runAuthUpgradeInspection(rejectedPlanWriter{}); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("output error = %v", err)
		}
	}
}

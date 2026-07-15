package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestEnsureEnvironmentDefaultsIgnoresCommentedAndSimilarKeys verifies only active exact dotenv assignments satisfy required defaults.
func TestEnsureEnvironmentDefaultsIgnoresCommentedAndSimilarKeys(t *testing.T) {
	root := t.TempDir()
	keys := []string{
		"APP_KEY", "APP_DIAG_TOKEN", "LIGHTHOUSE_URL", "LIGHTHOUSE_SECRET", "LIGHTHOUSE_ENABLED",
		"SWAGGER_ENABLED", "FORJ_MAKE_OPEN", "FORJ_EDITOR", "API_JWT_SECRET_KEY",
	}
	sourceLines := make([]string, 0, len(keys)*2+1)
	for _, key := range keys {
		sourceLines = append(sourceLines, "# "+key+"=commented", "MY_"+key+"=similar")
	}
	source := strings.Join(append(sourceLines, ""), "\n")
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(source), 0o600); err != nil {
		t.Fatalf("write owner environment: %v", err)
	}

	renderer := environmentDefaultsRenderer(t, root, project.Components{})
	if err := renderer.ensureEnvironmentDefaults(".env"); err != nil {
		t.Fatalf("ensure environment defaults: %v", err)
	}
	updated := readEnvironmentDefaultsFile(t, root)
	lines := strings.Split(updated, "\n")
	for _, key := range keys {
		if assignment := finalEnvironmentAssignment(lines, key); !assignment.exists() {
			t.Fatalf("active %s assignment missing:\n%s", key, updated)
		}
		for _, preserved := range []string{"# " + key + "=commented", "MY_" + key + "=similar"} {
			if !strings.Contains(updated, preserved+"\n") {
				t.Fatalf("owner line %q was not preserved:\n%s", preserved, updated)
			}
		}
	}
	for _, key := range []string{"APP_KEY", "APP_DIAG_TOKEN", "LIGHTHOUSE_SECRET", "API_JWT_SECRET_KEY"} {
		assignment := finalEnvironmentAssignment(lines, key)
		if strings.TrimSpace(assignment.value) == "" || assignment.value == "xxx" {
			t.Fatalf("generated %s value = %q, want concrete secret", key, assignment.value)
		}
	}
}

// TestEnsureEnvironmentDefaultsPreservesOwnerSyntaxAndDeduplicatesLighthouseSecret verifies parsed assignments are recognized without rewriting owner formatting.
func TestEnsureEnvironmentDefaultsPreservesOwnerSyntaxAndDeduplicatesLighthouseSecret(t *testing.T) {
	root := t.TempDir()
	source := strings.Join([]string{
		" export APP_KEY = 'owner-app-key'",
		"export APP_DIAG_TOKEN = \"owner-diagnostic-token\"",
		"export LIGHTHOUSE_URL = 'wss://owner.example/ws'",
		" export LIGHTHOUSE_SECRET = 'owner-secret'",
		"# LIGHTHOUSE_SECRET=commented-placeholder",
		"LIGHTHOUSE_SECRET=generated-duplicate",
		"export LIGHTHOUSE_ENABLED = 'false'",
		"export SWAGGER_ENABLED = 'false'",
		"export FORJ_MAKE_OPEN = 'never'",
		"export FORJ_EDITOR = 'vim'",
		"export API_JWT_SECRET_KEY = 'owner-jwt-secret'",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(source), 0o600); err != nil {
		t.Fatalf("write owner environment: %v", err)
	}

	renderer := environmentDefaultsRenderer(t, root, project.Components{})
	if err := renderer.ensureEnvironmentDefaults(".env"); err != nil {
		t.Fatalf("ensure environment defaults: %v", err)
	}
	want := strings.Replace(source, "LIGHTHOUSE_SECRET=generated-duplicate\n", "", 1)
	if got := readEnvironmentDefaultsFile(t, root); got != want {
		t.Fatalf("owner environment changed unexpectedly:\nwant:\n%s\ngot:\n%s", want, got)
	}
}

// TestEnsureEnvironmentDefaultsReplacesQuotedJWTPlaceholder verifies dotenv quoting cannot preserve a generated placeholder as a runtime secret.
func TestEnsureEnvironmentDefaultsReplacesQuotedJWTPlaceholder(t *testing.T) {
	root := t.TempDir()
	source := completeOwnerEnvironment("export API_JWT_SECRET_KEY = 'xxx'")
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(source), 0o600); err != nil {
		t.Fatalf("write owner environment: %v", err)
	}

	renderer := environmentDefaultsRenderer(t, root, project.Components{})
	if err := renderer.ensureEnvironmentDefaults(".env"); err != nil {
		t.Fatalf("ensure environment defaults: %v", err)
	}
	updated := readEnvironmentDefaultsFile(t, root)
	assignment := finalEnvironmentAssignment(strings.Split(updated, "\n"), "API_JWT_SECRET_KEY")
	if !assignment.exists() || assignment.value == "" || assignment.value == "xxx" {
		t.Fatalf("JWT assignment = %#v, want generated secret:\n%s", assignment, updated)
	}
	if strings.Contains(updated, "export API_JWT_SECRET_KEY = 'xxx'") {
		t.Fatalf("quoted JWT placeholder was retained:\n%s", updated)
	}
}

// TestEnsureEnvironmentDefaultsPreservesInterpolatedJWTSecret verifies lookup evaluates owner assignments in complete dotenv context.
func TestEnsureEnvironmentDefaultsPreservesInterpolatedJWTSecret(t *testing.T) {
	root := t.TempDir()
	source := completeOwnerEnvironment("JWT_SECRET_SOURCE=owner-jwt-secret", "API_JWT_SECRET_KEY=${JWT_SECRET_SOURCE}")
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(source), 0o600); err != nil {
		t.Fatalf("write owner environment: %v", err)
	}

	renderer := environmentDefaultsRenderer(t, root, project.Components{})
	if err := renderer.ensureEnvironmentDefaults(".env"); err != nil {
		t.Fatalf("ensure environment defaults: %v", err)
	}
	if got := readEnvironmentDefaultsFile(t, root); got != source {
		t.Fatalf("interpolated owner JWT secret changed unexpectedly:\nwant:\n%s\ngot:\n%s", source, got)
	}
}

// completeOwnerEnvironment supplies unrelated concrete defaults so JWT tests isolate only the assignment under test.
func completeOwnerEnvironment(jwtLines ...string) string {
	lines := []string{
		"APP_KEY=owner-app-key",
		"APP_DIAG_TOKEN=owner-diagnostic-token",
		"LIGHTHOUSE_URL=wss://owner.example/ws",
		"LIGHTHOUSE_SECRET=owner-secret",
		"LIGHTHOUSE_ENABLED=false",
		"SWAGGER_ENABLED=false",
		"FORJ_MAKE_OPEN=never",
		"FORJ_EDITOR=vim",
	}
	lines = append(lines, jwtLines...)
	return strings.Join(append(lines, ""), "\n")
}

// TestMigrateGeneratedEnvDefaultPreservesOwnerExpressions verifies only the renderer's literal historical assignment is eligible for migration.
func TestMigrateGeneratedEnvDefaultPreservesOwnerExpressions(t *testing.T) {
	tests := []struct {
		name        string
		lines       []string
		want        []string
		wantChanged bool
	}{
		{
			name:        "generated default migrates without mutating surrounding lines",
			lines:       []string{"APP_NAME=test", "GRAFANA_PORT=3001", "GRAFANA_ADMIN_USER=admin"},
			want:        []string{"APP_NAME=test", "GRAFANA_PORT=13001", "GRAFANA_ADMIN_USER=admin"},
			wantChanged: true,
		},
		{name: "literal spacing migrates", lines: []string{"GRAFANA_PORT = 3001"}, want: []string{"GRAFANA_PORT=13001"}, wantChanged: true},
		{name: "custom value stays owner-controlled", lines: []string{"GRAFANA_PORT=3100"}},
		{name: "commented value is ignored", lines: []string{"# GRAFANA_PORT=3001"}},
		{name: "interpolation stays owner-controlled", lines: []string{"DEFAULT_PORT=3001", "GRAFANA_PORT=${DEFAULT_PORT}"}},
		{name: "quoted value stays owner-controlled", lines: []string{"GRAFANA_PORT='3001'"}},
		{name: "export stays owner-controlled", lines: []string{"export GRAFANA_PORT=3001"}},
		{name: "final custom assignment controls migration", lines: []string{"GRAFANA_PORT=3001", "GRAFANA_PORT=3100"}},
		{
			name:        "final generated assignment migrates",
			lines:       []string{"GRAFANA_PORT=3100", "GRAFANA_PORT=3001"},
			want:        []string{"GRAFANA_PORT=3100", "GRAFANA_PORT=13001"},
			wantChanged: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			original := append([]string(nil), test.lines...)
			got, changed := migrateGeneratedEnvDefault(test.lines, "GRAFANA_PORT", "3001", "13001")
			if changed != test.wantChanged {
				t.Fatalf("migrateGeneratedEnvDefault() changed = %t, want %t: %#v", changed, test.wantChanged, got)
			}
			if strings.Join(test.lines, "\n") != strings.Join(original, "\n") {
				t.Fatalf("input slice mutated:\nwant: %#v\ngot:  %#v", original, test.lines)
			}
			want := test.want
			if want == nil {
				want = original
			}
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Fatalf("migrateGeneratedEnvDefault() result:\nwant: %#v\ngot:  %#v", want, got)
			}
		})
	}
}

// environmentDefaultsRenderer creates the smallest renderer that satisfies the environment-default workspace and config invariants.
func environmentDefaultsRenderer(t *testing.T, root string, components project.Components) *ProjectRenderer {
	t.Helper()
	workspace, err := resolveProjectRenderWorkspace(root)
	if err != nil {
		t.Fatalf("resolve renderer workspace: %v", err)
	}
	return &ProjectRenderer{
		workspace: workspace,
		config:    &project.Config{Render: project.RenderConfig{Components: components}},
	}
}

// readEnvironmentDefaultsFile reads the focused owner fixture after an atomic renderer update.
func readEnvironmentDefaultsFile(t *testing.T, root string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatalf("read owner environment: %v", err)
	}
	return string(content)
}

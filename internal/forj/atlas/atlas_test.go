package atlas

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/atlas/skills"
)

func TestProjectUsesGoForjConfig(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".goforj.yml"), `
project_name: demo
module_name: example.com/demo
render:
  goforj_version: 0.18.0
  starter_kit: vue
  queue_driver: redis
  components:
    cli: true
    web_api: true
    web_ui: true
    jobs: true
    database_sqlite: true
`)
	writeFile(t, filepath.Join(root, "cmd", "app", "main.go"), "package main\n")
	writeFile(t, filepath.Join(root, "app", "routes.go"), "package app\n")

	project := Project(root)
	if project.Name != "demo" {
		t.Fatalf("project name = %q", project.Name)
	}
	if project.GoForjVersion != "0.18.0" {
		t.Fatalf("goforj version = %q", project.GoForjVersion)
	}
	if project.FrontendKit != "vue" {
		t.Fatalf("frontend kit = %q", project.FrontendKit)
	}
	if project.DatabaseDriver != "sqlite" {
		t.Fatalf("database driver = %q", project.DatabaseDriver)
	}
	if !containsString(project.Components, "web-api") || !containsString(project.Components, "jobs") {
		t.Fatalf("components = %#v", project.Components)
	}
}

func TestInstallCmdWritesDefaultCodexFiles(t *testing.T) {
	root := t.TempDir()
	withWorkingDir(t, root)
	writeFile(t, ".goforj.yml", `
project_name: demo
module_name: example.com/demo
render:
  goforj_version: 0.18.0
  components:
    cli: true
`)

	cmd := &InstallCmd{Agent: []string{"codex"}, NoInteraction: true}
	if err := cmd.Run(); err != nil {
		t.Fatalf("run install: %v", err)
	}

	assertFileContains(t, "AGENTS.md", "GoForj Atlas")
	assertFileContains(t, filepath.Join(".codex", "config.toml"), "atlas:mcp")
	assertFileContains(t, filepath.Join(".agents", "skills", "goforj-make-commands", "SKILL.md"), "forj <app> make:*")
	assertFileContains(t, filepath.Join(".goforj", "atlas.json"), `"codex"`)
}

func TestInstallCmdWritesGeminiFiles(t *testing.T) {
	root := t.TempDir()
	withWorkingDir(t, root)
	writeFile(t, ".goforj.yml", `
project_name: demo
module_name: example.com/demo
render:
  goforj_version: 0.18.0
  components:
    cli: true
`)

	cmd := &InstallCmd{Agent: []string{"gemini"}, NoInteraction: true}
	if err := cmd.Run(); err != nil {
		t.Fatalf("run install: %v", err)
	}

	assertFileContains(t, "GEMINI.md", "GoForj Atlas")
	assertFileContains(t, filepath.Join(".gemini", "settings.json"), "atlas:mcp")
	assertFileContains(t, filepath.Join(".gemini", "skills", "goforj-make-commands", "GEMINI.md"), "forj <app> make:*")
	assertFileContains(t, filepath.Join(".goforj", "atlas.json"), `"gemini"`)
}

func TestListSkillsIncludesMakeCommands(t *testing.T) {
	if _, ok := skills.ByName("goforj-make-commands"); !ok {
		t.Fatal("expected built-in make command skill")
	}
}

func TestMakeSkillCmdCreatesProjectSkill(t *testing.T) {
	root := t.TempDir()
	withWorkingDir(t, root)

	cmd := &MakeSkillCmd{Name: "checkout-rules"}
	if err := cmd.Run(); err != nil {
		t.Fatalf("run make skill: %v", err)
	}

	assertFileContains(t, filepath.Join(".ai", "skills", "checkout-rules", "SKILL.md"), "Checkout Rules")
}

func TestListSkillsCmdIncludesProjectSkills(t *testing.T) {
	root := t.TempDir()
	withWorkingDir(t, root)
	writeFile(t, filepath.Join(".ai", "skills", "checkout-rules", "SKILL.md"), "# Checkout Rules\n")

	previous := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = write
	t.Cleanup(func() {
		os.Stdout = previous
	})

	cmd := &ListSkillsCmd{}
	if err := cmd.Run(); err != nil {
		t.Fatalf("run list skills: %v", err)
	}
	if err := write.Close(); err != nil {
		t.Fatalf("close stdout pipe: %v", err)
	}
	content, err := io.ReadAll(read)
	if err != nil {
		t.Fatalf("read stdout pipe: %v", err)
	}

	got := string(content)
	if !strings.Contains(got, "Built-in skills:") || !strings.Contains(got, "Project skills:") || !strings.Contains(got, "checkout-rules") {
		t.Fatalf("unexpected list skills output:\n%s", got)
	}
}

func withWorkingDir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
	if err := os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFileContains(t *testing.T, path string, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(content), want) {
		t.Fatalf("%s missing %q:\n%s", path, want, string(content))
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

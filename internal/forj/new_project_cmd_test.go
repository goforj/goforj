package forj

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
	"gopkg.in/yaml.v3"
)

// initialModel keeps default test setup concise without exposing a production-only forwarding helper.
func initialModel() model {
	return initialModelWithOptions(newProjectModelOptions{})
}

// setComponentSelectedByKey centralizes set component selected by key behavior so callers follow the same contract.
func setComponentSelectedByKey(t *testing.T, m *model, key project.ComponentKey, selected bool) {
	t.Helper()
	for idx, item := range m.componentList.Items() {
		component := item.(ListItem)
		if component.Key != key {
			continue
		}
		component.Selected = selected
		m.componentList.SetItem(idx, component)
		return
	}
	t.Fatalf("component %q not found", key)
}

// selectComponentRowByKey centralizes select component row by key behavior so callers follow the same contract.
func selectComponentRowByKey(t *testing.T, m *model, key project.ComponentKey) {
	t.Helper()
	for idx, item := range m.componentList.Items() {
		component := item.(ListItem)
		if component.Key != key {
			continue
		}
		m.componentList.Select(idx)
		return
	}
	t.Fatalf("component %q not found", key)
}

// componentSelectedByKey centralizes component selected by key behavior so callers follow the same contract.
func componentSelectedByKey(t *testing.T, m model, key project.ComponentKey) bool {
	t.Helper()
	for _, item := range m.componentList.Items() {
		component := item.(ListItem)
		if component.Key == key {
			return component.Selected
		}
	}
	t.Fatalf("component %q not found", key)
	return false
}

// selectStarterKitRow centralizes select starter kit row behavior so callers follow the same contract.
func selectStarterKitRow(t *testing.T, m *model, key project.StarterKit) {
	t.Helper()
	for idx, item := range m.starterKitList.Items() {
		starterKit := item.(StarterKitItem)
		if starterKit.Key != key {
			continue
		}
		m.starterKitList.Select(idx)
		return
	}
	t.Fatalf("starter kit %q not found", key)
}

// selectAtlasModeRow centralizes select atlas mode row behavior so callers follow the same contract.
func selectAtlasModeRow(t *testing.T, m *model, mode atlasMode) {
	t.Helper()
	for idx, item := range m.atlasModeList.Items() {
		atlasModeItem := item.(AtlasModeItem)
		if atlasModeItem.Mode != mode {
			continue
		}
		m.atlasModeList.Select(idx)
		return
	}
	t.Fatalf("atlas mode %q not found", mode)
}

func TestModelHandlesCtrlC(t *testing.T) {
	m := initialModel()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("expected quit command, got nil")
	}

	cancelledModel, ok := updated.(model)
	if !ok {
		t.Fatalf("expected model type %T, got %T", m, updated)
	}

	if !cancelledModel.cancelled {
		t.Fatalf("expected cancelled flag to be set")
	}

	msg := cmd()
	if msg == nil {
		t.Fatalf("expected QuitMsg from quit command, got nil")
	}
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg from quit command, got %T", msg)
	}
}

func TestModelBackNavigation(t *testing.T) {
	m := initialModel()
	m.projectInput.SetValue("MyApp")

	projectToModule, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	moduleStage, ok := projectToModule.(model)
	if !ok {
		t.Fatalf("expected model type after project stage advance")
	}
	if moduleStage.stage != StageModuleName {
		t.Fatalf("expected module stage, got %v", moduleStage.stage)
	}

	moduleStage.moduleInput.SetValue("github.com/example/myapp")

	backToProject, _ := moduleStage.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	projectStage := backToProject.(model)
	if projectStage.stage != StageProjectName {
		t.Fatalf("expected project stage after back navigation, got %v", projectStage.stage)
	}
	if projectStage.projectInput.Value() != "MyApp" {
		t.Fatalf("project name should be preserved on back navigation")
	}
}

// TestValidateNewProjectModulePathMatchesLocalGoInitRules rejects malformed input without requiring publication.
func TestValidateNewProjectModulePathMatchesLocalGoInitRules(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "unpublished remote-shaped path", path: "project.invalid/owner/not-published"},
		{name: "local-only module", path: "forj"},
		{name: "empty", path: "  ", wantErr: "required"},
		{name: "URL", path: "https://github.com/example/new-project", wantErr: `Use "github.com/example/new-project" instead`},
		{name: "invalid import path", path: "github.com/example/project name", wantErr: "invalid"},
		{name: "invalid major suffix", path: "github.com/example/project/v1", wantErr: "major-version suffix"},
		{name: "reserved", path: "go", wantErr: "reserved"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateNewProjectModulePath(test.path)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("validateNewProjectModulePath(%q) = %v", test.path, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateNewProjectModulePath(%q) = %v, want %q", test.path, err, test.wantErr)
			}
		})
	}
}

// TestModuleStageRejectsURLBeforeProjectConfiguration keeps malformed paths inside the field that owns them.
func TestModuleStageRejectsURLBeforeProjectConfiguration(t *testing.T) {
	m := initialModel()
	m.stage = StageModuleName
	m.moduleInput.SetValue("https://github.com/example/new-project")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.stage != StageModuleName {
		t.Fatalf("URL module advanced to stage %v", m.stage)
	}
	if !strings.Contains(m.errorMsg, "without a URL scheme") {
		t.Fatalf("URL module error = %q", m.errorMsg)
	}
	if m.config.GoModuleName != "" {
		t.Fatalf("invalid module was persisted as %q", m.config.GoModuleName)
	}
}

// TestCreateProjectRejectsInvalidModuleBeforeFilesystemChanges preserves an untouched target on defensive validation failure.
func TestCreateProjectRejectsInvalidModuleBeforeFilesystemChanges(t *testing.T) {
	target := filepath.Join(t.TempDir(), "not-created")
	m := initialModel()
	m.stage = StageDone
	m.targetPath = target
	m.config.GoModuleName = "https://github.com/example/new-project"
	cmd := NewNewProjectCmd(logger.NewSilentLogger(), NewProjectRenderer(logger.NewSilentLogger()))

	err := cmd.createProject(m)
	if err == nil || !strings.Contains(err.Error(), "without a URL scheme") {
		t.Fatalf("createProject() error = %v, want module guidance", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("invalid module created target: %v", statErr)
	}
}

// TestConfirmationUsesSelectedDirectoryAndDefersSuccess keeps the final wizard screen truthful before rendering starts.
func TestConfirmationUsesSelectedDirectoryAndDefersSuccess(t *testing.T) {
	m := initialModel()
	m.projectInput.SetValue("New Project")
	m.moduleInput.SetValue("example.com/new-project")
	m.pathInput.SetValue(filepath.Join(t.TempDir(), "chosen-directory"))
	m.stage = StageConfirm
	confirmation := ansi.Strip(m.View())
	if !strings.Contains(confirmation, "Directory » chosen-directory") {
		t.Fatalf("confirmation omitted selected directory:\n%s", confirmation)
	}

	m.stage = StageDone
	done := ansi.Strip(m.View())
	if !strings.Contains(done, "Configuration complete") || strings.Contains(done, "Project initialized") {
		t.Fatalf("completion view claimed publication before rendering:\n%s", done)
	}
}

func TestConfirmationFlow(t *testing.T) {
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	temp := t.TempDir()
	_ = os.Chdir(temp)

	m := initialModel()
	m.projectInput.SetValue("MyApp")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	m.moduleInput.SetValue("github.com/example/myapp")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageSelectComponents {
		t.Fatalf("expected to be on component selection stage")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageHelpFormat {
		t.Fatalf("expected to be on help format stage")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageStarterKit {
		t.Fatalf("expected to be on starter kit stage after help format")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageExtras {
		t.Fatalf("expected to be on extras stage after starter kit")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageAtlasSupport {
		t.Fatalf("expected to be on atlas support stage after extras, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageProjectPath {
		t.Fatalf("expected to be on project path stage after atlas support, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageConfirm {
		t.Fatalf("expected confirmation stage after project path")
	}

	if !m.config.Render.Components.CLI {
		t.Fatalf("expected CLI component to remain selected in config")
	}

	confirmedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	finalModel := confirmedModel.(model)
	if finalModel.stage != StageDone {
		t.Fatalf("expected final stage after confirmation")
	}
	if cmd == nil {
		t.Fatalf("expected quit command on confirmation")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg on confirmation")
	}
}

// TestValidatePathInputRejectsNonEmptyDirectoryByDefault keeps project creation conservative unless explicitly overridden.
func TestValidatePathInputRejectsNonEmptyDirectoryByDefault(t *testing.T) {
	temp := t.TempDir()
	if err := os.WriteFile(filepath.Join(temp, "README.md"), []byte("# existing\n"), 0644); err != nil {
		t.Fatalf("failed to seed target directory: %v", err)
	}

	m := initialModel()
	m.pathInput.SetValue(temp)

	err := m.validatePathInput()
	if err == nil {
		t.Fatalf("expected non-empty directory validation error")
	}
	if !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("expected non-empty error, got %v", err)
	}
}

func TestValidatePathInputAllowsNonEmptyDirectoryWithFlag(t *testing.T) {
	temp := t.TempDir()
	if err := os.WriteFile(filepath.Join(temp, "README.md"), []byte("# existing\n"), 0644); err != nil {
		t.Fatalf("failed to seed target directory: %v", err)
	}

	m := initialModelWithOptions(newProjectModelOptions{allowNonEmpty: true})
	m.pathInput.SetValue(temp)

	if err := m.validatePathInput(); err != nil {
		t.Fatalf("expected non-empty directory to validate with allow-non-empty, got %v", err)
	}
}

// TestProjectPathShowsResourceReconciliationErrors keeps owner-contract failures visible before confirmation.
func TestProjectPathShowsResourceReconciliationErrors(t *testing.T) {
	target := t.TempDir()
	environment := "QUEUE_DRIVER=redis\nQUEUE_SUPPORTED_DRIVERS=workerpool\n"
	if err := os.WriteFile(filepath.Join(target, ".env"), []byte(environment), 0o600); err != nil {
		t.Fatalf("write target environment: %v", err)
	}

	m := initialModelWithOptions(newProjectModelOptions{allowNonEmpty: true})
	m.stage = StageProjectPath
	m.projectInput.SetValue("Existing")
	m.moduleInput.SetValue("example.com/existing")
	m.pathInput.SetValue(target)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.stage != StageProjectPath {
		t.Fatalf("invalid owner contract advanced to stage %v", m.stage)
	}
	if !strings.Contains(m.errorMsg, "excludes active") {
		t.Fatalf("resource reconciliation error = %q", m.errorMsg)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "excludes active") {
		t.Fatalf("path view hid resource reconciliation error:\n%s", view)
	}
	contents, err := os.ReadFile(filepath.Join(target, ".env"))
	if err != nil {
		t.Fatalf("read target environment: %v", err)
	}
	if string(contents) != environment {
		t.Fatalf("read-only preview changed target environment:\n%s", contents)
	}
}

// TestProjectPathDoesNotCarryTargetResourceIntentForward keeps one existing target's owner state out of the next proposal.
func TestProjectPathDoesNotCarryTargetResourceIntentForward(t *testing.T) {
	firstTarget := t.TempDir()
	if err := os.WriteFile(filepath.Join(firstTarget, ".env"), []byte("COMPOSE_PROFILES=redis\n"), 0o600); err != nil {
		t.Fatalf("write first target environment: %v", err)
	}
	secondTarget := t.TempDir()

	m := initialModelWithOptions(newProjectModelOptions{allowNonEmpty: true})
	m.stage = StageProjectPath
	m.projectInput.SetValue("Existing")
	m.moduleInput.SetValue("example.com/existing")
	m.pathInput.SetValue(firstTarget)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.stage != StageConfirm {
		t.Fatalf("first target did not advance to confirmation: stage=%v error=%q", m.stage, m.errorMsg)
	}
	preparation, err := m.selectedResourcePreparation()
	if err != nil {
		t.Fatalf("prepare first target resources: %v", err)
	}
	if mode, ok := preparation.serviceIntent.Mode(project.ServiceRedis); !ok || mode != project.LocalServiceModeLocal {
		t.Fatalf("first target Redis intent = %q selected=%t, want local", mode, ok)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = updated.(model)
	m.pathInput.SetValue(secondTarget)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.stage != StageConfirm {
		t.Fatalf("second target did not advance to confirmation: stage=%v error=%q", m.stage, m.errorMsg)
	}
	preparation, err = m.selectedResourcePreparation()
	if err != nil {
		t.Fatalf("prepare second target resources: %v", err)
	}
	if mode, ok := preparation.serviceIntent.Mode(project.ServiceRedis); ok {
		t.Fatalf("second target inherited Redis intent %q", mode)
	}
}

// TestPathStatusAllowsNonEmptyDirectoryWithFlag ensures the wizard preview matches validation behavior.
func TestPathStatusAllowsNonEmptyDirectoryWithFlag(t *testing.T) {
	temp := t.TempDir()
	if err := os.WriteFile(filepath.Join(temp, "README.md"), []byte("# existing\n"), 0644); err != nil {
		t.Fatalf("failed to seed target directory: %v", err)
	}

	m := initialModelWithOptions(newProjectModelOptions{allowNonEmpty: true})
	m.pathInput.SetValue(temp)

	status, ok := m.pathStatus()
	if !ok {
		t.Fatalf("expected non-empty path status to be valid with allow-non-empty, got %q", status)
	}
	if !strings.Contains(status, "not empty") || !strings.Contains(status, "preserved") {
		t.Fatalf("expected status to explain existing files are preserved, got %q", status)
	}
}

// TestEnsureNewProjectConfigCanBeWrittenRejectsExistingConfig prevents accidental reinitialization.
func TestEnsureNewProjectConfigCanBeWrittenRejectsExistingConfig(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, ".goforj.yml")
	if err := os.WriteFile(configPath, []byte("project_name: existing\n"), 0644); err != nil {
		t.Fatalf("failed to seed config file: %v", err)
	}

	err := ensureNewProjectConfigCanBeWritten(configPath)
	if err == nil {
		t.Fatalf("expected existing .goforj.yml to be rejected")
	}
	if !strings.Contains(err.Error(), "already contains .goforj.yml") {
		t.Fatalf("expected existing config error, got %v", err)
	}
}

// TestNewProjectCreationKeepsWorkInsideTarget verifies project creation does not use process cwd as hidden renderer or command state.
func TestNewProjectCreationKeepsWorkInsideTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "project with spaces")
	frontendDir := filepath.Join(target, projectlayout.FrontendDir(".", project.DefaultApp()))
	if err := os.MkdirAll(frontendDir, 0o755); err != nil {
		t.Fatalf("create target frontend: %v", err)
	}
	if err := os.WriteFile(filepath.Join(frontendDir, "package.json"), []byte(`{"scripts":{"dev":"vite"}}`), 0o644); err != nil {
		t.Fatalf("write target package.json: %v", err)
	}

	sentinel := t.TempDir()
	t.Chdir(sentinel)
	commandLog := installNewProjectCommandProbe(t)
	m := initialModel()
	m.targetPath = target
	m.atlasMode = atlasModeRecommended
	m.atlasRecommendations = []string{"codex"}
	m.config.ProjectName = "Rooted App"
	m.config.GoModuleName = "example.com/rooted"
	m.config.Render.Components = project.Components{CLI: true, WebAPI: true, WebUI: true}
	m.config.Render.StarterKit = project.StarterKitNone
	appLogger := logger.NewSilentLogger()
	cmd := NewNewProjectCmd(appLogger, NewProjectRenderer(appLogger))
	if err := m.finalizeConfig(); err != nil {
		t.Fatalf("finalize project config: %v", err)
	}
	m.stage = StageDone
	if err := cmd.createProject(m); err != nil {
		t.Fatalf("create project: %v", err)
	}

	if cwd, err := os.Getwd(); err != nil || cwd != sentinel {
		t.Fatalf("working directory = %q, %v; want unchanged %q", cwd, err, sentinel)
	}
	for _, path := range []string{".goforj.yml", "README.md", "go.mod", filepath.Join("cmd", "app", "main.go")} {
		if _, err := os.Stat(filepath.Join(target, path)); err != nil {
			t.Fatalf("target output %s: %v", path, err)
		}
		if _, err := os.Stat(filepath.Join(sentinel, path)); !os.IsNotExist(err) {
			t.Fatalf("process cwd received project output %s: %v", path, err)
		}
	}
	readme, err := os.ReadFile(filepath.Join(target, "README.md"))
	if err != nil {
		t.Fatalf("read generated project README: %v", err)
	}
	for _, expected := range []string{"twitter-banner.png", "# Rooted App", "forj dev", "forj build"} {
		if !strings.Contains(string(readme), expected) {
			t.Errorf("generated project README missing %q", expected)
		}
	}
	assertNewProjectCommandsRootedAt(t, commandLog, target)
	for _, path := range []string{
		"AGENTS.md",
		filepath.Join(".agents", "skills"),
		filepath.Join(".codex", "config.toml"),
	} {
		if _, err := os.Stat(filepath.Join(target, path)); err != nil {
			t.Fatalf("selected Codex projection %s: %v", path, err)
		}
	}
	guidance, err := os.ReadFile(filepath.Join(target, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read selected Codex guidance: %v", err)
	}
	if !strings.Contains(string(guidance), "flat, self-contained, and portable") || !strings.Contains(string(guidance), "can stand on its own") {
		t.Fatalf("selected Codex guidance omitted package-boundary guidance:\n%s", guidance)
	}
	for _, path := range []string{
		"CLAUDE.md",
		"GEMINI.md",
		".claude",
		".gemini",
		".vscode",
		filepath.Join(".github", "copilot-instructions.md"),
		filepath.Join(".github", "instructions"),
		filepath.Join(".github", "prompts"),
	} {
		if _, err := os.Stat(filepath.Join(target, path)); !os.IsNotExist(err) {
			t.Fatalf("unselected agent projection %s was rendered: %v", path, err)
		}
	}

	config, err := project.LoadProjectConfigAt(target)
	if err != nil {
		t.Fatalf("load target config: %v", err)
	}
	if len(config.Dev.Watches) != 1 || config.Dev.Watches[0].Name != "NPM" {
		t.Fatalf("target package.json was not used for lifecycle discovery: %#v", config.Dev.Watches)
	}
}

// installNewProjectCommandProbe installs deterministic Go and Wire commands that record each process directory.
func installNewProjectCommandProbe(t *testing.T) string {
	t.Helper()
	toolsDir := t.TempDir()
	logPath := filepath.Join(toolsDir, "commands.log")
	script := `#!/bin/sh
printf '%s\n' "$PWD" >> "$FORJ_NEW_PROJECT_COMMAND_LOG"
if [ "$1" = "mod" ] && [ "$2" = "init" ]; then
  printf 'module %s\n\ngo 1.25\n' "$3" > go.mod
fi
`
	for _, name := range []string{"go", "wire"} {
		if err := os.WriteFile(filepath.Join(toolsDir, name), []byte(script), 0o755); err != nil {
			t.Fatalf("write fake %s command: %v", name, err)
		}
	}
	t.Setenv("FORJ_NEW_PROJECT_COMMAND_LOG", logPath)
	t.Setenv("PATH", toolsDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

// assertNewProjectCommandsRootedAt verifies every observed subprocess stayed within the new project tree.
func assertNewProjectCommandsRootedAt(t *testing.T, logPath string, target string) {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read command log: %v", err)
	}
	directories := make([]string, 0)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		directories = append(directories, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read command log lines: %v", err)
	}
	if len(directories) == 0 {
		t.Fatal("project creation did not run any observed commands")
	}
	for _, directory := range directories {
		withinTarget, err := newProjectCommandDirectoryWithinTarget(target, directory)
		if err != nil {
			t.Fatalf("resolve command directory %q against target %q: %v", directory, target, err)
		}
		if !withinTarget {
			t.Fatalf("command directory %q is outside target %q", directory, target)
		}
	}
}

// newProjectCommandDirectoryWithinTarget resolves filesystem aliases before checking command containment.
func newProjectCommandDirectoryWithinTarget(target string, directory string) (bool, error) {
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return false, fmt.Errorf("resolve target path: %w", err)
	}
	resolvedDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return false, fmt.Errorf("resolve command path: %w", err)
	}
	relative, err := filepath.Rel(resolvedTarget, resolvedDirectory)
	if err != nil {
		return false, err
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)), nil
}

// TestNewProjectCommandDirectoryWithinTargetResolvesAliases verifies physical containment without allowing symlink escapes.
func TestNewProjectCommandDirectoryWithinTargetResolvesAliases(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, "target")
	commandDirectory := filepath.Join(target, "cmd", "app")
	outside := filepath.Join(root, "outside")
	for _, directory := range []string{commandDirectory, outside} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create directory %s: %v", directory, err)
		}
	}
	alias := filepath.Join(root, "target-alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}

	withinTarget, err := newProjectCommandDirectoryWithinTarget(alias, commandDirectory)
	if err != nil {
		t.Fatalf("compare aliased target: %v", err)
	}
	if !withinTarget {
		t.Fatalf("physical command directory %q was outside alias %q", commandDirectory, alias)
	}
	withinTarget, err = newProjectCommandDirectoryWithinTarget(target, outside)
	if err != nil {
		t.Fatalf("compare outside directory: %v", err)
	}
	if withinTarget {
		t.Fatalf("outside command directory %q was accepted beneath target %q", outside, target)
	}

	escape := filepath.Join(target, "outside-alias")
	if err := os.Symlink(outside, escape); err != nil {
		t.Fatalf("create outside alias: %v", err)
	}
	withinTarget, err = newProjectCommandDirectoryWithinTarget(target, escape)
	if err != nil {
		t.Fatalf("compare outside alias: %v", err)
	}
	if withinTarget {
		t.Fatalf("outside command directory %q was accepted beneath target %q", outside, target)
	}
}

// TestNewProjectComponentsExposeConcreteDatabaseEngines keeps the stateful database choice in the component checklist.
func TestNewProjectComponentsExposeConcreteDatabaseEngines(t *testing.T) {
	m := initialModel()
	databaseRows := map[project.ComponentKey]ListItem{}
	for _, listItem := range m.componentList.Items() {
		item := listItem.(ListItem)
		if project.IsAppDatabaseComponent(item.Key) {
			databaseRows[item.Key] = item
		}
	}
	if len(databaseRows) != 3 {
		t.Fatalf("database rows = %#v, want MySQL, Postgres, and SQLite", databaseRows)
	}
	for key, label := range map[project.ComponentKey]string{
		project.ComponentDatabaseMySQL:    "Database (MySQL)",
		project.ComponentDatabasePostgres: "Database (Postgres)",
		project.ComponentDatabaseSQLite:   "Database (SQLite)",
	} {
		if item := databaseRows[key]; item.Name != label {
			t.Fatalf("database row %q label = %q, want %q", key, item.Name, label)
		}
		m.stage = StageSelectComponents
		if view := ansi.Strip(m.View()); !strings.Contains(view, label) {
			t.Fatalf("component view is missing %q:\n%s", label, view)
		}
	}
	if !databaseRows[project.ComponentDatabaseMySQL].Selected || databaseRows[project.ComponentDatabasePostgres].Selected || databaseRows[project.ComponentDatabaseSQLite].Selected {
		t.Fatalf("initial database selection = %#v, want only MySQL", databaseRows)
	}
}

// TestNewProjectExposesPrimitiveDefaultsAsComponents keeps the common path default-on without adding another wizard stage.
func TestNewProjectExposesPrimitiveDefaultsAsComponents(t *testing.T) {
	m := initialModel()
	want := map[project.ComponentKey]string{
		project.ComponentCache:   "Cache",
		project.ComponentEvents:  "Events",
		project.ComponentStorage: "File Storage",
		project.ComponentJobs:    "Background Jobs",
	}
	for _, raw := range m.componentList.Items() {
		item := raw.(ListItem)
		label, ok := want[item.Key]
		if !ok {
			continue
		}
		if item.Name != label || !item.Selected {
			t.Fatalf("primitive row %q = %#v, want selected %q", item.Key, item, label)
		}
		delete(want, item.Key)
	}
	if len(want) != 0 {
		t.Fatalf("new project wizard is missing primitive rows: %#v", want)
	}
	m.applyComponentSelection()
	if !m.config.Render.Components.Cache || !m.config.Render.Components.Events || !m.config.Render.Components.Storage || !m.config.Render.Components.Jobs {
		t.Fatalf("primitive defaults were lost while applying wizard selections: %#v", m.config.Render.Components)
	}
}

// TestDatabaseEngineSelectionIsExclusive verifies the component checklist owns one concrete database engine.
func TestDatabaseEngineSelectionIsExclusive(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents
	selectComponentRowByKey(t, &m, project.ComponentDatabasePostgres)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(model)
	m.applyComponentSelection()

	if componentSelectedByKey(t, m, project.ComponentDatabaseMySQL) || !componentSelectedByKey(t, m, project.ComponentDatabasePostgres) || componentSelectedByKey(t, m, project.ComponentDatabaseSQLite) {
		t.Fatalf("database rows did not select only Postgres")
	}
	if !m.config.Render.Components.DatabasePostgres || m.config.Render.Components.DatabaseMySQL || m.config.Render.Components.DatabaseSQLite {
		t.Fatalf("render components did not retain only Postgres: %#v", m.config.Render.Components)
	}
}

// TestSetAllComponentsKeepsConcreteDatabaseExclusive verifies the bulk shortcut preserves one chosen engine.
func TestSetAllComponentsKeepsConcreteDatabaseExclusive(t *testing.T) {
	m := initialModel()
	setComponentSelectedByKey(t, &m, project.ComponentDatabaseMySQL, false)
	setComponentSelectedByKey(t, &m, project.ComponentDatabasePostgres, true)

	m.setAllComponents(true)

	if componentSelectedByKey(t, m, project.ComponentDatabaseMySQL) || !componentSelectedByKey(t, m, project.ComponentDatabasePostgres) || componentSelectedByKey(t, m, project.ComponentDatabaseSQLite) {
		t.Fatal("select all did not preserve Postgres as the sole database")
	}
}

// TestAuthBlocksLastDatabaseDeselection makes the capability dependency immediate without changing the selected engine.
func TestAuthBlocksLastDatabaseDeselection(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents
	setComponentSelectedByKey(t, &m, project.ComponentAuth, true)
	setComponentSelectedByKey(t, &m, project.ComponentOAuth, false)
	selectComponentRowByKey(t, &m, project.ComponentDatabaseMySQL)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(model)

	if !componentSelectedByKey(t, m, project.ComponentDatabaseMySQL) {
		t.Fatalf("expected auth to keep MySQL selected")
	}
	if !strings.Contains(m.errorMsg, "Database remains enabled because Auth requires it.") {
		t.Fatalf("expected capability-level dependency message, got %q", m.errorMsg)
	}
	if strings.Contains(m.errorMsg, "MySQL") {
		t.Fatalf("database dependency should not prescribe a driver: %q", m.errorMsg)
	}
}

// TestDatabaseCanBeDisabledWithoutAuth preserves database-free project generation when nothing requires persistence.
func TestDatabaseCanBeDisabledWithoutAuth(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents
	setComponentSelectedByKey(t, &m, project.ComponentOAuth, false)
	setComponentSelectedByKey(t, &m, project.ComponentAuth, false)
	selectComponentRowByKey(t, &m, project.ComponentDatabaseMySQL)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(model)
	m.applyComponentSelection()

	if componentSelectedByKey(t, m, project.ComponentDatabaseMySQL) {
		t.Fatalf("expected MySQL to remain disabled")
	}
	if m.config.Render.Components.HasDatabase() {
		t.Fatalf("expected no database render flag when the capability is disabled")
	}
}

// TestDemoDatabaseLockRestoresUnlockedChoice verifies Demo's MySQL constraint is temporary and non-destructive.
func TestDemoDatabaseLockRestoresUnlockedChoice(t *testing.T) {
	m := initialModel()
	setComponentSelectedByKey(t, &m, project.ComponentDatabaseMySQL, false)
	setComponentSelectedByKey(t, &m, project.ComponentDatabasePostgres, true)
	setComponentSelectedByKey(t, &m, project.ComponentJobs, false)
	m.applyComponentSelection()
	if !m.config.Render.Components.DatabasePostgres {
		t.Fatalf("expected the unlocked Postgres choice before Demo")
	}

	m.extrasIndex = 1
	m.applyExtrasSelection()
	if !componentSelectedByKey(t, m, project.ComponentDatabasePostgres) || componentSelectedByKey(t, m, project.ComponentDatabaseMySQL) {
		t.Fatal("expected Demo to preserve the concrete Postgres row")
	}
	if !m.config.Render.Components.DatabaseMySQL || m.config.Render.Components.DatabasePostgres {
		t.Fatalf("expected Demo to temporarily force MySQL")
	}
	if !m.config.Render.Components.Jobs {
		t.Fatalf("expected Demo to force its core component selections")
	}
	componentSummary := strings.Join(m.selectedComponentNames(), ", ")
	if !strings.Contains(componentSummary, "Database (MySQL)") || strings.Contains(componentSummary, "Database (Postgres)") {
		t.Fatalf("expected Demo summary to describe its effective MySQL choice, got %q", componentSummary)
	}
	if !strings.Contains(componentSummary, "Jobs") {
		t.Fatalf("expected Demo summary to include its effective Jobs choice, got %q", componentSummary)
	}

	m.extrasIndex = 0
	m.applyExtrasSelection()
	if !m.config.Render.Components.DatabasePostgres || m.config.Render.Components.DatabaseMySQL {
		t.Fatalf("expected disabling Demo to restore Postgres")
	}
	if m.config.Render.Components.Jobs {
		t.Fatalf("expected disabling Demo to reconstruct pre-Demo component selections")
	}
	componentSummary = strings.Join(m.selectedComponentNames(), ", ")
	if !strings.Contains(componentSummary, "Database (Postgres)") || strings.Contains(componentSummary, "Database (MySQL)") {
		t.Fatalf("expected summary to restore the unlocked Postgres choice, got %q", componentSummary)
	}
}

func TestDemoAppEnablesCoreComponents(t *testing.T) {
	m := initialModel()
	m.config.Render.Components.CLI = true
	m.config.Render.Components.DatabaseMySQL = true
	m.config.Render.StarterKit = project.StarterKitVue
	m.extrasIndex = 1

	m.applyExtrasSelection()

	if !m.config.Render.Components.DemoApp {
		t.Fatalf("expected demo app to be enabled")
	}
	if !m.config.Render.Components.Auth || !m.config.Render.Components.WebAPI || !m.config.Render.Components.WebUI || !m.config.Render.Components.Scheduler || !m.config.Render.Components.Jobs {
		t.Fatalf("expected core demo components to be enabled")
	}
	if !m.config.Render.Components.DatabaseMySQL {
		t.Fatalf("expected mysql to be enabled for demo app")
	}
	if m.config.Render.Components.DatabaseSQLite || m.config.Render.Components.DatabasePostgres {
		t.Fatalf("expected other database selections to be cleared")
	}
	if m.config.Render.StarterKit != project.StarterKitNone {
		t.Fatalf("expected demo app to clear starter kit, got %q", m.config.Render.StarterKit)
	}
}

func TestOAuthSelectionAlsoEnablesAuth(t *testing.T) {
	m := initialModel()

	setComponentSelectedByKey(t, &m, project.ComponentOAuth, true)

	m.applyComponentSelection()

	if !m.config.Render.Components.OAuth {
		t.Fatalf("expected oauth component to be enabled")
	}
	if !m.config.Render.Components.Auth {
		t.Fatalf("expected oauth selection to force auth on")
	}
	if !m.config.Render.Components.Mail {
		t.Fatalf("expected auth selection to force mail on")
	}
}

func TestMailSelectionEnablesMailComponent(t *testing.T) {
	m := initialModel()

	setComponentSelectedByKey(t, &m, project.ComponentMail, true)

	m.applyComponentSelection()

	if !m.config.Render.Components.Mail {
		t.Fatalf("expected mail component to be enabled")
	}
}

func TestAuthSelectionAlsoEnablesDependencies(t *testing.T) {
	m := initialModel()
	m.config.Render.Components.Cache = false

	setComponentSelectedByKey(t, &m, project.ComponentAuth, true)

	m.applyComponentSelection()

	if !m.config.Render.Components.Auth {
		t.Fatalf("expected auth component to be enabled")
	}
	if !m.config.Render.Components.Mail {
		t.Fatalf("expected auth selection to force mail on")
	}
	if !m.config.Render.Components.WebAPI {
		t.Fatalf("expected auth selection to force the API capability used by generated auth routes")
	}
	if !m.config.Render.Components.Cache {
		t.Fatalf("expected auth selection to force its Cache dependency")
	}
}

func TestAuthToggleAutoSelectsDependenciesInWizard(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents

	setComponentSelectedByKey(t, &m, project.ComponentMail, false)
	setComponentSelectedByKey(t, &m, project.ComponentAuth, false)
	setComponentSelectedByKey(t, &m, project.ComponentOAuth, false)
	selectComponentRowByKey(t, &m, project.ComponentAuth)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(model)

	if !componentSelectedByKey(t, m, project.ComponentAuth) {
		t.Fatalf("expected auth to be selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentMail) {
		t.Fatalf("expected mail to be auto-selected when auth is selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentCache) {
		t.Fatalf("expected cache to be auto-selected when auth is selected")
	}
}

func TestOAuthToggleAutoSelectsAuthAndMailInWizard(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents

	setComponentSelectedByKey(t, &m, project.ComponentMail, false)
	setComponentSelectedByKey(t, &m, project.ComponentAuth, false)
	setComponentSelectedByKey(t, &m, project.ComponentOAuth, false)
	selectComponentRowByKey(t, &m, project.ComponentOAuth)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(model)

	if !componentSelectedByKey(t, m, project.ComponentOAuth) {
		t.Fatalf("expected oauth to be selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentAuth) {
		t.Fatalf("expected auth to be auto-selected when oauth is selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentMail) {
		t.Fatalf("expected mail to be auto-selected when oauth is selected")
	}
}

func TestGrafanaToggleAutoSelectsObservabilityChainInWizard(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents

	setComponentSelectedByKey(t, &m, project.ComponentGrafana, false)
	setComponentSelectedByKey(t, &m, project.ComponentObservability, false)
	setComponentSelectedByKey(t, &m, project.ComponentMetrics, false)
	setComponentSelectedByKey(t, &m, project.ComponentWebAPI, false)
	setComponentSelectedByKey(t, &m, project.ComponentDocker, false)
	selectComponentRowByKey(t, &m, project.ComponentGrafana)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(model)

	if !componentSelectedByKey(t, m, project.ComponentGrafana) {
		t.Fatalf("expected grafana to be selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentObservability) {
		t.Fatalf("expected observability to be auto-selected when grafana is selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentMetrics) {
		t.Fatalf("expected metrics to be auto-selected when grafana is selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentWebAPI) {
		t.Fatalf("expected web api to be auto-selected when grafana is selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentDocker) {
		t.Fatalf("expected docker to be auto-selected when grafana is selected")
	}
}

func TestAuthToggleAlsoClearsOAuthInWizard(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents

	setComponentSelectedByKey(t, &m, project.ComponentOAuth, true)
	setComponentSelectedByKey(t, &m, project.ComponentAuth, true)
	setComponentSelectedByKey(t, &m, project.ComponentMail, true)
	selectComponentRowByKey(t, &m, project.ComponentAuth)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(model)

	if componentSelectedByKey(t, m, project.ComponentOAuth) {
		t.Fatalf("expected oauth to be deselected when auth is deselected")
	}
	if componentSelectedByKey(t, m, project.ComponentAuth) {
		t.Fatalf("expected auth to be deselected")
	}
}

func TestMailToggleDoesNotClearAuthOrOAuthInWizard(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents

	setComponentSelectedByKey(t, &m, project.ComponentMail, true)
	setComponentSelectedByKey(t, &m, project.ComponentAuth, true)
	setComponentSelectedByKey(t, &m, project.ComponentOAuth, true)
	selectComponentRowByKey(t, &m, project.ComponentMail)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(model)

	if !componentSelectedByKey(t, m, project.ComponentAuth) {
		t.Fatalf("expected auth to remain selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentOAuth) {
		t.Fatalf("expected oauth to remain selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentMail) {
		t.Fatalf("expected mail to remain selected because auth depends on it")
	}
	if !strings.Contains(m.errorMsg, "Mail remains enabled because Auth requires it.") {
		t.Fatalf("expected explanatory error message, got %q", m.errorMsg)
	}
}

func TestStarterKitStageAppearsWhenWebUIEnabled(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents
	setComponentSelectedByKey(t, &m, project.ComponentWebUI, true)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.stage != StageHelpFormat {
		t.Fatalf("expected help format stage when web ui is enabled, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.stage != StageStarterKit {
		t.Fatalf("expected starter kit stage after help format when web ui is enabled, got %v", m.stage)
	}
}

func TestStarterKitStageSkippedWhenWebUIDisabled(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents
	m.config.Render.StarterKit = project.StarterKitVue
	setComponentSelectedByKey(t, &m, project.ComponentWebUI, false)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.stage != StageHelpFormat {
		t.Fatalf("expected help format stage when web ui is disabled, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.stage != StageExtras {
		t.Fatalf("expected extras stage after help format when web ui is disabled, got %v", m.stage)
	}
	if m.config.Render.StarterKit != project.StarterKitNone {
		t.Fatalf("expected starter kit to be cleared when web ui is disabled, got %q", m.config.Render.StarterKit)
	}
}

// TestDemoBackNavigationDoesNotInventSkippedStarterKit keeps reverse navigation on the path the user actually traversed.
func TestDemoBackNavigationDoesNotInventSkippedStarterKit(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents
	setComponentSelectedByKey(t, &m, project.ComponentWebUI, false)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageExtras || m.starterKitApplicable {
		t.Fatalf("expected the forward path to skip Starter Kit, stage=%v applicable=%t", m.stage, m.starterKitApplicable)
	}

	m.extrasIndex = 1
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageAtlasSupport || !m.config.Render.Components.WebUI {
		t.Fatalf("expected Demo to enable Web UI after the skipped stage, stage=%v components=%#v", m.stage, m.config.Render.Components)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = next.(model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = next.(model)
	if m.stage != StageHelpFormat {
		t.Fatalf("expected back navigation to return to Help, got %v", m.stage)
	}
}

// TestHelpFormatPreviewOrderMatchesOptionOrder prevents the side-by-side panes from drifting away from the option list.
func TestHelpFormatPreviewOrderMatchesOptionOrder(t *testing.T) {
	m := initialModel()
	m.stage = StageHelpFormat
	m.termWidth = 160
	m.helpFormatList.Select(1)

	view := ansi.Strip(m.renderHelpFormatPanel())

	frameworkIndex := strings.Index(view, "Framework Preview")
	guidedIndex := strings.Index(view, "Guided Preview")
	externalIndex := strings.Index(view, "External CLI Preview")
	if frameworkIndex < 0 || guidedIndex < 0 || externalIndex < 0 {
		t.Fatalf("expected all preview panels to be rendered, got %q", view)
	}
	if !(frameworkIndex < guidedIndex && guidedIndex < externalIndex) {
		t.Fatalf("expected preview panels to match option order, got %q", view)
	}
}

func TestVueStarterKitSelectionPersists(t *testing.T) {
	m := initialModel()
	m.stage = StageStarterKit
	m.config.Render.Components.WebUI = true
	selectStarterKitRow(t, &m, project.StarterKitVue)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.stage != StageExtras {
		t.Fatalf("expected extras stage after starter kit selection, got %v", m.stage)
	}
	if m.config.Render.StarterKit != project.StarterKitVue {
		t.Fatalf("expected vue starter kit, got %q", m.config.Render.StarterKit)
	}
}

// TestStarterKitComponentLibraryCanBeDisabled verifies the wizard persists the nested opt-out without changing the starter-kit scalar.
func TestStarterKitComponentLibraryCanBeDisabled(t *testing.T) {
	m := initialModel()
	m.stage = StageStarterKit
	m.config.Render.Components.WebUI = true
	selectStarterKitRow(t, &m, project.StarterKitVue)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.config.Render.StarterKit != project.StarterKitVue {
		t.Fatalf("starter kit = %q, want vue", m.config.Render.StarterKit)
	}
	if m.config.Render.StarterKitOptions.ComponentLibraryEnabled() {
		t.Fatal("component library = true, want false")
	}
}

// TestStarterKitComponentLibraryRendersAsRadioInputs verifies the project wizard exposes both option choices and their selected state.
func TestStarterKitComponentLibraryRendersAsRadioInputs(t *testing.T) {
	m := initialModel()
	m.stage = StageStarterKit
	m.config.Render.Components.WebUI = true
	selectStarterKitRow(t, &m, project.StarterKitVue)

	view := ansi.Strip(m.renderStarterKitOptions())
	for _, expected := range []string{"Options", "Component library", "● On", "○ Off"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("starter-kit options omitted %q:\n%s", expected, view)
		}
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	view = ansi.Strip(next.(model).renderStarterKitOptions())
	for _, expected := range []string{"○ On", "● Off"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("toggled starter-kit options omitted %q:\n%s", expected, view)
		}
	}
}

func TestAtlasRecommendedMovesToProjectPath(t *testing.T) {
	m := initialModel()
	m.stage = StageAtlasSupport
	selectAtlasModeRow(t, &m, atlasModeRecommended)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.stage != StageProjectPath {
		t.Fatalf("expected recommended atlas to continue to project path, got %v", m.stage)
	}
	if !m.atlasInstallEnabled() {
		t.Fatalf("expected recommended atlas install to be enabled")
	}
	surfaces := m.selectedAtlasSurfaces()
	if !surfaces.guidelines || !surfaces.skills || !surfaces.mcp {
		t.Fatalf("expected recommended atlas surfaces, got %#v", surfaces)
	}
	if len(m.selectedAtlasAgents()) == 0 {
		t.Fatalf("expected recommended atlas agents")
	}
	m.atlasRecommendations = []string{"wizard-snapshot"}
	t.Chdir(t.TempDir())
	if got := m.atlasInstallOptions(t.TempDir()).Agents; !reflect.DeepEqual(got, m.atlasRecommendations) {
		t.Fatalf("Atlas install agents = %v, want wizard snapshot %v", got, m.atlasRecommendations)
	}
}

func TestAtlasMinimalInstallsGuidelinesOnly(t *testing.T) {
	m := initialModel()
	m.stage = StageAtlasSupport
	selectAtlasModeRow(t, &m, atlasModeMinimal)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	surfaces := m.selectedAtlasSurfaces()
	if !surfaces.guidelines || surfaces.skills || surfaces.mcp {
		t.Fatalf("expected minimal atlas guidelines only, got %#v", surfaces)
	}
	if got := m.selectedAgentGuidance(); got != project.AgentGuidanceBaseline {
		t.Fatalf("minimal agent guidance = %q", got)
	}
}

func TestAtlasSkipDisablesInstall(t *testing.T) {
	m := initialModel()
	m.stage = StageAtlasSupport
	selectAtlasModeRow(t, &m, atlasModeSkip)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.stage != StageProjectPath {
		t.Fatalf("expected skip to continue to project path, got %v", m.stage)
	}
	if m.atlasInstallEnabled() {
		t.Fatalf("expected skip to disable atlas install")
	}
	if got := m.selectedAgentGuidance(); got != project.AgentGuidanceNone {
		t.Fatalf("skip agent guidance = %q", got)
	}
}

func TestAtlasCustomRequiresAgentAndSurface(t *testing.T) {
	m := initialModel()
	m.stage = StageAtlasSupport
	selectAtlasModeRow(t, &m, atlasModeCustom)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageAtlasAgents {
		t.Fatalf("expected custom to open agent selection, got %v", m.stage)
	}

	for idx, listItem := range m.atlasAgentList.Items() {
		item := listItem.(AtlasAgentItem)
		item.Selected = false
		m.atlasAgentList.SetItem(idx, item)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageAtlasAgents || !strings.Contains(m.errorMsg, "Select at least one agent") {
		t.Fatalf("expected custom agent validation, stage=%v error=%q", m.stage, m.errorMsg)
	}

	m.toggleAtlasAgentSelection()
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageAtlasSurfaces {
		t.Fatalf("expected custom to open surface selection, got %v", m.stage)
	}

	for idx, listItem := range m.atlasSurfaceList.Items() {
		item := listItem.(AtlasSurfaceItem)
		item.Selected = false
		m.atlasSurfaceList.SetItem(idx, item)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageAtlasSurfaces || !strings.Contains(m.errorMsg, "Select at least one install option") {
		t.Fatalf("expected custom surface validation, stage=%v error=%q", m.stage, m.errorMsg)
	}

	m.toggleAtlasSurfaceSelection()
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageProjectPath {
		t.Fatalf("expected custom selections to continue to project path, got %v", m.stage)
	}
}

// TestAtlasCustomWithoutGuidelinesKeepsBaselineAbsent separates optional Atlas features from native guidance.
func TestAtlasCustomWithoutGuidelinesKeepsBaselineAbsent(t *testing.T) {
	m := initialModel()
	m.atlasMode = atlasModeCustom
	for index, listItem := range m.atlasSurfaceList.Items() {
		item := listItem.(AtlasSurfaceItem)
		item.Selected = item.Surface == atlasSurfaceSkills
		m.atlasSurfaceList.SetItem(index, item)
	}
	if got := m.selectedAgentGuidance(); got != project.AgentGuidanceNone {
		t.Fatalf("custom skills-only agent guidance = %q", got)
	}
}

// TestDefaultResourcePlanCoordinatesJobsWithoutWizardStage verifies the shorter route starts Jobs locally with Redis included.
func TestDefaultResourcePlanCoordinatesJobsWithoutWizardStage(t *testing.T) {
	m := initialModel()
	m.projectInput.SetValue("MyApp")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	m.moduleInput.SetValue("github.com/example/myapp")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	// Select Jobs in component list.
	setComponentSelectedByKey(t, &m, project.ComponentJobs, true)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageHelpFormat {
		t.Fatalf("expected help format stage, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageStarterKit {
		t.Fatalf("expected starter kit stage after help format, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageExtras {
		t.Fatalf("expected extras stage after starter kit, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageAtlasSupport {
		t.Fatalf("expected Atlas after extras, got %v", m.stage)
	}
	preparation, err := m.selectedResourcePreparation()
	if err != nil {
		t.Fatalf("selectedResourcePreparation returned error: %v", err)
	}
	queue, ok := preparation.plan.Selection(project.ResourceQueue)
	if !ok || queue.Active != "workerpool" || !reflect.DeepEqual(queue.Supported, []string{"workerpool", "redis"}) {
		t.Fatalf("default queue selection = %#v, exists %v", queue, ok)
	}
}

// TestWizardOmitsResourcesStageAndPanel keeps internal driver defaults out of the normal decision flow.
func TestWizardOmitsResourcesStageAndPanel(t *testing.T) {
	m := initialModel()
	m.stage = StageExtras
	m.termWidth = wizardWidth

	progress := ansi.Strip(m.renderProgress())
	view := ansi.Strip(m.View())
	if strings.Contains(progress, "Resources") || strings.Contains(view, "App Resources") {
		t.Fatalf("wizard retained the removed resource stage:\nprogress=%s\nview=%s", progress, view)
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageAtlasSupport {
		t.Fatalf("expected Extras to continue directly to Atlas, got %v", m.stage)
	}
}

// TestDemoExtrasExplainsTemporaryMySQLConstraint makes the effective database change understandable without another decision screen.
func TestDemoExtrasExplainsTemporaryMySQLConstraint(t *testing.T) {
	m := initialModel()
	m.stage = StageExtras
	m.extrasIndex = 1
	m.termWidth = wizardWidth

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Demo currently requires MySQL") || !strings.Contains(view, "database choice returns") {
		t.Fatalf("Demo selection hid its temporary database constraint:\n%s", view)
	}

	m.extrasIndex = 0
	view = ansi.Strip(m.View())
	if strings.Contains(view, "Demo currently requires MySQL") {
		t.Fatalf("disabled Demo added irrelevant database guidance:\n%s", view)
	}
}

func TestFinalizeConfigKeepsResourceTopologyOutOfProjectYAML(t *testing.T) {
	m := initialModel()
	m.config.Render.Components.Jobs = true

	m.finalizeConfig()

	encoded, err := yaml.Marshal(m.config)
	if err != nil {
		t.Fatalf("marshal finalized config: %v", err)
	}
	for _, transient := range []string{"queue_driver:", "resource_shape:", "resource_plan:"} {
		if strings.Contains(string(encoded), transient) {
			t.Fatalf("wizard-only resource state %q was persisted:\n%s", transient, encoded)
		}
	}
	if m.config.Render.GoForjVersion != version.Semver() {
		t.Fatalf("expected goforj version %q, got %q", version.Semver(), m.config.Render.GoForjVersion)
	}
}

// TestFinalizeConfigUsesNativeDefaultAppLifecycle verifies generated runtime Apps expose their native lifecycle configuration.
func TestFinalizeConfigUsesNativeDefaultAppLifecycle(t *testing.T) {
	m := initialModel()
	m.config.Render.Components.WebAPI = true

	m.finalizeConfig()

	for _, task := range m.config.Dev.Pre {
		if task.Name == "Initial build" && task.Cmd == "forj build -o ./bin/app" {
			t.Fatalf("expected initial build to be owned by forj dev, got pre task %#v", task)
		}
	}

	for _, watch := range m.config.Dev.Watches {
		if watch.Name == "Build App" || watch.Name == "Run App" || watch.Name == "Wire" {
			t.Fatalf("expected app lifecycle to avoid framework raw watchers, got %#v", watch)
		}
	}
	app, ok := m.config.Dev.Apps[project.DefaultAppName]
	if !ok {
		t.Fatalf("expected native default app runtime, got %#v", app)
	}
	wantBuild := conventionalDevAppBuildCommand(&m.config, project.DefaultApp())
	if app.Build == nil || !reflect.DeepEqual(*app.Build, wantBuild) {
		t.Fatalf("generated build = %#v, want %#v", app.Build, wantBuild)
	}
	wantRun := conventionalDevAppRuntimeCommand(project.DefaultApp())
	if app.Run == nil || !reflect.DeepEqual(*app.Run, wantRun) {
		t.Fatalf("generated runtime = %#v, want %#v", app.Run, wantRun)
	}
	compiled, err := compileDevWatchers(&m.config)
	if err != nil {
		t.Fatalf("compile generated dev lifecycle: %v", err)
	}
	if len(compiled) != 2 || len(compiled[0].Watch.Excludes) != len(wantBuild.Ignore) || compiled[1].FullProcessOverride {
		t.Fatalf("generated explicit lifecycle changed native defaults: %#v", compiled)
	}
	if m.config.Dev.Run != nil {
		t.Fatalf("expected generated config not to use the legacy dev.run allowlist, got %#v", m.config.Dev.Run)
	}
	encoded, err := yaml.Marshal(m.config)
	if err != nil {
		t.Fatalf("marshal generated project config: %v", err)
	}
	var decoded project.Config
	if err := yaml.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal generated project config: %v", err)
	}
	decodedApp := decoded.Dev.Apps[project.DefaultAppName]
	if decodedApp.Build == nil || !reflect.DeepEqual(*decodedApp.Build, wantBuild) ||
		decodedApp.Run == nil || !reflect.DeepEqual(*decodedApp.Run, wantRun) {
		t.Fatalf("generated lifecycle changed across YAML round trip: %#v", decodedApp)
	}
	decodedCompiled, err := compileDevWatchers(&decoded)
	if err != nil {
		t.Fatalf("compile round-tripped dev lifecycle: %v", err)
	}
	if len(decodedCompiled) != 2 || decodedCompiled[1].FullProcessOverride {
		t.Fatalf("round-tripped conventional runtime became a process override: %#v", decodedCompiled)
	}
	for _, want := range []string{
		"apps:",
		"exec: forj build -o ./bin/app",
		"watch: [.go, .env, .env.*]",
		"ignore: [forj, _data, wire_gen.go, .git, .hg, .svn, .idea, .vscode, .settings, node_modules]",
		"root: .",
		"postpone: true",
		"exec: ./bin/app",
	} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("expected generated dev.apps YAML to contain %q, got:\n%s", want, encoded)
		}
	}
	if strings.Contains(string(encoded), "run: run") {
		t.Fatalf("expected generated runtime to use its complete command, got:\n%s", encoded)
	}
	for _, legacy := range []string{"watches:", "Build App", "Run App"} {
		if strings.Contains(string(encoded), legacy) {
			t.Fatalf("expected generated YAML to omit legacy %q config, got:\n%s", legacy, encoded)
		}
	}
}

// TestFinalizeConfigInstallsVueStarterDependencies verifies Vue generation exposes its full frontend lifecycle.
func TestFinalizeConfigInstallsVueStarterDependencies(t *testing.T) {
	m := initialModel()
	m.config.Render.Components.WebUI = true
	m.config.Render.StarterKit = project.StarterKitVue

	m.finalizeConfig()

	foundInstall := false
	for _, task := range m.config.Dev.Pre {
		if task == generatedDevFrontendInstallTask(project.DefaultApp()) {
			foundInstall = true
			break
		}
	}
	if !foundInstall {
		t.Fatalf("expected vue starter dev pre-task to install frontend dependencies, got %#v", m.config.Dev.Pre)
	}
	app := m.config.Dev.Apps[project.DefaultAppName]
	wantSPA := conventionalDevSPAConfig("./cmd/app/frontend")
	if spa, ok := app.SPAs[generatedFrontendSPAName]; !ok || !reflect.DeepEqual(spa, wantSPA) {
		t.Fatalf("generated Vue SPA = %#v, want %#v", spa, wantSPA)
	}
	encoded, err := yaml.Marshal(m.config)
	if err != nil {
		t.Fatalf("marshal generated Vue config: %v", err)
	}
	for _, want := range []string{
		"path: ./cmd/app/frontend",
		"build: npm run build -s -- --logLevel error",
		"watch: [.ts, .tsx, .js, .jsx, .vue, .css, .html, package.json, package-lock.json]",
		"ignore: [_data, node_modules, dist]",
	} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("expected generated Vue config to contain %q, got:\n%s", want, encoded)
		}
	}
}

func TestFinalizeConfigDoesNotAddGrafanaSeedTask(t *testing.T) {
	m := initialModel()
	m.config.Render.Components.Docker = true
	m.config.Render.Components.Grafana = true

	m.finalizeConfig()

	for _, task := range m.config.Dev.Pre {
		if task.Name == "Seed Grafana Dashboards" {
			t.Fatalf("expected grafana seed to run through docker-compose up, got separate task %#v", task)
		}
	}
}

// TestFinalizeConfigTemplStarterUsesOwnedFrontendLifecycle verifies generated SPAs replace raw frontend watcher wiring.
func TestFinalizeConfigTemplStarterUsesOwnedFrontendLifecycle(t *testing.T) {
	m := initialModel()
	m.config.Render.Components.WebUI = true
	m.config.Render.Components.WebAPI = true
	m.config.Render.StarterKit = project.StarterKitTemplHTMX

	m.finalizeConfig()

	for _, watch := range m.config.Dev.Watches {
		if watch.Name == "Build App" || watch.Name == "Run App" || watch.Name == "NPM" {
			t.Fatalf("expected templ lifecycle to avoid generated raw watchers, got %#v", watch)
		}
	}
	app := m.config.Dev.Apps[project.DefaultAppName]
	spa, ok := app.SPAs[generatedFrontendSPAName]
	wantSPA := conventionalDevSPAConfig("./cmd/app/frontend")
	wantBuild := conventionalDevAppBuildCommand(&m.config, project.DefaultApp())
	wantRun := conventionalDevAppRuntimeCommand(project.DefaultApp())
	if !ok || !reflect.DeepEqual(spa, wantSPA) || app.Build == nil || !reflect.DeepEqual(*app.Build, wantBuild) ||
		app.Run == nil || !reflect.DeepEqual(*app.Run, wantRun) {
		t.Fatalf("expected templ frontend ownership in the default app lifecycle, got %#v", app)
	}
	if !reflect.DeepEqual(app.Build.Watch, []string{".go", ".env", ".env.*", ".templ"}) ||
		app.Build.Ignore[len(app.Build.Ignore)-1] != `re:.*_templ\.go$` {
		t.Fatalf("expected templ-specific build matchers, got %#v", app.Build)
	}
}

// TestFinalizeConfigOmitsCLIOnlyAppFromDev verifies omission remains the unmanaged state.
func TestFinalizeConfigOmitsCLIOnlyAppFromDev(t *testing.T) {
	m := initialModel()
	m.config.Render.Components = project.Components{CLI: true}

	m.finalizeConfig()

	if len(m.config.Dev.Apps) != 0 {
		t.Fatalf("expected CLI-only generated app to remain unmanaged by dev, got %#v", m.config.Dev.Apps)
	}
	if !m.config.Dev.UsesStructuredApps() {
		t.Fatal("expected CLI-only generation to retain an explicit native-none Apps allowlist")
	}
	encoded, err := yaml.Marshal(m.config)
	if err != nil {
		t.Fatalf("marshal CLI-only generated config: %v", err)
	}
	if !strings.Contains(string(encoded), "apps: {}") {
		t.Fatalf("expected CLI-only generated YAML to retain apps: {}, got:\n%s", encoded)
	}
}

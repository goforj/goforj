package rendercheck

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/project"
)

// renderWorkspaceFS isolates the destructive preparation checks that workers must report rather than ignore.
type renderWorkspaceFS interface {
	removeAll(string) error
	stat(string) (fs.FileInfo, error)
}

// osRenderWorkspaceFS provides the production workspace operations.
type osRenderWorkspaceFS struct{}

// removeAll keeps destructive cleanup behind the worker's narrow filesystem boundary.
func (osRenderWorkspaceFS) removeAll(path string) error {
	return os.RemoveAll(path)
}

// stat keeps go.mod inspection behind the same boundary as workspace cleanup.
func (osRenderWorkspaceFS) stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

// renderComboWorker owns the filesystem and toolchain dependencies reused by one render worker.
type renderComboWorker struct {
	workspaceRoot  string
	moduleCache    string
	buildCache     string
	forjExecutable string
	runTests       bool
	filesystem     renderWorkspaceFS
}

// initModule creates the go.mod once for the shared render directory.
func (worker renderComboWorker) initModule() error {
	if _, err := worker.filesystem.stat(filepath.Join(worker.workspaceRoot, "go.mod")); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect go.mod: %w", err)
	}
	goMod := exec.Command("go", "mod", "init", "github.com/test/project")
	goMod.Dir = worker.workspaceRoot
	goMod.Env = append(os.Environ(),
		"GOMODCACHE="+worker.moduleCache,
		"GOCACHE="+worker.buildCache,
	)
	if err := goMod.Run(); err != nil {
		return err
	}
	return nil
}

// run prepares a clean workspace and returns any render failure to the coordinator for aggregation.
func (worker renderComboWorker) run(combo renderCombo) *renderComboFailure {
	comboID := combo.id
	apps, err := renderComboApps(combo)
	if err != nil {
		return newRenderComboFailure("invalid configured App", comboID, nil, err)
	}

	if err := worker.filesystem.removeAll(worker.workspaceRoot); err != nil {
		return newRenderComboFailure("workspace cleanup failed", comboID, nil, fmt.Errorf("remove prior workspace: %w", err))
	}
	if err := os.MkdirAll(worker.workspaceRoot, 0o755); err != nil {
		return newRenderComboFailure("workspace dir failed", comboID, nil, err)
	}
	defer func() {
		_ = worker.filesystem.removeAll(worker.workspaceRoot)
	}()

	if err := worker.initModule(); err != nil {
		return newRenderComboFailure("go mod init failed", comboID, nil, err)
	}
	cfg := project.Config{
		ProjectName:  fmt.Sprintf("TestProject%s", comboID),
		GoModuleName: "github.com/test/project",
		UpdatedAt:    time.Now().Format(time.RFC3339),
		Dev:          project.DevConfig{},
		Apps:         combo.apps,
		Render: project.RenderConfig{
			Components: combo.components,
			StarterKit: combo.starterKit,
		},
	}
	if repoRoot, err := os.Getwd(); err == nil {
		if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			cfg.Render.ModuleReplaces = map[string]string{"github.com/goforj/goforj": repoRoot}
		}
	}

	console.Actionf("Rendering components %s", strings.Join(combo.enabled, ", "))

	timer := newStepTimer()
	ymlPath := filepath.Join(worker.workspaceRoot, ".goforj.yml")

	if err := timer.Track("write_yaml", func() error {
		return writeRenderComboConfig(ymlPath, cfg, combo.legacyConfig)
	}); err != nil {
		return newRenderComboFailure("failed to write config", comboID, &cfg, err)
	}
	if err := timer.Track("seed_apps", func() error {
		return seedRenderComboApps(worker.workspaceRoot, apps)
	}); err != nil {
		return newRenderComboFailure("failed to seed configured Apps", comboID, &cfg, err)
	}

	if err := timer.Track("forj_render", func() error {
		return worker.runForj("render")
	}); err != nil {
		return newRenderComboFailure("render failed", comboID, &cfg, err)
	}

	if err := timer.Track("component_contract", func() error {
		return validateRenderedComponentContracts(worker.workspaceRoot, &cfg, apps)
	}); err != nil {
		return newRenderComboFailure("component contract failed", comboID, &cfg, err)
	}
	if combo.validateIdempotence {
		if err := timer.Track("render_idempotence", func() error {
			return worker.validateRenderIdempotence(&cfg, apps)
		}); err != nil {
			return newRenderComboFailure("render idempotence failed", comboID, &cfg, err)
		}
	}

	if combo.starterKit == project.StarterKitTemplHTMX {
		if err := timer.Track("templ_generate", func() error {
			templCmd := exec.Command("go", "run", "github.com/a-h/templ/cmd/templ@v0.3.1020", "generate")
			templCmd.Dir = worker.workspaceRoot
			templCmd.Env = append(os.Environ(),
				"GOMODCACHE="+worker.moduleCache,
				"GOCACHE="+worker.buildCache,
			)
			output, err := templCmd.CombinedOutput()
			if err != nil {
				return formatCommandFailure("templ generate", err, string(output), "")
			}
			return nil
		}); err != nil {
			return newRenderComboFailure("templ generate failed", comboID, &cfg, err)
		}
	}

	if err := timer.Track("wire_gen", func() error {
		for _, app := range apps {
			wireCmd := exec.Command("wire")
			wireCmd.Dir = filepath.Join(worker.workspaceRoot, app.WireDir)
			wireCmd.Env = append(os.Environ(),
				"GOMODCACHE="+worker.moduleCache,
				"GOCACHE="+worker.buildCache,
			)
			if output, err := wireCmd.CombinedOutput(); err != nil {
				return formatCommandFailure("wire generate "+app.Name, err, string(output), "")
			}
		}
		return nil
	}); err != nil {
		return newRenderComboFailure("wire generate failed", comboID, &cfg, err)
	}

	if err := timer.Track("go_build", func() error {
		binDir := filepath.Join(worker.workspaceRoot, "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			return fmt.Errorf("create bin dir: %w", err)
		}
		for _, app := range apps {
			args := []string{"build"}
			if renderBuildTraceEnabled() {
				args = append(args, "-x")
			}
			target := "./" + filepath.ToSlash(filepath.Dir(app.Entrypoint))
			args = append(args, "-o", filepath.Join(binDir, app.Name), target)
			build := exec.Command("go", args...)
			build.Dir = worker.workspaceRoot
			build.Env = append(os.Environ(),
				"GOMODCACHE="+worker.moduleCache,
				"GOCACHE="+worker.buildCache,
			)
			output, err := build.CombinedOutput()
			if err != nil {
				return formatCommandFailure("go build "+app.Name, err, string(output), "")
			}
			if renderBuildTraceEnabled() {
				console.Infof("go build trace for combo %s App %s:\n%s", comboID, app.Name, strings.TrimSpace(string(output)))
			}
		}
		return nil
	}); err != nil {
		return newRenderComboFailure("go build failed", comboID, &cfg, err)
	}

	if worker.runTests {
		if err := timer.Track("go_test", func() error {
			return runRenderedGoTests(worker.workspaceRoot, worker.moduleCache, worker.buildCache)
		}); err != nil {
			return newRenderComboFailure("go test failed", comboID, &cfg, err)
		}
	}

	timer.Report(fmt.Sprintf("combo %s (%s)", comboID, strings.Join(combo.enabled, ", ")))

	console.Successf("Passed")
	return nil
}

// renderComboApps returns every executable projection a render combo must compile.
func renderComboApps(combo renderCombo) ([]project.App, error) {
	names := make([]string, 0, len(combo.apps))
	for name := range combo.apps {
		if name == project.DefaultAppName {
			continue
		}
		if !project.IsSafeAppName(name) || project.IsReservedAppName(name) {
			return nil, fmt.Errorf("unsafe App name %q", name)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	apps := []project.App{project.DefaultApp()}
	for _, name := range names {
		apps = append(apps, project.DefaultNamedApp(name))
	}
	return apps, nil
}

// seedRenderComboApps makes configured named Apps discoverable before the first clean render replaces their markers.
func seedRenderComboApps(root string, apps []project.App) error {
	for _, app := range apps {
		if app.Name == project.DefaultAppName {
			continue
		}
		entrypoint := filepath.Join(root, app.Entrypoint)
		if err := os.MkdirAll(filepath.Dir(entrypoint), 0o755); err != nil {
			return fmt.Errorf("create App %s entrypoint directory: %w", app.Name, err)
		}
		if err := os.WriteFile(entrypoint, []byte("package main\n"), 0o644); err != nil {
			return fmt.Errorf("seed App %s entrypoint: %w", app.Name, err)
		}
	}
	return nil
}

// runRenderedGoTests clears inherited workspace flags so generated projects are tested as independent modules.
func runRenderedGoTests(dir, modCache, buildCache string) error {
	args := []string{"test", "-count=1", "./..."}
	goTest := exec.Command("go", args...)
	goTest.Dir = dir
	goTest.Env = append(os.Environ(),
		"GOMODCACHE="+modCache,
		"GOCACHE="+buildCache,
		"GOFLAGS=",
		"GOWORK=off",
	)
	output, err := goTest.CombinedOutput()
	if err != nil {
		return formatCommandFailure("go test", err, string(output), "")
	}
	if renderDebugEnabled() {
		console.Infof("go test packages: ./...")
	}
	return nil
}

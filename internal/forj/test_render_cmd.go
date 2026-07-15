package forj

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/testexec"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// TestRenderCmd renders a full project to a temp dir and verifies build + tests.
type TestRenderCmd struct {
	logger *logger.AppLogger

	// Silent suppresses shadow-printed commands.
	Silent bool `help:"Suppress command output" short:"s"`

	// Keep preserves the temp directory after completion.
	Keep bool `help:"Keep the temp directory after completion" short:"k"`
}

// Signature describes the hidden full-render validation command.
func (*TestRenderCmd) Signature() string {
	return `name:"test:render" help:"Render full project and run build/tests" hidden:""`
}

// NewTestRenderCmd creates a new TestRenderCmd instance.
func NewTestRenderCmd(logger *logger.AppLogger) *TestRenderCmd {
	return &TestRenderCmd{logger: logger}
}

// Run executes the render, build, and test steps against a temp project.
func (cmd *TestRenderCmd) Run() error {
	modCache, buildCache := testkit.GoCachePaths()
	caches := testexec.GoCaches{ModulePath: modCache, BuildPath: buildCache}

	dir, err := os.MkdirTemp("", "forj_render_")
	if err != nil {
		return err
	}
	if !cmd.Keep {
		defer os.RemoveAll(dir)
	}

	cfg := project.Config{
		ProjectName:  "Test Render",
		GoModuleName: "github.com/test/project",
		Dev: project.DevConfig{
			Pre:               []project.DevTask{},
			Down:              []project.DevTask{},
			DownOnExit:        false,
			SoundOnWatchError: false,
			Watches:           []project.DevWatch{},
		},
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:           true,
				Cache:         true,
				Docker:        true,
				Events:        true,
				WebAPI:        true,
				WebUI:         true,
				DatabaseMySQL: true,
				Scheduler:     true,
				Storage:       true,
				Jobs:          true,
			},
		},
		Apps: map[string]project.AppConfig{
			"customer-portal": {
				Components: project.Components{
					CLI:           true,
					Cache:         true,
					Events:        true,
					WebAPI:        true,
					WebUI:         true,
					DatabaseMySQL: true,
					Scheduler:     true,
					Storage:       true,
					Jobs:          true,
				},
			},
		},
	}
	if repoRoot, err := os.Getwd(); err == nil {
		if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			cfg.Render.ModuleReplaces = map[string]string{"github.com/goforj/goforj": repoRoot}
			webRoot := filepath.Clean(filepath.Join(repoRoot, "..", "web"))
			if _, err := os.Stat(filepath.Join(webRoot, "go.mod")); err == nil {
				cfg.Render.ModuleReplaces["github.com/goforj/web"] = webRoot
			}
		}
	}

	ymlPath := filepath.Join(dir, ".goforj.yml")
	if err := testkit.WriteProjectConfig(ymlPath, cfg); err != nil {
		return err
	}
	if err := writeConventionalAppMarker(dir, "customer-portal"); err != nil {
		return err
	}

	if !cmd.Silent {
		console.Actionf("Running test:render")
	}
	builtForj, err := testkit.BuildForjBinary(modCache, buildCache)
	if err != nil {
		return err
	}
	defer builtForj.Cleanup()
	forjExec := builtForj.Path
	workspace := testexec.NewWorkspace(cmd.logger, cmd.Silent, dir, caches)
	if err := workspace.Run("render", forjExec, "render"); err != nil {
		return err
	}
	if err := workspace.Run("default API index", forjExec, "build:api-index", "--strict"); err != nil {
		return err
	}
	if err := assertRenderedAPIIndexArtifacts(dir, project.DefaultAppName); err != nil {
		return err
	}
	if err := workspace.Run("customer-portal API index", forjExec, "customer-portal", "build:api-index", "--strict"); err != nil {
		return err
	}
	if err := assertRenderedAPIIndexArtifacts(dir, "customer-portal"); err != nil {
		return err
	}
	if err := workspace.Run("build", "go", "build", "./..."); err != nil {
		return err
	}
	if err := workspace.Run("resources describe", forjExec, "resources:describe", "--json"); err != nil {
		return err
	}
	if err := workspace.Run("customer-portal resources describe", forjExec, "customer-portal", "resources:describe", "--json"); err != nil {
		return err
	}
	if err := workspace.Run("backup plan", forjExec, "backup:plan", "--json"); err != nil {
		return err
	}
	if err := workspace.Run("build customer-portal", forjExec, "customer-portal", "build"); err != nil {
		return err
	}
	if err := workspace.Run("route list customer-portal", forjExec, "customer-portal", "route:list"); err != nil {
		return err
	}
	if err := workspace.Run("make customer-portal migration", forjExec, "customer-portal", "make:migration", "create_sessions", "--connection", "archive", "--no-open"); err != nil {
		return err
	}
	if err := assertGlobExists(filepath.Join(dir, "migrations", "customer-portal", "archive", "*create_sessions.up.sql")); err != nil {
		return err
	}
	if err := assertGlobExists(filepath.Join(dir, "migrations", "customer-portal", "archive", "*create_sessions.down.sql")); err != nil {
		return err
	}
	if err := workspace.Run("test", "go", "test", "./..."); err != nil {
		return err
	}
	if err := cmd.runCLIOnlyAPIIndexRender(forjExec, caches, cfg.Render.ModuleReplaces); err != nil {
		return err
	}

	if !cmd.Silent {
		console.Successf("Render/build/test completed")
	}
	if !cmd.Silent {
		cmd.logger.Info().Str("path", dir).Msg("Render/build/test completed")
	}
	return nil
}

// runCLIOnlyAPIIndexRender proves non-WebAPI renders compile and remove artifacts left by an earlier component selection.
func (cmd *TestRenderCmd) runCLIOnlyAPIIndexRender(forjExec string, caches testexec.GoCaches, moduleReplaces map[string]string) error {
	dir, err := os.MkdirTemp("/tmp", "forj_render_cli_")
	if err != nil {
		return err
	}
	if !cmd.Keep {
		defer os.RemoveAll(dir)
	} else if !cmd.Silent {
		console.Infof("CLI-only workspace: %s", dir)
	}

	config := project.Config{
		ProjectName:  "CLI API Index Render",
		GoModuleName: "github.com/test/cli-api-index",
		Render: project.RenderConfig{
			Components:     project.Components{CLI: true},
			ModuleReplaces: moduleReplaces,
		},
	}
	if err := testkit.WriteProjectConfig(filepath.Join(dir, ".goforj.yml"), config); err != nil {
		return err
	}
	workspace := testexec.NewWorkspace(cmd.logger, cmd.Silent, dir, caches)
	if err := workspace.Run("render CLI-only App", forjExec, "render"); err != nil {
		return err
	}
	paths := renderedAPIIndexArtifactPaths(dir, project.DefaultAppName)
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte("{\"stale\":true}\n"), 0o644); err != nil {
			return err
		}
	}
	if err := workspace.Run("clean CLI-only API index", forjExec, "build:api-index", "--strict"); err != nil {
		return err
	}
	for _, path := range paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			return fmt.Errorf("CLI-only API artifact %q was not removed: %v", path, err)
		}
	}
	if err := workspace.Run("build CLI-only App", "go", "build", "./..."); err != nil {
		return err
	}
	return workspace.Run("test CLI-only App", "go", "test", "./...")
}

// assertRenderedAPIIndexArtifacts verifies a WebAPI App produced the complete active artifact set.
func assertRenderedAPIIndexArtifacts(root string, appName string) error {
	for _, path := range renderedAPIIndexArtifactPaths(root, appName) {
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read rendered API index artifact %q: %w", path, err)
		}
		if len(strings.TrimSpace(string(content))) == 0 {
			return fmt.Errorf("rendered API index artifact %q is empty", path)
		}
	}
	return nil
}

// renderedAPIIndexArtifactPaths mirrors the public default/named App layout asserted by the build runner.
func renderedAPIIndexArtifactPaths(root string, appName string) []string {
	buildRoot := filepath.Join(root, "build")
	if appName != "" && appName != project.DefaultAppName {
		buildRoot = filepath.Join(buildRoot, appName)
	}
	return []string{
		filepath.Join(buildRoot, "api_index.json"),
		filepath.Join(buildRoot, "api_index.diagnostics.json"),
		filepath.Join(buildRoot, "openapi.json"),
	}
}

// writeConventionalAppMarker makes named-App selection exercise the same cmd/<name> convention used by real rendered projects.
func writeConventionalAppMarker(root string, name string) error {
	mainPath := filepath.Join(root, "cmd", name, "main.go")
	if err := os.MkdirAll(filepath.Dir(mainPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(mainPath, []byte("package main\n"), 0o644)
}

// assertGlobExists verifies a generated artifact was published under its timestamped name.
func assertGlobExists(pattern string) error {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("glob %s: %w", pattern, err)
	}
	if len(matches) == 0 {
		return fmt.Errorf("expected at least one file matching %s", pattern)
	}
	return nil
}

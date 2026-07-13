package forj

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/web/webindex"
)

// openAPIToolsImage pins validation and client generation to the same known CLI release.
const openAPIToolsImage = "openapitools/openapi-generator-cli:v7.6.0"

// TestOpenAPICmd validates generated OpenAPI and compiles a generated Go client.
type TestOpenAPICmd struct {
	logger *logger.AppLogger

	// Silent suppresses command output while retaining captured failures.
	Silent bool `help:"Suppress command output" short:"s"`
	// Keep preserves the /tmp workspace for inspection after the workflow completes.
	Keep bool `help:"Keep the temp workspace after completion" short:"k"`
	// Image selects the version-pinned CLI image shared by validation and generation.
	Image string `help:"OpenAPI validator and client generator image" default:"openapitools/openapi-generator-cli:v7.6.0"`

	validateSpecFn   func(image, specPath string, silent bool) error
	generateClientFn func(image, specPath, clientDir string, silent bool) error
	compileClientFn  func(clientDir string, silent bool) error
}

// Signature exposes the hidden OpenAPI integration workflow.
func (*TestOpenAPICmd) Signature() string {
	return `name:"test:openapi" help:"Build, validate, and compile generated OpenAPI client" hidden:""`
}

// NewTestOpenAPICmd wires replaceable integration hooks to the same pinned validator and client-generation workflow used outside tests.
func NewTestOpenAPICmd(logger *logger.AppLogger) *TestOpenAPICmd {
	cmd := &TestOpenAPICmd{
		logger: logger,
		Image:  openAPIToolsImage,
	}
	cmd.validateSpecFn = cmd.validateWithDocker
	cmd.generateClientFn = cmd.generateClientWithDocker
	cmd.compileClientFn = cmd.compileGeneratedClient
	return cmd
}

// Run executes OpenAPI generation, validation, and generated-client compilation against a fixture app.
func (cmd *TestOpenAPICmd) Run() error {
	tmpDir, err := os.MkdirTemp("/tmp", "forj_openapi_validation_")
	if err != nil {
		return err
	}
	if !cmd.Keep {
		defer os.RemoveAll(tmpDir)
	}

	if !cmd.Silent {
		console.Actionf("Running test:openapi")
		console.Infof("workspace: %s", tmpDir)
	}

	webModuleDir, err := resolveOpenAPIFixtureWebModule()
	if err != nil {
		return err
	}
	if err := writeOpenAPIFixture(tmpDir, webModuleDir); err != nil {
		return err
	}

	buildDir := filepath.Join(tmpDir, "build")
	out := filepath.Join(buildDir, "api_index.json")
	diagnostics := filepath.Join(buildDir, "api_index.diagnostics.json")
	openAPI := filepath.Join(buildDir, "openapi.json")
	clientDir := filepath.Join(tmpDir, "client")

	manifest, err := webindex.Run(context.Background(), webindex.IndexOptions{
		Root:                 tmpDir,
		OutPath:              out,
		DiagnosticsPath:      diagnostics,
		OpenAPIPath:          openAPI,
		RouteCompositionPath: filepath.Join("app", "routes.go"),
		Strict:               true,
	})
	if err != nil {
		return err
	}

	cmd.logger.Info().
		Any("operations", len(manifest.Operations)).
		Any("schemas", len(manifest.Schemas)).
		Any("diagnostics", len(manifest.Diagnostics)).
		Any("out", out).
		Any("diagnostics_out", diagnostics).
		Any("openapi_out", openAPI).
		Msg("API index generated")

	if err := cmd.validateSpecFn(cmd.Image, openAPI, cmd.Silent); err != nil {
		return err
	}
	if err := cmd.generateClientFn(cmd.Image, openAPI, clientDir, cmd.Silent); err != nil {
		return err
	}
	if err := cmd.compileClientFn(clientDir, cmd.Silent); err != nil {
		return err
	}

	if !cmd.Silent {
		console.Successf("OpenAPI validation and generated client compilation passed")
	}
	return nil
}

// writeOpenAPIFixture creates a native web.Context app whose returned route groups define the indexed API surface.
func writeOpenAPIFixture(root string, webModuleDir string) error {
	files := map[string]string{
		"go.mod": "module example.com/openapi-fixture\n\ngo 1.25.0\n\nrequire github.com/goforj/web v0.6.0\n\nreplace github.com/goforj/web => " + strconv.Quote(filepath.ToSlash(webModuleDir)) + "\n",
		".env":   "APP_NAME=OpenAPI Validation Fixture\n",
		filepath.Join("internal", "hello", "controller.go"): `package hello

import (
	"net/http"

	"github.com/goforj/web"
)

type payload struct {
	Name string ` + "`json:\"name\"`" + `
}

// Controller owns the fixture endpoints used to exercise request and response inference.
type Controller struct{}

// Routes keeps both fixture contracts in one provider so composition scoping must retain them together.
func (c *Controller) Routes() []web.Route {
	return []web.Route{
		web.NewRoute(http.MethodGet, "/items/:id", c.Get),
		web.NewRoute(http.MethodPost, "/items", c.Create),
	}
}

// Get exercises native path and query projection through web.Context.
func (c *Controller) Get(ctx web.Context) error {
	if ctx.Query("full") == "1" {
		return ctx.JSON(http.StatusOK, map[string]any{"ok": true, "id": ctx.Param("id"), "mode": "full"})
	}
	return ctx.JSON(http.StatusOK, map[string]any{"ok": true, "id": ctx.Param("id")})
}

// Create exercises native request-body projection through web.Context.
func (c *Controller) Create(ctx web.Context) error {
	var in payload
	if err := ctx.Bind(&in); err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]any{"ok": false, "error": "bad payload"})
	}
	return ctx.JSON(http.StatusCreated, map[string]any{"ok": true})
}
`,
		filepath.Join("app", "routes.go"): `package app

import (
	"github.com/goforj/web"

	"example.com/openapi-fixture/internal/hello"
)

// ProvideRoutes is the fixture app boundary consumed by app-scoped API indexing.
func ProvideRoutes(controller *hello.Controller) []web.RouteGroup {
	return []web.RouteGroup{
		web.NewRouteGroup("", controller.Routes()),
	}
}
`,
	}

	for rel, content := range files {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// resolveOpenAPIFixtureWebModule reuses the exact web module selected by the current GoForj build so strict type loading cannot drift.
func resolveOpenAPIFixtureWebModule() (string, error) {
	command := exec.Command("go", "list", "-f", "{{.Module.Dir}}", "github.com/goforj/web")
	command.Env = append(os.Environ(), "GOWORK=off")
	var stderr strings.Builder
	// Cold-cache download progress belongs on stderr so stdout remains a usable filesystem path.
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("resolve web module for OpenAPI fixture: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	moduleDir := strings.TrimSpace(string(output))
	if moduleDir == "" {
		return "", fmt.Errorf("resolve web module for OpenAPI fixture: Go returned an empty module directory")
	}
	info, err := os.Stat(moduleDir)
	if err != nil {
		return "", fmt.Errorf("inspect web module for OpenAPI fixture %q: %w", moduleDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("web module for OpenAPI fixture %q is not a directory", moduleDir)
	}
	return moduleDir, nil
}

// validateWithDocker runs the CLI validator in a disposable container before generation can consume the spec.
func (cmd *TestOpenAPICmd) validateWithDocker(image, openAPIPath string, silent bool) error {
	if _, err := os.Stat(openAPIPath); err != nil {
		return fmt.Errorf("openapi output missing: %w", err)
	}

	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not available: %w", err)
	}

	containerName := fmt.Sprintf("forj-openapi-validate-%d-%d", time.Now().UnixNano(), rand.Intn(10000))
	defer func() {
		_ = runQuietDocker([]string{"rm", "-f", containerName})
	}()

	const containerSpecPath = "/tmp/openapi.json"
	createArgs := []string{"create", "--name", containerName, image, "validate", "-i", containerSpecPath}
	if err := runDockerStep(createArgs, silent); err != nil {
		return fmt.Errorf("openapi validation failed: %w", err)
	}

	if err := runDockerStep([]string{"cp", openAPIPath, containerName + ":" + containerSpecPath}, silent); err != nil {
		return fmt.Errorf("openapi validation failed: %w", err)
	}

	if err := startDockerContainer(containerName, silent); err != nil {
		return fmt.Errorf("openapi validation failed: %w", err)
	}
	return nil
}

// generateClientWithDocker runs Go client generation separately from validation and copies its output into the command workspace.
func (cmd *TestOpenAPICmd) generateClientWithDocker(image, openAPIPath, clientDir string, silent bool) error {
	if _, err := os.Stat(openAPIPath); err != nil {
		return fmt.Errorf("openapi output missing: %w", err)
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not available: %w", err)
	}
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		return fmt.Errorf("create generated client directory: %w", err)
	}

	containerName := fmt.Sprintf("forj-openapi-client-%d-%d", time.Now().UnixNano(), rand.Intn(10000))
	defer func() {
		_ = runQuietDocker([]string{"rm", "-f", containerName})
	}()

	const containerSpecPath = "/tmp/openapi.json"
	const containerClientPath = "/tmp/client"
	createArgs := []string{
		"create", "--name", containerName, image,
		"generate", "-i", containerSpecPath, "-g", "go", "-o", containerClientPath,
		"--additional-properties=packageName=openapiclient,withGoMod=true",
	}
	if err := runDockerStep(createArgs, silent); err != nil {
		return fmt.Errorf("openapi client generation failed: %w", err)
	}
	if err := runDockerStep([]string{"cp", openAPIPath, containerName + ":" + containerSpecPath}, silent); err != nil {
		return fmt.Errorf("openapi client generation failed: %w", err)
	}
	if err := startDockerContainer(containerName, silent); err != nil {
		return fmt.Errorf("openapi client generation failed: %w", err)
	}
	if err := runDockerStep([]string{"cp", containerName + ":" + containerClientPath + "/.", clientDir}, silent); err != nil {
		return fmt.Errorf("openapi client generation failed: %w", err)
	}
	return nil
}

// compileGeneratedClient type-checks and compiles the generated package without depending on generator-owned sample tests.
func (cmd *TestOpenAPICmd) compileGeneratedClient(clientDir string, silent bool) error {
	if _, err := os.Stat(filepath.Join(clientDir, "go.mod")); err != nil {
		return fmt.Errorf("generated client module missing: %w", err)
	}

	goTest := execx.Command("go", "test", "-run", "^$", "-count=1", ".").
		Dir(clientDir).
		EnvAppend(map[string]string{
			"GOCACHE":    "/tmp/gocache",
			"GOMODCACHE": "/tmp/gomodcache",
			"GOFLAGS":    "",
			"GOWORK":     "off",
		})
	if !silent {
		goTest = goTest.StdoutWriter(os.Stdout).StderrWriter(os.Stderr)
		goTest = goTest.ShadowPrint(
			execx.WithFormatter(func(ev execx.ShadowEvent) string {
				switch ev.Phase {
				case execx.ShadowBefore:
					return fmt.Sprintf("%s %s", console.ActionMark(), ev.Command)
				case execx.ShadowAfter:
					return fmt.Sprintf("%s %s (%s)", console.InfoMark(), ev.Command, ev.Duration)
				default:
					return fmt.Sprintf("%s %s", console.InfoMark(), ev.Command)
				}
			}),
		)
	}
	res, err := goTest.Run()
	if err != nil || !res.OK() {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" && err != nil {
			msg = err.Error()
		}
		if msg == "" {
			msg = fmt.Sprintf("exit code %d", res.ExitCode)
		}
		return fmt.Errorf("generated client compilation failed: %s", msg)
	}
	return nil
}

// startDockerContainer attaches to a prepared container so CLI failures retain their captured output.
func startDockerContainer(containerName string, silent bool) error {
	startCmd := execx.Command("docker", "start", "-a", containerName)
	if !silent {
		startCmd = startCmd.StdoutWriter(os.Stdout).StderrWriter(os.Stderr)
		startCmd = startCmd.ShadowPrint(
			execx.WithFormatter(func(ev execx.ShadowEvent) string {
				switch ev.Phase {
				case execx.ShadowBefore:
					return fmt.Sprintf("%s %s", console.ActionMark(), ev.Command)
				case execx.ShadowAfter:
					return fmt.Sprintf("%s %s (%s)", console.InfoMark(), ev.Command, ev.Duration)
				default:
					return fmt.Sprintf("%s %s", console.InfoMark(), ev.Command)
				}
			}),
		)
	}
	res, err := startCmd.Run()
	if err != nil || !res.OK() {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" && err != nil {
			msg = err.Error()
		}
		if msg == "" {
			msg = fmt.Sprintf("exit code %d", res.ExitCode)
		}
		return fmt.Errorf("docker start -a %s: %s", containerName, msg)
	}
	return nil
}

// runDockerStep executes setup and copy operations while preserving actionable CLI output.
func runDockerStep(args []string, silent bool) error {
	c := execx.Command("docker", args...)
	if !silent {
		c = c.StdoutWriter(os.Stdout).StderrWriter(os.Stderr)
		c = c.ShadowPrint(
			execx.WithFormatter(func(ev execx.ShadowEvent) string {
				switch ev.Phase {
				case execx.ShadowBefore:
					return fmt.Sprintf("%s %s", console.ActionMark(), ev.Command)
				case execx.ShadowAfter:
					return fmt.Sprintf("%s %s (%s)", console.InfoMark(), ev.Command, ev.Duration)
				default:
					return fmt.Sprintf("%s %s", console.InfoMark(), ev.Command)
				}
			}),
		)
	}
	res, err := c.Run()
	if err != nil || !res.OK() {
		if err == nil {
			err = fmt.Errorf("exit code %d", res.ExitCode)
		}
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s: %s", strings.Join(append([]string{"docker"}, args...), " "), msg)
	}
	return nil
}

// runQuietDocker performs best-effort cleanup without obscuring the primary workflow result.
func runQuietDocker(args []string) error {
	c := exec.Command("docker", args...)
	c.Stdout = io.Discard
	c.Stderr = io.Discard
	if err := c.Run(); err != nil {
		return err
	}
	return nil
}

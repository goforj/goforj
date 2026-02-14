package forj

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/apix"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
)

// TestOpenAPICmd validates generated OpenAPI against a real validator.
type TestOpenAPICmd struct {
	logger *logger.AppLogger

	Silent bool   `help:"Suppress command output" short:"s"`
	Keep   bool   `help:"Keep the temp workspace after completion" short:"k"`
	Image  string `help:"Validator image" default:"openapitools/openapi-generator-cli:v7.6.0"`

	validateSpecFn func(image, buildDir string, silent bool) error
}

func (*TestOpenAPICmd) Signature() string {
	return `name:"test:openapi" help:"Build and validate generated OpenAPI" hidden:""`
}

// NewTestOpenAPICmd creates a new TestOpenAPICmd.
func NewTestOpenAPICmd(logger *logger.AppLogger) *TestOpenAPICmd {
	cmd := &TestOpenAPICmd{
		logger: logger,
	}
	cmd.validateSpecFn = cmd.validateWithDocker
	return cmd
}

// Run executes OpenAPI generation + validation against a fixture app.
func (cmd *TestOpenAPICmd) Run() error {
	tmpDir, err := os.MkdirTemp("", "forj_openapi_validation_")
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

	if err := writeOpenAPIFixture(tmpDir); err != nil {
		return err
	}

	buildDir := filepath.Join(tmpDir, "build")
	out := filepath.Join(buildDir, "api_index.json")
	diagnostics := filepath.Join(buildDir, "api_index.diagnostics.json")
	openAPI := filepath.Join(buildDir, "openapi.json")

	manifest, err := apix.Run(context.Background(), apix.IndexOptions{
		Root:            tmpDir,
		OutPath:         out,
		DiagnosticsPath: diagnostics,
		OpenAPIPath:     openAPI,
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

	if err := cmd.validateSpecFn(cmd.Image, buildDir, cmd.Silent); err != nil {
		return err
	}

	if !cmd.Silent {
		console.Successf("OpenAPI validation passed")
	}
	return nil
}

func writeOpenAPIFixture(root string) error {
	files := map[string]string{
		"go.mod": "module example.com/openapi-fixture\n\ngo 1.24\n",
		".env":   "APP_NAME=OpenAPI Validation Fixture\n",
		filepath.Join("internal", "hello", "controller.go"): `package hello

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type payload struct {
	Name string ` + "`json:\"name\"`" + `
}

type Controller struct{}

func (c *Controller) Routes() []any {
	return []any{
		http.NewRoute(http.MethodGet, "/items/:id", c.Get),
		http.NewRoute(http.MethodPost, "/items", c.Create),
	}
}

func (c *Controller) Get(ctx echo.Context) error {
	if ctx.QueryParam("full") == "1" {
		return ctx.JSON(http.StatusOK, map[string]any{"ok": true, "id": ctx.Param("id"), "mode": "full"})
	}
	return ctx.JSON(http.StatusOK, map[string]any{"ok": true, "id": ctx.Param("id")})
}

func (c *Controller) Create(ctx echo.Context) error {
	var in payload
	if err := ctx.Bind(&in); err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]any{"ok": false, "error": "bad payload"})
	}
	return ctx.JSON(http.StatusCreated, map[string]any{"ok": true})
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

func (cmd *TestOpenAPICmd) validateWithDocker(image, buildDir string, silent bool) error {
	absBuildDir, err := filepath.Abs(buildDir)
	if err != nil {
		return err
	}

	dockerCmd := execx.Command(
		"docker", "run", "--rm",
		"-v", absBuildDir+":/work",
		image,
		"validate", "-i", "/work/openapi.json",
	)

	if !silent {
		dockerCmd = dockerCmd.StdoutWriter(os.Stdout).StderrWriter(os.Stderr)
		dockerCmd = dockerCmd.ShadowPrint(
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

	res, err := dockerCmd.Run()
	if err != nil || !res.OK() {
		if err != nil {
			msg := strings.TrimSpace(res.Stderr)
			if msg == "" {
				msg = strings.TrimSpace(res.Stdout)
			}
			if msg == "" {
				msg = err.Error()
			}
			return fmt.Errorf("openapi validation failed: %s", msg)
		}
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if msg == "" {
			msg = fmt.Sprintf("exit code %d", res.ExitCode)
		}
		return fmt.Errorf("openapi validation failed: %s", msg)
	}
	return nil
}

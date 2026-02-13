package forj

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/goforj/goforj/internal/logger"
)

type MakeControllerCmd struct {
	Name      string `arg:"" help:"Name of the controller (e.g. Hello)"`
	OutputDir string `short:"d" help:"Directory to write the controller file to" default:"./internal/"`

	logger *logger.AppLogger
}

func (*MakeControllerCmd) Signature() string {
	return `name:"make:controller" help:"Generate a new controller"`
}

func NewMakeControllerCmd(logger *logger.AppLogger) *MakeControllerCmd {
	return &MakeControllerCmd{logger: logger}
}

func (c *MakeControllerCmd) Run() error {
	name := strings.TrimSuffix(c.Name, "Controller")
	packageDir := filepath.Join("internal", strings.ToLower(name))
	outputPath := filepath.Join(packageDir, "controller.go")

	if err := writeControllerFile(name, outputPath); err != nil {
		return err
	}

	if err := c.injectIntoInjectHttp(name, packageDir); err != nil {
		return err
	}

	if err := c.injectIntoAppRoutes(name, packageDir); err != nil {
		return err
	}

	c.logger.Info().
		Any("controller", name).
		Any("path", outputPath).
		Msg("Controller file created")

	return nil
}

func writeControllerFile(name, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	mod, err := getGoModuleName()
	if err != nil {
		return err
	}

	tmpl := template.Must(template.New("controller").Parse(controllerTemplate))
	data := map[string]string{
		"Package":    filepath.Base(filepath.Dir(path)),
		"ModulePath": mod,
		"StructName": "Controller",
		"RoutePath":  "/" + strings.ToLower(name),
		"LogText":    fmt.Sprintf("Hello from %s controller", strings.ToLower(name)),
	}
	if err := tmpl.Execute(f, data); err != nil {
		return err
	}

	return formatGoFile(path)
}

const controllerTemplate = `package {{ .Package }}

import (
	"github.com/labstack/echo/v4"
	"{{ .ModulePath }}/internal/http"
	"{{ .ModulePath }}/internal/logger"
)

type Controller struct {
	logger *logger.AppLogger
}

func NewController(logger *logger.AppLogger) *Controller {
	return &Controller{logger: logger}
}

func (c *Controller) Routes() []http.Route {
	return []http.Route{
		http.NewRoute(http.MethodGet, "{{ .RoutePath }}", c.Get),
	}
}

func (c *Controller) Get(e echo.Context) error {
	c.logger.Info().Msg("{{ .LogText }}")
	return e.String(http.StatusOK, "{{ .LogText }}")
}`

func (c *MakeControllerCmd) injectIntoInjectHttp(name, outputDir string) error {
	mod, err := getGoModuleName()
	if err != nil {
		return err
	}

	packageName := filepath.Base(outputDir)
	importPath := fmt.Sprintf("%s/%s", mod, filepath.ToSlash(outputDir))
	injectPath := "./wire/inject_http_controllers.go"

	data, err := os.ReadFile(injectPath)
	if err != nil {
		return fmt.Errorf("reading %s.go: %w", injectPath, err)
	}

	lines := strings.Split(string(data), "\n")
	importStmt := fmt.Sprintf("\t\"%s\"", importPath)
	constructor := fmt.Sprintf("%s.NewController", packageName)

	// Inject import if not present
	hasImport := false
	for _, line := range lines {
		if strings.Contains(line, importPath) {
			hasImport = true
			break
		}
	}
	if !hasImport {
		for i, line := range lines {
			if strings.HasPrefix(line, "import (") {
				lines = append(lines[:i+1], append([]string{importStmt}, lines[i+1:]...)...)
				break
			}
		}
	}

	// Add constructor to wire.NewSet
	if !strings.Contains(string(data), constructor) {
		for i, line := range lines {
			if strings.Contains(line, "var httpAppControllerSet = wire.NewSet(") {
				lines[i+1] = fmt.Sprintf("\t%s,", constructor) + "\n" + lines[i+1]
				break
			}
		}
	}

	formatted, err := format.Source([]byte(strings.Join(lines, "\n")))
	if err != nil {
		return fmt.Errorf("gofmt error: %w", err)
	}

	c.logger.Info().Any("injectPath", injectPath).Msgf("Injecting controller into %s", injectPath)

	return os.WriteFile(injectPath, formatted, 0644)
}

func (c *MakeControllerCmd) injectIntoAppRoutes(name string, outputDir string) error {
	mod, err := getGoModuleName()
	if err != nil {
		return err
	}

	packageName := filepath.Base(outputDir)
	importPath := fmt.Sprintf("%s/%s", mod, filepath.ToSlash(outputDir))
	injectPath := "./internal/router/app_routes.go"

	data, err := os.ReadFile(injectPath)
	if err != nil {
		return fmt.Errorf("reading %s.go: %w", injectPath, err)
	}

	lines := strings.Split(string(data), "\n")
	importStmt := fmt.Sprintf("\t\"%s\"", importPath)
	paramName := strings.ToLower(packageName) + "Controller"
	paramDecl := fmt.Sprintf("\t%s *%s.Controller,", paramName, packageName)
	appendLine := fmt.Sprintf("\troutes = append(routes, %s.Routes()...)", paramName)

	// Inject import if not present
	hasImport := false
	for _, line := range lines {
		if strings.Contains(line, importPath) {
			hasImport = true
			break
		}
	}
	if !hasImport {
		for i, line := range lines {
			if strings.HasPrefix(line, "import (") {
				lines = append(lines[:i+1], append([]string{importStmt}, lines[i+1:]...)...)
				break
			}
		}
	}

	// Add to provideAppRoutes param list
	alreadyInParams := strings.Contains(string(data), paramDecl)
	if !alreadyInParams {
		for i, line := range lines {
			if strings.HasPrefix(line, "func ProvideAppRoutes(") {
				lines = append(lines[:i+1], append([]string{paramDecl}, lines[i+1:]...)...)
				break
			}
		}
	}

	// Add route registration after routes declaration
	for i, line := range lines {
		if strings.Contains(line, "var routes []http.Route") {
			// Only add if it doesn’t already exist
			if !strings.Contains(string(data), appendLine) {
				lines = append(lines[:i+1], append([]string{appendLine}, lines[i+1:]...)...)
			}
			break
		}
	}

	formatted, err := format.Source([]byte(strings.Join(lines, "\n")))
	if err != nil {
		return fmt.Errorf("gofmt error: %w", err)
	}

	c.logger.Info().Any("injectPath", injectPath).Msgf("Injecting controller into %s", injectPath)

	return os.WriteFile(injectPath, formatted, 0644)
}

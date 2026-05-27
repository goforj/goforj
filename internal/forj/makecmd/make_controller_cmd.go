package makecmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/str"
)

// ControllerCmd generates an HTTP controller and wires it into the app.
type ControllerCmd struct {
	Name      string `arg:"" help:"Name of the controller (e.g. Hello)"`
	OutputDir string `short:"d" help:"Directory to write the controller file to. Grouped names default to their owning package path." default:"./internal"`
}

const defaultControllerOutputDir = "./internal"

// Signature returns the Kong metadata for the make:controller generator.
func (*ControllerCmd) Signature() string {
	return `name:"make:controller" help:"Generate a new controller"`
}

// NewControllerCmd creates the make:controller generator command.
func NewControllerCmd() *ControllerCmd {
	return &ControllerCmd{}
}

// Run generates the controller file and updates HTTP wiring.
func (c *ControllerCmd) Run() error {
	rawName := str.Of(c.Name).TrimSpace().ChopEnd("Controller").String()
	nameParts := commandPackagePartsFromName(str.Of(rawName).Split(":"))
	if len(nameParts) == 0 {
		nameParts = []string{"controller"}
	}

	packageDir := c.OutputDir
	if isDefaultControllerOutputDir(c.OutputDir) {
		packageDir = filepath.Join(append([]string{"internal"}, nameParts...)...)
	}
	outputPath := filepath.Join(packageDir, "controller.go")
	routePath := "/" + strings.Join(nameParts, "/")

	if err := writeControllerFile(rawName, outputPath, routePath); err != nil {
		return err
	}

	if err := c.injectIntoInjectHttp(rawName, packageDir); err != nil {
		return err
	}

	if err := c.injectIntoAppRoutes(rawName, packageDir); err != nil {
		return err
	}

	console.Successf("Generated controller file: %s", outputPath)

	return nil
}

// writeControllerFile renders the controller implementation into its package.
func writeControllerFile(name, path, routePath string) error {
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
		"RoutePath":  routePath,
		"LogText":    fmt.Sprintf("Hello from %s controller", str.Of(name).Snake(" ").String()),
	}
	if err := tmpl.Execute(f, data); err != nil {
		return err
	}

	return formatGoFile(path)
}

const controllerTemplate = `package {{ .Package }}

import (
	"github.com/goforj/web"
	"{{ .ModulePath }}/internal/logger"
	"net/http"
)

type Controller struct {
	logger *logger.AppLogger
}

// NewController creates a controller for this package.
func NewController(logger *logger.AppLogger) *Controller {
	return &Controller{logger: logger}
}

// Routes returns the HTTP routes handled by this controller.
func (c *Controller) Routes() []web.Route {
	return []web.Route{
		web.NewRoute(http.MethodGet, "{{ .RoutePath }}", c.Get),
	}
}

// Get handles GET requests for this controller.
func (c *Controller) Get(r web.Context) error {
	c.logger.Info().Msg("{{ .LogText }}")
	return r.Text(http.StatusOK, "{{ .LogText }}")
}`

// injectIntoInjectHttp registers the controller provider with the HTTP controller wire set.
func (c *ControllerCmd) injectIntoInjectHttp(name, outputDir string) error {
	mod, err := getGoModuleName()
	if err != nil {
		return err
	}

	packageName := commandPackageName(outputDir)
	packageRef := commandPackageRef(outputDir)
	importPath := fmt.Sprintf("%s/%s", mod, filepath.ToSlash(outputDir))
	injectPath := "./wire/inject_http_controllers.go"

	lines, _, err := readGeneratorGoFile(injectPath)
	if err != nil {
		return fmt.Errorf("reading %s.go: %w", injectPath, err)
	}

	constructor := fmt.Sprintf("%s.NewController", packageRef)
	constructorLine := fmt.Sprintf("\t%s,", constructor)

	lines = insertImportIfMissing(lines, importPath, importAliasForPackageRef(packageName, packageRef))
	lines = insertIntoCallBlock(lines, "var httpAppControllerSet = wire.NewSet(", constructorLine)

	if err := writeGeneratorGoLines(injectPath, lines); err != nil {
		return err
	}
	console.Successf("Injected into %s: %s", injectPath, constructor)
	return nil
}

// injectIntoAppRoutes registers the controller routes with the application route registry.
func (c *ControllerCmd) injectIntoAppRoutes(name string, outputDir string) error {
	mod, err := getGoModuleName()
	if err != nil {
		return err
	}

	packageName := commandPackageName(outputDir)
	packageRef := commandPackageRef(outputDir)
	importPath := fmt.Sprintf("%s/%s", mod, filepath.ToSlash(outputDir))
	injectPath := "./internal/router/routes_registry.go"

	lines, _, err := readGeneratorGoFile(injectPath)
	if err != nil {
		return fmt.Errorf("reading %s.go: %w", injectPath, err)
	}

	paramName := str.Of(packageRef).Camel().String() + "Controller"
	paramDecl := fmt.Sprintf("\t%s *%s.Controller,", paramName, packageRef)
	appendLine := fmt.Sprintf("\tpublicRoutes = append(publicRoutes, %s.Routes()...)", paramName)

	lines = insertImportIfMissing(lines, importPath, importAliasForPackageRef(packageName, packageRef))

	// Add to provideAppRoutes param list
	if !containsLine(lines, paramDecl) {
		lines = insertIntoFuncParams(lines, "ProvideAppRoutes", paramDecl)
	}

	// Add route registration after public routes declaration
	lines = insertAfterMarkerIfMissing(lines, "var publicRoutes []web.Route", appendLine)

	if err := writeGeneratorGoLines(injectPath, lines); err != nil {
		return err
	}
	console.Successf("Injected into %s: %s", injectPath, paramName)
	return nil
}

// isDefaultControllerOutputDir reports whether the user left the controller output at its default.
func isDefaultControllerOutputDir(outputDir string) bool {
	return filepath.Clean(outputDir) == filepath.Clean(defaultControllerOutputDir)
}

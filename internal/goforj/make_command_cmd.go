package goforj

import (
	"bytes"
	_ "embed"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/goforj/goforj/internal/logger"
)

type MakeCommandCmd struct {
	Name      string `arg:"" help:"Name of the command (e.g. HelloWorld)"`
	OutputDir string `short:"d" help:"Directory to write the command file to" default:"./internal/cmd"`

	logger *logger.AppLogger
}

func NewMakeCommandCmd(logger *logger.AppLogger) *MakeCommandCmd {
	return &MakeCommandCmd{logger: logger}
}

func (c *MakeCommandCmd) Run() error {
	structName := ensureCmdSuffix(c.Name)
	fileName := snakeCase(structName) + ".go"
	outputPath := filepath.Join(c.OutputDir, fileName)

	// Step 1: Write command file
	if err := c.writeCommandFile(structName, outputPath); err != nil {
		return err
	}

	// Step 2: Inject into wire/inject_cmd.go
	if err := c.injectIntoWireFile(structName); err != nil {
		return err
	}

	// Step 3: Inject into RootCmd
	if err := c.injectIntoRootCmd(structName); err != nil {
		return err
	}

	return nil
}

func (c *MakeCommandCmd) writeCommandFile(structName, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), os.ModePerm); err != nil {
		return err
	}

	moduleName, err := getGoModuleName()
	if err != nil {
		return err
	}

	// Step 1: Render into memory
	var buf bytes.Buffer
	tmpl := template.Must(template.New("cmd").Parse(commandTemplate))
	err = tmpl.Execute(&buf, map[string]string{
		"StructName":  structName,
		"ModulePath":  moduleName,
		"PackageName": strings.ToLower(filepath.Base(c.OutputDir)),
	})
	if err != nil {
		return err
	}

	// Step 2: Format
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("gofmt error: %w", err)
	}

	// Step 3: Write
	if err := os.WriteFile(outputPath, formatted, 0644); err != nil {
		return err
	}

	c.logger.Info().Str("path", outputPath).Msg("Generated command file")
	return nil
}

func (c *MakeCommandCmd) injectIntoWireFile(structName string) error {
	injectPath := "./internal/cmd/wire.go"

	moduleName, err := getGoModuleName()
	if err != nil {
		return err
	}

	packageAlias := filepath.Base(c.OutputDir)
	relPath := strings.TrimPrefix(filepath.ToSlash(c.OutputDir), "./")
	importPath := fmt.Sprintf("%s/%s", moduleName, relPath)
	constructor := fmt.Sprintf("New%s", structName)
	if packageAlias != "cmd" {
		constructor = fmt.Sprintf("%s.New%s", packageAlias, structName)
	}

	data, err := os.ReadFile(injectPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", injectPath, err)
	}
	content := string(data)

	if packageAlias != "cmd" && !strings.Contains(content, importPath) {
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.HasPrefix(line, "import (") {
				lines = append(lines[:i+1], append([]string{fmt.Sprintf("\t%q", importPath)}, lines[i+1:]...)...)
				break
			}
		}
		content = strings.Join(lines, "\n")
	}

	if !strings.Contains(content, constructor) {
		content = strings.Replace(
			content,
			"var AppCommandSet = wire.NewSet(\n",
			fmt.Sprintf("var AppCommandSet = wire.NewSet(\n\t%s,\n", constructor),
			1,
		)
	}

	if err := os.WriteFile(injectPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", injectPath, err)
	}

	c.logger.Info().
		Str("constructor", constructor).
		Str("import", importPath).
		Msg("Injected into " + injectPath)

	return nil
}

func (c *MakeCommandCmd) injectIntoRootCmd(structName string) error {
	rootPath := "./internal/cmd/app_commands.go"
	moduleName, err := getGoModuleName()
	if err != nil {
		return fmt.Errorf("getGoModuleName: %w", err)
	}

	data, err := os.ReadFile(rootPath)
	if err != nil {
		return fmt.Errorf("reading root_cmd.go: %w", err)
	}

	lines := strings.Split(string(data), "\n")

	if strings.Contains(string(data), structName) {
		c.logger.Info().Msgf("%s already exists in root_cmd.go", structName)
		return nil
	}

	// Names
	outputPkg := strings.ToLower(filepath.Base(c.OutputDir))
	rootPkg := "cmd"
	usePrefix := outputPkg != rootPkg

	// Prefix field with package name if different
	pkgPrefix := ""
	if usePrefix {
		pkgPrefix = strings.Title(outputPkg) // admin → Admin
	}

	fieldName := pkgPrefix + structName
	paramName := strings.ToLower(pkgPrefix) + structName
	typeName := structName

	// Handle imports if needed
	if usePrefix {
		relPath := strings.TrimPrefix(filepath.ToSlash(c.OutputDir), "./")
		importPath := fmt.Sprintf("%s/%s", moduleName, relPath)
		lines = insertImportIfMissing(lines, importPath)
	}

	// Inject field into RootCmd
	var fieldType string
	if outputPkg != rootPkg {
		fieldType = fmt.Sprintf("%s.%s", outputPkg, typeName)
	} else {
		fieldType = typeName
	}
	fieldLine := fmt.Sprintf("\t%s %s `cmd:\"\" name:\"%s\" help:\"\"`", fieldName, fieldType, strings.ToLower(strings.TrimSuffix(structName, "Cmd"))+":cmd")
	if !containsLine(lines, fieldLine) {
		lines = insertBeforeClosingBrace(lines, "type AppCommands struct {", fieldLine)
	}

	// Inject param into NewRootCmd
	var paramLine string
	if usePrefix {
		paramLine = fmt.Sprintf("\t%s *%s.%s,", paramName, outputPkg, typeName)
	} else {
		paramLine = fmt.Sprintf("\t%s *%s,", paramName, typeName)
	}
	if !containsLine(lines, paramLine) {
		lines = insertIntoFuncParams(lines, "NewAppCommands", paramLine)
	}

	// Inject assignment into return &RootCmd{}
	returnLine := fmt.Sprintf("\t\t%s: *%s,", fieldName, paramName)
	if !containsLine(lines, returnLine) {
		lines = insertBeforeClosingBrace(lines, "return &AppCommands{", returnLine)
	}

	// Format and write
	formatted, err := format.Source([]byte(strings.Join(lines, "\n")))
	if err != nil {
		return fmt.Errorf("gofmt error: %w", err)
	}
	if err := os.WriteFile(rootPath, formatted, 0644); err != nil {
		return fmt.Errorf("writing root_cmd.go: %w", err)
	}

	c.logger.Info().
		Str("field", fieldName).
		Str("param", paramName).
		Msg("Injected into AppCommands successfully")

	fmt.Printf(
		"\n✅ Don't forget to update your command signature in ./internal/cmd/app_commands.go:\n\n"+
			" %s %s `cmd:\"\" name:\"%s\" help:\"Your command help here\"`\n",
		fieldName,
		fieldType,
		strings.ToLower(strings.TrimSuffix(structName, "Cmd"))+":cmd",
	)

	return nil
}

// -- Template --

//go:embed make_command.tmpl
var commandTemplate string

func insertImportIfMissing(lines []string, importPath string) []string {
	for i, line := range lines {
		if strings.HasPrefix(line, "import (") {
			for j := i + 1; j < len(lines); j++ {
				if lines[j] == ")" {
					// Check if already present
					for _, imp := range lines[i+1 : j] {
						if strings.Contains(imp, importPath) {
							return lines
						}
					}
					lines = append(lines[:j], append([]string{fmt.Sprintf("\t%q", importPath)}, lines[j:]...)...)
					break
				}
			}
			break
		}
	}
	return lines
}

package makecmd

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/str"
)

// CommandCmd generates an application CLI command and wires it into the app.
type CommandCmd struct {
	Name                 string            `arg:"" help:"Name of the command (e.g. HelloWorld)"`
	OutputDir            string            `short:"d" help:"Directory to write the command file to. Grouped names default to their owning package path." default:"./internal/cmd"`
	CmdName              string            `name:"name" short:"n" aliases:"signature" help:"Override the command signature name (e.g. hello:world)"`
	ReservedCommandNames CommandNameOwners `kong:"-"`
}

// defaultCommandOutputDir is the fallback package for ungrouped commands.
const defaultCommandOutputDir = "./internal/cmd"

// Signature returns CLI metadata for the make:command generator.
func (*CommandCmd) Signature() string {
	return `name:"make:command" help:"Generate a new CLI command"`
}

// NewCommandCmd creates the make:command generator command.
func NewCommandCmd() *CommandCmd {
	return &CommandCmd{}
}

// Run generates the command file and updates the command wiring.
func (c *CommandCmd) Run() error {
	rawName := str.Of(c.Name).TrimSpace().String()
	structBase := rawName
	parts := str.Of(rawName).Split(":")
	if len(parts) > 1 {
		action := str.Of(parts[len(parts)-1]).TrimSpace().String()
		if action != "" && isDefaultCommandOutputDir(c.OutputDir) {
			if packageParts := commandPackagePartsFromName(parts[:len(parts)-1]); len(packageParts) > 0 {
				c.OutputDir = filepath.Join(append([]string{".", "internal"}, packageParts...)...)
			}
		}
		structBase = action
	}
	structBase = str.Of(structBase).Pascal().String()
	structName := ensureCmdSuffix(structBase)
	fileName := str.Of(structName).Snake("_").String() + ".go"
	outputPath := filepath.Join(c.OutputDir, fileName)

	commandName := str.Of(c.CmdName).TrimSpace().String()
	if commandName == "" {
		commandName = rawName
		if commandName == "" {
			base := str.Of(structName).ChopEnd("Cmd").String()
			commandName = str.Of(base).ToLower().String() + ":cmd"
		}
	}
	if err := c.validateCommandNameAvailable(commandName); err != nil {
		return err
	}
	helpText := str.Of(structName).ChopEnd("Cmd").String() + " command"

	// Step 1: Write command file
	if err := c.writeCommandFile(structName, outputPath, commandName, helpText); err != nil {
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

// writeCommandFile renders the command implementation into its owning package.
func (c *CommandCmd) writeCommandFile(structName, outputPath, commandName, helpText string) error {
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
		"PackageName": commandPackageName(c.OutputDir),
		"CommandName": commandName,
		"HelpText":    helpText,
		"AliasClause": "",
		"GroupClause": "",
	})
	if err != nil {
		return err
	}

	if err := writeGeneratorGoFile(outputPath, buf.Bytes()); err != nil {
		return err
	}

	console.Successf("Generated command file: %s", outputPath)
	return nil
}

// injectIntoWireFile registers the command constructor with the app command wire set.
func (c *CommandCmd) injectIntoWireFile(structName string) error {
	injectPath := "./internal/cmd/wire.go"

	moduleName, err := getGoModuleName()
	if err != nil {
		return err
	}

	packageName := commandPackageName(c.OutputDir)
	packageRef := commandPackageRef(c.OutputDir)
	relPath := strings.TrimPrefix(filepath.ToSlash(c.OutputDir), "./")
	importPath := fmt.Sprintf("%s/%s", moduleName, relPath)
	constructor := fmt.Sprintf("New%s", structName)
	if !isRootCommandOutputDir(c.OutputDir) {
		constructor = fmt.Sprintf("%s.New%s", packageRef, structName)
	}

	data, err := os.ReadFile(injectPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", injectPath, err)
	}
	content := string(data)

	if !isRootCommandOutputDir(c.OutputDir) && !strings.Contains(content, importPath) {
		lines := strings.Split(content, "\n")
		lines = insertImportIfMissing(lines, importPath, importAliasForPackageRef(packageName, packageRef))
		content = strings.Join(lines, "\n")
	}

	if !strings.Contains(content, constructor) {
		lines := strings.Split(content, "\n")
		lines = insertIntoCallBlock(lines, "var AppCommandSet = wire.NewSet(", fmt.Sprintf("\t%s,", constructor))
		content = strings.Join(lines, "\n")
	}
	if err := writeGeneratorGoFile(injectPath, []byte(content)); err != nil {
		return fmt.Errorf("writing %s: %w", injectPath, err)
	}

	console.Successf("Injected into %s: %s", injectPath, constructor)

	return nil
}

// injectIntoRootCmd registers the command on AppCommands so Kong can expose it.
func (c *CommandCmd) injectIntoRootCmd(structName string) error {
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

	// Names
	outputPkg := commandPackageName(c.OutputDir)
	packageRef := commandPackageRef(c.OutputDir)
	usePrefix := !isRootCommandOutputDir(c.OutputDir)

	// Prefix field with package name if different
	pkgPrefix := ""
	if usePrefix {
		pkgPrefix = str.Of(packageRef).UcFirst().String()
	}

	fieldName := pkgPrefix + structName
	paramName := str.Of(pkgPrefix).Camel().String() + structName
	typeName := structName

	// Handle imports if needed
	if usePrefix {
		relPath := strings.TrimPrefix(filepath.ToSlash(c.OutputDir), "./")
		importPath := fmt.Sprintf("%s/%s", moduleName, relPath)
		lines = insertImportIfMissing(lines, importPath, importAliasForPackageRef(outputPkg, packageRef))
	}

	// Inject field into RootCmd
	var fieldType string
	if usePrefix {
		fieldType = fmt.Sprintf("%s.%s", packageRef, typeName)
	} else {
		fieldType = typeName
	}
	fieldLine := fmt.Sprintf("\t%s %s `cmd:\"\"`", fieldName, fieldType)
	fieldExists := containsLine(lines, fieldLine)
	if !fieldExists {
		lines = insertBeforeClosingBrace(lines, "type AppCommands struct {", fieldLine)
	}

	// Inject param into NewRootCmd
	var paramLine string
	if usePrefix {
		paramLine = fmt.Sprintf("\t%s *%s.%s,", paramName, packageRef, typeName)
	} else {
		paramLine = fmt.Sprintf("\t%s *%s,", paramName, typeName)
	}
	paramExists := containsLine(lines, paramLine)
	if !paramExists {
		lines = insertIntoFuncParams(lines, "NewAppCommands", paramLine)
	}

	// Inject assignment into return &RootCmd{}
	returnLine := fmt.Sprintf("\t\t%s: *%s,", fieldName, paramName)
	returnExists := containsLine(lines, returnLine)
	if !returnExists {
		lines = insertBeforeClosingBrace(lines, "return &AppCommands{", returnLine)
	}

	if fieldExists && paramExists && returnExists {
		console.Infof("%s already exists in %s", fieldName, rootPath)
		return nil
	}

	lines = normalizeImports(lines)

	if err := writeGeneratorGoLines(rootPath, lines); err != nil {
		return fmt.Errorf("writing root_cmd.go: %w", err)
	}

	console.Successf("Injected into AppCommands: %s", fieldName)

	return nil
}

// commandTemplate contains the generated command implementation template.
//
//go:embed make_command.tmpl
var commandTemplate string

// commandPackageName returns the Go package name for a command output directory.
func commandPackageName(outputDir string) string {
	name := str.Of(filepath.Base(outputDir)).Snake("_").String()
	if name == "" {
		return "cmd"
	}
	return name
}

// isRootCommandOutputDir reports whether outputDir points at the CLI wiring package.
func isRootCommandOutputDir(outputDir string) bool {
	return strings.TrimPrefix(filepath.ToSlash(filepath.Clean(outputDir)), "./") == "internal/cmd"
}

// isDefaultCommandOutputDir reports whether the user left the command output at its default.
func isDefaultCommandOutputDir(outputDir string) bool {
	return filepath.Clean(outputDir) == filepath.Clean(defaultCommandOutputDir)
}

// commandPackagePartsFromName converts command name groups into package path segments.
func commandPackagePartsFromName(parts []string) []string {
	packageParts := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := str.Of(part).TrimSpace().Snake("_").String()
		if clean == "" {
			continue
		}
		packageParts = append(packageParts, clean)
	}
	return packageParts
}

// commandPackageRef returns the import reference used for a command package.
func commandPackageRef(outputDir string) string {
	rel := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(outputDir)), "./")
	parts := strings.Split(rel, "/")
	if len(parts) > 1 && parts[0] == "internal" {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return commandPackageName(outputDir)
	}

	var b strings.Builder
	for i, part := range parts {
		clean := str.Of(part).Snake("_").String()
		if clean == "" {
			continue
		}
		if i == 0 {
			b.WriteString(str.Of(clean).Camel().String())
			continue
		}
		b.WriteString(str.Of(clean).Pascal().String())
	}
	if b.Len() == 0 {
		return commandPackageName(outputDir)
	}
	return b.String()
}

// importAliasForPackageRef returns an import alias when the package ref differs from the package name.
func importAliasForPackageRef(packageName, packageRef string) string {
	if packageName == packageRef {
		return ""
	}
	return packageRef
}

package forj

import (
	"bytes"
	_ "embed"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/str"
)

type MakeCommandCmd struct {
	Name      string `arg:"" help:"Name of the command (e.g. HelloWorld)"`
	OutputDir string `short:"d" help:"Directory to write the command file to" default:"./internal/cmd"`
	CmdName   string `name:"name" short:"n" aliases:"signature" help:"Override the command signature name (e.g. hello:world)"`

	logger *logger.AppLogger
}

func (*MakeCommandCmd) Signature() string {
	return `name:"make:command" help:"Generate a new CLI command"`
}

func NewMakeCommandCmd(logger *logger.AppLogger) *MakeCommandCmd {
	return &MakeCommandCmd{logger: logger}
}

func (c *MakeCommandCmd) Run() error {
	rawName := str.Of(c.Name).TrimSpace().String()
	structBase := rawName
	parts := str.Of(rawName).Split(":")
	if len(parts) > 1 {
		group := str.Of(parts[0]).TrimSpace().String()
		action := str.Of(parts[len(parts)-1]).TrimSpace().String()
		if group != "" && action != "" && filepath.Clean(c.OutputDir) == "internal/cmd" {
			groupDir := str.Of(group).Snake("_").String()
			c.OutputDir = filepath.Join(".", "internal", groupDir)
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

func (c *MakeCommandCmd) writeCommandFile(structName, outputPath, commandName, helpText string) error {
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
		"PackageName": str.Of(filepath.Base(c.OutputDir)).ToLower().String(),
		"CommandName": commandName,
		"HelpText":    helpText,
		"AliasClause": "",
		"GroupClause": "",
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

	console.Successf("Generated command file: %s", outputPath)
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

	console.Successf("Injected into %s: %s", injectPath, constructor)

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

	// Names
	outputPkg := strings.ToLower(filepath.Base(c.OutputDir))
	rootPkg := "cmd"
	usePrefix := outputPkg != rootPkg

	// Prefix field with package name if different
	pkgPrefix := ""
	if usePrefix {
		pkgPrefix = str.Of(outputPkg).UcFirst().String() // admin → Admin
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
	fieldLine := fmt.Sprintf("\t%s %s `cmd:\"\"`", fieldName, fieldType)
	fieldExists := containsLine(lines, fieldLine)
	if !fieldExists {
		lines = insertBeforeClosingBrace(lines, "type AppCommands struct {", fieldLine)
	}

	// Inject param into NewRootCmd
	var paramLine string
	if usePrefix {
		paramLine = fmt.Sprintf("\t%s *%s.%s,", paramName, outputPkg, typeName)
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

	// Format and write
	formatted, err := format.Source([]byte(strings.Join(lines, "\n")))
	if err != nil {
		return fmt.Errorf("gofmt error: %w", err)
	}
	if err := os.WriteFile(rootPath, formatted, 0644); err != nil {
		return fmt.Errorf("writing root_cmd.go: %w", err)
	}

	console.Successf("Injected into AppCommands: %s", fieldName)

	return nil
}

// -- Template --

//go:embed make_command.tmpl
var commandTemplate string

func insertImportIfMissing(lines []string, importPath string) []string {
	hasImport := false
	for i, line := range lines {
		if strings.HasPrefix(line, "import (") {
			hasImport = true
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
	if hasImport {
		return normalizeImports(lines)
	}

	var importLines []int
	var imports []string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import ") {
			importLines = append(importLines, i)
			path := strings.TrimSpace(strings.TrimPrefix(trimmed, "import"))
			path = strings.Trim(path, "\"")
			imports = append(imports, path)
		}
	}
	if len(importLines) > 0 {
		for _, imp := range imports {
			if imp == importPath {
				return normalizeImports(lines)
			}
		}
		imports = append(imports, importPath)
		block := []string{"import ("}
		for _, imp := range imports {
			block = append(block, fmt.Sprintf("\t%q", imp))
		}
		block = append(block, ")")

		start := importLines[0]
		lines = append(lines[:start], append(block, lines[start+1:]...)...)
		for i := len(importLines) - 1; i > 0; i-- {
			idx := importLines[i]
			lines = append(lines[:idx], lines[idx+1:]...)
		}
		return normalizeImports(lines)
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "package ") {
			insert := []string{"", fmt.Sprintf("import %q", importPath), ""}
			lines = append(lines[:i+1], append(insert, lines[i+1:]...)...)
			return normalizeImports(lines)
		}
	}
	return lines
}

func normalizeImports(lines []string) []string {
	var blockStart int = -1
	var blockEnd int = -1
	var imports []string
	var singleImportLines []int
	seen := make(map[string]struct{})

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if blockStart == -1 && strings.HasPrefix(trimmed, "import (") {
			blockStart = i
			continue
		}
		if blockStart != -1 && blockEnd == -1 {
			if strings.TrimSpace(line) == ")" {
				blockEnd = i
				continue
			}
			path := strings.TrimSpace(line)
			path = strings.Trim(path, "\"")
			path = strings.TrimPrefix(path, "\t")
			path = strings.Trim(path, "\"")
			if path != "" && path != ")" {
				if _, ok := seen[path]; !ok {
					seen[path] = struct{}{}
					imports = append(imports, path)
				}
			}
			continue
		}

		if strings.HasPrefix(trimmed, "import ") && !strings.HasPrefix(trimmed, "import (") {
			singleImportLines = append(singleImportLines, i)
			path := strings.TrimSpace(strings.TrimPrefix(trimmed, "import"))
			path = strings.Trim(path, "\"")
			if path != "" {
				if _, ok := seen[path]; !ok {
					seen[path] = struct{}{}
					imports = append(imports, path)
				}
			}
		}
	}

	if blockStart != -1 {
		// Remove any single-line imports if a block exists.
		for i := len(singleImportLines) - 1; i >= 0; i-- {
			idx := singleImportLines[i]
			lines = append(lines[:idx], lines[idx+1:]...)
		}
		// Rebuild the block with de-duped imports.
		block := []string{"import ("}
		for _, imp := range imports {
			block = append(block, fmt.Sprintf("\t%q", imp))
		}
		block = append(block, ")")
		lines = append(lines[:blockStart], append(block, lines[blockEnd+1:]...)...)
		return lines
	}

	if len(singleImportLines) <= 1 {
		return lines
	}

	block := []string{"import ("}
	for _, imp := range imports {
		block = append(block, fmt.Sprintf("\t%q", imp))
	}
	block = append(block, ")")

	start := singleImportLines[0]
	lines = append(lines[:start], append(block, lines[start+1:]...)...)
	for i := len(singleImportLines) - 1; i > 0; i-- {
		idx := singleImportLines[i]
		lines = append(lines[:idx], lines[idx+1:]...)
	}
	return lines
}

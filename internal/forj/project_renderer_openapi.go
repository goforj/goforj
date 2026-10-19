package forj

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"

	"github.com/goforj/goforj/project"
)

// renderOpenAPI limits an upgrade to documentation handlers, their tests, and the existing server registration.
func renderOpenAPI(root string) error {
	workspace, err := resolveProjectRenderWorkspace(root)
	if err != nil {
		return err
	}
	config, err := workspace.loadProjectConfig()
	if err != nil {
		return err
	}
	components := project.ProjectComponents(config)
	if !components.WebAPI && !components.WebUI {
		return fmt.Errorf("OpenAPI rendering requires an existing HTTP App")
	}
	if config.GoModuleName == "" {
		return fmt.Errorf("OpenAPI rendering requires module_name in .goforj.yml")
	}
	if err := validateOpenAPIUpgradePaths(workspace); err != nil {
		return err
	}
	const serverPath = "internal/http/server.go"
	server, err := workspace.readFile(serverPath)
	if err != nil {
		return err
	}
	updatedServer, err := patchOpenAPIRouteRegistration(server)
	if err != nil {
		return err
	}

	// Prepare both templates before publication so unsupported source cannot leave a partial upgrade.
	data := struct {
		GoModuleName      string
		ProjectComponents project.Components
	}{config.GoModuleName, components}
	paths := []string{"internal/http/openapi.go", "internal/http/openapi_test.go", serverPath}
	contents := [][]byte{nil, nil, updatedServer}
	for index, path := range paths[:2] {
		source, err := templatesFS.ReadFile(path + ".tmpl")
		if err != nil {
			return err
		}
		tmpl, err := parseProjectTemplate(path+".tmpl", source)
		if err != nil {
			return err
		}
		var output bytes.Buffer
		if err := tmpl.Execute(&output, data); err != nil {
			return err
		}
		contents[index], err = maybeFormatGoSource(path, output.Bytes())
		if err != nil {
			return err
		}
	}
	for index, path := range paths {
		existing, err := workspace.readFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if bytes.Equal(existing, contents[index]) {
			continue
		}
		if err := workspace.writeFileAtomically(path, contents[index], 0o644); err != nil {
			return err
		}
	}
	for _, path := range []string{"internal/http/swagger.go", "internal/http/swagger_test.go"} {
		if _, err := workspace.removeFileIfExists(path); err != nil {
			return err
		}
	}
	return nil
}

// validateOpenAPIUpgradePaths prevents a scoped upgrade from following generated-file symlinks or replacing unrelated directories.
func validateOpenAPIUpgradePaths(workspace projectRenderWorkspace) error {
	for _, path := range []string{"internal", "internal/http"} {
		info, err := os.Lstat(workspace.path(filepath.FromSlash(path)))
		if err != nil {
			return workspace.logicalError(err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s must be an existing directory for an OpenAPI upgrade", path)
		}
	}
	for _, path := range []string{"internal/http/server.go", "internal/http/swagger.go", "internal/http/swagger_test.go", "internal/http/openapi.go", "internal/http/openapi_test.go"} {
		info, err := os.Lstat(workspace.path(filepath.FromSlash(path)))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return workspace.logicalError(err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s must be a regular file for an OpenAPI upgrade", path)
		}
	}
	return nil
}

// patchOpenAPIRouteRegistration changes only a recognized Server method call, preserving owner comments and unrelated wiring byte-for-byte.
func patchOpenAPIRouteRegistration(source []byte) ([]byte, error) {
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, "internal/http/server.go", source, 0)
	if err != nil {
		return nil, fmt.Errorf("parse OpenAPI server wiring: %w", err)
	}
	var registrations []*ast.Ident
	for _, declaration := range file.Decls {
		method, ok := declaration.(*ast.FuncDecl)
		if !ok || method.Body == nil || method.Recv == nil || len(method.Recv.List) != 1 || len(method.Recv.List[0].Names) != 1 {
			continue
		}
		receiver := method.Recv.List[0]
		pointer, ok := receiver.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		typeName, ok := pointer.X.(*ast.Ident)
		if !ok || typeName.Name != "Server" {
			continue
		}
		ast.Inspect(method.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) != 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "swaggerRoutes" && selector.Sel.Name != "openapiRoutes" {
				return true
			}
			owner, ok := selector.X.(*ast.Ident)
			if ok && owner.Name == receiver.Names[0].Name {
				registrations = append(registrations, selector.Sel)
			}
			return true
		})
	}
	if len(registrations) != 1 {
		return nil, fmt.Errorf("internal/http/server.go must contain exactly one Server documentation route registration; found %d", len(registrations))
	}
	name := registrations[0]
	if name.Name == "openapiRoutes" {
		return source, nil
	}
	start, end := positions.Position(name.Pos()).Offset, positions.Position(name.End()).Offset
	updated := append([]byte(nil), source[:start]...)
	updated = append(updated, "openapiRoutes"...)
	return append(updated, source[end:]...), nil
}

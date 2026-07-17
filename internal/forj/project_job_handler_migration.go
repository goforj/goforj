package forj

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"strconv"
	"strings"

	"github.com/goforj/goforj/project"
	"github.com/goforj/str/v2"
)

// legacyJobProvider describes one constructor whose generated job convention exposes a matching handler and type name.
type legacyJobProvider struct {
	packageName string
	typeName    string
}

// legacyJobRegistrationPlan keeps the providers guaranteed to implement the legacy generated-job contract explicit during migration.
type legacyJobRegistrationPlan struct {
	providers        []legacyJobProvider
	queuePackageName string
}

// syncLegacyJobHandlerRegistration adds the stable registration seam missing from older App-owned Jobs injectors.
func syncLegacyJobHandlerRegistration(content string, moduleName string, components project.Components) (string, error) {
	if !components.Jobs || strings.TrimSpace(moduleName) == "" {
		return content, nil
	}
	if strings.Contains(content, "func registerJobHandlers(") {
		if strings.Contains(content, "registerJobHandlers,") {
			return content, nil
		}
		updated := insertIntoWireSet(content, "appJobSet", "\tregisterJobHandlers,")
		if updated == content {
			return content, fmt.Errorf("cannot add registerJobHandlers to appJobSet")
		}
		return updated, nil
	}

	plan, err := legacyJobRegistrationPlanFor(content, moduleName)
	if err != nil {
		return content, err
	}
	updated := ensureGoImport(content, moduleName+"/internal/queues", "")
	updated = insertIntoWireSet(updated, "appJobSet", "\tregisterJobHandlers,")
	if !strings.Contains(updated, "registerJobHandlers,") {
		return content, fmt.Errorf("cannot add registerJobHandlers to appJobSet")
	}
	updated += legacyJobHandlerRegistrationSource(plan, !strings.Contains(updated, "type jobHandlerRegistration struct"))
	return updated, nil
}

// legacyJobRegistrationPlanFor recognizes framework jobs and the stable contract emitted by the legacy make:job generator.
func legacyJobRegistrationPlanFor(content string, moduleName string) (legacyJobRegistrationPlan, error) {
	plan := legacyJobRegistrationPlan{queuePackageName: "queues"}
	file, err := parser.ParseFile(token.NewFileSet(), "inject_jobs_app.go", content, 0)
	if err != nil {
		return plan, fmt.Errorf("parse App-owned Jobs injector: %w", err)
	}
	imports := make(map[string]string, len(file.Imports))
	queueImportPath := moduleName + "/internal/queues"
	for _, imported := range file.Imports {
		importPath, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			continue
		}
		packageName := path.Base(importPath)
		if imported.Name != nil {
			packageName = imported.Name.Name
		}
		if importPath == queueImportPath {
			if packageName == "_" || packageName == "." {
				return plan, fmt.Errorf("App-owned Jobs injector imports %s as %q; use a named import before rerendering", queueImportPath, packageName)
			}
			plan.queuePackageName = packageName
		}
		if packageName != "_" && packageName != "." {
			imports[packageName] = importPath
		}
	}
	seenProviders := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		value, ok := node.(*ast.ValueSpec)
		if !ok || !valueSpecNames(value, "appJobSet") {
			return true
		}
		for _, expression := range value.Values {
			call, ok := expression.(*ast.CallExpr)
			if !ok {
				continue
			}
			for _, argument := range call.Args {
				selector, ok := argument.(*ast.SelectorExpr)
				if !ok || selector.Sel == nil {
					continue
				}
				packageName, ok := selector.X.(*ast.Ident)
				constructor := selector.Sel.Name
				if !ok {
					continue
				}
				importPath := imports[packageName.Name]
				typeName, ok := legacyGeneratedJobTypeName(importPath, moduleName, constructor)
				if !ok {
					continue
				}
				providerKey := packageName.Name + "." + typeName
				if seenProviders[providerKey] {
					continue
				}
				plan.providers = append(plan.providers, legacyJobProvider{
					packageName: packageName.Name,
					typeName:    typeName,
				})
				seenProviders[providerKey] = true
			}
		}
		return false
	})
	return plan, nil
}

// legacyGeneratedJobTypeName restores only the module-local constructor contract written by the old make:job generator.
func legacyGeneratedJobTypeName(importPath string, moduleName string, constructor string) (string, bool) {
	if importPath == "" || (importPath != moduleName && !strings.HasPrefix(importPath, moduleName+"/")) {
		return "", false
	}
	typeName := strings.TrimPrefix(constructor, "New")
	if typeName == constructor || typeName == "" || !strings.HasSuffix(typeName, "Job") {
		return "", false
	}
	return typeName, true
}

// valueSpecNames reports whether a declaration assigns the requested package-level name.
func valueSpecNames(value *ast.ValueSpec, name string) bool {
	for _, identifier := range value.Names {
		if identifier.Name == name {
			return true
		}
	}
	return false
}

// legacyJobHandlerRegistrationSource renders the smallest additive seam needed to consume and register existing job providers.
func legacyJobHandlerRegistrationSource(plan legacyJobRegistrationPlan, includeType bool) string {
	var source strings.Builder
	if includeType {
		source.WriteString("\n// jobHandlerRegistration makes handler registration a prerequisite of App construction.\n")
		source.WriteString("type jobHandlerRegistration struct{}\n")
	}
	source.WriteString("\n")
	source.WriteString("// registerJobHandlers binds every application job to each configured queue runtime.\n")
	source.WriteString("func registerJobHandlers(\n\tqueueManager *" + plan.queuePackageName + ".Manager,\n")
	for _, provider := range plan.providers {
		parameter := str.Of(provider.packageName).Camel().String() + provider.typeName
		source.WriteString("\t" + parameter + " *" + provider.packageName + "." + provider.typeName + ",\n")
	}
	source.WriteString(") *jobHandlerRegistration {\n")
	for _, provider := range plan.providers {
		parameter := str.Of(provider.packageName).Camel().String() + provider.typeName
		source.WriteString("\tqueueManager.Register(" + provider.packageName + "." + provider.typeName + "TypeName, " + parameter + ".HandleTask)\n")
	}
	if len(plan.providers) == 0 {
		source.WriteString("\t_ = queueManager\n")
	}
	source.WriteString("\treturn &jobHandlerRegistration{}\n}\n")
	return source.String()
}

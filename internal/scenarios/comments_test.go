package scenarios

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestScenarioGoFilesDocumentDeclarations keeps published examples aligned with GoForj's commenting standard.
func TestScenarioGoFilesDocumentDeclarations(t *testing.T) {
	specs, err := List("")
	if err != nil {
		t.Fatalf("load specs: %v", err)
	}

	for _, spec := range specs {
		for _, step := range spec.Steps {
			if step.Write == nil || filepath.Ext(step.Write.Path) != ".go" {
				continue
			}

			t.Run(spec.ID+"/"+step.Write.Path, func(t *testing.T) {
				source := expandScenarioText(spec, step.Write.Content)
				fileSet := token.NewFileSet()
				file, err := parser.ParseFile(fileSet, step.Write.Path, source, parser.ParseComments)
				if err != nil {
					t.Fatalf("parse runnable example: %v", err)
				}
				assertScenarioDeclarationComments(t, fileSet, file)
			})
		}
	}
}

// assertScenarioDeclarationComments checks the declarations readers are expected to copy and maintain.
func assertScenarioDeclarationComments(t *testing.T, fileSet *token.FileSet, file *ast.File) {
	t.Helper()
	assertScenarioComment(t, fileSet, "Package "+file.Name.Name, file.Doc, file.Package)
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			assertScenarioComment(t, fileSet, declaration.Name.Name, declaration.Doc, declaration.Pos())
		case *ast.GenDecl:
			assertScenarioGeneralDeclarationComments(t, fileSet, declaration)
		}
	}
}

// assertScenarioGeneralDeclarationComments documents every type while value checks follow Go's exported declaration contract.
func assertScenarioGeneralDeclarationComments(t *testing.T, fileSet *token.FileSet, declaration *ast.GenDecl) {
	t.Helper()
	for _, specification := range declaration.Specs {
		switch specification := specification.(type) {
		case *ast.TypeSpec:
			assertScenarioComment(t, fileSet, specification.Name.Name, firstScenarioComment(specification.Doc, declaration.Doc), specification.Pos())
			assertScenarioInterfaceMethodComments(t, fileSet, specification)
		case *ast.ValueSpec:
			for _, name := range specification.Names {
				if name.IsExported() {
					assertScenarioComment(t, fileSet, name.Name, firstScenarioComment(specification.Doc, declaration.Doc), name.Pos())
				}
			}
		}
	}
}

// assertScenarioInterfaceMethodComments covers declared methods that the AST represents as fields rather than functions.
func assertScenarioInterfaceMethodComments(t *testing.T, fileSet *token.FileSet, specification *ast.TypeSpec) {
	t.Helper()
	value, ok := specification.Type.(*ast.InterfaceType)
	if !ok {
		return
	}
	for _, field := range value.Methods.List {
		for _, name := range field.Names {
			assertScenarioComment(t, fileSet, name.Name, field.Doc, name.Pos())
		}
	}
}

// firstScenarioComment gives declaration-specific documentation precedence over a shared declaration comment.
func firstScenarioComment(primary, fallback *ast.CommentGroup) *ast.CommentGroup {
	if primary != nil {
		return primary
	}
	return fallback
}

// assertScenarioComment reports the exact source location so spec authors can fix the published example directly.
func assertScenarioComment(t *testing.T, fileSet *token.FileSet, name string, comment *ast.CommentGroup, position token.Pos) {
	t.Helper()
	location := fileSet.Position(position)
	if comment == nil {
		t.Errorf("%s: %s is missing a doc comment", location, name)
		return
	}
	text := strings.TrimSpace(comment.Text())
	if text == "" {
		t.Errorf("%s: comment for %s is empty", location, name)
		return
	}
	if !strings.HasPrefix(text, name+" ") {
		t.Errorf("%s: comment for %s must start with its name", location, name)
	}
	if !strings.Contains(".!?", text[len(text)-1:]) {
		t.Errorf("%s: comment for %s must be a complete sentence", location, name)
	}
}

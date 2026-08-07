package codequality

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestGoDeclarationsHaveDocumentation keeps maintained Go declarations aligned with the repository's documentation standard.
func TestGoDeclarationsHaveDocumentation(t *testing.T) {
	root := repositoryRoot(t)
	packages := map[string]packageDocumentation{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if ignoredSourceDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
		if err != nil {
			t.Errorf("parse %s: %v", path, err)
			return nil
		}
		if ast.IsGenerated(file) {
			return nil
		}
		assertFileDeclarationComments(t, fileSet, file)
		key := filepath.Dir(path) + "\x00" + file.Name.Name
		state := packages[key]
		state.name = file.Name.Name
		state.position = fileSet.Position(file.Package)
		state.documented = state.documented || packageCommentMatches(file)
		packages[key] = state
		return nil
	})
	if err != nil {
		t.Fatalf("walk repository Go files: %v", err)
	}
	for _, state := range packages {
		if !state.documented {
			t.Errorf("%s: package %s needs a package comment", state.position, state.name)
		}
	}
}

// packageDocumentation tracks whether one maintained source file documents its package.
type packageDocumentation struct {
	name       string
	position   token.Position
	documented bool
}

// repositoryRoot anchors the audit independently of the package test working directory.
func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve comment-audit source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

// ignoredSourceDirectory prevents dependencies and repository metadata from becoming part of the project-owned source contract.
func ignoredSourceDirectory(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor":
		return true
	default:
		return false
	}
}

// packageCommentMatches accepts documentation only when it identifies the package explicitly.
func packageCommentMatches(file *ast.File) bool {
	if file.Doc == nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(file.Doc.Text()), "Package "+file.Name.Name+" ")
}

// assertFileDeclarationComments covers functions, exported values and types, and interface methods represented as fields.
func assertFileDeclarationComments(t *testing.T, fileSet *token.FileSet, file *ast.File) {
	t.Helper()
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			if isGoTestEntrypoint(fileSet.Position(declaration.Pos()).Filename, declaration.Name.Name) {
				continue
			}
			assertDeclarationComment(t, fileSet, declaration.Name.Name, declaration.Doc, declaration.Pos())
		case *ast.GenDecl:
			assertGeneralDeclarationComments(t, fileSet, declaration)
		}
	}
}

// isGoTestEntrypoint exempts self-describing functions that the testing package discovers by convention.
func isGoTestEntrypoint(filename string, name string) bool {
	if !strings.HasSuffix(filename, "_test.go") {
		return false
	}
	for _, prefix := range []string{"Test", "Benchmark", "Fuzz", "Example"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// assertGeneralDeclarationComments applies the exported declaration contract without requiring comments on private state holders.
func assertGeneralDeclarationComments(t *testing.T, fileSet *token.FileSet, declaration *ast.GenDecl) {
	t.Helper()
	for _, specification := range declaration.Specs {
		switch specification := specification.(type) {
		case *ast.TypeSpec:
			if specification.Name.IsExported() {
				assertDeclarationComment(t, fileSet, specification.Name.Name, preferredComment(specification.Doc, declaration.Doc), specification.Pos())
			}
			assertInterfaceMethodComments(t, fileSet, specification)
		case *ast.ValueSpec:
			for _, name := range specification.Names {
				if name.IsExported() {
					assertDeclarationComment(t, fileSet, name.Name, preferredComment(specification.Doc, declaration.Doc), specification.Pos())
					break
				}
			}
		}
	}
}

// assertInterfaceMethodComments includes named interface methods because the Go AST does not expose them as function declarations.
func assertInterfaceMethodComments(t *testing.T, fileSet *token.FileSet, specification *ast.TypeSpec) {
	t.Helper()
	value, ok := specification.Type.(*ast.InterfaceType)
	if !ok {
		return
	}
	for _, field := range value.Methods.List {
		for _, name := range field.Names {
			assertDeclarationComment(t, fileSet, name.Name, field.Doc, name.Pos())
		}
	}
}

// preferredComment lets declarations inside grouped blocks provide more precise documentation than the group heading.
func preferredComment(primary, fallback *ast.CommentGroup) *ast.CommentGroup {
	if primary != nil {
		return primary
	}
	return fallback
}

// assertDeclarationComment reports actionable source locations for missing, generic, or incomplete documentation.
func assertDeclarationComment(t *testing.T, fileSet *token.FileSet, name string, comment *ast.CommentGroup, position token.Pos) {
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

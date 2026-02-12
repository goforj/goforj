package apix

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
)

// IndexOptions controls API index generation behavior.
type IndexOptions struct {
	Root            string
	OutPath         string
	DiagnosticsPath string
	OpenAPIPath     string
}

type parsedFile struct {
	Path        string
	PackageName string
	File        *ast.File
}

// Run indexes API metadata from source and writes artifacts.
func Run(_ context.Context, opts IndexOptions) (Manifest, error) {
	root := opts.Root
	if root == "" {
		root = "."
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Manifest{}, err
	}

	parsed, fset, err := parseGoFilesWithSet(root)
	if err != nil {
		return Manifest{}, err
	}

	routes, handlers, prefixes, mapping := discoverRoutesAndHandlers(fset, parsed)
	ops, diagnostics := normalize(routes, handlers, prefixes, mapping)
	manifest := Manifest{
		Version:     ManifestVersion,
		Operations:  ops,
		Schemas:     collectSchemas(ops),
		Diagnostics: diagnostics,
	}

	if err := writeJSON(opts.OutPath, manifest); err != nil {
		return Manifest{}, fmt.Errorf("write manifest: %w", err)
	}
	if err := writeJSON(opts.DiagnosticsPath, diagnostics); err != nil {
		return Manifest{}, fmt.Errorf("write diagnostics: %w", err)
	}
	if opts.OpenAPIPath != "" {
		if err := writeJSON(opts.OpenAPIPath, toOpenAPI(manifest)); err != nil {
			return Manifest{}, fmt.Errorf("write openapi: %w", err)
		}
	}

	return manifest, nil
}

func parseGoFilesWithSet(root string) ([]*parsedFile, *token.FileSet, error) {
	fset := token.NewFileSet()
	parsed := make([]*parsedFile, 0, 128)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		base := d.Name()
		if d.IsDir() {
			switch base {
			case ".git", "vendor", "node_modules", ".cache", "tmp":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(base, ".go") || strings.HasSuffix(base, "_test.go") {
			return nil
		}
		if strings.Contains(filepath.ToSlash(path), "/templates/") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return nil
		}
		parsed = append(parsed, &parsedFile{
			Path:        path,
			PackageName: file.Name.Name,
			File:        file,
		})
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return parsed, fset, nil
}

func normalizeMethodExpr(expr string) string {
	s := strings.TrimSpace(expr)
	if strings.HasPrefix(s, "http.Method") {
		s = strings.TrimPrefix(s, "http.Method")
		return strings.ToLower(s)
	}
	switch strings.ToUpper(s) {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD":
		return strings.ToLower(s)
	default:
		return strings.ToLower(strings.Trim(s, `"`))
	}
}

func methodNameFromHandlerExpr(expr string) string {
	e := strings.TrimSpace(expr)
	parts := strings.Split(e, ".")
	return parts[len(parts)-1]
}

func typeNameFromExpr(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.StarExpr:
		return strings.TrimPrefix(typeNameFromExpr(e.X), "*")
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		left := typeNameFromExpr(e.X)
		if left == "" {
			return e.Sel.Name
		}
		return left + "." + e.Sel.Name
	case *ast.ArrayType:
		return "[]" + typeNameFromExpr(e.Elt)
	case *ast.MapType:
		return "map[" + typeNameFromExpr(e.Key) + "]" + typeNameFromExpr(e.Value)
	default:
		return exprString(expr)
	}
}

func exprString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	var b strings.Builder
	_ = printer.Fprint(&b, token.NewFileSet(), expr)
	return b.String()
}

func intToString(n int) string { return fmt.Sprintf("%d", n) }

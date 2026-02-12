package apix

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type discoveredRoute struct {
	MethodExpr  string
	Path        string
	HandlerExpr string
	File        string
	Line        int
}

type discoveredHandler struct {
	Package  string
	Receiver string
	Name     string
	File     string
	Line     int
	Decl     *ast.FuncDecl
}

func discoverRoutesAndHandlers(fset *token.FileSet, parsed []*parsedFile) ([]discoveredRoute, []discoveredHandler, []string) {
	var routes []discoveredRoute
	var handlers []discoveredHandler
	groupPrefixes := map[string]struct{}{}

	for _, pf := range parsed {
		for _, decl := range pf.File.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok {
				pos := fset.Position(fn.Pos())
				handlers = append(handlers, discoveredHandler{
					Package:  pf.PackageName,
					Receiver: receiverName(fn),
					Name:     fn.Name.Name,
					File:     filepath.ToSlash(pos.Filename),
					Line:     pos.Line,
					Decl:     fn,
				})
			}
		}

		ast.Inspect(pf.File, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			xIdent, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if xIdent.Name != "http" {
				return true
			}

			switch sel.Sel.Name {
			case "NewRoute":
				if len(call.Args) < 3 {
					return true
				}
				path := extractStringLiteral(call.Args[1])
				if path == "" {
					return true
				}
				pos := fset.Position(call.Pos())
				routes = append(routes, discoveredRoute{
					MethodExpr:  exprString(call.Args[0]),
					Path:        path,
					HandlerExpr: exprString(call.Args[2]),
					File:        filepath.ToSlash(pos.Filename),
					Line:        pos.Line,
				})
			case "NewRouteGroup":
				if len(call.Args) > 0 {
					if prefix := extractStringLiteral(call.Args[0]); prefix != "" {
						groupPrefixes[prefix] = struct{}{}
					}
				}
			}

			return true
		})
	}

	prefixes := make([]string, 0, len(groupPrefixes))
	for p := range groupPrefixes {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)
	return routes, handlers, prefixes
}

func extractStringLiteral(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return s
}

func receiverName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	return typeNameFromExpr(fn.Recv.List[0].Type)
}

func joinPath(prefix, path string) string {
	if prefix == "" {
		return path
	}
	if path == "" {
		return prefix
	}
	p := strings.TrimSuffix(prefix, "/")
	s := path
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	return p + s
}

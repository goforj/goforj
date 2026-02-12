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
	MethodExpr          string
	Path                string
	HandlerExpr         string
	HandlerFunction     string
	HandlerPackageHint  string
	HandlerReceiverHint string
	MiddlewareExprs     []string
	File                string
	Line                int
}

type discoveredHandler struct {
	Package  string
	Receiver string
	Name     string
	File     string
	Line     int
	Decl     *ast.FuncDecl
}

type routerMapping struct {
	PrefixByOwner      map[string]string
	MiddlewareByOwner  map[string][]string
	DefaultMiddlewares []string
}

func discoverRoutesAndHandlers(fset *token.FileSet, parsed []*parsedFile) ([]discoveredRoute, []discoveredHandler, []string, routerMapping) {
	var routes []discoveredRoute
	var handlers []discoveredHandler
	groupPrefixes := map[string]struct{}{}
	mapping := buildRouterMapping(parsed)

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

		for _, decl := range pf.File.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			localTypes := collectFuncVarTypes(fn)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				xIdent, ok := sel.X.(*ast.Ident)
				if !ok || xIdent.Name != "http" {
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
					handlerExpr := exprString(call.Args[2])
					handlerFn := methodNameFromHandlerExpr(handlerExpr)
					hintPkg, hintRecv := inferHandlerHints(call.Args[2], fn, localTypes, pf.PackageName)
					pos := fset.Position(call.Pos())
					routes = append(routes, discoveredRoute{
						MethodExpr:          exprString(call.Args[0]),
						Path:                path,
						HandlerExpr:         handlerExpr,
						HandlerFunction:     handlerFn,
						HandlerPackageHint:  hintPkg,
						HandlerReceiverHint: hintRecv,
						MiddlewareExprs:     middlewareExprs(call.Args[3:]),
						File:                filepath.ToSlash(pos.Filename),
						Line:                pos.Line,
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
	}

	prefixes := make([]string, 0, len(groupPrefixes))
	for p := range groupPrefixes {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)
	return routes, handlers, prefixes, mapping
}

func middlewareExprs(args []ast.Expr) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	for _, arg := range args {
		switch e := arg.(type) {
		case *ast.CallExpr:
			out = append(out, exprString(e.Fun))
		default:
			out = append(out, exprString(arg))
		}
	}
	return out
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

func collectFuncVarTypes(fn *ast.FuncDecl) map[string]string {
	out := map[string]string{}
	if fn.Recv != nil {
		for _, field := range fn.Recv.List {
			t := typeNameFromExpr(field.Type)
			for _, n := range field.Names {
				out[n.Name] = t
			}
		}
	}
	if fn.Type != nil && fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			t := typeNameFromExpr(field.Type)
			for _, n := range field.Names {
				out[n.Name] = t
			}
		}
	}
	return out
}

func inferHandlerHints(handlerExpr ast.Expr, fn *ast.FuncDecl, locals map[string]string, defaultPkg string) (string, string) {
	sel, ok := handlerExpr.(*ast.SelectorExpr)
	if !ok {
		return defaultPkg, receiverName(fn)
	}
	xid, ok := sel.X.(*ast.Ident)
	if !ok {
		return defaultPkg, ""
	}
	typ := locals[xid.Name]
	if typ == "" {
		return defaultPkg, ""
	}
	typ = strings.TrimPrefix(typ, "*")
	parts := strings.Split(typ, ".")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return defaultPkg, typ
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

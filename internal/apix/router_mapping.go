package apix

import "go/ast"

func buildRouterMapping(parsed []*parsedFile) routerMapping {
	fieldToPrefix := map[string]string{}
	ownerToField := map[string]string{}

	for _, pf := range parsed {
		if pf.PackageName != "router" {
			continue
		}
		for _, decl := range pf.File.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			switch fn.Name.Name {
			case "ProvideRoutes":
				parseProvideRoutes(fieldToPrefix, fn)
			case "ProvideAppRoutes":
				parseProvideAppRoutes(ownerToField, fn)
			}
		}
	}

	out := routerMapping{PrefixByOwner: map[string]string{}}
	for owner, field := range ownerToField {
		if prefix, ok := fieldToPrefix[field]; ok {
			out.PrefixByOwner[owner] = prefix
		}
	}
	return out
}

func parseProvideRoutes(fieldToPrefix map[string]string, fn *ast.FuncDecl) {
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		xid, ok := sel.X.(*ast.Ident)
		if !ok || xid.Name != "http" || sel.Sel.Name != "NewRouteGroup" {
			return true
		}
		prefix := extractStringLiteral(call.Args[0])
		if prefix == "" {
			return true
		}
		argSel, ok := call.Args[1].(*ast.SelectorExpr)
		if !ok {
			return true
		}
		field := argSel.Sel.Name
		if field != "" {
			fieldToPrefix[field] = prefix
		}
		return true
	})
}

func parseProvideAppRoutes(ownerToField map[string]string, fn *ast.FuncDecl) {
	paramOwner := map[string]string{}
	if fn.Type != nil && fn.Type.Params != nil {
		for _, p := range fn.Type.Params.List {
			t := typeNameFromExpr(p.Type)
			for _, name := range p.Names {
				if t != "" {
					paramOwner[name.Name] = t
				}
			}
		}
	}

	accOwner := map[string]string{}
	fieldByAccumulator := map[string]string{}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if ok {
			ident, ok := call.Fun.(*ast.Ident)
			if ok && ident.Name == "append" && len(call.Args) >= 2 {
				acc, ok := call.Args[0].(*ast.Ident)
				if ok {
					if owner := ownerFromRoutesArg(call.Args[1], paramOwner); owner != "" {
						accOwner[acc.Name] = owner
					}
				}
			}
		}

		un, ok := n.(*ast.UnaryExpr)
		if ok {
			cl, ok := un.X.(*ast.CompositeLit)
			if ok {
				tname := typeNameFromExpr(cl.Type)
				if tname == "AppRoutes" || tname == "router.AppRoutes" {
					for _, elt := range cl.Elts {
						kv, ok := elt.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						field, ok := kv.Key.(*ast.Ident)
						if !ok {
							continue
						}
						val, ok := kv.Value.(*ast.Ident)
						if !ok {
							continue
						}
						fieldByAccumulator[val.Name] = field.Name
					}
				}
			}
		}
		return true
	})

	for acc, owner := range accOwner {
		if field, ok := fieldByAccumulator[acc]; ok {
			ownerToField[owner] = field
		}
	}
}

func ownerFromRoutesArg(expr ast.Expr, paramOwner map[string]string) string {
	switch e := expr.(type) {
	case *ast.CallExpr:
		sel, ok := e.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Routes" {
			return ""
		}
		controller, ok := sel.X.(*ast.Ident)
		if !ok {
			return ""
		}
		return paramOwner[controller.Name]
	case *ast.SelectorExpr:
		if e.Sel.Name != "Routes" {
			return ""
		}
		controller, ok := e.X.(*ast.Ident)
		if !ok {
			return ""
		}
		return paramOwner[controller.Name]
	default:
		return ""
	}
}

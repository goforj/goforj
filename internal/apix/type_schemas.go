package apix

import (
	"go/ast"
	"go/token"
	"sort"
	"strconv"
	"strings"
)

func buildTypeSchemaIndex(parsed []*parsedFile) map[string]any {
	out := map[string]any{}
	for _, pf := range parsed {
		for _, decl := range pf.File.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				key := typeSchemaKey(pf.PackageName, ts.Name.Name)
				out[key] = structSchema(st)
			}
		}
	}
	return out
}

func typeSchemaKey(pkg, name string) string {
	return pkg + "." + strings.TrimPrefix(name, "*")
}

func structSchema(st *ast.StructType) map[string]any {
	props := map[string]any{}
	required := make([]string, 0)
	if st == nil || st.Fields == nil {
		return map[string]any{"type": "object", "properties": props}
	}
	for _, field := range st.Fields.List {
		if field == nil || len(field.Names) == 0 {
			continue
		}
		schema := schemaFromTypeExpr(field.Type)
		for _, name := range field.Names {
			if name == nil {
				continue
			}
			prop := name.Name
			omitempty := false
			if tagName, omitEmpty, ok := jsonTag(field.Tag); ok {
				if tagName == "-" {
					continue
				}
				if tagName != "" {
					prop = tagName
				}
				omitempty = omitEmpty
			}
			props[prop] = schema
			if !omitempty && !isPointerType(field.Type) {
				required = append(required, prop)
			}
		}
	}
	out := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		sort.Strings(required)
		out["required"] = required
	}
	return out
}

func jsonTag(tag *ast.BasicLit) (name string, omitEmpty bool, ok bool) {
	if tag == nil || tag.Kind != token.STRING {
		return "", false, false
	}
	raw, err := strconv.Unquote(tag.Value)
	if err != nil {
		return "", false, false
	}
	parts := strings.Split(raw, " ")
	for _, part := range parts {
		if !strings.HasPrefix(part, "json:\"") {
			continue
		}
		value := strings.TrimPrefix(part, "json:\"")
		value = strings.TrimSuffix(value, "\"")
		if value == "" {
			return "", false, true
		}
		values := strings.Split(value, ",")
		tagName := values[0]
		for _, opt := range values[1:] {
			if opt == "omitempty" {
				return tagName, true, true
			}
		}
		return tagName, false, true
	}
	return "", false, false
}

func isPointerType(expr ast.Expr) bool {
	_, ok := expr.(*ast.StarExpr)
	return ok
}

func schemaFromTypeExpr(expr ast.Expr) map[string]any {
	switch e := expr.(type) {
	case *ast.Ident:
		switch e.Name {
		case "string":
			return map[string]any{"type": "string"}
		case "bool":
			return map[string]any{"type": "boolean"}
		case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
			return map[string]any{"type": "integer"}
		case "float32", "float64":
			return map[string]any{"type": "number"}
		default:
			return map[string]any{"type": "object", "x-forj-type": e.Name}
		}
	case *ast.ArrayType:
		return map[string]any{
			"type":  "array",
			"items": schemaFromTypeExpr(e.Elt),
		}
	case *ast.MapType:
		return map[string]any{
			"type": "object",
		}
	case *ast.StarExpr:
		return schemaFromTypeExpr(e.X)
	case *ast.SelectorExpr:
		return map[string]any{"type": "object", "x-forj-type": exprString(e)}
	default:
		return map[string]any{"type": "string"}
	}
}

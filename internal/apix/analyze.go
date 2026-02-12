package apix

import (
	"encoding/json"
	"go/ast"
	"go/token"
	"sort"
	"strconv"
	"strings"
)

type analyzedHandler struct {
	PathParams  []Parameter
	QueryParams []Parameter
	Headers     []Parameter
	Body        *BodyShape
	Responses   []ResponseShape
	Dynamic     []analyzedDynamicParam
}

type analyzedDynamicParam struct {
	Kind string
	Expr string
}

func analyzeHandler(fn *ast.FuncDecl) analyzedHandler {
	out := analyzedHandler{}
	if fn == nil || fn.Body == nil {
		return out
	}

	localTypes := collectLocalTypes(fn.Body)
	pathSeen := map[string]struct{}{}
	querySeen := map[string]struct{}{}
	headerSeen := map[string]struct{}{}
	respSeen := map[string]struct{}{}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		switch sel.Sel.Name {
		case "Param":
			if len(call.Args) == 1 {
				if name := extractStringLiteral(call.Args[0]); name != "" {
					if _, ok := pathSeen[name]; !ok {
						pathSeen[name] = struct{}{}
						out.PathParams = append(out.PathParams, Parameter{
							Name:       name,
							In:         "path",
							Required:   true,
							Confidence: "high",
						})
					}
				} else {
					out.Dynamic = append(out.Dynamic, analyzedDynamicParam{
						Kind: "path",
						Expr: exprString(call.Args[0]),
					})
				}
			}
		case "QueryParam":
			if len(call.Args) == 1 {
				if name := extractStringLiteral(call.Args[0]); name != "" {
					if _, ok := querySeen[name]; !ok {
						querySeen[name] = struct{}{}
						out.QueryParams = append(out.QueryParams, Parameter{
							Name:       name,
							In:         "query",
							Required:   false,
							Confidence: "high",
						})
					}
				} else {
					out.Dynamic = append(out.Dynamic, analyzedDynamicParam{
						Kind: "query",
						Expr: exprString(call.Args[0]),
					})
				}
			}
		case "Get":
			if len(call.Args) != 1 {
				return true
			}
			// QueryParams().Get("page")
			if queryCall, ok := sel.X.(*ast.CallExpr); ok {
				if fnSel, ok := queryCall.Fun.(*ast.SelectorExpr); ok && fnSel.Sel.Name == "QueryParams" {
					if name := extractStringLiteral(call.Args[0]); name != "" {
						if _, ok := querySeen[name]; !ok {
							querySeen[name] = struct{}{}
							out.QueryParams = append(out.QueryParams, Parameter{
								Name:       name,
								In:         "query",
								Required:   false,
								Confidence: "medium",
							})
						}
					} else {
						out.Dynamic = append(out.Dynamic, analyzedDynamicParam{
							Kind: "query",
							Expr: exprString(call.Args[0]),
						})
					}
					return true
				}
			}
			// Header.Get("X-Foo")
			if parentSel, ok := sel.X.(*ast.SelectorExpr); ok && parentSel.Sel.Name == "Header" {
				if name := extractStringLiteral(call.Args[0]); name != "" {
					if _, ok := headerSeen[name]; !ok {
						headerSeen[name] = struct{}{}
						out.Headers = append(out.Headers, Parameter{
							Name:       name,
							In:         "header",
							Required:   false,
							Confidence: "medium",
						})
					}
				} else {
					out.Dynamic = append(out.Dynamic, analyzedDynamicParam{
						Kind: "header",
						Expr: exprString(call.Args[0]),
					})
				}
			}
		case "Bind":
			if out.Body == nil && len(call.Args) == 1 {
				if typeName := inferArgTypeName(call.Args[0], localTypes); typeName != "" {
					out.Body = &BodyShape{TypeName: typeName, Source: "c.Bind", Confidence: "high"}
				}
			}
		case "JSON", "String", "NoContent", "XML", "Blob", "HTML":
			status := 0
			if len(call.Args) > 0 {
				status = parseStatusCode(call.Args[0])
			}
				if status > 0 {
					resp := ResponseShape{StatusCode: status, Source: "echo." + sel.Sel.Name, Confidence: "high"}
				if sel.Sel.Name == "JSON" && len(call.Args) > 1 {
					resp.TypeName = inferArgTypeName(call.Args[1], localTypes)
					resp.Schema = inferJSONSchemaExpr(call.Args[1])
					if resp.TypeName == "" {
						resp.Confidence = "medium"
					}
				}
					key := strconv.Itoa(resp.StatusCode) + "|" + resp.TypeName + "|" + resp.Source + "|" + schemaFingerprint(resp.Schema)
					if _, ok := respSeen[key]; !ok {
						respSeen[key] = struct{}{}
						out.Responses = append(out.Responses, resp)
					}
				}
		}
		return true
	})

	sort.Slice(out.PathParams, func(i, j int) bool { return out.PathParams[i].Name < out.PathParams[j].Name })
	sort.Slice(out.QueryParams, func(i, j int) bool { return out.QueryParams[i].Name < out.QueryParams[j].Name })
	sort.Slice(out.Headers, func(i, j int) bool { return out.Headers[i].Name < out.Headers[j].Name })
	sort.Slice(out.Responses, func(i, j int) bool {
		if out.Responses[i].StatusCode == out.Responses[j].StatusCode {
			return out.Responses[i].TypeName < out.Responses[j].TypeName
		}
		return out.Responses[i].StatusCode < out.Responses[j].StatusCode
	})

	return out
}

func collectLocalTypes(body *ast.BlockStmt) map[string]string {
	out := map[string]string{}
	for _, stmt := range body.List {
		switch s := stmt.(type) {
		case *ast.DeclStmt:
			gen, ok := s.Decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				typeName := typeNameFromExpr(vs.Type)
				for idx, name := range vs.Names {
					if name == nil {
						continue
					}
					if typeName != "" {
						out[name.Name] = typeName
						continue
					}
					if idx < len(vs.Values) {
						if inferred := inferExprTypeName(vs.Values[idx], out); inferred != "" {
							out[name.Name] = inferred
						}
					}
				}
			}
		case *ast.AssignStmt:
			if s.Tok != token.DEFINE && s.Tok != token.ASSIGN {
				continue
			}
			for i := range s.Lhs {
				id, ok := s.Lhs[i].(*ast.Ident)
				if !ok || i >= len(s.Rhs) {
					continue
				}
				// Keep an existing variable type on plain assignment to avoid
				// downgrading typed variables into function-call names.
				if s.Tok == token.ASSIGN {
					if _, exists := out[id.Name]; exists {
						continue
					}
				}
				if inferred := inferExprTypeName(s.Rhs[i], out); inferred != "" {
					out[id.Name] = inferred
				}
			}
		}
	}
	return out
}

func parseStatusCode(expr ast.Expr) int {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.INT {
			n, _ := strconv.Atoi(e.Value)
			return n
		}
	case *ast.SelectorExpr:
		if id, ok := e.X.(*ast.Ident); ok && id.Name == "http" && strings.HasPrefix(e.Sel.Name, "Status") {
			return httpStatusMap[e.Sel.Name]
		}
	}
	return 0
}

func inferArgTypeName(expr ast.Expr, locals map[string]string) string {
	switch e := expr.(type) {
	case *ast.UnaryExpr:
		if e.Op == token.AND {
			return inferExprTypeName(e.X, locals)
		}
	}
	return inferExprTypeName(expr, locals)
}

func inferExprTypeName(expr ast.Expr, locals map[string]string) string {
	switch e := expr.(type) {
	case *ast.Ident:
		if t, ok := locals[e.Name]; ok {
			return t
		}
	case *ast.CompositeLit:
		return typeNameFromExpr(e.Type)
	case *ast.CallExpr:
		// new(T)
		if id, ok := e.Fun.(*ast.Ident); ok && id.Name == "new" && len(e.Args) == 1 {
			return typeNameFromExpr(e.Args[0])
		}
		// constructor: pkg.NewX() -> pkg.NewX
		return exprString(e.Fun)
	case *ast.SelectorExpr:
		return exprString(e)
	}
	return ""
}

func inferJSONSchemaExpr(expr ast.Expr) any {
	switch e := expr.(type) {
	case *ast.UnaryExpr:
		if e.Op == token.AND {
			return inferJSONSchemaExpr(e.X)
		}
	case *ast.CompositeLit:
		switch t := e.Type.(type) {
		case *ast.MapType:
			if key, ok := t.Key.(*ast.Ident); ok && key.Name == "string" {
				props := map[string]any{}
				for _, elt := range e.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					name := extractStringLiteral(kv.Key)
					if name == "" {
						continue
					}
					if schema := inferJSONSchemaExpr(kv.Value); schema != nil {
						props[name] = schema
					} else {
						props[name] = map[string]any{"type": "string"}
					}
				}
				return map[string]any{
					"type":       "object",
					"properties": props,
				}
			}
		case *ast.ArrayType:
			item := map[string]any{"type": "string"}
			if len(e.Elts) > 0 {
				if inferred := inferJSONSchemaExpr(e.Elts[0]); inferred != nil {
					if m, ok := inferred.(map[string]any); ok {
						item = m
					}
				}
			}
			return map[string]any{
				"type":  "array",
				"items": item,
			}
		default:
			if len(e.Elts) > 0 {
				props := map[string]any{}
				for _, elt := range e.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					name := extractStructFieldName(kv.Key)
					if name == "" {
						continue
					}
					if schema := inferJSONSchemaExpr(kv.Value); schema != nil {
						props[name] = schema
					} else {
						props[name] = map[string]any{"type": "string"}
					}
				}
				if len(props) > 0 {
					return map[string]any{
						"type":       "object",
						"properties": props,
					}
				}
			}

			typeName := typeNameFromExpr(t)
			if typeName != "" {
				return map[string]any{
					"type":        "object",
					"x-forj-type": typeName,
				}
			}
		}
	case *ast.BasicLit:
		switch e.Kind {
		case token.STRING:
			return map[string]any{"type": "string"}
		case token.INT:
			return map[string]any{"type": "integer"}
		case token.FLOAT:
			return map[string]any{"type": "number"}
		}
	case *ast.Ident:
		switch e.Name {
		case "true", "false":
			return map[string]any{"type": "boolean"}
		case "nil":
			return map[string]any{"nullable": true}
		}
	}
	return nil
}

func extractStructFieldName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	default:
		return ""
	}
}

func schemaFingerprint(schema any) string {
	if schema == nil {
		return ""
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return ""
	}
	return string(data)
}

var httpStatusMap = map[string]int{
	"StatusOK":                            200,
	"StatusCreated":                       201,
	"StatusAccepted":                      202,
	"StatusNoContent":                     204,
	"StatusBadRequest":                    400,
	"StatusUnauthorized":                  401,
	"StatusForbidden":                     403,
	"StatusNotFound":                      404,
	"StatusMethodNotAllowed":              405,
	"StatusConflict":                      409,
	"StatusUnprocessableEntity":           422,
	"StatusTooManyRequests":               429,
	"StatusInternalServerError":           500,
	"StatusNotImplemented":                501,
	"StatusBadGateway":                    502,
	"StatusServiceUnavailable":            503,
	"StatusGatewayTimeout":                504,
	"StatusHTTPVersionNotSupported":       505,
	"StatusNetworkAuthenticationRequired": 511,
}

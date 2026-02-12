package apix

import (
	"fmt"
	"sort"
	"strings"

	"github.com/goforj/str"
)

func normalize(routes []discoveredRoute, handlers []discoveredHandler, prefixes []string, mapping routerMapping, typeSchemas map[string]any) ([]Operation, []Diagnostic) {
	diag := make([]Diagnostic, 0)
	handlerByName := map[string][]discoveredHandler{}
	for _, h := range handlers {
		key := h.Name
		handlerByName[key] = append(handlerByName[key], h)
	}

	effectivePrefix := ""
	if len(prefixes) == 1 {
		effectivePrefix = prefixes[0]
	}

	ops := make([]Operation, 0, len(routes))
	for _, r := range routes {
		method := normalizeMethodExpr(r.MethodExpr)
		path := r.Path
		prefix := routePrefix(r, mapping, effectivePrefix)
		path = joinPath(prefix, r.Path)
		normalizedMethod := str.Of(method).ToUpper().String()
		opID := fmt.Sprintf("%s:%s", normalizedMethod, path)
		handlerFn := r.HandlerFunction
		if handlerFn == "" {
			handlerFn = methodNameFromHandlerExpr(r.HandlerExpr)
		}

		op := Operation{
			ID:     opID,
			Method: normalizedMethod,
			Path:   path,
			Handler: HandlerRef{
				Expression: r.HandlerExpr,
				Function:   handlerFn,
				File:       r.File,
				Line:       r.Line,
			},
			Inputs:  InputShape{},
			Outputs: OutputShape{},
		}
		op.Middleware = mergeMiddlewares(routeMiddlewares(r, mapping), r.MiddlewareExprs)

		candidates := filterHandlerCandidates(handlerByName[handlerFn], r.HandlerPackageHint, r.HandlerReceiverHint)
		if len(candidates) == 0 {
			candidates = handlerByName[handlerFn]
		}
		if len(candidates) == 0 {
			diag = append(diag, Diagnostic{
				Severity:  "warn",
				Code:      "handler_not_found",
				Message:   fmt.Sprintf("unable to resolve handler %q", r.HandlerExpr),
				File:      r.File,
				Line:      r.Line,
				Operation: opID,
			})
			ops = append(ops, op)
			continue
		}
		if len(candidates) > 1 {
			diag = append(diag, Diagnostic{
				Severity:  "warn",
				Code:      "handler_ambiguous",
				Message:   fmt.Sprintf("multiple handlers matched %q, using first", r.HandlerExpr),
				File:      r.File,
				Line:      r.Line,
				Operation: opID,
			})
		}

		h := pickBestCandidate(candidates, r.HandlerPackageHint, r.HandlerReceiverHint)
		op.Handler.Package = h.Package
		op.Handler.Receiver = h.Receiver
		op.Handler.Function = h.Name
		op.Handler.File = h.File
		op.Handler.Line = h.Line

		analyzed := analyzeHandler(h.Decl)
		op.Inputs.PathParams = mergePathParams(extractPathParamsFromRoute(op.Path), analyzed.PathParams)
		op.Inputs.QueryParams = analyzed.QueryParams
		op.Inputs.Headers = analyzed.Headers
		op.Inputs.Body = analyzed.Body
		dynamicSeen := map[string]struct{}{}
		for _, d := range analyzed.Dynamic {
			key := d.Kind + "|" + d.Expr
			if _, exists := dynamicSeen[key]; exists {
				continue
			}
			dynamicSeen[key] = struct{}{}
			diag = append(diag, Diagnostic{
				Severity:  "info",
				Code:      "dynamic_param_key",
				Message:   fmt.Sprintf("dynamic %s parameter key (%s) could not be inferred", d.Kind, d.Expr),
				File:      h.File,
				Line:      h.Line,
				Operation: opID,
			})
		}
		if op.Inputs.Body != nil && op.Inputs.Body.TypeName != "" {
			if schema, ok := resolveTypeSchema(h.Package, op.Inputs.Body.TypeName, typeSchemas); ok {
				op.Inputs.Body.Schema = schema
			}
		}
		op.Outputs.Responses = analyzed.Responses

		ops = append(ops, op)
	}

	sort.Slice(ops, func(i, j int) bool {
		if ops[i].Path == ops[j].Path {
			return ops[i].Method < ops[j].Method
		}
		return ops[i].Path < ops[j].Path
	})

	return ops, diag
}

func routePrefix(r discoveredRoute, mapping routerMapping, fallback string) string {
	if r.HandlerPackageHint != "" && r.HandlerReceiverHint != "" {
		key := r.HandlerPackageHint + "." + r.HandlerReceiverHint
		if p, ok := mapping.PrefixByOwner[key]; ok {
			return p
		}
	}
	if fallback != "" {
		return fallback
	}
	return ""
}

func routeMiddlewares(r discoveredRoute, mapping routerMapping) []string {
	if r.HandlerPackageHint != "" && r.HandlerReceiverHint != "" {
		key := r.HandlerPackageHint + "." + r.HandlerReceiverHint
		if mws, ok := mapping.MiddlewareByOwner[key]; ok {
			return append([]string(nil), mws...)
		}
	}
	if len(mapping.DefaultMiddlewares) > 0 {
		return append([]string(nil), mapping.DefaultMiddlewares...)
	}
	return nil
}

func mergeMiddlewares(groupMws, routeMws []string) []string {
	if len(groupMws) == 0 && len(routeMws) == 0 {
		return nil
	}
	out := make([]string, 0, len(groupMws)+len(routeMws))
	seen := map[string]struct{}{}
	appendOne := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, exists := seen[name]; exists {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	for _, mw := range groupMws {
		appendOne(mw)
	}
	for _, mw := range routeMws {
		appendOne(mw)
	}
	return out
}

func filterHandlerCandidates(candidates []discoveredHandler, pkgHint, recvHint string) []discoveredHandler {
	out := make([]discoveredHandler, 0, len(candidates))
	for _, c := range candidates {
		if pkgHint != "" && c.Package != pkgHint {
			continue
		}
		if recvHint != "" && strings.TrimPrefix(c.Receiver, "*") != recvHint {
			continue
		}
		out = append(out, c)
	}
	return out
}

func pickBestCandidate(candidates []discoveredHandler, pkgHint, recvHint string) discoveredHandler {
	if len(candidates) == 1 {
		return candidates[0]
	}
	for _, c := range candidates {
		if pkgHint != "" && recvHint != "" && c.Package == pkgHint && strings.TrimPrefix(c.Receiver, "*") == recvHint {
			return c
		}
	}
	for _, c := range candidates {
		if recvHint != "" && strings.TrimPrefix(c.Receiver, "*") == recvHint {
			return c
		}
	}
	return candidates[0]
}

func extractPathParamsFromRoute(path string) []Parameter {
	parts := strings.Split(path, "/")
	out := make([]Parameter, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		if part == "" {
			continue
		}
		name := ""
		switch {
		case strings.HasPrefix(part, ":"):
			name = strings.TrimPrefix(part, ":")
		case strings.HasPrefix(part, "*"):
			name = strings.TrimPrefix(part, "*")
		}
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, Parameter{
			Name:       name,
			In:         "path",
			Required:   true,
			Confidence: "high",
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func mergePathParams(fromRoute []Parameter, fromHandler []Parameter) []Parameter {
	merged := map[string]Parameter{}
	for _, p := range fromRoute {
		merged[p.Name] = p
	}
	for _, p := range fromHandler {
		if existing, ok := merged[p.Name]; ok {
			// Prefer higher confidence labels when there is a conflict.
			if existing.Confidence == "medium" && p.Confidence == "high" {
				merged[p.Name] = p
			}
			continue
		}
		merged[p.Name] = p
	}
	out := make([]Parameter, 0, len(merged))
	for _, p := range merged {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func resolveTypeSchema(handlerPkg, typeName string, typeSchemas map[string]any) (any, bool) {
	name := strings.TrimPrefix(typeName, "*")
	if name == "" {
		return nil, false
	}
	// Local package type name.
	if schema, ok := typeSchemas[typeSchemaKey(handlerPkg, name)]; ok {
		return schema, true
	}
	// Qualified type name (e.g. dto.CreateInput).
	if strings.Contains(name, ".") {
		parts := strings.SplitN(name, ".", 2)
		if len(parts) == 2 {
			if schema, ok := typeSchemas[typeSchemaKey(parts[0], parts[1])]; ok {
				return schema, true
			}
			// Import aliases may differ from package names. Fallback by matching
			// the concrete type name suffix across known schema keys.
			typeNameOnly := parts[1]
			var matched any
			matchCount := 0
			for key, schema := range typeSchemas {
				if strings.HasSuffix(key, "."+typeNameOnly) {
					matched = schema
					matchCount++
				}
			}
			if matchCount == 1 {
				return matched, true
			}
		}
	}
	return nil, false
}

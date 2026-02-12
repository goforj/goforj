package apix

import (
	"fmt"
	"sort"
	"strings"

	"github.com/goforj/str"
)

func normalize(routes []discoveredRoute, handlers []discoveredHandler, prefixes []string, mapping routerMapping) ([]Operation, []Diagnostic) {
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

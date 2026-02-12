package apix

import (
	"fmt"
	"sort"
	"strings"
)

func normalize(routes []discoveredRoute, handlers []discoveredHandler, prefixes []string) ([]Operation, []Diagnostic) {
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
	for i, r := range routes {
		method := normalizeMethodExpr(r.MethodExpr)
		path := r.Path
		if effectivePrefix != "" {
			path = joinPath(effectivePrefix, r.Path)
		}
		opID := fmt.Sprintf("%s:%s", strings.ToUpper(method), path)
		handlerFn := methodNameFromHandlerExpr(r.HandlerExpr)

		op := Operation{
			ID:     opID,
			Method: strings.ToUpper(method),
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

		candidates := handlerByName[handlerFn]
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

		h := candidates[0]
		op.Handler.Package = h.Package
		op.Handler.Receiver = h.Receiver
		op.Handler.Function = h.Name
		op.Handler.File = h.File
		op.Handler.Line = h.Line

		analyzed := analyzeHandler(h.Decl)
		op.Inputs.PathParams = analyzed.PathParams
		op.Inputs.QueryParams = analyzed.QueryParams
		op.Inputs.Headers = analyzed.Headers
		op.Inputs.Body = analyzed.Body
		op.Outputs.Responses = analyzed.Responses

		ops = append(ops, op)
		_ = i
	}

	sort.Slice(ops, func(i, j int) bool {
		if ops[i].Path == ops[j].Path {
			return ops[i].Method < ops[j].Method
		}
		return ops[i].Path < ops[j].Path
	})

	return ops, diag
}

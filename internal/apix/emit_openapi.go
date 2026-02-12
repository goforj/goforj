package apix

import "sort"

// OpenAPIDocument is a minimal OpenAPI projection generated from the API index.
type OpenAPIDocument struct {
	OpenAPI string                          `json:"openapi"`
	Info    map[string]string               `json:"info"`
	Paths   map[string]map[string]OpenAPIOp `json:"paths"`
}

// OpenAPIOp is a minimal operation model for OpenAPI output.
type OpenAPIOp struct {
	OperationID string                    `json:"operationId"`
	Responses   map[string]map[string]any `json:"responses"`
}

func toOpenAPI(m Manifest) OpenAPIDocument {
	doc := OpenAPIDocument{
		OpenAPI: "3.0.3",
		Info: map[string]string{
			"title":   "Forj Generated API",
			"version": "1.0.0",
		},
		Paths: map[string]map[string]OpenAPIOp{},
	}
	for _, op := range m.Operations {
		method := normalizeMethodExpr(op.Method)
		if doc.Paths[op.Path] == nil {
			doc.Paths[op.Path] = map[string]OpenAPIOp{}
		}
		responses := map[string]map[string]any{}
		if len(op.Outputs.Responses) == 0 {
			responses["200"] = map[string]any{"description": "OK"}
		} else {
			sorted := append([]ResponseShape(nil), op.Outputs.Responses...)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i].StatusCode < sorted[j].StatusCode })
			for _, resp := range sorted {
				code := "default"
				if resp.StatusCode > 0 {
					code = intToString(resp.StatusCode)
				}
				respObj := map[string]any{"description": "response"}
				if resp.TypeName != "" {
					respObj["x-forj-type"] = resp.TypeName
				}
				responses[code] = respObj
			}
		}
		doc.Paths[op.Path][method] = OpenAPIOp{
			OperationID: op.ID,
			Responses:   responses,
		}
	}
	return doc
}

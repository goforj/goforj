package apix

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// OpenAPIDocument is a minimal OpenAPI projection generated from the API index.
type OpenAPIDocument struct {
	OpenAPI    string                          `json:"openapi"`
	Info       map[string]string               `json:"info"`
	Paths      map[string]map[string]OpenAPIOp `json:"paths"`
	Components map[string]any                  `json:"components,omitempty"`
}

// OpenAPIOp is a minimal operation model for OpenAPI output.
type OpenAPIOp struct {
	OperationID string                    `json:"operationId"`
	Parameters  []OpenAPIParameter        `json:"parameters,omitempty"`
	RequestBody map[string]any            `json:"requestBody,omitempty"`
	Responses   map[string]map[string]any `json:"responses"`
}

// OpenAPIParameter is a minimal parameter projection.
type OpenAPIParameter struct {
	Name     string            `json:"name"`
	In       string            `json:"in"`
	Required bool              `json:"required,omitempty"`
	Schema   map[string]string `json:"schema,omitempty"`
}

func toOpenAPI(m Manifest) OpenAPIDocument {
	return toOpenAPIWithTitle(m, "Forj Generated API")
}

func toOpenAPIWithTitle(m Manifest, title string) OpenAPIDocument {
	if strings.TrimSpace(title) == "" {
		title = "Forj Generated API"
	}
	doc := OpenAPIDocument{
		OpenAPI: "3.0.3",
		Info: map[string]string{
			"title":   title,
			"version": "1.0.0",
		},
		Paths:      map[string]map[string]OpenAPIOp{},
		Components: map[string]any{"schemas": map[string]any{}},
	}
	components := newSchemaComponents()
	for _, op := range m.Operations {
		method := normalizeMethodExpr(op.Method)
		openAPIPath := toOpenAPIPath(op.Path)
		if doc.Paths[openAPIPath] == nil {
			doc.Paths[openAPIPath] = map[string]OpenAPIOp{}
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
				if contentType, schema := responseContent(resp); schema != nil {
					schema = components.refOrStore(schema)
					respObj["content"] = map[string]any{
						contentType: map[string]any{"schema": schema},
					}
				}
				if existing, ok := responses[code]; ok {
					responses[code] = mergeOpenAPIResponse(existing, respObj)
				} else {
					responses[code] = respObj
				}
			}
		}
		doc.Paths[openAPIPath][method] = OpenAPIOp{
			OperationID: op.ID,
			Parameters:  toOpenAPIParameters(op.Inputs),
			RequestBody: toOpenAPIRequestBody(op.Inputs, components),
			Responses:   responses,
		}
	}
	if len(components.schemas) > 0 {
		doc.Components["schemas"] = components.schemas
	} else {
		doc.Components = nil
	}
	return doc
}

type schemaComponents struct {
	schemas        map[string]any
	byFingerprint  map[string]string
	nameUseCounter map[string]int
}

func newSchemaComponents() *schemaComponents {
	return &schemaComponents{
		schemas:        map[string]any{},
		byFingerprint:  map[string]string{},
		nameUseCounter: map[string]int{},
	}
}

func (c *schemaComponents) refOrStore(schema any) any {
	obj, ok := schema.(map[string]any)
	if !ok || len(obj) == 0 {
		return schema
	}
	fp := openAPISchemaFingerprint(obj)
	if fp == "" {
		return schema
	}
	if name, ok := c.byFingerprint[fp]; ok {
		return map[string]any{"$ref": "#/components/schemas/" + name}
	}
	name := c.allocateName(obj)
	c.byFingerprint[fp] = name
	c.schemas[name] = obj
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func (c *schemaComponents) allocateName(schema map[string]any) string {
	base := "Schema"
	if raw, ok := schema["x-forj-type"].(string); ok && strings.TrimSpace(raw) != "" {
		base = componentNameFromType(raw)
	}
	if c.nameUseCounter[base] == 0 && c.schemas[base] == nil {
		c.nameUseCounter[base] = 1
		return base
	}
	c.nameUseCounter[base]++
	return fmt.Sprintf("%s%d", base, c.nameUseCounter[base])
}

func componentNameFromType(raw string) string {
	trimmed := strings.TrimPrefix(strings.TrimSpace(raw), "*")
	if trimmed == "" {
		return "Schema"
	}
	parts := strings.Split(trimmed, ".")
	typeName := parts[len(parts)-1]
	var b strings.Builder
	for _, r := range typeName {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	name := b.String()
	if name == "" {
		return "Schema"
	}
	if name[0] >= '0' && name[0] <= '9' {
		return "Type" + name
	}
	return name
}

func openAPISchemaFingerprint(schema map[string]any) string {
	data, err := json.Marshal(schema)
	if err != nil {
		return ""
	}
	return string(data)
}

func responseContent(resp ResponseShape) (string, any) {
	if resp.Schema != nil {
		return "application/json", resp.Schema
	}
	if resp.TypeName == "" {
		switch resp.Source {
		case "echo.String":
			return "text/plain", map[string]any{"type": "string"}
		case "echo.HTML":
			return "text/html", map[string]any{"type": "string"}
		case "echo.XML":
			return "application/xml", map[string]any{"type": "object"}
		case "echo.Blob":
			return "application/octet-stream", map[string]any{"type": "string", "format": "binary"}
		default:
			return "", nil
		}
	}
	switch resp.TypeName {
	case "map[string]string", "map[string]any", "map[string]interface{}":
		return "application/json", map[string]any{
			"type": "object",
		}
	default:
		if strings.HasPrefix(resp.TypeName, "[]") {
			return "application/json", map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
				},
			}
		}
		return "application/json", map[string]any{
			"type":        "object",
			"x-forj-type": resp.TypeName,
		}
	}
}

func toOpenAPIRequestBody(inputs InputShape, components *schemaComponents) map[string]any {
	if inputs.Body == nil || inputs.Body.TypeName == "" {
		return nil
	}
	schemaAny := any(map[string]any{
		"type":        "object",
		"x-forj-type": inputs.Body.TypeName,
	})
	if bodySchema, ok := inputs.Body.Schema.(map[string]any); ok && len(bodySchema) > 0 {
		schemaAny = bodySchema
	}
	if components != nil {
		schemaAny = components.refOrStore(schemaAny)
	}
	return map[string]any{
		"required": true,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": schemaAny,
			},
		},
	}
}

func mergeOpenAPIResponse(existing, incoming map[string]any) map[string]any {
	out := map[string]any{
		"description": "response",
	}
	if desc, ok := existing["description"].(string); ok && desc != "" {
		out["description"] = desc
	}
	if desc, ok := incoming["description"].(string); ok && desc != "" {
		out["description"] = desc
	}

	content := map[string]any{}
	if c, ok := existing["content"].(map[string]any); ok {
		for k, v := range c {
			content[k] = v
		}
	}
	if c, ok := incoming["content"].(map[string]any); ok {
		for contentType, value := range c {
			newBody, _ := value.(map[string]any)
			if oldRaw, exists := content[contentType]; exists {
				oldBody, _ := oldRaw.(map[string]any)
				content[contentType] = mergeOpenAPIContentBody(oldBody, newBody)
			} else {
				content[contentType] = newBody
			}
		}
	}
	if len(content) > 0 {
		out["content"] = content
	}
	return out
}

func mergeOpenAPIContentBody(existing, incoming map[string]any) map[string]any {
	if len(existing) == 0 {
		return incoming
	}
	if len(incoming) == 0 {
		return existing
	}
	oldSchema, oldOK := existing["schema"]
	newSchema, newOK := incoming["schema"]
	if !oldOK {
		return incoming
	}
	if !newOK {
		return existing
	}
	if schemasEquivalent(oldSchema, newSchema) {
		return existing
	}

	oneOf := make([]any, 0, 2)
	if current, ok := oldSchema.(map[string]any); ok {
		if existingOneOf, ok := current["oneOf"].([]any); ok && len(current) == 1 {
			oneOf = append(oneOf, existingOneOf...)
		} else {
			oneOf = append(oneOf, current)
		}
	} else {
		oneOf = append(oneOf, oldSchema)
	}
	oneOf = append(oneOf, newSchema)
	oneOf = dedupeSchemas(oneOf)
	return map[string]any{"schema": map[string]any{"oneOf": oneOf}}
}

func schemasEquivalent(a, b any) bool {
	am, aok := a.(map[string]any)
	bm, bok := b.(map[string]any)
	if !aok || !bok {
		return false
	}
	return openAPISchemaFingerprint(am) == openAPISchemaFingerprint(bm)
}

func dedupeSchemas(items []any) []any {
	seen := map[string]struct{}{}
	out := make([]any, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			out = append(out, item)
			continue
		}
		fp := openAPISchemaFingerprint(m)
		if fp == "" {
			out = append(out, item)
			continue
		}
		if _, exists := seen[fp]; exists {
			continue
		}
		seen[fp] = struct{}{}
		out = append(out, item)
	}
	return out
}

func toOpenAPIPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") && len(part) > 1 {
			parts[i] = "{" + strings.TrimPrefix(part, ":") + "}"
			continue
		}
		if strings.HasPrefix(part, "*") && len(part) > 1 {
			parts[i] = "{" + strings.TrimPrefix(part, "*") + "}"
		}
	}
	return strings.Join(parts, "/")
}

func toOpenAPIParameters(inputs InputShape) []OpenAPIParameter {
	out := make([]OpenAPIParameter, 0, len(inputs.PathParams)+len(inputs.QueryParams)+len(inputs.Headers))
	seen := map[string]struct{}{}
	appendParams := func(in string, params []Parameter, forceRequired bool) {
		for _, p := range params {
			key := in + "|" + p.Name
			if p.Name == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			required := p.Required
			if forceRequired {
				required = true
			}
			out = append(out, OpenAPIParameter{
				Name:     p.Name,
				In:       in,
				Required: required,
				Schema:   map[string]string{"type": "string"},
			})
		}
	}
	appendParams("path", inputs.PathParams, true)
	appendParams("query", inputs.QueryParams, false)
	appendParams("header", inputs.Headers, false)
	return out
}

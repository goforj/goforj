package apix

import "sort"

func collectSchemas(ops []Operation) []Schema {
	seen := map[string]struct{}{}
	for _, op := range ops {
		if op.Inputs.Body != nil && op.Inputs.Body.TypeName != "" {
			seen[op.Inputs.Body.TypeName] = struct{}{}
		}
		for _, resp := range op.Outputs.Responses {
			if resp.TypeName != "" {
				seen[resp.TypeName] = struct{}{}
			}
		}
	}
	out := make([]Schema, 0, len(seen))
	for name := range seen {
		confidence := "medium"
		kind := "unknown"
		if name != "" {
			if name == "map[string]string" || name == "map[string]any" || name == "map[string]interface{}" {
				kind = "map"
				confidence = "high"
			} else if name[0] == '[' {
				kind = "array"
			} else {
				kind = "object"
				confidence = "high"
			}
		}
		out = append(out, Schema{Name: name, Kind: kind, Confidence: confidence})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

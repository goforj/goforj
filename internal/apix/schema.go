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
		out = append(out, Schema{Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

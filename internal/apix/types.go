package apix

// ManifestVersion is the schema version for the API index output.
const ManifestVersion = "1"

// Manifest is the canonical API index artifact.
type Manifest struct {
	Version     string       `json:"version"`
	Operations  []Operation  `json:"operations"`
	Schemas     []Schema     `json:"schemas"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Operation describes one HTTP operation discovered in source.
type Operation struct {
	ID         string      `json:"id"`
	Method     string      `json:"method"`
	Path       string      `json:"path"`
	Handler    HandlerRef  `json:"handler"`
	Middleware []string    `json:"middleware,omitempty"`
	Inputs     InputShape  `json:"inputs"`
	Outputs    OutputShape `json:"outputs"`
}

// HandlerRef points to the handler function/method.
type HandlerRef struct {
	Expression string `json:"expression"`
	Package    string `json:"package,omitempty"`
	Receiver   string `json:"receiver,omitempty"`
	Function   string `json:"function,omitempty"`
	File       string `json:"file,omitempty"`
	Line       int    `json:"line,omitempty"`
}

// InputShape describes request inputs inferred from AST.
type InputShape struct {
	PathParams  []Parameter `json:"path_params,omitempty"`
	QueryParams []Parameter `json:"query_params,omitempty"`
	Headers     []Parameter `json:"headers,omitempty"`
	Body        *BodyShape  `json:"body,omitempty"`
}

// BodyShape describes inferred request body information.
type BodyShape struct {
	TypeName   string `json:"type_name,omitempty"`
	Source     string `json:"source,omitempty"`
	Confidence string `json:"confidence,omitempty"`
}

// Parameter describes an input parameter.
type Parameter struct {
	Name       string `json:"name"`
	In         string `json:"in"`
	Required   bool   `json:"required"`
	Confidence string `json:"confidence"`
}

// OutputShape describes response outputs inferred from AST.
type OutputShape struct {
	Responses []ResponseShape `json:"responses,omitempty"`
}

// ResponseShape describes one possible response.
type ResponseShape struct {
	StatusCode int    `json:"status_code"`
	TypeName   string `json:"type_name,omitempty"`
	Source     string `json:"source,omitempty"`
	Confidence string `json:"confidence,omitempty"`
}

// Schema is a placeholder for future expanded schema modeling.
type Schema struct {
	Name       string `json:"name"`
	Kind       string `json:"kind,omitempty"`
	Confidence string `json:"confidence,omitempty"`
}

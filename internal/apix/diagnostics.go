package apix

// Diagnostic captures parser/indexer warnings and informational findings.
type Diagnostic struct {
	Severity  string `json:"severity"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	File      string `json:"file,omitempty"`
	Line      int    `json:"line,omitempty"`
	Operation string `json:"operation,omitempty"`
}

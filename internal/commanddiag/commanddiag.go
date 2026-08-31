// Package commanddiag formats captured subprocess failures for terminal users.
package commanddiag

import (
	"errors"
	"strings"
)

// diagnosticError retains a process failure while presenting captured output as an indented block.
type diagnosticError struct {
	action  string
	message string
	cause   error
}

// Error returns the user-facing process failure and its captured diagnostics.
func (e *diagnosticError) Error() string {
	return e.message
}

// Unwrap preserves the underlying process error for errors.Is and errors.As.
func (e *diagnosticError) Unwrap() error {
	return e.cause
}

// Wrap formats captured command output without collapsing actionable multiline diagnostics.
func Wrap(action string, cause error, outputs ...string) error {
	return &diagnosticError{
		action:  strings.TrimSpace(action),
		message: Format(action, cause, outputs...),
		cause:   cause,
	}
}

// HasAction reports whether an error chain already identifies the named command boundary.
func HasAction(err error, action string) bool {
	action = strings.TrimSpace(action)
	for current := err; current != nil; current = errors.Unwrap(current) {
		diagnostic, ok := current.(*diagnosticError)
		if ok && diagnostic.action == action {
			return true
		}
	}
	return false
}

// Format builds a compact first line followed by deduplicated, indented command output.
func Format(action string, cause error, outputs ...string) string {
	message := cause.Error()
	if strings.TrimSpace(action) != "" {
		message = strings.TrimSpace(action) + ": " + message
	}

	seen := make(map[string]bool)
	diagnostics := make([]string, 0, len(outputs))
	for _, output := range outputs {
		output = strings.TrimSpace(output)
		if output == "" || seen[output] {
			continue
		}
		seen[output] = true
		diagnostics = append(diagnostics, output)
	}
	if len(diagnostics) == 0 {
		return message
	}
	detail := strings.Join(diagnostics, "\n")
	return message + "\n  " + strings.ReplaceAll(detail, "\n", "\n  ")
}

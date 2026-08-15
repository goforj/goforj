package project

import "testing"

// TestAgentGuidanceValid keeps persisted guidance values closed and explicit.
func TestAgentGuidanceValid(t *testing.T) {
	for _, test := range []struct {
		value AgentGuidance
		valid bool
	}{
		{value: AgentGuidanceBaseline, valid: true},
		{value: AgentGuidanceNone, valid: true},
		{value: "", valid: false},
		{value: "recommended", valid: false},
	} {
		if test.value.Valid() != test.valid {
			t.Fatalf("AgentGuidance(%q).Valid() = %t, want %t", test.value, test.value.Valid(), test.valid)
		}
	}
}

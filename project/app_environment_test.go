package project

import "testing"

// TestAppEnvironmentPrefixDefinesOneOverlayConvention verifies every App-facing subsystem can share the same normalization.
func TestAppEnvironmentPrefixDefinesOneOverlayConvention(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: ""},
		{name: DefaultAppName},
		{name: "billing", want: "BILLING"},
		{name: "billing-api", want: "BILLING_API"},
		{name: " Billing.API v2 ", want: "BILLING_API_V2"},
	}
	for _, test := range tests {
		if got := AppEnvironmentPrefix(test.name); got != test.want {
			t.Errorf("AppEnvironmentPrefix(%q) = %q, want %q", test.name, got, test.want)
		}
	}
}

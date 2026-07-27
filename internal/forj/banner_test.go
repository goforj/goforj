package forj

import "testing"

// TestColorizeGradientLineUsesTemperDisplayGradient keeps CLI branding aligned with the design system.
func TestColorizeGradientLineUsesTemperDisplayGradient(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")

	got := colorizeGradientLine("abc", false)
	want := "\033[38;2;255;198;133ma" +
		"\033[38;2;255;168;97mb" +
		"\033[38;2;255;138;61mc" +
		"\033[0m"
	if got != want {
		t.Fatalf("colorizeGradientLine() = %q, want %q", got, want)
	}
}

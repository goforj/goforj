package forj

import (
	"strings"
	"testing"
)

// TestTestRendersCmdRejectsUnknownProfile verifies direct command callers cannot silently run a smaller render matrix.
func TestTestRendersCmdRejectsUnknownProfile(t *testing.T) {
	err := (&TestRendersCmd{Profile: "typo", List: true}).Run()
	if err == nil || !strings.Contains(err.Error(), "valid profiles: smoke, pr, full") {
		t.Fatalf("Run() error = %v, want valid-profile error", err)
	}
}

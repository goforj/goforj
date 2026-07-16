package build

import "testing"

// TestCompilePackageName protects the compiler flag parsing used by the profile tool shim.
func TestCompilePackageName(t *testing.T) {
	got := compilePackageName([]string{"-o", "/tmp/out.a", "-p", "example.com/app/internal/http"})
	if got != "example.com/app/internal/http" {
		t.Fatalf("package = %q", got)
	}
}

// TestHandleProfileToolIgnoresNormalArgs keeps ordinary CLI commands out of the compiler tool path.
func TestHandleProfileToolIgnoresNormalArgs(t *testing.T) {
	if HandleProfileTool([]string{"build"}) {
		t.Fatal("expected normal args not to trigger profile tool")
	}
}

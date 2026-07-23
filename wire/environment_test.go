package wire

import "testing"

// TestInitializeApplicationSignature keeps application wiring independent of launcher arguments.
func TestInitializeApplicationSignature(t *testing.T) {
	var initialize func() (App, error) = InitializeApplication
	if initialize == nil {
		t.Fatal("InitializeApplication is nil")
	}
}

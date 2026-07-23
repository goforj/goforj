package wire

import "testing"

// TestInitializeApplicationCompatibilitySignature keeps the original zero-argument constructor available to callers.
func TestInitializeApplicationCompatibilitySignature(t *testing.T) {
	var initialize func() (App, error) = InitializeApplication
	if initialize == nil {
		t.Fatal("InitializeApplication is nil")
	}
}

// TestEmptyInheritedEnvironmentLeavesRuntimeFallbackAvailable verifies compatibility construction does not inject an empty map.
func TestEmptyInheritedEnvironmentLeavesRuntimeFallbackAvailable(t *testing.T) {
	if inherited := provideEmptyInheritedEnvironment(); inherited != nil {
		t.Fatalf("empty inherited environment = %#v, want nil", inherited)
	}
}

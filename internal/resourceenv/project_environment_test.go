package resourceenv

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestReadProjectEnvironmentAppliesOwnerPrecedence verifies concrete project values override checked-in defaults without consulting the process environment.
func TestReadProjectEnvironmentAppliesOwnerPrecedence(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte("DB_DRIVER=sqlite\nAPP_KEY=example-secret\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("DB_DRIVER=mysql\nAPP_KEY=owner-secret\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	values, err := ReadProjectEnvironment(root)
	if err != nil {
		t.Fatalf("ReadProjectEnvironment() error = %v", err)
	}
	if want := map[string]string{"DB_DRIVER": "mysql", "APP_KEY": "owner-secret"}; !reflect.DeepEqual(values, want) {
		t.Fatalf("ReadProjectEnvironment() = %#v, want %#v", values, want)
	}
}

// TestReadProjectEnvironmentHandlesMissingLayers verifies a new or partially initialized checkout remains readable.
func TestReadProjectEnvironmentHandlesMissingLayers(t *testing.T) {
	t.Parallel()
	values, err := ReadProjectEnvironment(t.TempDir())
	if err != nil {
		t.Fatalf("ReadProjectEnvironment() error = %v", err)
	}
	if len(values) != 0 {
		t.Fatalf("ReadProjectEnvironment() = %#v, want empty map", values)
	}
}

// TestReadProjectEnvironmentRejectsMalformedOwnerSource verifies malformed dotenv content cannot be interpreted as topology.
func TestReadProjectEnvironmentRejectsMalformedOwnerSource(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("BROKEN=\"unterminated\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := ReadProjectEnvironment(root); err == nil {
		t.Fatal("ReadProjectEnvironment() error = nil, want malformed dotenv error")
	}
}

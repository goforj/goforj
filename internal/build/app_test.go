package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommandArgumentsReturnAppPackageProbeErrors verifies build and run fail instead of compiling the repository root after a filesystem probe error.
func TestCommandArgumentsReturnAppPackageProbeErrors(t *testing.T) {
	tests := []struct {
		name    string
		resolve func(string) error
	}{
		{
			name: "build",
			resolve: func(root string) error {
				_, err := (&Cmd{}).buildArgs(root)
				return err
			},
		},
		{
			name: "run",
			resolve: func(root string) error {
				_, err := (&RunCmd{}).runArgsAt(root)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			commandDir := filepath.Join(root, "cmd")
			if err := os.Mkdir(commandDir, 0o755); err != nil {
				t.Fatalf("create command directory: %v", err)
			}
			packagePath := filepath.Join(commandDir, "app")
			if err := os.Symlink("app", packagePath); err != nil {
				t.Fatalf("create package probe cycle: %v", err)
			}

			err := test.resolve(root)
			if err == nil || !strings.Contains(err.Error(), "inspect App package "+packagePath) {
				t.Fatalf("package probe error = %v, want path context", err)
			}
		})
	}
}

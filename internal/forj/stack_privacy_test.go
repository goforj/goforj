package forj

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/stacks"
	"github.com/goforj/goforj/project"
)

// TestStackOpeningMasksInvalidDrivers protects the initial screen before the user selects an action or repairs malformed configuration.
func TestStackOpeningMasksInvalidDrivers(t *testing.T) {
	for _, key := range []string{"DB_DRIVER", "DB_REPORTS_DRIVER", "CACHE_DRIVER"} {
		t.Run(key, func(t *testing.T) {
			root := stackWizardFixture(t)
			content := key + "=postgres://user:private-secret@host/db\n"
			if err := os.WriteFile(filepath.Join(root, ".env"), []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if err := (&StackCmd{root: root, ui: moduleRenameTestConsole("7\n", &output)}).Run(); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), "private-secret") || !strings.Contains(output.String(), key+": (invalid)") {
				t.Fatal("opening preview did not redact the invalid driver")
			}
			data, err := os.ReadFile(filepath.Join(root, ".env"))
			if err != nil || string(data) != content {
				t.Fatal("viewing invalid configuration changed the environment")
			}
		})
	}
}

// TestStackActivationMasksInvalidPreviousDrivers protects repair previews, including malformed supported-driver lists and unknown resource keys.
func TestStackActivationMasksInvalidPreviousDrivers(t *testing.T) {
	root := stackWizardFixture(t)
	content := "DB_DRIVER=postgres://user:private-secret@host/db\nDB_SUPPORTED_DRIVERS=sqlite,private-secret\nREDIS_CUSTOM_DRIVER=private-secret\n"
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := stacks.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{"DB_DRIVER": "sqlite", "DB_SUPPORTED_DRIVERS": "sqlite"}
	var output bytes.Buffer
	if err := activateStack(moduleRenameTestConsole("no\n", &output), s, "", values); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "private-secret") {
		t.Fatal("repair preview exposed invalid previous settings")
	}
	for _, key := range []string{"DB_DRIVER", "DB_SUPPORTED_DRIVERS"} {
		if !strings.Contains(output.String(), key+": (invalid) -> sqlite") {
			t.Fatalf("repair preview omitted redacted change for %s", key)
		}
	}
}

// TestStackDriverDisplayPreservesKnownValues keeps valid aliases, empty selections, and driver lists readable while rejecting misplaced values.
func TestStackDriverDisplayPreservesKnownValues(t *testing.T) {
	var database project.ResourceDefinition
	for _, definition := range project.ResourceCatalog() {
		if definition.Key == project.ResourceDatabase {
			database = definition
		}
	}
	for _, tc := range []struct {
		value, want string
		multiple    bool
	}{
		{"", "", false}, {" ", " ", false}, {"sqlite3", "sqlite3", false}, {"postgresql", "postgresql", false},
		{"sqlite,postgres", "(invalid)", false}, {"sqlite,postgres", "sqlite,postgres", true},
		{"sqlite,, mariadb,", "sqlite,, mariadb,", true}, {"sqlite,private-secret", "(invalid)", true},
		{"private-secret", "(invalid)", false},
	} {
		if got := stackDriverDisplay(database, tc.value, tc.multiple); got != tc.want {
			t.Errorf("driver display = %q, want %q", got, tc.want)
		}
	}
}

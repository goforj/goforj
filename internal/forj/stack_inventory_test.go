package forj

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/console"
	"github.com/goforj/goforj/internal/stacks"
)

// TestStackSavedResourcePreviews includes providers that appear only in the selected definition before editing or confirming a build.
func TestStackSavedResourcePreviews(t *testing.T) {
	for _, input := range []string{"4\n3\n1\n\n", "5\n1\nno\n"} {
		t.Run(input, func(t *testing.T) {
			root := stackWizardFixture(t)
			definition := "DB_DRIVER=sqlite\nDB_SUPPORTED_DRIVERS=sqlite,postgres\nDB_ANALYTICS_DRIVER=postgres\n"
			if err := os.WriteFile(filepath.Join(root, ".env.stack.services"), []byte(definition), 0600); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(root, ".env"))
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if err := (&StackCmd{root: root, ui: moduleRenameTestConsole(input, &output)}).Run(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "DB_ANALYTICS_DRIVER: postgres") {
				t.Fatalf("selected provider missing from preview: %s", output.String())
			}
			after, err := os.ReadFile(filepath.Join(root, ".env"))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("preview changed active environment: %v", err)
			}
		})
	}
}

// TestStackEditorUpdatesSelectedAndNewResources verifies menu indices after selected or newly entered settings expand the resource inventory.
func TestStackEditorUpdatesSelectedAndNewResources(t *testing.T) {
	for _, newlyEntered := range []bool{false, true} {
		t.Run(fmt.Sprint(newlyEntered), func(t *testing.T) {
			root := stackWizardFixture(t)
			s, err := stacks.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			values := copyStackValues(s.Current)
			values["DB_SUPPORTED_DRIVERS"] = "mysql,postgres,sqlite"
			values["DB_ANALYTICS_DRIVER"] = "postgres"
			input := ""
			if newlyEntered {
				input = fmt.Sprintf("%d\nDB_ANALYTICS_DRIVER\n", len(s.Resources)+3)
			}
			found := false
			for index, resource := range s.ResourcesFor(values) {
				if resource.Key != "DB_ANALYTICS_DRIVER" {
					continue
				}
				for choice, driver := range resource.Definition.Drivers {
					if driver.Name == "sqlite" {
						input += fmt.Sprintf("%d\n%d\n1\n", index+2, choice+1)
						found = true
					}
				}
			}
			if !found {
				t.Fatal("selected-only database missing from inventory")
			}
			if newlyEntered {
				delete(values, "DB_ANALYTICS_DRIVER")
			}
			var output bytes.Buffer
			interactive, enabled := true, false
			ui := console.New(console.Config{Stdin: strings.NewReader(input), Stdout: &output, Stderr: &output, InteractiveEnabled: &interactive, ColorEnabled: &enabled, UnicodeEnabled: &enabled, ReadSecret: func() (string, error) { return "postgres", nil }})
			edited, err := editStackDrivers(ui, s, values)
			if err != nil {
				t.Fatal(err)
			}
			if edited["DB_ANALYTICS_DRIVER"] != "sqlite" || edited["DB_DRIVER"] != "mysql" {
				t.Fatalf("named edit changed the wrong resource: %v", edited)
			}
			if edited["DB_ANALYTICS_SQLITE_DATABASE"] == "" {
				t.Fatal("new SQLite resource has no database path")
			}
		})
	}
}

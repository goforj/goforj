package forj

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/stacks"
)

// TestStackWizardRetainsSQLiteTargetThroughDriverRoundTrip exercises actual menu selection, preview, and activation.
func TestStackWizardRetainsSQLiteTargetThroughDriverRoundTrip(t *testing.T) {
	root := stackWizardFixture(t)
	content := "DB_DRIVER=sqlite\nDB_SQLITE_DATABASE=./data/existing.db\nCACHE_DRIVER=redis\n"
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := stacks.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	input := "1\n2\n"
	for index, resource := range s.Resources {
		if resource.Key != "DB_DRIVER" {
			continue
		}
		for _, name := range []string{"mysql", "sqlite"} {
			for choice, driver := range resource.Definition.Drivers {
				if driver.Name == name {
					input += fmt.Sprintf("%d\n%d\n", index+2, choice+1)
				}
			}
		}
	}
	var output bytes.Buffer
	if err := (&StackCmd{root: root, ui: moduleRenameTestConsole(input+"1\nyes\n", &output)}).Run(); err != nil {
		t.Fatal(err)
	}
	s, err = stacks.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if s.Current["DB_DRIVER"] != "sqlite" || s.Current["DB_SQLITE_DATABASE"] != "./data/existing.db" {
		t.Fatal("driver round trip changed the existing database")
	}
}

// TestStackActivationShowsSQLitePathsWithoutCredentials makes an explicit target change reviewable before confirmation.
func TestStackActivationShowsSQLitePathsWithoutCredentials(t *testing.T) {
	root := stackWizardFixture(t)
	content := "DB_DRIVER=sqlite\nDB_SQLITE_DATABASE=./data/existing.db\nDB_DSN=file:private-dsn.db\nDB_PASSWORD=private-password\n"
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := stacks.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	values := copyStackValues(s.Current)
	values["DB_SQLITE_DATABASE"] = "./data/selected.db"
	values["DB_DSN"] = "file:new-private-dsn.db"
	values["DB_PASSWORD"] = "new-private-password"
	values[stacks.SQLiteDSNsKey] = `{"DB_DRIVER":"file:private-metadata.db"}`
	var output bytes.Buffer
	if err := activateStack(moduleRenameTestConsole("no\n", &output), s, "", values); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "./data/existing.db -> ./data/selected.db") {
		t.Fatalf("target change missing from preview: %s", output.String())
	}
	if strings.Contains(output.String(), "private-dsn.db") || strings.Contains(output.String(), "private-password") || strings.Contains(output.String(), "private-metadata.db") || strings.Contains(output.String(), stacks.SQLiteDSNsKey) {
		t.Fatal("preview exposed private connection values")
	}
	data, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil || string(data) != content {
		t.Fatal("canceled preview changed the active configuration")
	}
}

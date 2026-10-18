package forj

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/goforj/goforj/internal/localproviders"
)

// localProvidersFixture models the original MySQL and Redis use case.
func localProvidersFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, value := range map[string]string{
		".goforj.yml": "project_name: demo\nmodule_name: example.org/demo\nrender:\n  components: [database_mysql,cache,jobs]\n",
		".env":        "# owner comment\nAPP_KEY=synthetic-unrelated-secret\nDB_DRIVER=mysql\nDB_DSN=synthetic-private-dsn\nDB_SUPPORTED_DRIVERS=mysql\nCACHE_DRIVER=redis\nQUEUE_DRIVER=redis\nCOMPOSE_PROFILES=mysql,redis,grafana\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestLocalProvidersCommandSwitchAndRestore exercises the supported CLI flow through real configuration writes.
func TestLocalProvidersCommandSwitchAndRestore(t *testing.T) {
	root := localProvidersFixture(t)
	var output bytes.Buffer
	command := LocalProvidersCmd{root: root, ui: moduleRenameTestConsole("\ny\n", &output)}
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	session, err := localproviders.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"DB_DRIVER": "sqlite", "CACHE_DRIVER": "memory", "QUEUE_DRIVER": "workerpool", "COMPOSE_PROFILES": "grafana"} {
		if session.Current[key] != want {
			t.Fatalf("%s=%s, want %s", key, session.Current[key], want)
		}
	}
	if strings.Contains(output.String(), "synthetic-private-dsn") || strings.Contains(output.String(), "synthetic-unrelated-secret") {
		t.Fatal("preview exposed credentials")
	}
	for _, want := range []string{"DB_DRIVER", "mysql -> sqlite", "Run forj build"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("preview omitted %q", want)
		}
	}
	output.Reset()
	command.ui = moduleRenameTestConsole("\ny\n", &output)
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	session, err = localproviders.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if session.HasPrevious() || session.Current["DB_DSN"] != "synthetic-private-dsn" || session.Current["COMPOSE_PROFILES"] != "mysql,redis,grafana" {
		t.Fatal("restore did not recover service settings")
	}
}

// TestLocalProvidersCommandCancellationAndInputFailure never writes unconfirmed changes.
func TestLocalProvidersCommandCancellationAndInputFailure(t *testing.T) {
	for _, input := range []string{"\nn\n", "", "5\n"} {
		t.Run(input, func(t *testing.T) {
			root := localProvidersFixture(t)
			before, _ := os.ReadFile(filepath.Join(root, ".env"))
			var output bytes.Buffer
			command := LocalProvidersCmd{root: root, ui: moduleRenameTestConsole(input, &output)}
			_ = command.Run()
			after, _ := os.ReadFile(filepath.Join(root, ".env"))
			if !bytes.Equal(before, after) {
				t.Fatal("cancelled input changed .env")
			}
			s, err := localproviders.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			if s.HasPrevious() {
				t.Fatal("cancelled input created restore point")
			}
		})
	}
}

// TestLocalProvidersCommandExclusionKeepsItsService exercises the resource checkbox rather than an arbitrary settings editor.
func TestLocalProvidersCommandExclusionKeepsItsService(t *testing.T) {
	root := localProvidersFixture(t)
	session, err := localproviders.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	index := -1
	for i, resource := range session.Resources {
		if resource.Key == "CACHE_DRIVER" {
			index = i + 2
		}
	}
	if index < 0 {
		t.Fatal("cache resource missing")
	}
	var output bytes.Buffer
	input := fmt.Sprintf("%d\n1\ny\n", index)
	command := LocalProvidersCmd{root: root, ui: moduleRenameTestConsole(input, &output)}
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	session, err = localproviders.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if session.Current["CACHE_DRIVER"] != "redis" || session.Current["QUEUE_DRIVER"] != "workerpool" || session.Current["COMPOSE_PROFILES"] != "redis,grafana" {
		t.Fatal("partial switch removed a required service")
	}
}

// TestLocalProvidersCommandReportsRestoreConflicts keeps manual edits out of automatic restoration.
func TestLocalProvidersCommandReportsRestoreConflicts(t *testing.T) {
	root := localProvidersFixture(t)
	session, err := localproviders.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	values, err := session.Portable()
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Apply(values); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, ".env"))
	raw = bytes.ReplaceAll(raw, []byte("DB_DRIVER=sqlite"), []byte("DB_DRIVER=mysql"))
	if err := os.WriteFile(filepath.Join(root, ".env"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	command := LocalProvidersCmd{root: root, ui: moduleRenameTestConsole("\n", &output)}
	if err := command.Run(); err == nil || !strings.Contains(err.Error(), "changed since") {
		t.Fatalf("expected conflict, got %v", err)
	}
}

// TestLocalProvidersCommandRegistration exposes the focused action through Kong.
func TestLocalProvidersCommandRegistration(t *testing.T) {
	root := RootCmd{}
	parser, err := kong.New(&root, kong.Name("forj"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"env:local"}); err != nil {
		t.Fatal(err)
	}
}

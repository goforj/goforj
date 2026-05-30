package makecmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateCommandNameAvailableRejectsDiscoveredOwner(t *testing.T) {
	owners := map[string]string{"build": "GoForj command"}

	err := validateCommandNameAvailable("build", owners)
	if err == nil {
		t.Fatal("expected command name conflict")
	}
	if !strings.Contains(err.Error(), "GoForj command") {
		t.Fatalf("expected owner in error, got %v", err)
	}
}

func TestValidateCommandNameAvailableAllowsUnknownName(t *testing.T) {
	if err := validateCommandNameAvailable("reports:sync", map[string]string{"build": "GoForj command"}); err != nil {
		t.Fatalf("expected app-specific command to be allowed, got %v", err)
	}
}

func TestCommandCmdRejectsDiscoveredNativeCommandName(t *testing.T) {
	cmd := &CommandCmd{
		Name: "build",
		ReservedCommandNames: func() map[string]string {
			return map[string]string{"build": "GoForj command"}
		},
	}

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected reserved command name error")
	}
	if !strings.Contains(err.Error(), "GoForj command") {
		t.Fatalf("expected native command owner in error, got %v", err)
	}
}

func TestDiscoverProjectCommandNamesReadsStructTagsAndSignatures(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "internal", "cmd", "root_cmd.go"), `package cmd

type RootCmd struct {
	RunCmd RunCmd `+"`cmd:\"\" name:\"run\" aliases:\"app\"`"+`
	WorkerCmd WorkerCmd `+"`cmd:\"\" name:\"queue:work\" aliases:\"worker\"`"+`
}
`)
	writeFile(t, filepath.Join(root, "internal", "reports", "sync_cmd.go"), `package reports

type SyncCmd struct{}

func (*SyncCmd) Signature() string {
	return `+"`name:\"reports:sync\" aliases:\"reports:run\" help:\"Sync reports\"`"+`
}
`)

	owners := discoverProjectCommandNames(root)
	for _, name := range []string{"run", "app", "queue:work", "worker", "reports:sync", "reports:run"} {
		if _, ok := owners[name]; !ok {
			t.Fatalf("expected discovered command name %q in %#v", name, owners)
		}
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

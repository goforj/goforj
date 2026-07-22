package forj

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/goforj/console"
)

// TestDevCmdExplainsMissingProject verifies normal directory navigation does not become a fatal filesystem error.
func TestDevCmdExplainsMissingProject(t *testing.T) {
	t.Chdir(t.TempDir())
	commandConsole, stdout, stderr := newDevProjectTestConsole()
	previousConsole := console.Default()
	console.SetDefault(commandConsole)
	defer console.SetDefault(previousConsole)

	if err := (&DevCmd{}).Run(); err != nil {
		t.Fatalf("DevCmd.Run() error = %v", err)
	}
	want := "· No GoForj project found\n" +
		"  Run forj dev from a project root containing .goforj.yml.\n" +
		"  Starting fresh? Run forj new.\n"
	if got := stdout.String(); got != want {
		t.Fatalf("missing-project output = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("missing-project stderr = %q, want empty", stderr.String())
	}
	if _, err := os.Stat(".forj-dev.lock"); !os.IsNotExist(err) {
		t.Fatalf("missing-project command created a dev lock: %v", err)
	}
}

// TestLoadDevProjectConfigPreservesMalformedConfigurationFailure verifies actionable guidance does not hide a real project error.
func TestLoadDevProjectConfigPreservesMalformedConfigurationFailure(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".goforj.yml", []byte("render: [\n"), 0o644); err != nil {
		t.Fatalf("write malformed project config: %v", err)
	}
	commandConsole, stdout, stderr := newDevProjectTestConsole()

	config, err := loadDevProjectConfig(commandConsole)
	if err == nil {
		t.Fatal("loadDevProjectConfig() accepted malformed project configuration")
	}
	if config != nil {
		t.Fatalf("loadDevProjectConfig() config = %#v, want nil", config)
	}
	if !strings.Contains(err.Error(), "load project configuration") {
		t.Fatalf("loadDevProjectConfig() error = %q, want configuration context", err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("malformed configuration printed guidance: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

// newDevProjectTestConsole returns deterministic streams for project-entry guidance tests.
func newDevProjectTestConsole() (*console.Console, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	color := false
	unicode := true
	return console.New(console.Config{
		Stdout:         stdout,
		Stderr:         stderr,
		ColorEnabled:   &color,
		UnicodeEnabled: &unicode,
	}), stdout, stderr
}

package konghelp

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/goforj/goforj/project"
)

// TestSortedKeysReturnsAlphabeticalNamespaces protects deterministic framework help grouping.
func TestSortedKeysReturnsAlphabeticalNamespaces(t *testing.T) {
	sections := map[string][]*kong.Node{
		"make":      nil,
		"migrate":   nil,
		"app":       nil,
		"auth":      nil,
		"benchmark": nil,
	}

	got := sortedKeys(sections)
	want := []string{"app", "auth", "benchmark", "make", "migrate"}
	if !slices.Equal(got, want) {
		t.Fatalf("sortedKeys() = %v, want %v", got, want)
	}
}

// TestCommandVisibleInHelp protects the hidden-command policy for user and maintainer help.
func TestCommandVisibleInHelp(t *testing.T) {
	tests := []struct {
		name           string
		node           *kong.Node
		maintainerMode bool
		want           bool
	}{
		{
			name: "regular command visible",
			node: &kong.Node{
				Type: kong.CommandNode,
				Name: "build:api-index",
				Tag:  &kong.Tag{Hidden: false},
			},
			maintainerMode: false,
			want:           true,
		},
		{
			name: "hidden test command invisible by default",
			node: &kong.Node{
				Type: kong.CommandNode,
				Name: "test:integration",
				Tag:  &kong.Tag{Hidden: true},
			},
			maintainerMode: false,
			want:           false,
		},
		{
			name: "hidden test command visible in maintainer mode",
			node: &kong.Node{
				Type: kong.CommandNode,
				Name: "test:integration",
				Tag:  &kong.Tag{Hidden: true},
			},
			maintainerMode: true,
			want:           true,
		},
		{
			name: "hidden scenario command visible in maintainer mode",
			node: &kong.Node{
				Type: kong.CommandNode,
				Name: "scenario:generate",
				Tag:  &kong.Tag{Hidden: true},
			},
			maintainerMode: true,
			want:           true,
		},
		{
			name: "hidden scenario command invisible by default",
			node: &kong.Node{
				Type: kong.CommandNode,
				Name: "scenario:test",
				Tag:  &kong.Tag{Hidden: true},
			},
			maintainerMode: false,
			want:           false,
		},
		{
			name: "other hidden command remains hidden in maintainer mode",
			node: &kong.Node{
				Type: kong.CommandNode,
				Name: "build:binary",
				Tag:  &kong.Tag{Hidden: true},
			},
			maintainerMode: true,
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commandVisibleInHelp(tt.node, tt.maintainerMode)
			if got != tt.want {
				t.Fatalf("commandVisibleInHelp() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMaintainerHelpEnabledFromArgs keeps developer-only command visibility explicit at the CLI.
func TestMaintainerHelpEnabledFromArgs(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })

	origEnv, hadEnv := os.LookupEnv("FORJ_DEV")
	t.Cleanup(func() {
		if hadEnv {
			_ = os.Setenv("FORJ_DEV", origEnv)
		} else {
			_ = os.Unsetenv("FORJ_DEV")
		}
	})
	_ = os.Unsetenv("FORJ_DEV")

	cases := []struct {
		name string
		args []string
		want bool
	}{
		{name: "no toggle", args: []string{"forj"}, want: false},
		{name: "dev flag", args: []string{"forj", "--dev"}, want: true},
		{name: "dev flag explicit true", args: []string{"forj", "--dev=true"}, want: true},
		{name: "x alias", args: []string{"forj", "--x"}, want: true},
		{name: "x alias explicit true", args: []string{"forj", "--x=true"}, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			os.Args = tc.args
			got := maintainerHelpEnabled()
			if got != tc.want {
				t.Fatalf("maintainerHelpEnabled() = %v, want %v (args=%v)", got, tc.want, tc.args)
			}
		})
	}
}

// TestMaintainerHelpEnabledFromEnv keeps environment-driven maintainer help opt-in compatible with dev loops.
func TestMaintainerHelpEnabledFromEnv(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })
	os.Args = []string{"forj"}

	origEnv, hadEnv := os.LookupEnv("FORJ_DEV")
	t.Cleanup(func() {
		if hadEnv {
			_ = os.Setenv("FORJ_DEV", origEnv)
		} else {
			_ = os.Unsetenv("FORJ_DEV")
		}
	})

	_ = os.Setenv("FORJ_DEV", "1")
	if !maintainerHelpEnabled() {
		t.Fatalf("expected maintainerHelpEnabled() from FORJ_DEV=1")
	}

	_ = os.Setenv("FORJ_DEV", "0")
	if maintainerHelpEnabled() {
		t.Fatalf("expected maintainerHelpEnabled() false from FORJ_DEV=0")
	}
}

// TestFrameworkPreviewShowsCategoryActionCommands keeps the framework preview representative of GoForj commands.
func TestFrameworkPreviewShowsCategoryActionCommands(t *testing.T) {
	preview := Preview(string(project.HelpFormatFramework))
	for _, want := range []string{"make:command", "make:migration", "cache:shell", "db:shell"} {
		if !strings.Contains(preview, want) {
			t.Fatalf("expected framework preview to contain %q:\n%s", want, preview)
		}
	}
	if strings.Contains(preview, "redis:shell") {
		t.Fatalf("framework preview exposed provider-specific redis:shell:\n%s", preview)
	}
}

// TestFrameworkFormatterRendersSelectedCommandHelp prevents selected command help from falling back to root help.
func TestFrameworkFormatterRendersSelectedCommandHelp(t *testing.T) {
	output := renderTestHelp(t, project.HelpFormatFramework, []string{"make:command"})
	for _, want := range []string{"Create a new CLI command"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected framework command help to contain %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "make:migration") {
		t.Fatalf("expected selected command help, got root command list:\n%s", output)
	}
}

// TestExternalCLIFormatterRendersSelectedCommandArgumentsAndFlags protects the public CLI command detail layout.
func TestExternalCLIFormatterRendersSelectedCommandArgumentsAndFlags(t *testing.T) {
	output := renderTestHelp(t, project.HelpFormatExternalCLI, []string{"add", "Review PR"})
	for _, want := range []string{
		"add",
		"Add a task",
		"Usage",
		"tasks add <title> [flags]",
		"Arguments",
		"title",
		"Task title",
		"Flags",
		"--due",
		"Due date",
		"-t, --tag",
		"Task tag",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected external CLI command help to contain %q:\n%s", want, output)
		}
	}
}

// TestGuidedFormatterRendersSelectedCommandExamplesArgumentsAndFlags protects the examples-first command detail layout.
func TestGuidedFormatterRendersSelectedCommandExamplesArgumentsAndFlags(t *testing.T) {
	output := renderTestHelp(t, project.HelpFormatGuided, []string{"add", "Review PR"})
	for _, want := range []string{
		"tasks add",
		"Add a task",
		"Examples",
		`tasks add "Review PR"`,
		`tasks add "Ship release notes" --due tomorrow --tag docs`,
		"Usage",
		"tasks add <title> [flags]",
		"Arguments",
		"title",
		"Flags",
		"--due",
		"-h, --help",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected guided command help to contain %q:\n%s", want, output)
		}
	}
	if got := strings.Count(output, "--help"); got != 1 {
		t.Fatalf("expected guided command help to contain one --help row, got %d:\n%s", got, output)
	}
}

// renderTestHelp exercises real Kong parsing so formatter tests do not depend on handcrafted nodes.
func renderTestHelp(t *testing.T, format project.HelpFormat, args []string) string {
	t.Helper()
	formatKey := string(project.NormalizeHelpFormat(format))
	parser, err := kong.New(
		previewCommandSurface(formatKey),
		previewName(formatKey),
		previewDescription(formatKey),
	)
	if err != nil {
		t.Fatalf("kong.New() error = %v", err)
	}
	ctx, err := kong.Trace(parser, args)
	if err != nil {
		t.Fatalf("kong.Trace(%v) error = %v", args, err)
	}

	var out bytes.Buffer
	switch project.NormalizeHelpFormat(format) {
	case project.HelpFormatGuided:
		renderGuidedFormatter(&out, kong.HelpOptions{}, ctx)
	case project.HelpFormatExternalCLI:
		renderExternalCLIFormatter(&out, kong.HelpOptions{}, ctx)
	default:
		renderFrameworkFormatter(&out, kong.HelpOptions{}, ctx)
	}
	return out.String()
}

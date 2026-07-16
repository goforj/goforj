package build

import (
	"bytes"
	"testing"

	"github.com/goforj/console"
)

// TestBuildProgressTTYReporterUsesConsoleLoader verifies pipeline steps share the package loader lifecycle.
func TestBuildProgressTTYReporterUsesConsoleLoader(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	colorEnabled := false
	unicodeEnabled := false
	runtime := console.New(console.Config{
		Stdout:         &output,
		Stderr:         &output,
		ColorEnabled:   &colorEnabled,
		UnicodeEnabled: &unicodeEnabled,
	})
	reporter := &buildProgressTTYReporter{loader: runtime.Loader("")}

	reporter.Step(1, 2, "generate")
	reporter.Step(2, 2, "go build")
	reporter.State("done")
	reporter.State("done")

	if got, want := output.String(), "- 1/2 generate\n+ 2/2 go build\n"; got != want {
		t.Fatalf("progress output = %q, want %q", got, want)
	}
}

// TestBuildProgressTTYReporterClearsOrFails verifies terminal state mapping remains explicit.
func TestBuildProgressTTYReporterClearsOrFails(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		state       string
		clearOnDone bool
		want        string
	}{
		{name: "clear completed progress", state: "done", clearOnDone: true, want: "- 1/1 build\n"},
		{name: "retain failed progress", state: "failed", want: "- 1/1 build\nx 1/1 build\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			colorEnabled := false
			unicodeEnabled := false
			runtime := console.New(console.Config{
				Stdout:         &output,
				Stderr:         &output,
				ColorEnabled:   &colorEnabled,
				UnicodeEnabled: &unicodeEnabled,
			})
			reporter := &buildProgressTTYReporter{
				loader:      runtime.Loader(""),
				clearOnDone: test.clearOnDone,
			}

			reporter.Step(1, 1, "build")
			reporter.State(test.state)

			if got := output.String(); got != test.want {
				t.Fatalf("progress output = %q, want %q", got, test.want)
			}
		})
	}
}

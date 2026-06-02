package makecmd

import (
	"errors"
	"reflect"
	"testing"
)

func TestGeneratedFileOpenDecisionFor(t *testing.T) {
	tests := []struct {
		name         string
		openFlag     bool
		noOpenFlag   bool
		mode         string
		interactive  bool
		ci           bool
		wantOpen     bool
		wantExplicit bool
		wantInvalid  string
	}{
		{
			name:         "open flag wins",
			openFlag:     true,
			mode:         generatedFileOpenNever,
			interactive:  false,
			ci:           true,
			wantOpen:     true,
			wantExplicit: true,
		},
		{
			name:         "no open flag wins",
			noOpenFlag:   true,
			mode:         generatedFileOpenAlways,
			interactive:  true,
			wantOpen:     false,
			wantExplicit: true,
		},
		{
			name:        "auto opens in interactive shells",
			mode:        generatedFileOpenAuto,
			interactive: true,
			wantOpen:    true,
		},
		{
			name:        "auto skips in CI",
			mode:        generatedFileOpenAuto,
			interactive: true,
			ci:          true,
			wantOpen:    false,
		},
		{
			name:         "always opens explicitly",
			mode:         generatedFileOpenAlways,
			interactive:  false,
			ci:           true,
			wantOpen:     true,
			wantExplicit: true,
		},
		{
			name:         "never skips explicitly",
			mode:         generatedFileOpenNever,
			interactive:  true,
			wantOpen:     false,
			wantExplicit: true,
		},
		{
			name:        "invalid mode falls back to auto",
			mode:        "sometimes",
			interactive: true,
			wantOpen:    true,
			wantInvalid: "sometimes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, invalid := generatedFileOpenDecisionFor(tt.openFlag, tt.noOpenFlag, tt.mode, tt.interactive, tt.ci)
			if got.open != tt.wantOpen {
				t.Fatalf("open = %v, want %v", got.open, tt.wantOpen)
			}
			if got.explicit != tt.wantExplicit {
				t.Fatalf("explicit = %v, want %v", got.explicit, tt.wantExplicit)
			}
			if invalid != tt.wantInvalid {
				t.Fatalf("invalid = %q, want %q", invalid, tt.wantInvalid)
			}
		})
	}
}

func TestValidateGeneratedFileOpenFlagsRejectsConflict(t *testing.T) {
	if err := validateGeneratedFileOpenFlags(true, true); err == nil {
		t.Fatal("expected conflicting open flags to fail")
	}
}

func TestResolveGeneratedFileEditorCommandUsesConfiguredEditorTemplates(t *testing.T) {
	got, ok := resolveGeneratedFileEditorCommand(
		"code --reuse-window --goto {location}",
		"internal/users/controller.go",
		7,
		func(string) (string, error) { return "", errors.New("not found") },
	)
	if !ok {
		t.Fatal("expected configured editor to resolve")
	}
	if got.command != "code" {
		t.Fatalf("command = %q, want code", got.command)
	}
	if !reflect.DeepEqual(got.args, []string{"--reuse-window", "--goto", generatedFileOpenPath("internal/users/controller.go") + ":7"}) {
		t.Fatalf("args = %#v", got.args)
	}
}

func TestResolveGeneratedFileEditorCommandFallsBackToKnownEditor(t *testing.T) {
	got, ok := resolveGeneratedFileEditorCommand(
		"",
		"internal/users/controller.go",
		0,
		func(name string) (string, error) {
			if name == "cursor" {
				return "/usr/bin/cursor", nil
			}
			return "", errors.New("not found")
		},
	)
	if !ok {
		t.Fatal("expected fallback editor to resolve")
	}
	if got.command != "/usr/bin/cursor" {
		t.Fatalf("command = %q, want /usr/bin/cursor", got.command)
	}
	wantArgs := []string{"--reuse-window", "--goto", generatedFileOpenPath("internal/users/controller.go") + ":1"}
	if !reflect.DeepEqual(got.args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", got.args, wantArgs)
	}
}

func TestResolveGeneratedFileEditorCommandUsesTerminalEditorHints(t *testing.T) {
	got, ok := resolveGeneratedFileEditorCommandWith(
		"",
		"internal/users/controller.go",
		2,
		generatedFileEditorResolver{
			lookPath: fakeGeneratedFileLookPath(map[string]string{
				"code":   "/usr/bin/code",
				"cursor": "/usr/bin/cursor",
			}),
			env: fakeGeneratedFileEnv(map[string]string{
				"CURSOR_TRACE_ID": "trace",
			}),
			processes: func() []string { return nil },
		},
	)
	if !ok {
		t.Fatal("expected terminal editor hint to resolve")
	}
	if got.command != "/usr/bin/cursor" {
		t.Fatalf("command = %q, want /usr/bin/cursor", got.command)
	}
}

func TestResolveGeneratedFileEditorCommandPrefersRunningEditorBeforePathFallback(t *testing.T) {
	got, ok := resolveGeneratedFileEditorCommandWith(
		"",
		"internal/users/controller.go",
		4,
		generatedFileEditorResolver{
			lookPath: fakeGeneratedFileLookPath(map[string]string{
				"code":   "/usr/bin/code",
				"goland": "/usr/bin/goland",
			}),
			env:       fakeGeneratedFileEnv(nil),
			processes: func() []string { return []string{"GoLand"} },
		},
	)
	if !ok {
		t.Fatal("expected running editor to resolve")
	}
	if got.command != "/usr/bin/goland" {
		t.Fatalf("command = %q, want /usr/bin/goland", got.command)
	}
	wantArgs := []string{"--line", "4", generatedFileOpenPath("internal/users/controller.go")}
	if !reflect.DeepEqual(got.args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", got.args, wantArgs)
	}
}

func TestResolveGeneratedFileEditorCommandUsesPriorityPathFallback(t *testing.T) {
	got, ok := resolveGeneratedFileEditorCommandWith(
		"",
		"internal/users/controller.go",
		5,
		generatedFileEditorResolver{
			lookPath: fakeGeneratedFileLookPath(map[string]string{
				"code":   "/usr/bin/code",
				"goland": "/usr/bin/goland",
			}),
			env:       fakeGeneratedFileEnv(nil),
			processes: func() []string { return nil },
		},
	)
	if !ok {
		t.Fatal("expected priority path fallback to resolve")
	}
	if got.command != "/usr/bin/goland" {
		t.Fatalf("command = %q, want /usr/bin/goland", got.command)
	}
}

func TestResolveGeneratedFileEditorCommandIgnoresCommentOnlyConfiguredEditor(t *testing.T) {
	got, ok := resolveGeneratedFileEditorCommand(
		"# optional editor command",
		"internal/users/controller.go",
		3,
		func(name string) (string, error) {
			if name == "code" {
				return "/usr/bin/code", nil
			}
			return "", errors.New("not found")
		},
	)
	if !ok {
		t.Fatal("expected comment-only editor value to fall back to known editors")
	}
	if got.command != "/usr/bin/code" {
		t.Fatalf("command = %q, want /usr/bin/code", got.command)
	}
}

func fakeGeneratedFileLookPath(commands map[string]string) func(string) (string, error) {
	return func(name string) (string, error) {
		if command, ok := commands[name]; ok {
			return command, nil
		}
		return "", errors.New("not found")
	}
}

func fakeGeneratedFileEnv(values map[string]string) func(string) string {
	return func(name string) string {
		return values[name]
	}
}

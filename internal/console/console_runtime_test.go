package console

import (
	"bytes"
	"strings"
	"testing"
)

func TestConsoleDebugf_EnvDriven(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	env := map[string]string{"FORJ_DEBUG": "1"}

	c := New(Config{
		Stdout: out,
		Stderr: errOut,
		Getenv: func(key string) string { return env[key] },
	})

	c.Debugf("hello %s", "world")
	if !strings.Contains(out.String(), "hello world") {
		t.Fatalf("expected debug output, got %q", out.String())
	}
}

func TestConsoleColorize_RespectsNoColor(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	env := map[string]string{"NO_COLOR": "1"}

	c := New(Config{
		Stdout: out,
		Stderr: errOut,
		Getenv: func(key string) string { return env[key] },
	})

	got := c.Colorize(ColorGreen, "ok")
	if got != "ok" {
		t.Fatalf("expected no color output, got %q", got)
	}
}

func TestConsoleColorize_ForceColor(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	env := map[string]string{"CLICOLOR_FORCE": "1"}

	c := New(Config{
		Stdout: out,
		Stderr: errOut,
		Getenv: func(key string) string { return env[key] },
	})

	got := c.Colorize(ColorGreen, "ok")
	if !strings.Contains(got, ColorGreen) || !strings.Contains(got, ColorReset) {
		t.Fatalf("expected ansi wrapped output, got %q", got)
	}
}

func TestConsoleInfof_UsesProvidedWriters(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	force := true

	c := New(Config{
		Stdout:       out,
		Stderr:       errOut,
		ColorEnabled: &force,
	})

	c.Infof("test")
	if !strings.Contains(out.String(), "test") {
		t.Fatalf("expected message in output, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

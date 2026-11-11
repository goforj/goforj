package main

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestParseTests preserves package identities, subtests, malformed lines, and exit-zero failures.
func TestParseTests(t *testing.T) {
	input := strings.Join([]string{`noise`, `[]`, `{"Action":"start","Package":"a"}`, `{"Action":"pass","Package":"a","Test":"Parent/child"}`, `{"Action":"pass","Package":"a","Test":"Parent/child"}`, `{"Action":"fail","Package":"b","Test":"Parent/child"}`, `{"Action":"skip","Package":"a","Test":"Skipped"}`, `{"Action":"fail","Package":"b"}`, `{"Action":"build-fail","ImportPath":"c"}`, `{"Action":"skip","Package":"empty"}`}, "\n")
	result, err := parseTests(strings.NewReader(input), 0)
	if err != nil {
		t.Fatal(err)
	}
	expected := testResult{ExitCode: 0, Passed: []string{"a::Parent/child"}, Failed: []string{"b::Parent/child"}, Skipped: []string{"a::Skipped"}, PackageFailures: []string{"b", "c"}, Packages: []string{"a", "b", "c", "empty"}}
	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("got %+v, want %+v", result, expected)
	}
	empty, err := parseTests(strings.NewReader("compiler output\n"), 1)
	if err != nil || empty.ExitCode != 1 || len(empty.Passed) != 0 {
		t.Fatalf("empty: %+v, %v", empty, err)
	}
	_, err = parseTests(failingReader{}, 0)
	if err == nil {
		t.Fatal("reader failure was discarded")
	}
}

// failingReader makes retained-log read failures distinguishable from empty test runs.
type failingReader struct{}

// Read injects a log storage failure.
func (failingReader) Read([]byte) (int, error) { return 0, errors.New("broken evidence") }

// TestInventory excludes private and generated trees while retaining nested modules and link identities.
func TestInventory(t *testing.T) {
	root := t.TempDir()
	for name, value := range map[string]string{"source.go": "source", "nested/go.mod": "module nested", ".ENV/private": "secret", "nested/.Env.testing": "secret", "node_modules/pkg/index": "generated", ".git/config": "private"} {
		name = filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(value), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(root, "link")
	linked := os.Symlink("outside-missing", link) == nil
	files, err := inventory(root)
	if err != nil {
		t.Fatal(err)
	}
	if !files[".ENV"].Private || !files["nested/.Env.testing"].Private || files["nested/go.mod"].SHA256 == "" {
		t.Fatalf("incomplete inventory: %+v", files)
	}
	if linked && files["link"].Symlink != "outside-missing" {
		t.Fatalf("link target lost: %+v", files)
	}
	for name := range files {
		if strings.HasPrefix(name, ".git/") || strings.HasPrefix(name, "node_modules/") || name == ".ENV/private" {
			t.Fatalf("private/generated path opened: %s", name)
		}
	}
	if runtime.GOOS != "windows" && files["source.go"].Mode != 0o640 {
		t.Fatal("mode lost")
	}
	if _, err := inventory(filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing root accepted")
	}
}

// TestArchivePolicy exercises private case variants, nested layers, symlinks, unsafe paths, and permissions.
func TestArchivePolicy(t *testing.T) {
	for _, unsafe := range []string{"", "../escape", "/absolute", "nested/../escape", "windows\\escape"} {
		t.Run(unsafe, func(t *testing.T) {
			var archive bytes.Buffer
			writer := tar.NewWriter(&archive)
			names := []string{".env", ".ENV.production", "nested/.Env.testing", ".envdir/config", "internal/owner.go"}
			if unsafe != "" {
				names = append(names, unsafe)
			}
			for _, name := range names {
				if err := writer.WriteHeader(&tar.Header{Name: name, Size: 6, Mode: 0o751, Typeflag: tar.TypeReg}); err != nil {
					t.Fatal(err)
				}
				if _, err := writer.Write([]byte("canary")); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.WriteHeader(&tar.Header{Name: "private-link", Typeflag: tar.TypeSymlink, Linkname: "/outside"}); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			err := extractCommittedArchive(&archive, root)
			if unsafe != "" {
				if err == nil {
					t.Fatal("unsafe path accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			files, err := inventory(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != 1 || files["internal/owner.go"].SHA256 == "" {
				t.Fatalf("exported private layers: %+v", files)
			}
			if runtime.GOOS != "windows" && files["internal/owner.go"].Mode != 0o751 {
				t.Fatal("archive permissions lost")
			}
		})
	}
	if err := extractCommittedArchive(strings.NewReader("invalid tar"), t.TempDir()); err == nil {
		t.Fatal("invalid archive accepted")
	}
}

// TestTrialEnvironment refuses inherited provider credentials and production settings.
func TestTrialEnvironment(t *testing.T) {
	t.Setenv("UNRELATED_PROVIDER_KEY", "canary")
	t.Setenv("APP_ENV", "production")
	t.Setenv("GOCACHE", "")
	env := strings.Join(trialEnvironment(), "\n")
	for _, required := range []string{"APP_ENV=testing", "TMPDIR=/tmp", "GOWORK=off", "DB_DRIVER=sqlite", "GOCACHE=/tmp/gocache", "API_JWT_SECRET_KEY=public-agent-upgrade-test-key-0123456789"} {
		if !strings.Contains(env, required) {
			t.Fatalf("missing %s", required)
		}
	}
	if strings.Contains(env, "canary") || strings.Contains(env, "production") {
		t.Fatal("inherited private environment")
	}
}

// TestCommandHelper supplies portable native descendants without a shell or interpreter.
func TestCommandHelper(t *testing.T) {
	if os.Getenv("UPGRADE_SPIKE_HELPER") != "1" {
		return
	}
	switch os.Getenv("UPGRADE_SPIKE_HELPER_MODE") {
	case "exit":
		os.Exit(7)
	case "child":
		time.Sleep(1200 * time.Millisecond)
		if err := os.WriteFile(os.Getenv("UPGRADE_SPIKE_MARKER"), []byte("survived"), 0o600); err != nil {
			os.Exit(4)
		}
		os.Exit(0)
	case "success", "timeout":
		child := exec.Command(os.Args[0], "-test.run=^TestCommandHelper$")
		child.Env = append(os.Environ(), "UPGRADE_SPIKE_HELPER_MODE=child")
		if err := child.Start(); err != nil {
			os.Exit(3)
		}
		if err := os.WriteFile(os.Getenv("UPGRADE_SPIKE_READY"), []byte("ready"), 0o600); err != nil {
			os.Exit(4)
		}
		if os.Getenv("UPGRADE_SPIKE_HELPER_MODE") == "success" {
			os.Exit(0)
		}
		time.Sleep(time.Hour)
	}
}

// TestRunCommand distinguishes launch failures, real exit codes, and canceled process trees.
func TestRunCommand(t *testing.T) {
	args := []string{os.Args[0], "-test.run=^TestCommandHelper$"}
	env := append(trialEnvironment(), "UPGRADE_SPIKE_HELPER=1", "UPGRADE_SPIKE_HELPER_MODE=exit")
	code, err := runCommand(context.Background(), t.TempDir(), args, env, io.Discard, io.Discard)
	if err != nil || code != 7 {
		t.Fatalf("exit: %d %v", code, err)
	}
	if _, err := runCommand(context.Background(), t.TempDir(), []string{filepath.Join(t.TempDir(), "missing")}, nil, io.Discard, io.Discard); err == nil {
		t.Fatal("launch failure discarded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runCommand(ctx, t.TempDir(), args, env, io.Discard, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	for _, mode := range []string{"success", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			marker, ready := filepath.Join(root, "marker"), filepath.Join(root, "ready")
			env := append(trialEnvironment(), "UPGRADE_SPIKE_HELPER=1", "UPGRADE_SPIKE_HELPER_MODE="+mode, "UPGRADE_SPIKE_MARKER="+marker, "UPGRADE_SPIKE_READY="+ready)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			done := make(chan struct{})
			var code int
			var err error
			go func() { code, err = runCommand(ctx, root, args, env, io.Discard, io.Discard); close(done) }()
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, err := os.Stat(ready); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("helper never became ready")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if mode == "timeout" {
				cancel()
			}
			<-done
			if mode == "success" && (err != nil || code != 0) {
				t.Fatalf("success: %d %v", code, err)
			}
			if mode == "timeout" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel: %d %v", code, err)
			}
			time.Sleep(1300 * time.Millisecond)
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("descendant survived: %v", err)
			}
		})
	}
}

// TestRunGo retains real Go JSON evidence and treats expired deadlines as failed validation.
func TestRunGo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("validation environment intentionally uses /tmp")
	}
	root := t.TempDir()
	for name, source := range map[string]string{"go.mod": "module example.invalid/upgradefixture\n\ngo 1.26.0\n", "fixture_test.go": "package fixture\nimport \"testing\"\nfunc TestBehavior(t *testing.T) { t.Fatal(\"behavior failed\") }\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := runGo(context.Background(), root, root, "unit", []string{"test", "-json", "./..."})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode == 0 || !reflect.DeepEqual(result.Failed, []string{"example.invalid/upgradefixture::TestBehavior"}) {
		t.Fatalf("lost failure evidence: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "unit.json")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	result, err = runGo(ctx, root, root, "timeout", []string{"test", "./..."})
	if err != nil || result.ExitCode != 124 {
		t.Fatalf("deadline: %+v %v", result, err)
	}
}

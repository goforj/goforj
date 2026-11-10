package projectupgrade

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestReadInputConfinesConfiguration rejects aliases and oversized inputs before configuration decoding.
func TestReadInputConfinesConfiguration(t *testing.T) {
	root := t.TempDir()
	if _, err := ReadInput(root, "../outside"); err == nil {
		t.Fatal("accepted escaping input path")
	}
	if _, err := ReadInput(root, ".goforj.yml"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing input error=%v", err)
	}
	writeFixture(t, root, ".goforj.yml", "module_name: example.com/app\n")
	if source, err := ReadInput(root, ".goforj.yml"); err != nil || string(source) != "module_name: example.com/app\n" {
		t.Fatalf("stable input not read: %v", err)
	}
}

// TestLargeRawAssetsAreStreamed distinguishes legitimate binary assets from bounded source or configuration payloads.
func TestLargeRawAssetsAreStreamed(t *testing.T) {
	directory := t.TempDir()
	content := bytes.Repeat([]byte{0xff, 0, 0xfe, 1}, (5<<20)/4)
	writeFixture(t, directory, "containers/large.bin", string(content))
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, _, err := readRegular(root, "containers/large.bin"); err == nil {
		t.Fatal("source limit was bypassed")
	}
	data, state, err := readManaged(root, "containers/large.bin", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 || state.digest != Digest(content) {
		t.Fatal("raw asset was not streamed accurately")
	}
	if err := verifyObservations(root, map[string]observedFile{"containers/large.bin": state}); err != nil {
		t.Fatal(err)
	}
	target := testSnapshot("package auth\n")
	target.Artifacts = []Artifact{{Path: "containers/large.bin", Ownership: "framework-asset", Digest: Digest(content)}}
	plan, err := Build(directory, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Files[0].Action != "matches-target" {
		t.Fatalf("large raw asset was not compared: %+v", plan.Files[0])
	}
}

// TestVerifyObservationsDetectsManagedEdits prevents stale plans after writes, deletion, creation, or mode changes.
func TestVerifyObservationsDetectsManagedEdits(t *testing.T) {
	for _, change := range []string{"rewrite", "delete", "create", "mode"} {
		t.Run(change, func(t *testing.T) {
			if change == "mode" && runtime.GOOS == "windows" {
				t.Skip("Windows does not preserve Unix executable bits")
			}
			directory := t.TempDir()
			name := "internal/runtime/runtime.go"
			if change != "create" {
				writeFixture(t, directory, name, "package runtime\n")
			}
			root, err := os.OpenRoot(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			_, state, err := readRegular(root, name)
			if err != nil {
				t.Fatal(err)
			}
			observations := map[string]observedFile{name: state}
			if err := verifyObservations(root, observations); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "rewrite":
				writeFixture(t, directory, name, "package changed\n")
			case "delete":
				if err := os.Remove(filepath.Join(directory, filepath.FromSlash(name))); err != nil {
					t.Fatal(err)
				}
			case "create":
				writeFixture(t, directory, name, "package runtime\n")
			case "mode":
				if err := os.Chmod(filepath.Join(directory, filepath.FromSlash(name)), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := verifyObservations(root, observations); err == nil {
				t.Fatal("accepted changed managed state")
			}
		})
	}
}

// TestRootedReadSurvivesConcurrentDirectoryReplacement never reads the external target during a parent symlink race.
func TestRootedReadSurvivesConcurrentDirectoryReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows directory rename sharing differs; static symlink cases remain covered")
	}
	directory := t.TempDir()
	outside := t.TempDir()
	writeFixture(t, directory, "managed/source.go", "SAFE")
	writeFixture(t, outside, "source.go", "OUTSIDE_CANARY")
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 150 {
			if os.Rename(filepath.Join(directory, "managed"), filepath.Join(directory, "held")) != nil {
				continue
			}
			if os.Symlink(outside, filepath.Join(directory, "managed")) == nil {
				_ = os.Remove(filepath.Join(directory, "managed"))
			}
			_ = os.Rename(filepath.Join(directory, "held"), filepath.Join(directory, "managed"))
		}
	}()
	for range 300 {
		data, _, err := readRegular(root, "managed/source.go")
		if err == nil && len(data) > 0 && string(data) != "SAFE" {
			<-done
			t.Fatalf("read outside project: %q", data)
		}
	}
	<-done
}

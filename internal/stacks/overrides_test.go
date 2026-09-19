package stacks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/env/v2"
)

// TestOverridesMatchesRuntimeAncestorDiscovery compares a portable activation preview with the real dotenv loader in an isolated process.
func TestOverridesMatchesRuntimeAncestorDiscovery(t *testing.T) {
	for _, layer := range []string{"production", "testing"} {
		t.Run(layer, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "project")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			put(t, root, ".goforj.yml", "project_name: nested\nmodule_name: example.org/nested\nrender:\n  components: [cli, database_mysql]\n")
			put(t, root, ".env", "APP_ENV="+layer+"\nDB_DRIVER=mysql\nDB_SUPPORTED_DRIVERS=mysql,postgres\n")
			put(t, parent, ".env."+layer, "DB_DRIVER=postgres\nDB_PASSWORD=must-not-display\n")
			s := openTest(t, root)
			before := os.Environ()
			layers, err := s.Overrides()
			if err != nil {
				t.Fatal(err)
			}
			want := filepath.Join("..", ".env."+layer) + ": DB_DRIVER, DB_PASSWORD"
			if !strings.Contains(strings.Join(layers, "\n"), want) || strings.Contains(strings.Join(layers, "\n"), "must-not-display") {
				t.Fatalf("unexpected layer preview: %v", layers)
			}
			if !reflect.DeepEqual(before, os.Environ()) {
				t.Fatal("preview changed the process environment")
			}
			portable, err := s.Portable()
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Activate("", portable, false); err != nil {
				t.Fatal(err)
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			command := exec.Command(executable, "-test.run=^TestStackRuntimeLayerProbe$")
			command.Dir = root
			for _, entry := range os.Environ() {
				key, _, _ := strings.Cut(entry, "=")
				if key != "APP_ENV" && !strings.HasPrefix(key, "DB_") {
					command.Env = append(command.Env, entry)
				}
			}
			command.Env = append(command.Env, "FORJ_STACK_LAYER_PROBE=1")
			output, err := command.CombinedOutput()
			if err != nil || !strings.Contains(string(output), "runtime driver=postgres") {
				t.Fatalf("runtime did not apply the advertised override: %v\n%s", err, output)
			}
		})
	}
}

// TestStackRuntimeLayerProbe exercises the pinned runtime loader without leaking its global state into other tests.
func TestStackRuntimeLayerProbe(t *testing.T) {
	if os.Getenv("FORJ_STACK_LAYER_PROBE") != "1" {
		return
	}
	if err := env.Load(); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("runtime driver=%s\n", os.Getenv("DB_DRIVER"))
}

// TestNearestRuntimeLayerUsesTheNearestFile protects per-filename shadowing, including empty files and regular-file symlinks.
func TestNearestRuntimeLayerUsesTheNearestFile(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	put(t, parent, ".env.production", "DB_DRIVER=postgres\n")
	put(t, root, ".env.production", "")
	f, err := nearestRuntimeLayer(root, ".env.production")
	if err != nil || !f.exists || f.name != ".env.production" || len(f.before) != 0 {
		t.Fatalf("empty local file did not shadow ancestor: %+v, %v", f, err)
	}
	if err := os.Remove(filepath.Join(root, ".env.production")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(parent, ".env.production"), filepath.Join(root, ".env.production")); err != nil {
		t.Fatal(err)
	}
	f, err = nearestRuntimeLayer(root, ".env.production")
	if err != nil || !f.exists || f.name != ".env.production" || !strings.Contains(string(f.before), "postgres") {
		t.Fatalf("regular-file symlink did not match runtime discovery: %+v, %v", f, err)
	}
}

// TestNearestRuntimeLayerHonorsSearchBoundaries keeps warnings aligned with the runtime's fixed ancestor limit and error behavior.
func TestNearestRuntimeLayerHonorsSearchBoundaries(t *testing.T) {
	root := t.TempDir()
	put(t, root, ".env.testing", "DB_DRIVER=postgres\n")
	child := root
	for level := 1; level <= env.MaxDirectorySeekLevels; level++ {
		child = filepath.Join(child, "child")
		if err := os.Mkdir(child, 0700); err != nil {
			t.Fatal(err)
		}
		f, err := nearestRuntimeLayer(child, ".env.testing")
		if err != nil || f.exists != (level < env.MaxDirectorySeekLevels) {
			t.Fatalf("discovery at depth %d: exists=%v, err=%v", level, f.exists, err)
		}
	}
	if _, err := nearestRuntimeLayer(filepath.Join(root, ".env.testing"), ".env.production"); err == nil {
		t.Fatal("ignored an invalid search directory")
	}
	if err := os.Mkdir(filepath.Join(child, ".env.host"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := nearestRuntimeLayer(child, ".env.host"); err == nil {
		t.Fatal("accepted a non-regular runtime layer")
	}
	f, err := nearestRuntimeLayer(string(filepath.Separator), ".env.stack-review-missing")
	if err != nil || f.exists {
		t.Fatalf("filesystem root did not terminate search: %+v, %v", f, err)
	}
}

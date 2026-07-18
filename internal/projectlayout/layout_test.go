package projectlayout

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestNormalizeAppPreservesOverrides verifies convention filling never replaces owner-specified paths.
func TestNormalizeAppPreservesOverrides(t *testing.T) {
	tests := []struct {
		name string
		app  project.App
		want project.App
	}{
		{
			name: "empty app uses default",
			app:  project.App{},
			want: project.DefaultApp(),
		},
		{
			name: "named app uses conventions",
			app:  project.App{Name: "billing"},
			want: project.DefaultNamedApp("billing"),
		},
		{
			name: "partial overrides fill missing paths",
			app: project.App{
				Name:       "billing",
				Entrypoint: filepath.Join("services", "billing", "main.go"),
			},
			want: project.App{
				Name:       "billing",
				Entrypoint: filepath.Join("services", "billing", "main.go"),
				AppDir:     filepath.Join("app", "billing"),
				WireDir:    filepath.Join("app", "billing", "wire"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NormalizeApp(test.app); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("NormalizeApp() = %#v, want %#v", got, test.want)
			}
		})
	}
}

// TestAppPathsResolveAgainstExplicitRoot verifies callers can leave process working-directory state untouched.
func TestAppPathsResolveAgainstExplicitRoot(t *testing.T) {
	root := t.TempDir()
	app := project.DefaultNamedApp("billing")
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "entrypoint", got: Entrypoint(root, app), want: filepath.Join(root, "cmd", "billing", "main.go")},
		{name: "command directory", got: CommandDir(root, app), want: filepath.Join(root, "cmd", "billing")},
		{name: "app directory", got: AppDir(root, app), want: filepath.Join(root, "app", "billing")},
		{name: "Wire directory", got: WireDir(root, app), want: filepath.Join(root, "app", "billing", "wire")},
		{name: "frontend directory", got: FrontendDir(root, app), want: filepath.Join(root, "cmd", "billing", "frontend")},
		{name: "frontend index", got: FrontendDistIndex(root, app), want: filepath.Join(root, "cmd", "billing", "frontend", "dist", "index.html")},
		{name: "runtime executable", got: RuntimeExecutable(root, app), want: filepath.ToSlash(filepath.Join(root, "bin", "billing"))},
		{name: "runtime binary", got: RuntimeBinary(root, app), want: filepath.Join(root, "bin", "billing")},
		{name: "runtime ready stamp", got: RuntimeReadyStamp(root, app), want: filepath.ToSlash(filepath.Join(root, "bin", ".billing.ready"))},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("path = %q, want %q", test.got, test.want)
			}
		})
	}
}

// TestCurrentRootPathsPreserveCommandSpelling verifies migration to explicit roots does not churn generated command strings.
func TestCurrentRootPathsPreserveCommandSpelling(t *testing.T) {
	app := project.DefaultApp()
	if got := Entrypoint(".", app); got != "cmd/app/main.go" {
		t.Fatalf("Entrypoint() = %q", got)
	}
	if got := RuntimeExecutable(".", app); got != "./bin/app" {
		t.Fatalf("RuntimeExecutable() = %q", got)
	}
	if got := RuntimeBinary(".", app); got != filepath.Join("bin", "app") {
		t.Fatalf("RuntimeBinary() = %q", got)
	}
	if got := RuntimeReadyStamp(".", app); got != "./bin/.app.ready" {
		t.Fatalf("RuntimeReadyStamp() = %q", got)
	}
	overridden := project.App{
		Name:       "billing",
		Entrypoint: "./services/billing/main.go",
		AppDir:     "./services/billing",
		WireDir:    "./services/billing/wire",
	}
	if got := Entrypoint(".", overridden); got != overridden.Entrypoint {
		t.Fatalf("overridden Entrypoint() = %q, want %q", got, overridden.Entrypoint)
	}
	if got := AppDir(".", overridden); got != overridden.AppDir {
		t.Fatalf("overridden AppDir() = %q, want %q", got, overridden.AppDir)
	}
	if got := WireDir(".", overridden); got != overridden.WireDir {
		t.Fatalf("overridden WireDir() = %q, want %q", got, overridden.WireDir)
	}
}

// TestDiscoverUsesConventionalOwnershipMarkers verifies discovery is rooted, filtered, deduplicated, and sorted.
func TestDiscoverUsesConventionalOwnershipMarkers(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{
		filepath.Join("cmd", "reporting", "main.go"),
		filepath.Join("cmd", "customer-portal", "main.go"),
		filepath.Join("app", "customer-portal", "wire", "wire.go"),
		filepath.Join("app", "billing", "routes.go"),
		filepath.Join("app", "unowned", "models.go"),
		filepath.Join("app", "wire", "wire.go"),
		filepath.Join("cmd", "invalid_name", "main.go"),
	} {
		writeLayoutTestFile(t, filepath.Join(root, path))
	}

	discovery, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	apps := discovery.NamedApps()
	want := []project.App{
		project.DefaultNamedApp("billing"),
		project.DefaultNamedApp("customer-portal"),
		project.DefaultNamedApp("reporting"),
	}
	if !reflect.DeepEqual(apps, want) {
		t.Fatalf("Discovery.NamedApps() = %#v, want %#v", apps, want)
	}
}

// TestAppInventoriesKeepTheirDistinctSources verifies render/dev discovery stays physical while runtime inventory includes configured and pending Apps.
func TestAppInventoriesKeepTheirDistinctSources(t *testing.T) {
	root := t.TempDir()
	writeLayoutTestFile(t, filepath.Join(root, "cmd", "billing", "main.go"))
	config := &project.Config{Apps: map[string]project.AppConfig{
		"accounts":  {},
		"reporting": {},
	}}
	pendingBilling := project.App{
		Name:       "billing",
		Entrypoint: filepath.Join("services", "billing", "main.go"),
		AppDir:     filepath.Join("services", "billing"),
		WireDir:    filepath.Join("services", "billing", "wire"),
	}

	discovery, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	conventional := discovery.ConventionalApps()
	wantConventional := []project.App{project.DefaultApp(), project.DefaultNamedApp("billing")}
	if !reflect.DeepEqual(conventional, wantConventional) {
		t.Fatalf("ConventionalApps() = %#v, want %#v", conventional, wantConventional)
	}

	runtimeApps := discovery.RuntimeApps(config, pendingBilling, project.DefaultNamedApp("worker"))
	wantNames := []string{"app", "accounts", "billing", "reporting", "worker"}
	if len(runtimeApps) != len(wantNames) {
		t.Fatalf("RuntimeApps() = %#v, want names %#v", runtimeApps, wantNames)
	}
	for index, name := range wantNames {
		if runtimeApps[index].Name != name {
			t.Fatalf("RuntimeApps()[%d].Name = %q, want %q", index, runtimeApps[index].Name, name)
		}
	}
	if runtimeApps[2] != pendingBilling {
		t.Fatalf("pending App paths = %#v, want %#v", runtimeApps[2], pendingBilling)
	}
}

// TestDiscoverReturnsFilesystemErrors verifies callers can distinguish an empty layout from an inventory that could not be inspected completely.
func TestDiscoverReturnsFilesystemErrors(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, root string)
		wantError string
		wantApps  []project.App
	}{
		{
			name: "command root is not a directory",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeLayoutTestFile(t, filepath.Join(root, "cmd"))
				writeLayoutTestFile(t, filepath.Join(root, "app", "billing", "routes.go"))
			},
			wantError: "discover Apps in",
			wantApps:  []project.App{project.DefaultApp(), project.DefaultNamedApp("billing")},
		},
		{
			name: "App root is not a directory",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeLayoutTestFile(t, filepath.Join(root, "cmd", "billing", "main.go"))
				writeLayoutTestFile(t, filepath.Join(root, "app"))
			},
			wantError: "discover Apps in",
			wantApps:  []project.App{project.DefaultApp(), project.DefaultNamedApp("billing")},
		},
		{
			name: "command marker cannot be inspected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				marker := filepath.Join(root, "cmd", "billing", "main.go")
				if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
					t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(marker), err)
				}
				if err := os.Symlink("main.go", marker); err != nil {
					t.Fatalf("Symlink(%s) error = %v", marker, err)
				}
			},
			wantError: "inspect App marker",
			wantApps:  []project.App{project.DefaultApp()},
		},
		{
			name: "App marker cannot be inspected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				marker := filepath.Join(root, "app", "billing", "routes.go")
				if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
					t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(marker), err)
				}
				if err := os.Symlink("routes.go", marker); err != nil {
					t.Fatalf("Symlink(%s) error = %v", marker, err)
				}
			},
			wantError: "inspect App marker",
			wantApps:  []project.App{project.DefaultApp()},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			test.setup(t, root)

			discovery, err := Discover(root)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Discover() error = %v, want containing %q", err, test.wantError)
			}
			if got := discovery.ConventionalApps(); !reflect.DeepEqual(got, test.wantApps) {
				t.Fatalf("partial Discovery.ConventionalApps() = %#v, want %#v", got, test.wantApps)
			}

			if got := ConventionalApps(root); !reflect.DeepEqual(got, test.wantApps) {
				t.Fatalf("best-effort ConventionalApps() = %#v, want %#v", got, test.wantApps)
			}
		})
	}
}

// writeLayoutTestFile creates one ownership marker without relying on process-wide working-directory changes.
func writeLayoutTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("package app\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

package build

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/goforj/goforj/internal/appassets"
)

// TestAppAssetCommandWritersHideSuccessfulPreparation verifies npm's routine success chatter stays behind build progress.
func TestAppAssetCommandWritersHideSuccessfulPreparation(t *testing.T) {
	var visibleStdout bytes.Buffer
	var visibleStderr bytes.Buffer
	stdout, stderr, reveal := appAssetCommandWriters("prepare", &visibleStdout, &visibleStderr)
	_, _ = io.WriteString(stdout, "up to date in 305ms\n")
	_, _ = io.WriteString(stderr, "dependency warning\n")
	if visibleStdout.Len() != 0 || visibleStderr.Len() != 0 {
		t.Fatalf("successful preparation output was visible: stdout=%q stderr=%q", visibleStdout.String(), visibleStderr.String())
	}
	reveal()
	if visibleStdout.String() != "up to date in 305ms\n" || visibleStderr.String() != "dependency warning\n" {
		t.Fatalf("revealed preparation output = stdout %q stderr %q", visibleStdout.String(), visibleStderr.String())
	}
}

// TestAppAssetCommandWritersLeaveBuildOutputVisible verifies frontend compiler diagnostics retain their existing streaming behavior.
func TestAppAssetCommandWritersLeaveBuildOutputVisible(t *testing.T) {
	var visibleStdout bytes.Buffer
	var visibleStderr bytes.Buffer
	stdout, stderr, reveal := appAssetCommandWriters("build", &visibleStdout, &visibleStderr)
	_, _ = io.WriteString(stdout, "built\n")
	_, _ = io.WriteString(stderr, "warning\n")
	reveal()
	if visibleStdout.String() != "built\n" || visibleStderr.String() != "warning\n" {
		t.Fatalf("build output = stdout %q stderr %q", visibleStdout.String(), visibleStderr.String())
	}
}

// TestPrepareAppAssetsBuildsOnlyWhenMetadataIsStale verifies the transparent fast path and successful receipt boundary.
func TestPrepareAppAssetsBuildsOnlyWhenMetadataIsStale(t *testing.T) {
	root := t.TempDir()
	asset := appassets.Asset{App: "app", Name: "frontend", Root: filepath.Join("frontend"), Prepare: "frontend-install", Command: "frontend-build"}
	writeBuildAssetTestFile(t, root, "frontend/src/app.ts", "app")
	commands := make([]string, 0, 2)
	runner := func(root string, asset appassets.Asset, phase string, command string) error {
		commands = append(commands, phase+":"+command)
		if phase == "build" {
			writeBuildAssetTestFile(t, root, "frontend/dist/app.js", "built")
		}
		return nil
	}

	status, err := prepareAppAssetsWithRunner(root, []appassets.Asset{asset}, runner)
	if err != nil {
		t.Fatalf("prepare stale asset: %v", err)
	}
	if status != "1 built" || len(commands) != 2 || commands[0] != "prepare:frontend-install" || commands[1] != "build:frontend-build" {
		t.Fatalf("first preparation = %q with commands %v, want dependency setup then build", status, commands)
	}
	status, err = prepareAppAssetsWithRunner(root, []appassets.Asset{asset}, runner)
	if err != nil {
		t.Fatalf("prepare current asset: %v", err)
	}
	if status != "current" || len(commands) != 2 {
		t.Fatalf("second preparation = %q with commands %v, want cache hit", status, commands)
	}
}

// TestPrepareAppAssetsDoesNotAdvanceFailedWork proves a failed command is retried on the next build.
func TestPrepareAppAssetsDoesNotAdvanceFailedWork(t *testing.T) {
	root := t.TempDir()
	asset := appassets.Asset{App: "app", Name: "frontend", Root: filepath.Join("frontend"), Command: "frontend-build"}
	writeBuildAssetTestFile(t, root, "frontend/src/app.ts", "app")
	wantErr := errors.New("frontend failed")
	if _, err := prepareAppAssetsWithRunner(root, []appassets.Asset{asset}, func(string, appassets.Asset, string, string) error {
		return wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("prepare failed asset error = %v, want %v", err, wantErr)
	}
	built := 0
	if _, err := prepareAppAssetsWithRunner(root, []appassets.Asset{asset}, func(root string, asset appassets.Asset, phase string, command string) error {
		if phase == "build" {
			built++
			writeBuildAssetTestFile(t, root, "frontend/dist/app.js", "built")
		}
		return nil
	}); err != nil {
		t.Fatalf("retry failed asset: %v", err)
	}
	if built != 1 {
		t.Fatalf("failed asset retry builds = %d, want 1", built)
	}
}

// TestPrepareAppAssetsStopsAfterDependencyFailure verifies missing tooling cannot fall through to a misleading build attempt or receipt.
func TestPrepareAppAssetsStopsAfterDependencyFailure(t *testing.T) {
	root := t.TempDir()
	asset := appassets.Asset{App: "app", Name: "frontend", Root: "frontend", Prepare: "npm install", Command: "npm run build"}
	writeBuildAssetTestFile(t, root, "frontend/src/app.ts", "app")
	wantErr := errors.New("dependency install failed")
	commands := make([]string, 0, 1)
	_, err := prepareAppAssetsWithRunner(root, []appassets.Asset{asset}, func(root string, asset appassets.Asset, phase string, command string) error {
		commands = append(commands, phase+":"+command)
		return wantErr
	})
	if !errors.Is(err, wantErr) || len(commands) != 1 || commands[0] != "prepare:npm install" {
		t.Fatalf("dependency failure = %v with commands %v, want prepare failure only", err, commands)
	}
	current, currentErr := appassets.Current(root, asset)
	if currentErr != nil {
		t.Fatalf("inspect failed dependency receipt: %v", currentErr)
	}
	if current {
		t.Fatal("failed dependency setup published a current receipt")
	}
}

// TestConfiguredAppAssetsIncludesEveryAppAndConfiguredCommand verifies one build sees the complete configured frontend set.
func TestConfiguredAppAssetsIncludesEveryAppAndConfiguredCommand(t *testing.T) {
	root := t.TempDir()
	config := `dev:
  apps:
    app:
      spas:
        frontend: ./cmd/app/frontend
    admin:
      spas:
        dashboard:
          path: ./cmd/admin/dashboard
          build: pnpm build
`
	writeBuildAssetTestFile(t, root, ".goforj.yml", config)
	assets, err := configuredAppAssets(root)
	if err != nil {
		t.Fatalf("load App assets: %v", err)
	}
	if len(assets) != 2 || assets[0].App != "admin" || assets[0].Name != "dashboard" || assets[0].Prepare != "" || assets[0].Command != "pnpm build" ||
		assets[1].App != "app" || assets[1].Name != "frontend" || assets[1].Prepare != conventionalSPAInstallCommand || assets[1].Command != conventionalSPABuildCommand {
		t.Fatalf("configured assets = %#v", assets)
	}
}

// writeBuildAssetTestFile creates one build fixture beneath its explicit root.
func writeBuildAssetTestFile(t *testing.T, root string, relative string, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create %s parent: %v", relative, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", relative, err)
	}
}

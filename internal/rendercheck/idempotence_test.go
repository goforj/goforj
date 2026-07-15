package rendercheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestCaptureAppOwnedFileSnapshotsMarksRepresentativeFiles proves the sentinel stays intentionally narrow across default and named Apps.
func TestCaptureAppOwnedFileSnapshotsMarksRepresentativeFiles(t *testing.T) {
	root := t.TempDir()
	apps := []project.App{project.DefaultApp(), project.DefaultNamedApp("worker")}
	wantPaths := []string{
		filepath.Join("app", "lifecycle.go"),
		filepath.Join("app", "wire", "inject_services_app.go"),
		filepath.Join("app", "worker", "lifecycle.go"),
		filepath.Join("app", "worker", "wire", "inject_services_app.go"),
		filepath.Join("app", "worker", "wire", "inject_jobs_app.go"),
	}
	for _, path := range wantPaths {
		fullPath := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("create owner directory: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte("package owner\n"), 0o644); err != nil {
			t.Fatalf("write owner fixture: %v", err)
		}
	}

	snapshots, err := captureAppOwnedFileSnapshots(root, apps)
	if err != nil {
		t.Fatalf("captureAppOwnedFileSnapshots() error: %v", err)
	}
	if len(snapshots) != len(wantPaths) {
		t.Fatalf("snapshot count = %d, want %d", len(snapshots), len(wantPaths))
	}
	for index, snapshot := range snapshots {
		if snapshot.path != wantPaths[index] {
			t.Fatalf("snapshot[%d] path = %q, want %q", index, snapshot.path, wantPaths[index])
		}
		contents, err := os.ReadFile(filepath.Join(root, snapshot.path))
		if err != nil {
			t.Fatalf("read marked owner file: %v", err)
		}
		if string(contents) != string(snapshot.contents) || !strings.Contains(string(contents), appOwnedIdempotenceMarker) {
			t.Fatalf("owner file %s was not captured after marking:\n%s", snapshot.path, contents)
		}
	}
}

// TestValidateRenderedFilesUnchangedRejectsOwnerDrift keeps overwrite failures tied to the exact App-owned path.
func TestValidateRenderedFilesUnchangedRejectsOwnerDrift(t *testing.T) {
	root := t.TempDir()
	app := project.DefaultApp()
	for _, path := range []string{
		filepath.Join(app.AppDir, "lifecycle.go"),
		filepath.Join(app.WireDir, "inject_services_app.go"),
	} {
		fullPath := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("create owner directory: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte("package owner\n"), 0o644); err != nil {
			t.Fatalf("write owner fixture: %v", err)
		}
	}

	snapshots, err := captureAppOwnedFileSnapshots(root, []project.App{app})
	if err != nil {
		t.Fatalf("captureAppOwnedFileSnapshots() error: %v", err)
	}
	if err := validateRenderedFilesUnchanged(root, snapshots); err != nil {
		t.Fatalf("unchanged owner files failed validation: %v", err)
	}
	driftedPath := snapshots[0].path
	if err := os.WriteFile(filepath.Join(root, driftedPath), []byte("package overwritten\n"), 0o644); err != nil {
		t.Fatalf("rewrite owner fixture: %v", err)
	}
	if err := validateRenderedFilesUnchanged(root, snapshots); err == nil || !strings.Contains(err.Error(), driftedPath) {
		t.Fatalf("owner drift error = %v, want path %s", err, driftedPath)
	}
}

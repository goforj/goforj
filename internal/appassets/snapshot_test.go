package appassets

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// BenchmarkSnapshotCurrent measures the steady metadata-only path across a representative source tree.
func BenchmarkSnapshotCurrent(b *testing.B) {
	root := b.TempDir()
	asset := Asset{App: "app", Name: "frontend", Root: "frontend", Command: "vite build"}
	for index := 0; index < 1000; index++ {
		writeSnapshotBenchmarkFile(b, root, filepath.Join("frontend", "src", fmt.Sprintf("component-%04d.ts", index)), "export const value = 1")
	}
	writeSnapshotBenchmarkFile(b, root, filepath.Join("frontend", "dist", "app.js"), "compiled")
	writeSnapshotBenchmarkFile(b, root, filepath.Join("frontend", "node_modules", "package", "index.js"), "ignored")
	if err := Record(root, asset); err != nil {
		b.Fatalf("record snapshot: %v", err)
	}
	b.ResetTimer()
	for b.Loop() {
		current, err := Current(root, asset)
		if err != nil {
			b.Fatalf("inspect snapshot: %v", err)
		}
		if !current {
			b.Fatal("unchanged benchmark asset was stale")
		}
	}
}

// TestSnapshotTracksInputAndOutputMetadata verifies ordinary source and emitted-file changes invalidate successful work.
func TestSnapshotTracksInputAndOutputMetadata(t *testing.T) {
	root := t.TempDir()
	asset := Asset{App: "app", Name: "frontend", Root: filepath.Join("cmd", "app", "frontend"), Command: "npm run build"}
	writeSnapshotTestFile(t, root, "cmd/app/frontend/src/app.ts", "export const value = 1")
	writeSnapshotTestFile(t, root, "cmd/app/frontend/dist/app.js", "compiled")

	current, err := Current(root, asset)
	if err != nil {
		t.Fatalf("inspect missing snapshot: %v", err)
	}
	if current {
		t.Fatal("asset without a successful snapshot was current")
	}
	if err := Record(root, asset); err != nil {
		t.Fatalf("record snapshot: %v", err)
	}
	requireSnapshotCurrent(t, root, asset, true)

	source := filepath.Join(root, "cmd", "app", "frontend", "src", "app.ts")
	writeSnapshotTestFile(t, root, "cmd/app/frontend/src/app.ts", "export const value = 2")
	changedAt := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(source, changedAt, changedAt); err != nil {
		t.Fatalf("advance source time: %v", err)
	}
	requireSnapshotCurrent(t, root, asset, false)

	if err := Record(root, asset); err != nil {
		t.Fatalf("record changed source: %v", err)
	}
	output := filepath.Join(root, "cmd", "app", "frontend", "dist", "app.js")
	writeSnapshotTestFile(t, root, "cmd/app/frontend/dist/app.js", "externally changed")
	if err := os.Chtimes(output, changedAt.Add(time.Second), changedAt.Add(time.Second)); err != nil {
		t.Fatalf("advance output time: %v", err)
	}
	requireSnapshotCurrent(t, root, asset, false)
}

// TestSnapshotRejectsUnsafeIdentityAndEscapingRoot covers both receipt and source path containment boundaries.
func TestSnapshotRejectsUnsafeIdentityAndEscapingRoot(t *testing.T) {
	root := t.TempDir()
	unsafeIdentity := Asset{App: "../app", Name: "frontend", Root: "frontend", Command: "vite build"}
	if _, err := Current(root, unsafeIdentity); err == nil {
		t.Fatal("Current accepted an unsafe App identity")
	}
	escapingRoot := Asset{App: "app", Name: "frontend", Root: filepath.Join("..", "frontend"), Command: "vite build"}
	if _, err := Current(root, escapingRoot); err == nil {
		t.Fatal("Current accepted a frontend outside the Project root")
	}
}

// TestSnapshotDetectsPathSetAndCommandChanges covers additions, removals, and build-contract changes.
func TestSnapshotDetectsPathSetAndCommandChanges(t *testing.T) {
	root := t.TempDir()
	asset := Asset{App: "app", Name: "frontend", Root: "frontend", Command: "vite build"}
	writeSnapshotTestFile(t, root, "frontend/src/app.ts", "app")
	writeSnapshotTestFile(t, root, "frontend/dist/app.js", "dist")
	if err := Record(root, asset); err != nil {
		t.Fatalf("record snapshot: %v", err)
	}

	writeSnapshotTestFile(t, root, "frontend/src/new.ts", "new")
	requireSnapshotCurrent(t, root, asset, false)
	if err := os.Remove(filepath.Join(root, "frontend", "src", "new.ts")); err != nil {
		t.Fatalf("remove added source: %v", err)
	}
	requireSnapshotCurrent(t, root, asset, true)

	changedCommand := asset
	changedCommand.Command = "vite build --mode staging"
	requireSnapshotCurrent(t, root, changedCommand, false)
	changedPrepare := asset
	changedPrepare.Prepare = "npm install"
	requireSnapshotCurrent(t, root, changedPrepare, false)
}

// TestSnapshotPrunesDependenciesAndSeparatesDist verifies dependency churn is ignored while output remains validated.
func TestSnapshotPrunesDependenciesAndSeparatesDist(t *testing.T) {
	root := t.TempDir()
	asset := Asset{App: "app", Name: "frontend", Root: "frontend", Command: "vite build"}
	writeSnapshotTestFile(t, root, "frontend/src/app.ts", "app")
	writeSnapshotTestFile(t, root, "frontend/node_modules/pkg/index.js", "dependency")
	writeSnapshotTestFile(t, root, "frontend/dist/app.js", "dist")
	if err := Record(root, asset); err != nil {
		t.Fatalf("record snapshot: %v", err)
	}

	writeSnapshotTestFile(t, root, "frontend/node_modules/pkg/index.js", "changed dependency")
	requireSnapshotCurrent(t, root, asset, true)
	if err := os.RemoveAll(filepath.Join(root, "frontend", "dist")); err != nil {
		t.Fatalf("remove dist: %v", err)
	}
	requireSnapshotCurrent(t, root, asset, false)
}

// TestRecordRejectsMissingAndEmptyOutputs ensures successful commands cannot publish unusable receipts.
func TestRecordRejectsMissingAndEmptyOutputs(t *testing.T) {
	root := t.TempDir()
	asset := Asset{App: "app", Name: "frontend", Root: "frontend", Command: "vite build"}
	writeSnapshotTestFile(t, root, "frontend/src/app.ts", "app")
	if err := Record(root, asset); err == nil {
		t.Fatal("record accepted a missing dist directory")
	}
	if err := os.MkdirAll(filepath.Join(root, "frontend", "dist"), 0o755); err != nil {
		t.Fatalf("create empty dist: %v", err)
	}
	if err := Record(root, asset); err == nil {
		t.Fatal("record accepted an empty dist directory")
	}
}

// TestCorruptSnapshotBecomesCacheMiss verifies damaged local state cannot produce a false cache hit.
func TestCorruptSnapshotBecomesCacheMiss(t *testing.T) {
	root := t.TempDir()
	asset := Asset{App: "app", Name: "frontend", Root: "frontend", Command: "vite build"}
	writeSnapshotTestFile(t, root, "frontend/src/app.ts", "app")
	writeSnapshotTestFile(t, root, "frontend/dist/app.js", "dist")
	paths, err := resolvePaths(root, asset)
	if err != nil {
		t.Fatalf("resolve paths: %v", err)
	}
	writeSnapshotTestFile(t, root, mustSnapshotRelativePath(t, root, paths.receipt), "not-json")
	requireSnapshotCurrent(t, root, asset, false)
}

// requireSnapshotCurrent keeps freshness assertions concise and includes inspection failures.
func requireSnapshotCurrent(t *testing.T, root string, asset Asset, want bool) {
	t.Helper()
	got, err := Current(root, asset)
	if err != nil {
		t.Fatalf("inspect snapshot: %v", err)
	}
	if got != want {
		t.Fatalf("Current() = %t, want %t", got, want)
	}
}

// writeSnapshotTestFile creates one fixture beneath its explicit test root.
func writeSnapshotTestFile(t *testing.T, root string, relative string, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create %s parent: %v", relative, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", relative, err)
	}
}

// mustSnapshotRelativePath converts an absolute receipt path for the shared fixture writer.
func mustSnapshotRelativePath(t *testing.T, root string, path string) string {
	t.Helper()
	relative, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatalf("resolve receipt relative path: %v", err)
	}
	return relative
}

// writeSnapshotBenchmarkFile creates one benchmark fixture using platform-native path construction.
func writeSnapshotBenchmarkFile(b *testing.B, root string, relative string, contents string) {
	b.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		b.Fatalf("create %s parent: %v", relative, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		b.Fatalf("write %s: %v", relative, err)
	}
}

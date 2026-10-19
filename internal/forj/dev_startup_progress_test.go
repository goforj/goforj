package forj

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestDevStartupProgressRecordsBoundedOrderedSpans verifies independent machine diagnostics without child output.
func TestDevStartupProgressRecordsBoundedOrderedSpans(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "startup.jsonl")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	progress, err := openDevStartupProgress(path, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	finish := progress.phase("task", "Run Docker Compose")
	finish(false)
	progress.emit("startup", "Development startup", "failed", 10)
	progress.emit("task", "Ignored after terminal event", "started", 0)
	progress.close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d records, want 3", len(lines))
	}
	for index, line := range lines {
		var event devStartupProgressEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.SchemaVersion != 1 || event.Session != "session-1" || event.Sequence != uint64(index+1) || event.ElapsedMS < 0 {
			t.Fatalf("invalid event: %#v", event)
		}
	}
}

// TestDevStartupProgressReportsConfigurationFailure exercises the real dev entrypoint before any generated process starts.
func TestDevStartupProgressReportsConfigurationFailure(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	path := filepath.Join(root, "events")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("render: [\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FORJ_DEV_EVENTS_FILE", path)
	t.Setenv("FORJ_DEV_EVENTS_SESSION", "session")
	t.Setenv("FORJ_DEV_PRESENTATION", "embedded")
	if err := (&DevCmd{}).Run(); err == nil {
		t.Fatal("malformed project accepted")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected outer startup span, got %s", data)
	}
	var event devStartupProgressEvent
	if err := json.Unmarshal([]byte(lines[1]), &event); err != nil {
		t.Fatal(err)
	}
	if event.Phase != "startup" || event.Status != "failed" {
		t.Fatalf("configuration failure missing: %#v", event)
	}
	if os.Getenv("FORJ_DEV_EVENTS_FILE") != "" || os.Getenv("FORJ_DEV_EVENTS_SESSION") != "" || os.Getenv("FORJ_DEV_PRESENTATION") != "" {
		t.Fatal("diagnostic selectors leaked into child environment")
	}
}

// TestDevStartupProgressKeepsFailureOutputOutOfEvents proves task results are useful without exporting subprocess text.
func TestDevStartupProgressKeepsFailureOutputOutOfEvents(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "events")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	progress, err := openDevStartupProgress(path, "session")
	if err != nil {
		t.Fatal(err)
	}
	err = runDevTasks([]project.DevTask{{Name: "Custom task", Cmd: "printf 'fake-private-output'; exit 9"}}, progress)
	progress.close()
	if err == nil || !strings.Contains(err.Error(), "fake-private-output") {
		t.Fatalf("normal failure diagnostic missing: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "fake-private-output") || !strings.Contains(string(data), `"status":"failed"`) {
		t.Fatalf("unsafe or incomplete event stream: %s", data)
	}
}

// TestDevStartupProgressRejectsUnsafeDestinations verifies telemetry cannot clobber an existing or indirect file.
func TestDevStartupProgressRejectsUnsafeDestinations(t *testing.T) {
	for _, failure := range []string{"relative", "missing", "nonempty", "directory", "linked", "permissions", "session"} {
		t.Run(failure, func(t *testing.T) {
			if runtime.GOOS == "windows" && (failure == "permissions" || failure == "linked") {
				t.Skip("Unix file-mode and symlink checks")
			}
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "events")
			session := "session-1"
			if err := os.WriteFile(path, nil, 0600); err != nil {
				t.Fatal(err)
			}
			switch failure {
			case "relative":
				path = "events"
			case "missing":
				path += "-missing"
			case "nonempty":
				if err := os.WriteFile(path, []byte("keep me"), 0600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				path = root
			case "linked":
				link := filepath.Join(root, "link")
				if err := os.Symlink(path, link); err != nil {
					t.Fatal(err)
				}
				path = link
			case "permissions":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "session":
				session = "../wrong"
			}
			if stream, err := openDevStartupProgress(path, session); err == nil {
				stream.close()
				t.Fatal("unsafe destination accepted")
			}
		})
	}
}

// TestDevStartupProgressDisabledAndBrokenStreamsDoNotFailWork covers the optional consumer boundary.
func TestDevStartupProgressDisabledAndBrokenStreamsDoNotFailWork(t *testing.T) {
	progress, err := openDevStartupProgress("", "")
	if progress != nil || err != nil {
		t.Fatalf("disabled progress: %v %v", progress, err)
	}
	progress.phase("task", "Optional")(true)
	progress.close()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "events")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	progress, err = openDevStartupProgress(path, "session")
	if err != nil {
		t.Fatal(err)
	}
	progress.close()
	progress.emit("task", "Task", "started", 0)
	if !progress.disabled {
		t.Fatal("broken stream not disabled")
	}
}

// TestDevStartupProgressBoundsRecordsAndLabels verifies unbounded task configuration cannot grow the stream.
func TestDevStartupProgressBoundsRecordsAndLabels(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "events")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	progress, err := openDevStartupProgress(path, "session")
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 300; index++ {
		progress.emit("task", strings.Repeat("x", 500), "started", 0)
	}
	progress.close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 64*1024 || strings.Count(string(data), "\n") > 256 || strings.Contains(string(data), strings.Repeat("x", 500)) {
		t.Fatal("startup event bounds were not enforced")
	}
}

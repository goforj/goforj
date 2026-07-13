package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadStatusUsesNewestManifest(t *testing.T) {
	root := t.TempDir()
	created := time.Date(2026, time.July, 12, 10, 0, 0, 0, time.UTC)
	for i, stamp := range []time.Time{created.Add(-time.Hour), created} {
		dir := filepath.Join(root, "backup", stamp.Format("150405"))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := WriteManifest(dir, Manifest{Version: 1, CreatedAt: stamp, Resources: []Resource{{ID: string(rune('a' + i)), Artifact: "artifact"}}}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "artifact"), []byte("ok"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	status, err := ReadStatus(filepath.Join(root, "backup"), created.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !status.Found || !status.CreatedAt.Equal(created) || status.Resources != 1 {
		t.Fatalf("unexpected status: %#v", status)
	}
	if !strings.Contains(FormatStatus(status), "status=ok") {
		t.Fatal("expected ok status")
	}
}

func TestHookRegistryRunsRegisteredHooks(t *testing.T) {
	called := []HookEvent{}
	registry := HookRegistry{BeforeCreate: []Hook{func(_ context.Context, event HookEvent) error { called = append(called, event); return nil }}}
	if err := registry.Run(context.Background(), HookBeforeCreate); err != nil {
		t.Fatal(err)
	}
	if len(called) != 1 || called[0] != HookBeforeCreate {
		t.Fatalf("hooks = %#v", called)
	}
}

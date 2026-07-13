package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrunePolicyKeepsConfiguredCalendarBuckets(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, time.July, 12, 12, 0, 0, 0, time.UTC)
	for i, created := range []time.Time{now, now.Add(-time.Hour), now.Add(-8 * 24 * time.Hour), now.Add(-40 * 24 * time.Hour)} {
		dir := filepath.Join(root, string(rune('a'+i)))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := WriteManifest(dir, Manifest{Version: 1, CreatedAt: created, Resources: []Resource{{ID: "db.main", Artifact: "x"}}}); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := PrunePolicy(root, RetentionPolicy{Daily: 1, Weekly: 1, Monthly: 1}, now, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 {
		t.Fatalf("removed %d backups, want 1", len(removed))
	}
}

package scenarios

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestDigestScenarioTreeContextProducesStableDigest keeps the bounded implementation compatible with the original digest contract.
func TestDigestScenarioTreeContextProducesStableDigest(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fixture.txt"), []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacy, err := digestScenarioTree(root)
	if err != nil {
		t.Fatal(err)
	}
	bounded, err := digestScenarioTreeContext(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if bounded != legacy {
		t.Fatalf("digestScenarioTreeContext() = %s, want %s", bounded, legacy)
	}
}

// TestDigestScenarioTreeContextStopsOnCancellation ensures tree verification does not continue after evaluation cancellation.
func TestDigestScenarioTreeContextStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := digestScenarioTreeContext(ctx, t.TempDir())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("digestScenarioTreeContext() error = %v, want context cancellation", err)
	}
}

// TestClonePreparedContextStopsOnCancellation ensures candidate copies observe the evaluation context before creating a workspace.
func TestClonePreparedContextStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ClonePreparedContext(ctx, &PreparedScenario{Root: t.TempDir(), ScenarioID: "fixture"}, t.TempDir())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ClonePreparedContext() error = %v, want context cancellation", err)
	}
}

// TestDigestScenarioTreeContextRejectsTooManyEntries prevents an oversized project from growing the digest path list without bound.
func TestDigestScenarioTreeContextRejectsTooManyEntries(t *testing.T) {
	root := t.TempDir()
	for index := 0; index <= scenarioTreeEntryLimit; index++ {
		path := filepath.Join(root, strconv.Itoa(index))
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, err := digestScenarioTreeContext(context.Background(), root)
	if !errors.Is(err, ErrScenarioTreeEntryLimit) {
		t.Fatalf("digestScenarioTreeContext() error = %v, want entry limit", err)
	}
}

// TestClonePreparedContextRejectsTooManyBytes avoids copying a tree beyond the supported project size.
func TestClonePreparedContextRejectsTooManyBytes(t *testing.T) {
	root := t.TempDir()
	file, err := os.Create(filepath.Join(root, "oversized"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(scenarioTreeByteLimit + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = ClonePreparedContext(context.Background(), &PreparedScenario{Root: root, ScenarioID: "fixture"}, t.TempDir())
	if !errors.Is(err, ErrScenarioTreeByteLimit) {
		t.Fatalf("ClonePreparedContext() error = %v, want byte limit", err)
	}
}

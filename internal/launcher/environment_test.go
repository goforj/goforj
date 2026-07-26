package launcher

import "testing"

// TestSetAndSnapshotCopyValues keeps both the caller and snapshot maps outside the shared state boundary.
func TestSetAndSnapshotCopyValues(t *testing.T) {
	previous := Snapshot()
	t.Cleanup(func() {
		set(previous)
	})

	values := map[string]string{"PROCESS_VALUE": "initial"}
	set(values)
	values["PROCESS_VALUE"] = "caller-change"

	snapshot := Snapshot()
	if got := snapshot["PROCESS_VALUE"]; got != "initial" {
		t.Fatalf("stored value = %q, want initial", got)
	}
	snapshot["PROCESS_VALUE"] = "snapshot-change"
	if got := Snapshot()["PROCESS_VALUE"]; got != "initial" {
		t.Fatalf("shared value = %q, want initial", got)
	}
}

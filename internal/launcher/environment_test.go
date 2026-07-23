package launcher

import "testing"

// TestSetAndSnapshotCopyValues keeps both the caller and snapshot maps outside the shared state boundary.
func TestSetAndSnapshotCopyValues(t *testing.T) {
	environment := Provide()
	if environment != Provide() {
		t.Fatal("Provide returned different environment instances")
	}
	previous := environment.Snapshot()
	t.Cleanup(func() {
		environment.Set(previous)
	})

	values := map[string]string{"PROCESS_VALUE": "initial"}
	environment.Set(values)
	values["PROCESS_VALUE"] = "caller-change"

	snapshot := environment.Snapshot()
	if got := snapshot["PROCESS_VALUE"]; got != "initial" {
		t.Fatalf("stored value = %q, want initial", got)
	}
	snapshot["PROCESS_VALUE"] = "snapshot-change"
	if got := environment.Snapshot()["PROCESS_VALUE"]; got != "initial" {
		t.Fatalf("shared value = %q, want initial", got)
	}
}

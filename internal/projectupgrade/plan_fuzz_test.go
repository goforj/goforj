package projectupgrade

import (
	"encoding/json"
	"reflect"
	"testing"
)

// FuzzSnapshotValidation exercises malformed protocol inputs without opening application files or invoking renderer tools.
func FuzzSnapshotValidation(f *testing.F) {
	seed, _ := json.Marshal(testSnapshot("package auth\nfunc Login(){}\n"))
	f.Add(seed)
	f.Add([]byte(`{"protocol":3,"artifacts":[null]}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, source []byte) {
		if len(source) > 64<<10 {
			t.Skip()
		}
		var snapshot Snapshot
		if json.Unmarshal(source, &snapshot) != nil {
			return
		}
		if ValidateSnapshot(snapshot) != nil {
			return
		}
		encoded, err := json.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		var roundTrip Snapshot
		if err := json.Unmarshal(encoded, &roundTrip); err != nil {
			t.Fatal(err)
		}
		if err := ValidateSnapshot(roundTrip); err != nil {
			t.Fatal("validated contract did not survive serialization", err)
		}
	})
}

// FuzzDeclarationHints checks symmetry and stability for valid and malformed Go source, including directive-like strings.
func FuzzDeclarationHints(f *testing.F) {
	f.Add("package app\nfunc Login(){}\n", "package app\nfunc Login(){verify()}\n")
	f.Add("//go:build linux\n\npackage app\n", "//go:build windows\n\npackage app\n")
	f.Add("package app\nconst S = `//go:embed fake`\n", "not Go")
	f.Fuzz(func(t *testing.T, before, after string) {
		if len(before)+len(after) > 64<<10 {
			t.Skip()
		}
		forward, invalidForward := changedDeclarations(before, after)
		backward, invalidBackward := changedDeclarations(after, before)
		if invalidForward != invalidBackward || !reflect.DeepEqual(forward, backward) {
			t.Fatal("declaration comparison is asymmetric")
		}
		if !invalidForward {
			same, invalid := changedDeclarations(before, before)
			if invalid || len(same) > 0 {
				t.Fatal("unchanged source produced declaration changes")
			}
		}
	})
}

// FuzzSnapshotTextRoundTrip starts from raw Go strings so malformed UTF-8 cannot hide behind JSON decoding's replacements.
func FuzzSnapshotTextRoundTrip(f *testing.F) {
	f.Add([]byte("package auth\n// 世界\n"), "dev-試験")
	f.Add([]byte{0xff}, "dev")
	f.Add([]byte("package auth\n"), string([]byte{0xff}))
	f.Fuzz(func(t *testing.T, source []byte, version string) {
		if len(source)+len(version) > 64<<10 {
			t.Skip()
		}
		snapshot := testSnapshot(string(source))
		snapshot.Engine.Version = version
		snapshot.Artifacts[0].Digest = Digest(source)
		if ValidateSnapshot(snapshot) != nil {
			return
		}
		encoded, err := json.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		var decoded Snapshot
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if err := ValidateSnapshot(decoded); err != nil {
			t.Fatal("accepted source did not survive JSON", err)
		}
		if !reflect.DeepEqual(snapshot, decoded) {
			t.Fatal("accepted snapshot changed during JSON serialization")
		}
	})
}

package forj

import (
	"encoding/json"
	"testing"
)

// FuzzSnapshotJSONProtocol bounds malformed renderer metadata independently of generated-source payload contents.
func FuzzSnapshotJSONProtocol(f *testing.F) {
	for _, seed := range []string{`{"protocol":3}`, `{"engine":{"version":"dev","version":"other"}}`, `{"artifacts":[{"source":"//go:embed fake"}]}`, `[]`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, source []byte) {
		if len(source) > 64<<10 {
			t.Skip()
		}
		if validateSnapshotJSON(source) == nil && !json.Valid(source) {
			t.Fatal("invalid JSON passed renderer protocol preflight")
		}
	})
}

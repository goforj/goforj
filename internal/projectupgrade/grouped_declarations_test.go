package projectupgrade

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestDeclarationHintsPreserveGroupedKeys keeps every declared name in source order, including repeated blanks.
func TestDeclarationHintsPreserveGroupedKeys(t *testing.T) {
	for _, test := range []struct {
		name, before, after, key string
	}{
		{"var", "var (First, Second = 1, 2; _ = 3; _ = 4)", "var (First, Second = 1, 5; _ = 3; _ = 4)", "var First Second _ _"},
		{"const", "const (First, Second = 1, 2; Third, Fourth)", "const (First, Second = 1, 5; Third, Fourth)", "const First Second Third Fourth"},
		{"type", "type (First = int; Second[T any] struct{ Value T })", "type (First = string; Second[T any] struct{ Value T })", "type First Second"},
	} {
		t.Run(test.name, func(t *testing.T) {
			before, after := "package app\n"+test.before, "package app\n"+test.after
			fingerprints, invalid := declarations(before)
			if invalid || fingerprints[test.key] == "" {
				t.Fatalf("grouped key changed: fingerprints=%v invalid=%v", fingerprints, invalid)
			}
			wantOrder := ""
			if test.name == "var" {
				wantOrder = test.key
			}
			if got := fingerprints["variable-initialization-order"]; got != wantOrder {
				t.Fatalf("initialization order=%q, want %q", got, wantOrder)
			}
			changed, invalid := changedDeclarations(before, after)
			if invalid || !reflect.DeepEqual(changed, []string{test.key}) {
				t.Fatalf("grouped hint=%v invalid=%v, want %q", changed, invalid, test.key)
			}
		})
	}
}

// TestDeclarationHintsPreserveReceiverKeys keeps method hints stable beside grouped type declarations.
func TestDeclarationHintsPreserveReceiverKeys(t *testing.T) {
	before := "package app\ntype (First int; Second int)\nfunc (First) Value() int { return 1 }\nfunc (*Second) Value() int { return 2 }\n"
	after := strings.ReplaceAll(before, "return 1", "return 3")
	after = strings.ReplaceAll(after, "return 2", "return 4")
	changed, invalid := changedDeclarations(before, after)
	if invalid || !reflect.DeepEqual(changed, []string{"First.Value", "Second.Value"}) {
		t.Fatalf("receiver hints=%v invalid=%v", changed, invalid)
	}
}

// groupedDeclarationsBenchmarkSource exposes copying costs as the number of names in one key grows.
func groupedDeclarationsBenchmarkSource(count int) string {
	var source strings.Builder
	source.WriteString("package app\nvar (\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&source, "v%06d int\n", i)
	}
	source.WriteString(")\n")
	return source.String()
}

// BenchmarkDeclarationHintsGroupedDeclarations measures cumulative allocation for growing grouped keys.
func BenchmarkDeclarationHintsGroupedDeclarations(b *testing.B) {
	for _, count := range []int{1000, 4000, 16000} {
		source := groupedDeclarationsBenchmarkSource(count)
		b.Run(fmt.Sprintf("names=%d", count), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for b.Loop() {
				if _, invalid := declarations(source); invalid {
					b.Fatal("valid grouped variables were rejected")
				}
			}
		})
	}
}

// BenchmarkBuildGroupedDeclarations measures grouped hint costs across the complete planning path.
func BenchmarkBuildGroupedDeclarations(b *testing.B) {
	for _, count := range []int{1000, 4000, 16000} {
		source := groupedDeclarationsBenchmarkSource(count)
		b.Run(fmt.Sprintf("names=%d", count), func(b *testing.B) {
			root := b.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "internal/auth"), 0o755); err != nil {
				b.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "internal/auth/service.go"), []byte(source), 0o644); err != nil {
				b.Fatal(err)
			}
			baseline := testSnapshot(source)
			target := testSnapshot(source)
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for b.Loop() {
				if _, err := Build(root, target, &baseline); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

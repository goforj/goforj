package generate

import (
	"reflect"
	"testing"
)

// TestOutputPathsSelectsOnlyEnabledGenerators keeps upgrade ownership aligned with the actual generator publication contracts.
func TestOutputPathsSelectsOnlyEnabledGenerators(t *testing.T) {
	for _, test := range []struct {
		name      string
		selection GenerationSelection
		want      []string
	}{
		{"none", GenerationSelection{}, nil},
		{"cache", GenerationSelection{Cache: true}, []string{CacheManagerPath, CacheAccessorsPath}},
		{"queue", GenerationSelection{Queue: true}, []string{QueueManagerPath, QueueAccessorsPath}},
		{"storage", GenerationSelection{Storage: true}, []string{StorageManagerPath, StorageAccessorsPath}},
		{"events", GenerationSelection{Events: true}, []string{EventsManagerPath, EventsAccessorsPath}},
		{"mail", GenerationSelection{Mail: true}, []string{MailManagerPath, MailAccessorsPath}},
		{"database", GenerationSelection{Database: true}, []string{DatabaseConnectionsPath}},
		{"observability", GenerationSelection{Observability: true}, []string{ObservabilityTargetsPath}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := OutputPaths(test.selection); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("output contracts=%v, want %v", got, test.want)
			}
		})
	}
	selection := GenerationSelection{Cache: true, Queue: true, Storage: true, Events: true, Mail: true, Database: true, Observability: true}
	paths := OutputPaths(selection)
	seen := map[string]bool{}
	for _, name := range paths {
		if seen[name] {
			t.Fatalf("duplicate generator output %s", name)
		}
		seen[name] = true
	}
	if len(paths) != 12 {
		t.Fatalf("combined output contracts=%v", paths)
	}
	paths[0] = "owner mutation"
	if OutputPaths(selection)[0] == "owner mutation" {
		t.Fatal("output catalogue shares mutable caller state")
	}
}

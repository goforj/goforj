package localproviders

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConventionalAppFeatureRoundTrip keeps provider switches aligned with runtime App discovery without persisting inferred Apps.
func TestConventionalAppFeatureRoundTrip(t *testing.T) {
	for _, test := range []struct {
		name, marker, prefix string
	}{
		{"command", "cmd/billing/main.go", "BILLING"},
		{"composition", "app/billing/commands.go", "BILLING"},
		{"resource prefix", "cmd/queue/main.go", "QUEUE"},
		{"overlapping resource prefix", "cmd/queue-feature/main.go", "QUEUE_FEATURE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := fixture(t)
			config := "module_name: example.org/conventional\nrender:\n  components: [cli, jobs, feature]\n"
			put(t, root, ".goforj.yml", config)
			if err := os.MkdirAll(filepath.Join(root, filepath.Dir(test.marker)), 0700); err != nil {
				t.Fatal(err)
			}
			put(t, root, test.marker, "package main\n")
			if test.prefix == "QUEUE_FEATURE" {
				if err := os.MkdirAll(filepath.Join(root, "cmd/queue"), 0700); err != nil {
					t.Fatal(err)
				}
				put(t, root, "cmd/queue/main.go", "package main\n")
			}
			key := test.prefix + "_FEATURE_DRIVER"
			pathKey := test.prefix + "_FEATURE_PATH"
			put(t, root, ".env", "QUEUE_DRIVER=workerpool\nFEATURE_DRIVER=memory\n"+key+"=file\n"+pathKey+"=flags.json\n")
			session := openTest(t, root)
			original := clone(session.Current)
			portable, err := session.Portable()
			if err != nil {
				t.Fatal(err)
			}
			if portable[key] != "memory" || portable[pathKey] != "flags.json" {
				t.Fatalf("portable settings lost App ownership: %v", portable)
			}
			for _, resource := range session.Resources {
				if resource.Key == key && resource.Definition.EnvironmentPrefix != "FEATURE" {
					t.Fatalf("App feature interpreted as %s", resource.Definition.EnvironmentPrefix)
				}
				if resource.Definition.EnvironmentPrefix == "FEATURE" && resource.Key != "FEATURE_DRIVER" && resource.Key != key && !(test.prefix == "QUEUE_FEATURE" && resource.Key == "QUEUE_FEATURE_DRIVER") {
					t.Fatalf("App declaration leaked into another evaluator: %s", resource.Key)
				}
			}
			if err := session.Apply(portable); err != nil {
				t.Fatal(err)
			}
			session = openTest(t, root)
			if err := session.Restore(); err != nil {
				t.Fatal(err)
			}
			if restored := openTest(t, root); !equalValues(restored.Current, original) {
				t.Fatalf("restore changed App settings: %v", restored.Current)
			}
			body, err := os.ReadFile(filepath.Join(root, ".goforj.yml"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(body, []byte(config)) {
				t.Fatal("provider switch persisted inferred App configuration")
			}
		})
	}
}

// TestConventionalAppFeatureParticipation preserves explicit opt-outs and inherits only the default App selection.
func TestConventionalAppFeatureParticipation(t *testing.T) {
	for _, test := range []struct{ name, config string }{
		{"explicit opt-out", "render:\n  components: [cli, feature]\napps:\n  billing:\n    components: [cli]\n"},
		{"sibling opt-in", "render:\n  components: [cli]\napps:\n  reports:\n    components: [cli, feature]\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/conventional\n"+test.config)
			if err := os.MkdirAll(filepath.Join(root, "cmd/billing"), 0700); err != nil {
				t.Fatal(err)
			}
			put(t, root, "cmd/billing/main.go", "package main\n")
			put(t, root, ".env", "BILLING_FEATURE_DRIVER=file\nBILLING_FEATURE_PATH=flags.json\n")
			session := openTest(t, root)
			portable, err := session.Portable()
			if err != nil {
				t.Fatal(err)
			}
			if portable["BILLING_FEATURE_DRIVER"] != "file" {
				t.Fatal("provider switch enabled an opted-out App resource")
			}
			for _, resource := range session.Resources {
				if resource.Key == "BILLING_FEATURE_DRIVER" {
					t.Fatal("disabled App feature appeared in provider inventory")
				}
			}
		})
	}
}

// TestOpenRejectsAppDiscoveryFailure prevents provider edits based on an incomplete App ownership inventory.
func TestOpenRejectsAppDiscoveryFailure(t *testing.T) {
	root := fixture(t)
	put(t, root, "cmd", "not a directory")
	if _, err := Open(root); err == nil || !strings.Contains(err.Error(), "discover Apps") {
		t.Fatalf("expected App discovery error, got %v", err)
	}
}

package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFeatureAppKeysDoNotDeclareLegacyResources protects every legacy catalog family while preserving disabled-Feature behavior.
func TestFeatureAppKeysDoNotDeclareLegacyResources(t *testing.T) {
	for _, family := range []struct {
		prefix, component, driver string
		generate                  func(generationInput) (int, error)
		discover                  func(generationInput) []string
	}{
		{"CACHE", "cache", "memory", generateCacheFiles, discoverCacheStoreNames},
		{"DB", "database_sqlite", "sqlite", generateDBFiles, discoverDBConnectionNames},
		{"QUEUE", "jobs", "workerpool", generateQueueFiles, discoverQueueNames},
		{"EVENTS", "events", "inproc", generateEventFiles, discoverEventNames},
		{"STORAGE", "storage", "local", generateStorageFiles, discoverStorageDiskNames},
		{"MAIL", "mail", "log", generateMailFiles, discoverMailNames},
	} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/feature=%t", family.prefix, enabled), func(t *testing.T) {
				root := t.TempDir()
				appComponents := "cli"
				driver := family.driver
				if enabled {
					appComponents += ", feature"
					driver = "file"
				}
				config := fmt.Sprintf("render:\n  components: [cli, %s]\napps:\n  %s:\n    components: [%s]\n", family.component, strings.ToLower(family.prefix), appComponents)
				if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte(config), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/collision\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(root, "flags.json")
				if err := os.WriteFile(path, []byte(`{"schema":1,"flags":{}}`), 0o600); err != nil {
					t.Fatal(err)
				}
				values := map[string]string{family.prefix + "_DRIVER": family.driver, family.prefix + "_FEATURE_DRIVER": driver}
				if enabled {
					values[family.prefix+"_FEATURE_PATH"] = path
				}
				input := fixtureGenerationInput(root, values)
				if enabled {
					if _, err := generateFeatureFiles(input); err != nil {
						t.Fatalf("generate App Feature: %v", err)
					}
				}
				names := family.discover(input)
				found := false
				for _, name := range names {
					found = found || strings.EqualFold(name, "feature")
				}
				if found == enabled {
					t.Fatalf("legacy names = %v, Feature enabled=%t", names, enabled)
				}
				if _, err := family.generate(input); err != nil {
					t.Fatalf("generate legacy resource: %v", err)
				}
				if changed, err := family.generate(input); err != nil || changed != 0 {
					t.Fatalf("regeneration changed=%d, error=%v", changed, err)
				}
			})
		}
	}
}

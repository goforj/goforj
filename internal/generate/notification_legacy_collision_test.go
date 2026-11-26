package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNotificationAppKeysDoNotDeclareLegacyResources protects every legacy catalog family while preserving disabled-Notification behavior.
func TestNotificationAppKeysDoNotDeclareLegacyResources(t *testing.T) {
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
			t.Run(fmt.Sprintf("%s/notification=%t", family.prefix, enabled), func(t *testing.T) {
				root := t.TempDir()
				appComponents := "cli"
				driver := family.driver
				if enabled {
					appComponents += ", notification"
					driver = "webhook"
				}
				config := fmt.Sprintf("render:\n  components: [cli, %s]\napps:\n  %s:\n    components: [%s]\n", family.component, strings.ToLower(family.prefix), appComponents)
				if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte(config), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/collision\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				values := map[string]string{family.prefix + "_DRIVER": family.driver, family.prefix + "_NOTIFICATION_DRIVER": driver}
				if enabled {
					values[family.prefix+"_NOTIFICATION_WEBHOOK_URL"] = "https://example.com/hook"
				}
				input := fixtureGenerationInput(root, values)
				if enabled {
					if _, err := generateNotificationFiles(input); err != nil {
						t.Fatalf("generate App Notification: %v", err)
					}
				}
				names := family.discover(input)
				found := false
				for _, name := range names {
					found = found || strings.EqualFold(name, "notification")
				}
				if found == enabled {
					t.Fatalf("legacy names = %v, Notification enabled=%t", names, enabled)
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

package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// appNamedAccessorTest describes one generator's App-prefixed named-resource contract.
type appNamedAccessorTest struct {
	name              string
	activeKey         string
	baseDriver        string
	supportedKey      string
	configKey         string
	configValue       string
	generatedPath     string
	requiredAccessor  string
	configAccessor    string
	forbiddenAccessor string
	prepare           func(*testing.T, string)
	generate          func(string) (int, error)
}

// TestGenerateResourceFilesDiscoversAppOnlyNamedAccessors verifies App overlays contribute named accessors without promoting the App root overlay to a resource name.
func TestGenerateResourceFilesDiscoversAppOnlyNamedAccessors(t *testing.T) {
	for _, tt := range appNamedAccessorTests() {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.prepare(t, root)
			configureAppNamedAccessorEnvironment(t, tt)

			if _, err := tt.generate(root); err != nil {
				t.Fatalf("generate %s App-only named accessor: %v", tt.name, err)
			}

			source, err := os.ReadFile(filepath.Join(root, tt.generatedPath))
			if err != nil {
				t.Fatalf("read generated %s accessors: %v", tt.name, err)
			}
			if !strings.Contains(string(source), tt.requiredAccessor) {
				t.Fatalf("generated %s accessors do not contain %q", tt.name, tt.requiredAccessor)
			}
			if !strings.Contains(string(source), tt.configAccessor) {
				t.Fatalf("generated %s accessors do not contain config-only scope %q", tt.name, tt.configAccessor)
			}
			if strings.Contains(string(source), tt.forbiddenAccessor) {
				t.Fatalf("generated %s accessors unexpectedly contain App-root accessor %q", tt.name, tt.forbiddenAccessor)
			}
		})
	}
}

// TestGenerateResourceFilesRejectsUnknownAppNamedKeys verifies validation reports the original App-prefixed key instead of accepting an unusable runtime setting.
func TestGenerateResourceFilesRejectsUnknownAppNamedKeys(t *testing.T) {
	for _, tt := range appNamedAccessorTests() {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.prepare(t, root)
			configureAppNamedAccessorEnvironment(t, tt)
			badKey := "BILLING_" + strings.TrimSuffix(tt.activeKey, "_DRIVER") + "_REPORTS_BADKEY"
			t.Setenv(badKey, "unexpected")

			if _, err := tt.generate(root); err == nil {
				t.Fatalf("generate %s with unknown App-named key unexpectedly succeeded", tt.name)
			} else if !strings.Contains(err.Error(), badKey) {
				t.Fatalf("generation error %q does not identify %s", err, badKey)
			}
		})
	}
}

// TestGenerateResourceFilesRejectsImplicitAppNamedFallbackOutsideManifest keeps config-only scopes from selecting an uncompiled native driver.
func TestGenerateResourceFilesRejectsImplicitAppNamedFallbackOutsideManifest(t *testing.T) {
	configKeys := map[string]string{
		"cache":   "BILLING_CACHE_ARCHIVE_PREFIX",
		"events":  "BILLING_EVENTS_ARCHIVE_REDIS_CHANNEL_PREFIX",
		"mail":    "BILLING_MAIL_ARCHIVE_FROM_ADDRESS",
		"storage": "BILLING_STORAGE_ARCHIVE_PREFIX",
	}
	fallbacks := map[string]string{
		"cache":   "memory",
		"events":  "inproc",
		"mail":    "log",
		"storage": "local",
	}
	for _, tt := range resourceAppDriverManifestTests() {
		configKey, relevant := configKeys[tt.name]
		if !relevant {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.prepare(t, root)
			resourcePrefix := strings.TrimSuffix(tt.activeKey, "_DRIVER")
			unsetGenerationEnvironment(t, resourcePrefix+"_ARCHIVE_DRIVER", "BILLING_"+resourcePrefix+"_ARCHIVE_DRIVER")
			t.Setenv(tt.activeKey, tt.appDriver)
			t.Setenv(tt.supportedKey, tt.appDriver)
			t.Setenv("BILLING_"+tt.activeKey, tt.appDriver)
			t.Setenv(configKey, "configured")

			if _, err := tt.generate(root); err == nil {
				t.Fatalf("generate %s with omitted named fallback unexpectedly succeeded", tt.name)
			} else {
				for _, expected := range []string{"BILLING_" + resourcePrefix + "_ARCHIVE_DRIVER", fallbacks[tt.name], tt.supportedKey} {
					if !strings.Contains(err.Error(), expected) {
						t.Fatalf("generation error %q does not contain %q", err, expected)
					}
				}
			}
		})
	}
}

// TestGenerateDBFilesAcceptsAppPrefixedSlowQueryThreshold keeps validation aligned with root and named runtime database scopes.
func TestGenerateDBFilesAcceptsAppPrefixedSlowQueryThreshold(t *testing.T) {
	root := t.TempDir()
	prepareManifestPackage(filepath.Join("internal", "database"), false)(t, root)
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_SUPPORTED_DRIVERS", "sqlite")
	t.Setenv("BILLING_DB_DRIVER", "sqlite")
	t.Setenv("BILLING_DB_SLOW_QUERY_THRESHOLD", "500ms")
	t.Setenv("BILLING_DB_REPORTS_SLOW_QUERY_THRESHOLD", "750ms")

	if _, err := GenerateDBFiles(root); err != nil {
		t.Fatalf("generate database App slow-query settings: %v", err)
	}
}

// configureAppNamedAccessorEnvironment supplies only an App-prefixed reports scope while preserving an ordinary root selection.
func configureAppNamedAccessorEnvironment(t *testing.T, tt appNamedAccessorTest) {
	t.Helper()
	resourcePrefix := strings.TrimSuffix(tt.activeKey, "_DRIVER")
	unsetGenerationEnvironment(t, resourcePrefix+"_REPORTS_DRIVER", resourcePrefix+"_REPORTS_BADKEY")
	t.Setenv(tt.activeKey, tt.baseDriver)
	t.Setenv(tt.supportedKey, tt.baseDriver)
	t.Setenv("BILLING_"+tt.activeKey, tt.baseDriver)
	t.Setenv("BILLING_"+resourcePrefix+"_REPORTS_DRIVER", tt.baseDriver)
	t.Setenv(tt.configKey, tt.configValue)
}

// appNamedAccessorTests returns equivalent App-only named-resource cases for every generated resource manager.
func appNamedAccessorTests() []appNamedAccessorTest {
	manifestTests := resourceAppDriverManifestTests()
	tests := make([]appNamedAccessorTest, 0, len(manifestTests))
	for _, manifest := range manifestTests {
		test := appNamedAccessorTest{
			name:         manifest.name,
			activeKey:    manifest.activeKey,
			baseDriver:   manifest.baseDriver,
			supportedKey: manifest.supportedKey,
			prepare:      manifest.prepare,
			generate:     manifest.generate,
		}
		switch manifest.name {
		case "cache":
			test.configKey = "BILLING_CACHE_ARCHIVE_MEMORY_CLEANUP_SECONDS"
			test.configValue = "60"
			test.generatedPath = filepath.Join("internal", "caches", "accessors_gen.go")
			test.requiredAccessor = "func (m *Manager) Reports()"
			test.configAccessor = "func (m *Manager) Archive()"
			test.forbiddenAccessor = "func (m *Manager) Billing()"
		case "database":
			test.configKey = "BILLING_DB_ARCHIVE_DSN"
			test.configValue = "archive.db"
			test.generatedPath = filepath.Join("internal", "database", "connections_gen.go")
			test.requiredAccessor = "func (c *Connections) GetReports()"
			test.configAccessor = "func (c *Connections) GetArchive()"
			test.forbiddenAccessor = "func (c *Connections) GetBilling()"
		case "events":
			test.configKey = "BILLING_EVENTS_ARCHIVE_INPROC_WORKERS"
			test.configValue = "2"
			test.generatedPath = filepath.Join("internal", "events", "accessors_gen.go")
			test.requiredAccessor = "func (m *Manager) Reports() Bus"
			test.configAccessor = "func (m *Manager) Archive() Bus"
			test.forbiddenAccessor = "func (m *Manager) Billing() Bus"
		case "mail":
			test.configKey = "BILLING_MAIL_ARCHIVE_LOG_BODIES"
			test.configValue = "true"
			test.generatedPath = filepath.Join("internal", "mail", "accessors_gen.go")
			test.requiredAccessor = "func (m *Manager) Reports() *goforjmail.Mailer"
			test.configAccessor = "func (m *Manager) Archive() *goforjmail.Mailer"
			test.forbiddenAccessor = "func (m *Manager) Billing() *goforjmail.Mailer"
		case "queue":
			test.configKey = "BILLING_QUEUE_ARCHIVE_WORKERPOOL_WORKERS"
			test.configValue = "2"
			test.generatedPath = filepath.Join("internal", "queues", "accessors_gen.go")
			test.requiredAccessor = "func (m *Manager) Reports()"
			test.configAccessor = "func (m *Manager) Archive()"
			test.forbiddenAccessor = "func (m *Manager) Billing()"
		case "storage":
			test.configKey = "BILLING_STORAGE_ARCHIVE_ROOT"
			test.configValue = "storage/app/archive"
			test.generatedPath = filepath.Join("internal", "storages", "accessors_gen.go")
			test.requiredAccessor = "func (m *Manager) Reports() storage.Storage"
			test.configAccessor = "func (m *Manager) Archive() storage.Storage"
			test.forbiddenAccessor = "func (m *Manager) Billing() storage.Storage"
		}
		tests = append(tests, test)
	}
	return tests
}

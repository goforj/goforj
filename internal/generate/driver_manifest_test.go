package generate

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeneratedResourceManagersEmbedLocalDrivers verifies local implementations remain available beside selected external drivers.
func TestGeneratedResourceManagersEmbedLocalDrivers(t *testing.T) {
	tests := []struct {
		name           string
		activeKey      string
		activeDriver   string
		supportedKey   string
		compiledDriver string
		localDrivers   []string
		fallbackCase   string
		manifestName   string
		managerPath    string
		prepare        func(*testing.T, string)
		generate       func(string) (int, error)
	}{
		{
			name:           "cache",
			activeKey:      "CACHE_DRIVER",
			activeDriver:   "redis",
			supportedKey:   "CACHE_SUPPORTED_DRIVERS",
			compiledDriver: "redis",
			localDrivers:   []string{"file", "memory", "null"},
			fallbackCase:   "case driverMemory:",
			manifestName:   "compiledCacheDrivers",
			managerPath:    filepath.Join("internal", "caches", "manager_gen.go"),
			prepare:        prepareManifestPackage(filepath.Join("internal", "caches"), false),
			generate:       GenerateCacheFiles,
		},
		{
			name:           "queue",
			activeKey:      "QUEUE_DRIVER",
			activeDriver:   "redis",
			supportedKey:   "QUEUE_SUPPORTED_DRIVERS",
			compiledDriver: "redis",
			localDrivers:   []string{"null", "sync", "workerpool"},
			fallbackCase:   "case driverWorkerpool:",
			manifestName:   "compiledQueueDrivers",
			managerPath:    filepath.Join("internal", "queues", "manager_gen.go"),
			prepare:        prepareManifestPackage(filepath.Join("internal", "queues"), true),
			generate:       GenerateQueueFiles,
		},
		{
			name:           "events",
			activeKey:      "EVENTS_DRIVER",
			activeDriver:   "redis",
			supportedKey:   "EVENTS_SUPPORTED_DRIVERS",
			compiledDriver: "redis",
			localDrivers:   []string{"inproc", "null"},
			fallbackCase:   "activeDriverForScope(scope) == DriverInproc",
			manifestName:   "compiledEventDrivers",
			managerPath:    filepath.Join("internal", "events", "manager_gen.go"),
			prepare:        prepareManifestPackage(filepath.Join("internal", "events"), false),
			generate:       GenerateEventFiles,
		},
		{
			name:           "storage",
			activeKey:      "STORAGE_DRIVER",
			activeDriver:   "s3",
			supportedKey:   "STORAGE_SUPPORTED_DRIVERS",
			compiledDriver: "s3",
			localDrivers:   []string{"local"},
			fallbackCase:   "case driverLocal:",
			manifestName:   "compiledStorageDrivers",
			managerPath:    filepath.Join("internal", "storages", "manager_gen.go"),
			prepare:        prepareManifestPackage(filepath.Join("internal", "storages"), false),
			generate:       GenerateStorageFiles,
		},
		{
			name:           "mail",
			activeKey:      "MAIL_DRIVER",
			activeDriver:   "resend",
			supportedKey:   "MAIL_SUPPORTED_DRIVERS",
			compiledDriver: "resend",
			localDrivers:   []string{"log"},
			fallbackCase:   "case driverLog:",
			manifestName:   "compiledMailDrivers",
			managerPath:    filepath.Join("internal", "mail", "manager_gen.go"),
			prepare:        prepareManifestPackage(filepath.Join("internal", "mail"), false),
			generate:       GenerateMailFiles,
		},
		{
			name:           "database",
			activeKey:      "DB_DRIVER",
			activeDriver:   "mysql",
			supportedKey:   "DB_SUPPORTED_DRIVERS",
			compiledDriver: "mysql",
			localDrivers:   []string{"sqlite"},
			fallbackCase:   `case "sqlite", "sqlite3":`,
			manifestName:   "compiledDatabaseDrivers",
			managerPath:    filepath.Join("internal", "database", "connections_gen.go"),
			prepare:        prepareManifestPackage(filepath.Join("internal", "database"), false),
			generate:       GenerateDBFiles,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.activeKey, tt.activeDriver)
			t.Setenv(tt.supportedKey, tt.compiledDriver)
			root := t.TempDir()
			tt.prepare(t, root)
			if _, err := tt.generate(root); err != nil {
				t.Fatalf("generate resource manager: %v", err)
			}

			path := filepath.Join(root, tt.managerPath)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read generated manager: %v", err)
			}
			manifest := generatedDriverManifest(t, string(before), tt.manifestName)
			assertManifestIncludes(t, manifest, append([]string{tt.compiledDriver}, tt.localDrivers...)...)
			if !strings.Contains(string(before), tt.fallbackCase) {
				t.Fatalf("generated source does not retain local implementation %q", tt.fallbackCase)
			}
			for _, localDriver := range tt.localDrivers {
				t.Setenv(tt.activeKey, localDriver)
				if _, err := tt.generate(root); err != nil {
					t.Fatalf("regenerate with local driver %q outside explicit manifest: %v", localDriver, err)
				}
				localSource, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("read manager generated for local driver %q: %v", localDriver, err)
				}
				if !bytes.Equal(before, localSource) {
					t.Fatalf("selecting already-compiled local driver %q changed generated source", localDriver)
				}
			}

			t.Setenv(tt.activeKey, tt.activeDriver)
			t.Setenv(tt.supportedKey, strings.Join(append([]string{tt.compiledDriver}, tt.localDrivers...), ","))
			if _, err := tt.generate(root); err != nil {
				t.Fatalf("regenerate with explicit local drivers: %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reread generated manager: %v", err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("changing runtime supported-driver environment unexpectedly changed the generated manifest")
			}
		})
	}
}

// TestGeneratePrimitiveFilesAllowsLocalDefaultsOutsideSupportedDrivers verifies external manifests do not disable local drivers.
func TestGeneratePrimitiveFilesAllowsLocalDefaultsOutsideSupportedDrivers(t *testing.T) {
	tests := []struct {
		name            string
		activeKey       string
		defaultDriver   string
		supportedKey    string
		supportedDriver string
		manifestName    string
		managerPath     string
		prepare         func(*testing.T, string)
		generate        func(string) (int, error)
	}{
		{
			name:            "cache",
			activeKey:       "CACHE_DRIVER",
			defaultDriver:   "memory",
			supportedKey:    "CACHE_SUPPORTED_DRIVERS",
			supportedDriver: "redis",
			manifestName:    "compiledCacheDrivers",
			managerPath:     filepath.Join("internal", "caches", "manager_gen.go"),
			prepare:         prepareManifestPackage(filepath.Join("internal", "caches"), false),
			generate:        GenerateCacheFiles,
		},
		{
			name:            "queue",
			activeKey:       "QUEUE_DRIVER",
			defaultDriver:   "workerpool",
			supportedKey:    "QUEUE_SUPPORTED_DRIVERS",
			supportedDriver: "redis",
			manifestName:    "compiledQueueDrivers",
			managerPath:     filepath.Join("internal", "queues", "manager_gen.go"),
			prepare:         prepareManifestPackage(filepath.Join("internal", "queues"), true),
			generate:        GenerateQueueFiles,
		},
		{
			name:            "events",
			activeKey:       "EVENTS_DRIVER",
			defaultDriver:   "inproc",
			supportedKey:    "EVENTS_SUPPORTED_DRIVERS",
			supportedDriver: "redis",
			manifestName:    "compiledEventDrivers",
			managerPath:     filepath.Join("internal", "events", "manager_gen.go"),
			prepare:         prepareManifestPackage(filepath.Join("internal", "events"), false),
			generate:        GenerateEventFiles,
		},
		{
			name:            "storage",
			activeKey:       "STORAGE_DRIVER",
			defaultDriver:   "local",
			supportedKey:    "STORAGE_SUPPORTED_DRIVERS",
			supportedDriver: "s3",
			manifestName:    "compiledStorageDrivers",
			managerPath:     filepath.Join("internal", "storages", "manager_gen.go"),
			prepare:         prepareManifestPackage(filepath.Join("internal", "storages"), false),
			generate:        GenerateStorageFiles,
		},
		{
			name:            "mail",
			activeKey:       "MAIL_DRIVER",
			defaultDriver:   "log",
			supportedKey:    "MAIL_SUPPORTED_DRIVERS",
			supportedDriver: "resend",
			manifestName:    "compiledMailDrivers",
			managerPath:     filepath.Join("internal", "mail", "manager_gen.go"),
			prepare:         prepareManifestPackage(filepath.Join("internal", "mail"), false),
			generate:        GenerateMailFiles,
		},
		{
			name:            "database",
			activeKey:       "DB_DRIVER",
			defaultDriver:   "sqlite",
			supportedKey:    "DB_SUPPORTED_DRIVERS",
			supportedDriver: "mysql",
			manifestName:    "compiledDatabaseDrivers",
			managerPath:     filepath.Join("internal", "database", "connections_gen.go"),
			prepare:         prepareManifestPackage(filepath.Join("internal", "database"), false),
			generate:        GenerateDBFiles,
		},
	}

	for _, tt := range tests {
		for _, activeValue := range []string{"missing", "blank"} {
			t.Run(tt.name+"/"+activeValue, func(t *testing.T) {
				if activeValue == "missing" {
					unsetGenerationEnvironment(t, tt.activeKey)
				} else {
					t.Setenv(tt.activeKey, "")
				}
				t.Setenv(tt.supportedKey, tt.supportedDriver)
				root := t.TempDir()
				tt.prepare(t, root)

				if _, err := tt.generate(root); err != nil {
					t.Fatalf("generate %s with %s local root driver: %v", tt.name, activeValue, err)
				}
				source, err := os.ReadFile(filepath.Join(root, tt.managerPath))
				if err != nil {
					t.Fatalf("read generated %s manager: %v", tt.name, err)
				}
				manifest := generatedDriverManifest(t, string(source), tt.manifestName)
				assertManifestIncludes(t, manifest, tt.defaultDriver, tt.supportedDriver)
			})
		}
	}
}

// TestGeneratePrimitiveFilesCompilesDefaultsForBlankDrivers verifies blank values and runtime fallbacks share one effective driver.
func TestGeneratePrimitiveFilesCompilesDefaultsForBlankDrivers(t *testing.T) {
	tests := []struct {
		name              string
		activeKey         string
		defaultDriver     string
		supportedKey      string
		omittedDriver     string
		manifestName      string
		regenerateCommand string
		managerPath       string
		fallbackCase      string
		prepare           func(*testing.T, string)
		generate          func(string) (int, error)
	}{
		{
			name:              "cache",
			activeKey:         "CACHE_DRIVER",
			defaultDriver:     "memory",
			supportedKey:      "CACHE_SUPPORTED_DRIVERS",
			omittedDriver:     "redis",
			manifestName:      "compiledCacheDrivers",
			regenerateCommand: "forj generate --cache",
			managerPath:       filepath.Join("internal", "caches", "manager_gen.go"),
			fallbackCase:      "case driverMemory:",
			prepare:           prepareManifestPackage(filepath.Join("internal", "caches"), false),
			generate:          GenerateCacheFiles,
		},
		{
			name:              "queue",
			activeKey:         "QUEUE_DRIVER",
			defaultDriver:     "workerpool",
			supportedKey:      "QUEUE_SUPPORTED_DRIVERS",
			omittedDriver:     "redis",
			manifestName:      "compiledQueueDrivers",
			regenerateCommand: "forj generate --queue",
			managerPath:       filepath.Join("internal", "queues", "manager_gen.go"),
			fallbackCase:      "case driverWorkerpool:",
			prepare:           prepareManifestPackage(filepath.Join("internal", "queues"), true),
			generate:          GenerateQueueFiles,
		},
		{
			name:              "events",
			activeKey:         "EVENTS_DRIVER",
			defaultDriver:     "inproc",
			supportedKey:      "EVENTS_SUPPORTED_DRIVERS",
			omittedDriver:     "redis",
			manifestName:      "compiledEventDrivers",
			regenerateCommand: "forj generate --events",
			managerPath:       filepath.Join("internal", "events", "manager_gen.go"),
			fallbackCase:      "activeDriverForScope(scope) == DriverInproc",
			prepare:           prepareManifestPackage(filepath.Join("internal", "events"), false),
			generate:          GenerateEventFiles,
		},
		{
			name:              "storage",
			activeKey:         "STORAGE_DRIVER",
			defaultDriver:     "local",
			supportedKey:      "STORAGE_SUPPORTED_DRIVERS",
			omittedDriver:     "s3",
			manifestName:      "compiledStorageDrivers",
			regenerateCommand: "forj generate --storage",
			managerPath:       filepath.Join("internal", "storages", "manager_gen.go"),
			fallbackCase:      "case driverLocal:",
			prepare:           prepareManifestPackage(filepath.Join("internal", "storages"), false),
			generate:          GenerateStorageFiles,
		},
		{
			name:              "mail",
			activeKey:         "MAIL_DRIVER",
			defaultDriver:     "log",
			supportedKey:      "MAIL_SUPPORTED_DRIVERS",
			omittedDriver:     "resend",
			manifestName:      "compiledMailDrivers",
			regenerateCommand: "forj generate --mail",
			managerPath:       filepath.Join("internal", "mail", "manager_gen.go"),
			fallbackCase:      "case driverLog:",
			prepare:           prepareManifestPackage(filepath.Join("internal", "mail"), false),
			generate:          GenerateMailFiles,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.activeKey, "")
			unsetGenerationEnvironment(t, tt.supportedKey)
			root := t.TempDir()
			tt.prepare(t, root)

			if _, err := tt.generate(root); err != nil {
				t.Fatalf("generate %s with blank root driver: %v", tt.name, err)
			}
			source, err := os.ReadFile(filepath.Join(root, tt.managerPath))
			if err != nil {
				t.Fatalf("read generated %s manager: %v", tt.name, err)
			}
			assertGeneratedDriverManifest(t, string(source), tt.manifestName, tt.defaultDriver, tt.omittedDriver, tt.fallbackCase, tt.regenerateCommand)
		})
	}
}

// TestGenerateResourceFilesRejectAppPrefixedDriversOutsideSupportedManifest verifies every runtime App overlay is constrained by the artifact contract.
func TestGenerateResourceFilesRejectAppPrefixedDriversOutsideSupportedManifest(t *testing.T) {
	tests := resourceAppDriverManifestTests()
	for _, tt := range tests {
		for _, scope := range []string{"root", "named"} {
			t.Run(tt.name+"/"+scope, func(t *testing.T) {
				t.Setenv(tt.activeKey, tt.baseDriver)
				t.Setenv(tt.supportedKey, tt.baseDriver)
				appKey := "BILLING_" + tt.activeKey
				if scope == "named" {
					appKey = "BILLING_" + strings.TrimSuffix(tt.activeKey, "_DRIVER") + "_REPORTS_DRIVER"
				}
				t.Setenv(appKey, tt.appDriver)
				root := t.TempDir()
				tt.prepare(t, root)

				if _, err := tt.generate(root); err == nil {
					t.Fatalf("generate %s with excluded App driver unexpectedly succeeded", tt.name)
				} else {
					for _, expected := range []string{appKey, tt.appDriver, tt.supportedKey} {
						if !strings.Contains(err.Error(), expected) {
							t.Fatalf("generation error %q does not contain %q", err, expected)
						}
					}
				}
				if _, err := os.Stat(filepath.Join(root, tt.managerPath)); !os.IsNotExist(err) {
					t.Fatalf("manager artifact exists after rejected generation: %v", err)
				}
			})
		}
	}
}

// TestGenerateResourceFilesCompileAppPrefixedDriversWithoutSupportedManifest verifies inferred manifests cover root and named App overlays.
func TestGenerateResourceFilesCompileAppPrefixedDriversWithoutSupportedManifest(t *testing.T) {
	tests := resourceAppDriverManifestTests()
	for _, tt := range tests {
		for _, scope := range []string{"root", "named"} {
			t.Run(tt.name+"/"+scope, func(t *testing.T) {
				t.Setenv(tt.activeKey, tt.baseDriver)
				unsetGenerationEnvironment(t, tt.supportedKey)
				appKey := "BILLING_" + tt.activeKey
				if scope == "named" {
					appKey = "BILLING_" + strings.TrimSuffix(tt.activeKey, "_DRIVER") + "_REPORTS_DRIVER"
				}
				t.Setenv(appKey, tt.appDriver)
				root := t.TempDir()
				tt.prepare(t, root)

				if _, err := tt.generate(root); err != nil {
					t.Fatalf("generate %s with App driver: %v", tt.name, err)
				}
				source, err := os.ReadFile(filepath.Join(root, tt.managerPath))
				if err != nil {
					t.Fatalf("read generated manager: %v", err)
				}
				manifest := generatedDriverManifest(t, string(source), tt.manifestName)
				for _, driver := range []string{tt.baseDriver, tt.appDriver} {
					if !strings.Contains(manifest, `"`+driver+`"`) {
						t.Fatalf("compiled manifest %s does not include %q", tt.manifestName, driver)
					}
				}
			})
		}
	}
}

func TestAppPrefixedActiveDriversDoNotClassifyResourceFirstScopes(t *testing.T) {
	t.Setenv("CACHE_REPORTS_DRIVER", "redis")
	t.Setenv("CACHE_PAGE_CACHE_DRIVER", "memcached")
	t.Setenv("STORAGE_CACHE_DRIVER", "memory")
	drivers := appPrefixedActiveDrivers(ambientGenerationInput(t.TempDir()), "CACHE", "memory", false)
	for _, active := range drivers {
		switch active.key {
		case "CACHE_REPORTS_DRIVER", "CACHE_PAGE_CACHE_DRIVER", "STORAGE_CACHE_DRIVER":
			t.Fatalf("ordinary resource scope %s was classified as an App overlay", active.key)
		}
	}
}

// TestGenerateStorageFilesIgnoresDisabledAppOverlays verifies stale App env cannot widen a shared Storage manifest or accessor set.
func TestGenerateStorageFilesIgnoresDisabledAppOverlays(t *testing.T) {
	root := t.TempDir()
	prepareManifestPackage(filepath.Join("internal", "storages"), false)(t, root)
	config := `project_name: Storage overlays
module_name: example.com/storage-overlays
render:
  components: [cli, storage]
apps:
  api:
    components: [cli]
  files:
    components: [cli, storage]
`
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_SUPPORTED_DRIVERS", "local,s3")
	t.Setenv("FILES_STORAGE_DRIVER", "s3")
	t.Setenv("API_STORAGE_REPORTS_DRIVER", "ftp")

	if _, err := GenerateStorageFiles(root); err != nil {
		t.Fatalf("generate Storage with stale disabled-App overlay: %v", err)
	}
	manager, err := os.ReadFile(filepath.Join(root, "internal", "storages", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read generated Storage manager: %v", err)
	}
	manifest := generatedDriverManifest(t, string(manager), "compiledStorageDrivers")
	for _, driver := range []string{"local", "s3"} {
		if !strings.Contains(manifest, `"`+driver+`"`) {
			t.Fatalf("Storage manifest omitted participating driver %q: %s", driver, manifest)
		}
	}
	if strings.Contains(manifest, `"ftp"`) || strings.Contains(string(manager), "ftpstorage") {
		t.Fatalf("Storage manifest retained a disabled-App driver:\n%s", manager)
	}
	accessors, err := os.ReadFile(filepath.Join(root, "internal", "storages", "accessors_gen.go"))
	if err != nil {
		t.Fatalf("read generated Storage accessors: %v", err)
	}
	if strings.Contains(string(accessors), "Reports") {
		t.Fatalf("Storage accessors retained a disabled-App disk:\n%s", accessors)
	}
}

// resourceAppDriverManifestTest describes one generator's App-overlay manifest contract.
type resourceAppDriverManifestTest struct {
	name         string
	activeKey    string
	baseDriver   string
	appDriver    string
	supportedKey string
	manifestName string
	managerPath  string
	prepare      func(*testing.T, string)
	generate     func(string) (int, error)
}

// resourceAppDriverManifestTests returns equivalent cases for every generated resource manager.
func resourceAppDriverManifestTests() []resourceAppDriverManifestTest {
	return []resourceAppDriverManifestTest{
		{name: "cache", activeKey: "CACHE_DRIVER", baseDriver: "memory", appDriver: "redis", supportedKey: "CACHE_SUPPORTED_DRIVERS", manifestName: "compiledCacheDrivers", managerPath: filepath.Join("internal", "caches", "manager_gen.go"), prepare: prepareManifestPackage(filepath.Join("internal", "caches"), false), generate: GenerateCacheFiles},
		{name: "queue", activeKey: "QUEUE_DRIVER", baseDriver: "workerpool", appDriver: "redis", supportedKey: "QUEUE_SUPPORTED_DRIVERS", manifestName: "compiledQueueDrivers", managerPath: filepath.Join("internal", "queues", "manager_gen.go"), prepare: prepareManifestPackage(filepath.Join("internal", "queues"), true), generate: GenerateQueueFiles},
		{name: "events", activeKey: "EVENTS_DRIVER", baseDriver: "inproc", appDriver: "redis", supportedKey: "EVENTS_SUPPORTED_DRIVERS", manifestName: "compiledEventDrivers", managerPath: filepath.Join("internal", "events", "manager_gen.go"), prepare: prepareManifestPackage(filepath.Join("internal", "events"), false), generate: GenerateEventFiles},
		{name: "storage", activeKey: "STORAGE_DRIVER", baseDriver: "local", appDriver: "s3", supportedKey: "STORAGE_SUPPORTED_DRIVERS", manifestName: "compiledStorageDrivers", managerPath: filepath.Join("internal", "storages", "manager_gen.go"), prepare: prepareManifestPackage(filepath.Join("internal", "storages"), false), generate: GenerateStorageFiles},
		{name: "mail", activeKey: "MAIL_DRIVER", baseDriver: "log", appDriver: "resend", supportedKey: "MAIL_SUPPORTED_DRIVERS", manifestName: "compiledMailDrivers", managerPath: filepath.Join("internal", "mail", "manager_gen.go"), prepare: prepareManifestPackage(filepath.Join("internal", "mail"), false), generate: GenerateMailFiles},
		{name: "database", activeKey: "DB_DRIVER", baseDriver: "sqlite", appDriver: "mysql", supportedKey: "DB_SUPPORTED_DRIVERS", manifestName: "compiledDatabaseDrivers", managerPath: filepath.Join("internal", "database", "connections_gen.go"), prepare: prepareManifestPackage(filepath.Join("internal", "database"), false), generate: GenerateDBFiles},
	}
}

// generatedDriverManifest extracts one declaration so assertions ignore retained compatibility implementations.
func generatedDriverManifest(t *testing.T, source string, manifestName string) string {
	t.Helper()
	declaration := "var " + manifestName + " = []string{"
	start := strings.Index(source, declaration)
	if start < 0 {
		t.Fatalf("generated source does not declare %s", manifestName)
	}
	remainder := source[start+len(declaration):]
	end := strings.Index(remainder, "\n}")
	if end < 0 {
		t.Fatalf("generated source does not terminate %s", manifestName)
	}
	return remainder[:end]
}

// assertManifestIncludes reports every driver missing from a generated manifest.
func assertManifestIncludes(t *testing.T, manifest string, drivers ...string) {
	t.Helper()
	for _, driver := range drivers {
		if !strings.Contains(manifest, `"`+driver+`"`) {
			t.Errorf("compiled manifest does not include %q", driver)
		}
	}
}

// prepareManifestPackage creates the minimum package layout needed for source-only generation.
func prepareManifestPackage(packageDir string, needsModule bool) func(*testing.T, string) {
	return func(t *testing.T, root string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, packageDir), 0o755); err != nil {
			t.Fatalf("create package directory: %v", err)
		}
		if !needsModule {
			return
		}
		if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/drivermanifest\n\ngo 1.25\n"), 0o644); err != nil {
			t.Fatalf("write go.mod: %v", err)
		}
	}
}

// assertGeneratedDriverManifest checks the embedded authority without confusing it with retained fallback implementation code.
func assertGeneratedDriverManifest(t *testing.T, source, manifestName, compiledDriver, omittedDriver, fallbackCase, regenerateCommand string) {
	t.Helper()
	declaration := "var " + manifestName + " = []string{"
	start := strings.Index(source, declaration)
	if start < 0 {
		t.Fatalf("generated source does not declare %s", manifestName)
	}
	remainder := source[start+len(declaration):]
	end := strings.Index(remainder, "\n}")
	if end < 0 {
		t.Fatalf("generated source does not terminate %s", manifestName)
	}
	manifest := remainder[:end]
	if !strings.Contains(manifest, `"`+compiledDriver+`"`) {
		t.Fatalf("compiled manifest %s does not include %q", manifestName, compiledDriver)
	}
	if strings.Contains(manifest, `"`+omittedDriver+`"`) {
		t.Fatalf("compiled manifest %s unexpectedly includes driver %q", manifestName, omittedDriver)
	}
	if !strings.Contains(source, fallbackCase) {
		t.Fatalf("generated source does not retain native fallback implementation %q", fallbackCase)
	}
	for _, expected := range []string{"active driver %q", "compiled choices: %s", regenerateCommand} {
		if !strings.Contains(source, expected) {
			t.Fatalf("generated manifest error does not contain %q", expected)
		}
	}
	if strings.Contains(source, `Get("SUPPORTED_DRIVERS"`) {
		t.Fatal("generated runtime must not reinterpret the supported-driver generation input")
	}
}

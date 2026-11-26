package generate

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestLegacyFeatureWordAppPrefixes preserves historical App boundaries in configless resource generators.
func TestLegacyFeatureWordAppPrefixes(t *testing.T) {
	for _, family := range []struct {
		prefix   string
		key      project.ResourceKey
		driver   string
		generate func(generationInput) (int, error)
	}{
		{"CACHE", project.ResourceCache, "memory", generateCacheFiles},
		{"DB", project.ResourceDatabase, "sqlite", generateDBFiles},
		{"EVENTS", project.ResourceEvents, "inproc", generateEventFiles},
		{"MAIL", project.ResourceMail, "log", generateMailFiles},
		{"QUEUE", project.ResourceQueue, "workerpool", generateQueueFiles},
		{"STORAGE", project.ResourceStorage, "local", generateStorageFiles},
	} {
		for _, prefix := range []string{"FEATURE", "FEATURE_CHILD", "MISSING_FEATURE"} {
			t.Run(family.prefix+"/"+prefix, func(t *testing.T) {
				root := t.TempDir()
				if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/legacy\n"), 0600); err != nil {
					t.Fatal(err)
				}
				key := prefix + "_" + family.prefix + "_REPORTS_DRIVER"
				values := map[string]string{family.prefix + "_DRIVER": family.driver, key: family.driver}
				if got := ResourceNames(root, values)[family.key]; !slices.Equal(got, []string{"reports"}) {
					t.Fatalf("names=%v", got)
				}
				input := fixtureGenerationInput(root, values)
				if _, err := family.generate(input); err != nil {
					t.Fatal(err)
				}
				if count, err := family.generate(input); err != nil || count != 0 {
					t.Fatalf("regeneration count=%d err=%v", count, err)
				}
				values[key] = "legacy-unsupported-driver"
				if _, err := family.generate(fixtureGenerationInput(root, values)); err == nil || !strings.Contains(err.Error(), key) {
					t.Fatalf("invalid driver error=%v", err)
				}
				values[key] = family.driver
				values[prefix+"_"+family.prefix+"_REPORTS_BADKEY"] = "ignored"
				if _, err := family.generate(fixtureGenerationInput(root, values)); err != nil {
					t.Fatalf("unknown application setting: %v", err)
				}
			})
		}
	}
}

// TestGenerateProjectFilesLegacyFeatureWordCache preserves accessors and compiled drivers through the production snapshot.
func TestGenerateProjectFilesLegacyFeatureWordCache(t *testing.T) {
	for _, prefix := range []string{"FEATURE", "FEATURE_CHILD", "MISSING_FEATURE"} {
		t.Run(prefix, func(t *testing.T) {
			root := newGenerationCacheProject(t)
			writeGenerationEnvironmentFile(t, root, ".env", "CACHE_DRIVER=memory\n"+prefix+"_CACHE_REPORTS_DRIVER=redis\n")
			if _, err := GenerateProjectFiles(root, GenerationSelection{Cache: true}); err != nil {
				t.Fatal(err)
			}
			source, err := os.ReadFile(filepath.Join(root, "internal", "caches", "accessors_gen.go"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(source), "func (m *Manager) Reports()") {
				t.Fatal("missing Reports accessor")
			}
			manager := readGeneratedCacheManager(t, root)
			for _, want := range []string{`"github.com/goforj/cache/driver/rediscache"`, "rediscache.New(", `"redis"`} {
				if !strings.Contains(manager, want) {
					t.Fatalf("missing %q", want)
				}
			}
			if count, err := GenerateProjectFiles(root, GenerationSelection{Cache: true}); err != nil || count.ChangedFiles != 0 {
				t.Fatalf("regeneration result=%+v err=%v", count, err)
			}
		})
	}
}

// TestLegacyFeaturePrefixConfiguredOwnership preserves explicit disabled and active App component selection.
func TestLegacyFeaturePrefixConfiguredOwnership(t *testing.T) {
	for _, components := range []string{"cli", "cli, cache", "cli, cache, feature"} {
		t.Run(components, func(t *testing.T) {
			root := t.TempDir()
			writeGenerationEnvironmentFile(t, root, ".goforj.yml", "render:\n  components: [cli]\napps:\n  missing-feature:\n    components: ["+components+"]\n")
			names := ResourceNames(root, map[string]string{"CACHE_DRIVER": "memory", "MISSING_FEATURE_CACHE_REPORTS_DRIVER": "redis"})
			var want []string
			if strings.Contains(components, "cache") {
				want = []string{"reports"}
			}
			if !slices.Equal(names[project.ResourceCache], want) {
				t.Fatalf("names=%v want=%v", names[project.ResourceCache], want)
			}
		})
	}
}

// TestLegacyPrefixCorrectionKeepsFeatureInference preserves direct Feature overlays and root resource namespace guards.
func TestLegacyPrefixCorrectionKeepsFeatureInference(t *testing.T) {
	root := t.TempDir()
	values := map[string]string{"MISSING_FEATURE_REPORTS_DRIVER": "memory"}
	for _, resource := range []string{"CACHE", "DB", "EVENTS", "MAIL", "QUEUE", "STORAGE"} {
		values[resource+"_FEATURE_"+resource+"_SHADOW_DRIVER"] = "memory"
	}
	input := fixtureGenerationInput(root, values)
	if got := discoverFeatureNames(input); !slices.Equal(got, []string{"reports"}) {
		t.Fatalf("Feature names=%v", got)
	}
	if _, err := generateFeatureFiles(input); err != nil {
		t.Fatal(err)
	}
	if source := readFeatureInventoryArtifact(t, root, FeatureAccessorsPath); !strings.Contains(source, "Reports()") {
		t.Fatal("missing direct Feature accessor")
	}
	if prefixes := legacyResourceGenerationInput(input, "CACHE").appPrefixes; len(prefixes) != 0 {
		t.Fatalf("resource-first App prefixes=%v", prefixes)
	}
}

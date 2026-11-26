package localproviders

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestDisabledFeatureNamespaceRemainsApplicationOwned protects unrelated settings throughout a provider switch and restore.
func TestDisabledFeatureNamespaceRemainsApplicationOwned(t *testing.T) {
	for _, driver := range []string{"launchdarkly", "file", "memory"} {
		t.Run(driver, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/optin\nrender:\n  components: [cli, cache]\n")
			application := "FEATURE_DRIVER=" + driver + "\nFEATURE_SUPPORTED_DRIVERS=application-defined\nFEATURE_SDK_KEY=application-owned\nFEATURE_PATH=application.json\n"
			put(t, root, ".env", "CACHE_DRIVER=redis\nCACHE_SUPPORTED_DRIVERS=memory,redis\n"+application)
			session := openTest(t, root)
			for _, key := range []string{"FEATURE_DRIVER", "FEATURE_SUPPORTED_DRIVERS", "FEATURE_SDK_KEY", "FEATURE_PATH"} {
				if _, exists := session.Current[key]; exists || managedKey(session.config, key) || len(resourceDefinitions(session.config, key)) != 0 {
					t.Fatalf("disabled Feature claimed %s", key)
				}
			}
			portable, err := session.Portable()
			if err != nil {
				t.Fatal(err)
			}
			if portable["CACHE_DRIVER"] != "memory" {
				t.Fatal("portable did not convert Cache")
			}
			for key := range portable {
				if strings.HasPrefix(key, "FEATURE_") {
					t.Fatalf("portable claimed %s", key)
				}
			}
			if err := session.Apply(portable); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(filepath.Join(root, ".env"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(raw, []byte(application)) {
				t.Fatal("provider switch rewrote application Feature settings")
			}
			backup, err := os.ReadFile(filepath.Join(root, stateName))
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(backup, []byte("FEATURE_")) || bytes.Contains(backup, []byte("application-owned")) {
				t.Fatal("restore point captured application Feature settings")
			}
			put(t, root, ".env", strings.ReplaceAll(string(raw), "application-owned", "updated-application-secret"))
			session = openTest(t, root)
			if err := session.Restore(); err != nil {
				t.Fatal(err)
			}
			session = openTest(t, root)
			if session.Current["CACHE_DRIVER"] != "redis" || session.env.values["FEATURE_DRIVER"] != driver || session.env.values["FEATURE_SDK_KEY"] != "updated-application-secret" {
				t.Fatal("restore changed application settings or lost the Cache driver")
			}
		})
	}
}

// TestFeatureOptInRetainsDriverValidation keeps root defaults validated when either the default App or a sibling opts in.
func TestFeatureOptInRetainsDriverValidation(t *testing.T) {
	for _, components := range []string{
		"render:\n  components: [cli, cache, feature]\n",
		"render:\n  components: [cli, cache]\napps:\n  billing:\n    components: [cli, feature]\n",
	} {
		for _, key := range []string{"FEATURE_DRIVER", "FEATURE_SUPPORTED_DRIVERS"} {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/optin\n"+components)
			put(t, root, ".env", "CACHE_DRIVER=memory\nFEATURE_DRIVER=memory\n"+key+"=launchdarkly\n")
			if _, err := Open(root); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("active Feature accepted unsupported %s: %v", key, err)
			}
		}
	}
}

// TestDisabledFeatureNamespaceRetainsFeatureAppCache preserves legacy App resource interpretation despite the shared prefix.
func TestDisabledFeatureNamespaceRetainsFeatureAppCache(t *testing.T) {
	for _, driver := range []string{"redis", "unsupported"} {
		t.Run(driver, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/optin\nrender:\n  components: [cli, cache]\napps:\n  feature:\n    components: [cli, cache]\n")
			put(t, root, ".env", "CACHE_DRIVER=redis\nCACHE_SUPPORTED_DRIVERS=memory,redis\nFEATURE_DRIVER=launchdarkly\nFEATURE_SDK_KEY=application-owned\nFEATURE_CACHE_DRIVER="+driver+"\n")
			session, err := Open(root)
			if driver == "unsupported" {
				if err == nil || !strings.Contains(err.Error(), "FEATURE_CACHE_DRIVER") {
					t.Fatalf("App Cache accepted unsupported driver: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			definitions := resourceDefinitions(session.config, "FEATURE_CACHE_DRIVER")
			if len(definitions) != 1 || definitions[0].Key != project.ResourceCache || session.Current["FEATURE_CACHE_DRIVER"] != "redis" {
				t.Fatal("Feature App Cache lost its legacy owner")
			}
			portable, err := session.Portable()
			if err != nil {
				t.Fatal(err)
			}
			if portable["FEATURE_CACHE_DRIVER"] != "memory" {
				t.Fatal("portable skipped the Feature App Cache")
			}
		})
	}
}

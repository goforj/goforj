package generate

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestFeatureResourceWordAppsOwnTheirEnvironment keeps conventional and configured App names valid without leaking their overlays.
func TestFeatureResourceWordAppsOwnTheirEnvironment(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "conventional", true: "configured"}[configured], func(t *testing.T) {
			config := ""
			if configured {
				config = "apps:\n  feature:\n    components: [cli, feature]\n  feature-billing:\n    components: [cli, feature]\n  feature-feature:\n    components: [cli, feature]\n  feature-disabled:\n    components: [cli]\n"
			}
			root := featureInventoryProject(t, config, "feature", "feature-billing", "feature-feature", "feature-disabled")
			if !configured {
				config = "apps:\n  feature-disabled:\n    components: [cli]\n"
				if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("render:\n  components: [cli, feature]\n"+config), 0600); err != nil {
					t.Fatal(err)
				}
			}
			writeGenerationEnvironmentFile(t, root, ".env", "FEATURE_FEATURE_DRIVER=file\nFEATURE_FEATURE_PATH=feature.json\nFEATURE_FEATURE_OWN_DRIVER=memory\nFEATURE_BILLING_FEATURE_DRIVER=memory\nFEATURE_BILLING_FEATURE_CHECKOUT_DRIVER=memory\nFEATURE_FEATURE_FEATURE_DRIVER=memory\nFEATURE_FEATURE_FEATURE_NESTED_DRIVER=memory\nFEATURE_DISABLED_FEATURE_DRIVER=hosted\n")
			input, err := loadProjectGenerationInput(root)
			if err != nil {
				t.Fatal(err)
			}
			if names := appScopedChildNames(input, "FEATURE", "", featureRootKeys); len(names) != 0 {
				t.Fatalf("root names = %v", names)
			}
			if names := discoverFeatureNames(input); !reflect.DeepEqual(names, []string{"checkout", "nested", "own"}) {
				t.Fatalf("names = %v", names)
			}
			if _, err := generateFeatureFiles(input); err != nil {
				t.Fatal(err)
			}
			drivers := appPrefixedActiveDrivers(input, "FEATURE", "memory", false)
			for _, driver := range drivers {
				if strings.Contains(driver.key, "DISABLED") {
					t.Fatalf("disabled App driver retained: %#v", driver)
				}
			}
			for _, prefix := range []string{"FEATURE", "FEATURE_BILLING", "FEATURE_FEATURE"} {
				if !strings.Contains(readFeatureInventoryArtifact(t, root, FeatureManagerPath), `"`+prefix+`":`) {
					t.Fatalf("missing scope %s", prefix)
				}
			}
		})
	}
}

// TestFeatureResourceWordAppValidation preserves driver validation within the longest owning App scope.
func TestFeatureResourceWordAppValidation(t *testing.T) {
	root := featureInventoryProject(t, "", "feature", "feature-feature")
	writeGenerationEnvironmentFile(t, root, ".env", "FEATURE_FEATURE_DRIVER=file\nFEATURE_FEATURE_PATH=feature.json\nFEATURE_FEATURE_FEATURE_CHILD_DRIVER=memory\nFEATURE_FEATURE_FEATURE_CHILD_PATH=invalid.json\n")
	input, err := loadProjectGenerationInput(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := generateFeatureFiles(input); err == nil || !strings.Contains(err.Error(), `FEATURE_FEATURE_FEATURE_CHILD_PATH is not supported for feature driver "memory"`) {
		t.Fatalf("validation = %v", err)
	}
}

// TestGeneratedFeatureResourceWordAppRuntime verifies default isolation and distinct native source selection in compiled managers.
func TestGeneratedFeatureResourceWordAppRuntime(t *testing.T) {
	root := featureInventoryProject(t, "apps:\n  feature-disabled:\n    components: [cli]\n", "feature", "feature-feature", "feature-disabled", "billing", "feature-child")
	writeGenerationEnvironmentFile(t, root, ".env", "FEATURE_FEATURE_DRIVER=file\nFEATURE_FEATURE_PATH=feature.json\nFEATURE_FEATURE_FEATURE_DRIVER=memory\nFEATURE_DISABLED_FEATURE_DRIVER=hosted\nFEATURE_CACHE_DRIVER=redis\nFEATURE_CHILD_CACHE_DRIVER=redis\nBILLING_FEATURE_FEATURE_DOCUMENT={\"schema\":1,\"flags\":{}}\n")
	input, err := loadProjectGenerationInput(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := generateFeatureFiles(input); err != nil {
		t.Fatal(err)
	}
	writeFixtureGoMod(t, root, fixtureGoModSpec{module: "example.com/featureownership", required: []string{"github.com/goforj/feature", "github.com/goforj/env/v2", "github.com/fsnotify/fsnotify", "golang.org/x/text"}})
	if err := os.WriteFile(filepath.Join(root, "internal/features/ownership_test.go"), []byte(featureOwnershipRuntimeTests), 0600); err != nil {
		t.Fatal(err)
	}
	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/features", "", nil)
}

const featureOwnershipRuntimeTests = `package features
import (
 "os"
 "path/filepath"
 "testing"
)
// TestAppOwnership keeps App-owned documents and drivers out of default resource fallback.
func TestAppOwnership(t *testing.T) {
 path := filepath.Join(t.TempDir(),"flags.json")
 if err := os.WriteFile(path,[]byte("{\"schema\":1,\"flags\":{}}"),0600);err != nil { t.Fatal(err) }
 t.Setenv("FEATURE_FEATURE_DRIVER","file")
 t.Setenv("FEATURE_FEATURE_PATH",path)
 t.Setenv("FEATURE_FEATURE_DOCUMENT","invalid App document")
 t.Setenv("FEATURE_FEATURE_FEATURE_DRIVER","memory")
 t.Setenv("FEATURE_FEATURE_FEATURE_DOCUMENT","{\"schema\":1,\"flags\":{}}")
 t.Setenv("FEATURE_DISABLED_FEATURE_DRIVER","hosted")
 t.Setenv("FEATURE_CACHE_DRIVER","redis")
 t.Setenv("FEATURE_CHILD_CACHE_DRIVER","redis")
 t.Setenv("BILLING_FEATURE_FEATURE_DOCUMENT","{\"schema\":1,\"flags\":{}}")
 for _,test := range []struct{prefix,driver string}{{"","memory"},{"FEATURE","file"},{"FEATURE_FEATURE","memory"}} {
  manager,err := NewManagerForApp(test.prefix);if err != nil { t.Fatal(err) }
  if len(manager.Instances())!=1 || manager.Instances()[0].Driver!=test.driver { t.Fatalf("%s instances = %#v",test.prefix,manager.Instances()) }
  if err := manager.Close();err != nil { t.Fatal(err) }
 }
 billing,err := NewManagerForApp("BILLING");if err != nil { t.Fatal(err) }
 if len(billing.Instances())!=2 || billing.Instances()[1].Name!="feature" || billing.Instances()[1].Driver!="memory" { t.Fatalf("billing named fallback = %#v",billing.Instances()) }
 if err := billing.Close();err != nil { t.Fatal(err) }
 for _,name := range []string{"feature","disabled_feature","feature_feature","cache","child_cache"} {
  resource,err := openEvaluator("",name);if err != nil { t.Fatalf("default %s: %v",name,err) }
  if resource.driver!="memory" || resource.path!="" { t.Fatalf("default %s inherited App source",name) }
 }
 if value := featureValue("FEATURE","feature","DOCUMENT","fallback");value!="fallback" { t.Fatalf("nested App overlay leaked: %q",value) }
}
`

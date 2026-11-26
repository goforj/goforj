package generate

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestConfiglessFeatureNotificationWordApps preserves Feature's pre-Notification inference vocabulary across inventories and generation.
func TestConfiglessFeatureNotificationWordApps(t *testing.T) {
	for _, prefix := range []string{"NOTIFICATION", "NOTIFICATION_CHILD", "MISSING_NOTIFICATION"} {
		t.Run(prefix, func(t *testing.T) {
			root := newGenerationCacheProject(t)
			values := map[string]string{prefix + "_FEATURE_CHECKOUT_DRIVER": "memory"}
			writeGenerationEnvironmentFile(t, root, ".env", prefix+"_FEATURE_CHECKOUT_DRIVER=memory\n")
			input, err := loadProjectGenerationInput(root)
			if err != nil {
				t.Fatal(err)
			}
			if got := ResourceNames(root, values)[project.ResourceFeature]; !reflect.DeepEqual(got, []string{"checkout"}) {
				t.Fatalf("exported names=%v", got)
			}
			if got := discoverFeatureNames(input); !reflect.DeepEqual(got, []string{"checkout"}) {
				t.Fatalf("snapshot names=%v", got)
			}
			scopes, err := generationFeatureScopes(input)
			if err != nil || !reflect.DeepEqual(scopes, []featureScope{{Names: []string{}}, {Prefix: prefix, Names: []string{"CHECKOUT"}}}) {
				t.Fatalf("scopes=%#v err=%v", scopes, err)
			}
			if _, err := GenerateProjectFiles(root, GenerationSelection{Feature: true}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(readFeatureInventoryArtifact(t, root, FeatureAccessorsPath), "Checkout()") {
				t.Fatal("missing accessor")
			}
			if !strings.Contains(readFeatureInventoryArtifact(t, root, FeatureManagerPath), `"`+prefix+`":`) {
				t.Fatal("missing compiled App scope")
			}
			if result, err := GenerateProjectFiles(root, GenerationSelection{Feature: true}); err != nil || result.ChangedFiles != 0 {
				t.Fatalf("regeneration=%+v err=%v", result, err)
			}
			values[prefix+"_FEATURE_CHECKOUT_DRIVER"] = "hosted"
			if _, err := generateFeatureFiles(fixtureGenerationInput(root, values)); err == nil || !strings.Contains(err.Error(), "unsupported driver") {
				t.Fatalf("invalid driver=%v", err)
			}
		})
	}
}

// TestGeneratedConfiglessFeatureNotificationApp proves inferred Feature scopes remain usable by runtime App managers.
func TestGeneratedConfiglessFeatureNotificationApp(t *testing.T) {
	root := t.TempDir()
	input := fixtureGenerationInput(root, map[string]string{"NOTIFICATION_FEATURE_CHECKOUT_DRIVER": "memory"})
	if _, err := generateFeatureFiles(input); err != nil {
		t.Fatal(err)
	}
	writeFixtureGoMod(t, root, fixtureModuleSpec("example.com/featurecompatibility", []string{"github.com/goforj/env/v2", "github.com/goforj/feature"}, nil, nil))
	if err := os.WriteFile(filepath.Join(root, "internal/features/compatibility_test.go"), []byte(configlessFeatureNotificationRuntimeTests), 0600); err != nil {
		t.Fatal(err)
	}
	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/features", "", nil)
}

const configlessFeatureNotificationRuntimeTests = `package features
import "testing"
// TestInferredApp keeps the historical App registration available after adding Notification.
func TestInferredApp(t *testing.T) {
 t.Setenv("NOTIFICATION_FEATURE_CHECKOUT_DRIVER","memory")
 manager,err:=NewManagerForApp("NOTIFICATION")
 if err!=nil { t.Fatal(err) }
 defer manager.Close()
 if manager.Checkout()==nil || len(manager.Instances())!=2 { t.Fatal("missing inferred checkout evaluator") }
 root,err:=NewManagerForApp("")
 if err!=nil { t.Fatal(err) }
 defer root.Close()
 if len(root.Instances())!=1 { t.Fatal("App evaluator leaked to root") }
}
`

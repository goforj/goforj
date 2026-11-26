package generate

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestConfiglessNotificationSearchWordApps preserves Notification's App vocabulary before Search was introduced.
func TestConfiglessNotificationSearchWordApps(t *testing.T) {
	for _, prefix := range []string{"SEARCH", "SEARCH_CHILD", "BILLING_SEARCH", "MISSING_SEARCH"} {
		t.Run(prefix, func(t *testing.T) {
			root := newGenerationCacheProject(t)
			values := map[string]string{prefix + "_NOTIFICATION_ALERTS_DRIVER": "log"}
			writeGenerationEnvironmentFile(t, root, ".env", prefix+"_NOTIFICATION_ALERTS_DRIVER=log\n")
			input, err := loadProjectGenerationInput(root)
			if err != nil {
				t.Fatal(err)
			}
			if got := ResourceNames(root, values)[project.ResourceNotification]; !reflect.DeepEqual(got, []string{"alerts"}) {
				t.Fatalf("public names=%v", got)
			}
			if got := discoverNotificationNames(input); !reflect.DeepEqual(got, []string{"alerts"}) {
				t.Fatalf("snapshot names=%v", got)
			}
			scopes, err := generationNotificationScopes(input)
			if err != nil || !reflect.DeepEqual(scopes, []notificationScope{{Names: []string{}}, {Prefix: prefix, Names: []string{"ALERTS"}}}) {
				t.Fatalf("scopes=%#v err=%v", scopes, err)
			}
			if _, err := GenerateProjectFiles(root, GenerationSelection{Notification: true}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(readFeatureInventoryArtifact(t, root, NotificationAccessorsPath), "Alerts()") {
				t.Fatal("missing public accessor")
			}
			if !strings.Contains(readFeatureInventoryArtifact(t, root, NotificationManagerPath), `"`+prefix+`":`) {
				t.Fatal("missing compiled App scope")
			}
			if result, err := GenerateProjectFiles(root, GenerationSelection{Notification: true}); err != nil || result.ChangedFiles != 0 {
				t.Fatalf("regeneration=%#v err=%v", result, err)
			}
			values[prefix+"_NOTIFICATION_ALERTS_DRIVER"] = "hosted"
			if _, err := generateNotificationFiles(fixtureGenerationInput(root, values)); err == nil || !strings.Contains(err.Error(), "unsupported driver") {
				t.Fatalf("invalid inferred driver=%v", err)
			}
		})
	}
}

// TestGeneratedConfiglessNotificationSearchApps proves inferred Apps remain registered in generated runtime managers.
func TestGeneratedConfiglessNotificationSearchApps(t *testing.T) {
	root := t.TempDir()
	input := fixtureGenerationInput(root, map[string]string{"SEARCH_NOTIFICATION_ALERTS_DRIVER": "log", "SEARCH_CHILD_NOTIFICATION_ALERTS_DRIVER": "log", "BILLING_SEARCH_NOTIFICATION_ALERTS_DRIVER": "log"})
	if _, err := generateNotificationFiles(input); err != nil {
		t.Fatal(err)
	}
	writeFixtureGoMod(t, root, fixtureModuleSpec("example.com/notificationcompatibility", []string{"github.com/goforj/env/v2", "github.com/goforj/notification"}, nil, nil))
	source := `package notifications
import "testing"
// TestInferredApps verifies manager registrations without sending notifications.
func TestInferredApps(t *testing.T) {
 for _,prefix:=range []string{"SEARCH","SEARCH_CHILD","BILLING_SEARCH"} {
  manager,err:=NewManagerForApp(prefix)
  if err!=nil {t.Fatal(err)}
  if manager.Alerts()==nil || len(manager.Instances())!=2 {t.Fatalf("missing alerts sender for %s",prefix)}
  if err:=manager.Close();err!=nil {t.Fatal(err)}
 }
 root,err:=NewManagerForApp("")
 if err!=nil {t.Fatal(err)}
 defer root.Close()
 if len(root.Instances())!=1 {t.Fatal("App sender leaked into root")}
}
`
	if err := os.WriteFile(filepath.Join(root, "internal/notifications/compatibility_test.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/notifications", "", nil)
}

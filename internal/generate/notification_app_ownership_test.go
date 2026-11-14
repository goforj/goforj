package generate

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestNotificationResourceWordAppsOwnTheirEnvironment keeps conventional and configured App names valid without leaking their overlays.
func TestNotificationResourceWordAppsOwnTheirEnvironment(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "conventional", true: "configured"}[configured], func(t *testing.T) {
			config := ""
			if configured {
				config = "apps:\n  notification:\n    components: [cli, notification]\n  notification-worker:\n    components: [cli, notification]\n  notification-notification:\n    components: [cli, notification]\n  notification-disabled:\n    components: [cli]\n"
			}
			root := notificationInventoryProject(t, config, "notification", "notification-worker", "notification-notification", "notification-disabled")
			if !configured {
				config = "apps:\n  notification-disabled:\n    components: [cli]\n"
				if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("render:\n  components: [cli, notification]\n"+config), 0600); err != nil {
					t.Fatal(err)
				}
			}
			writeGenerationEnvironmentFile(t, root, ".env", "NOTIFICATION_NOTIFICATION_DRIVER=webhook\nNOTIFICATION_NOTIFICATION_WEBHOOK_URL=https://example.com/hook\nNOTIFICATION_NOTIFICATION_OWN_DRIVER=log\nNOTIFICATION_WORKER_NOTIFICATION_DRIVER=log\nNOTIFICATION_WORKER_NOTIFICATION_CHECKOUT_DRIVER=log\nNOTIFICATION_NOTIFICATION_NOTIFICATION_DRIVER=log\nNOTIFICATION_NOTIFICATION_NOTIFICATION_NESTED_DRIVER=log\nNOTIFICATION_DISABLED_NOTIFICATION_DRIVER=hosted\n")
			input, err := loadProjectGenerationInput(root)
			if err != nil {
				t.Fatal(err)
			}
			if names := appScopedChildNames(input, "NOTIFICATION", "", notificationRootKeys); len(names) != 0 {
				t.Fatalf("root names = %v", names)
			}
			if names := discoverNotificationNames(input); !reflect.DeepEqual(names, []string{"checkout", "nested", "own"}) {
				t.Fatalf("names = %v", names)
			}
			if _, err := generateNotificationFiles(input); err != nil {
				t.Fatal(err)
			}
			drivers := appPrefixedActiveDrivers(input, "NOTIFICATION", "log", false)
			for _, driver := range drivers {
				if strings.Contains(driver.key, "DISABLED") {
					t.Fatalf("disabled App driver retained: %#v", driver)
				}
			}
			for _, prefix := range []string{"NOTIFICATION", "NOTIFICATION_WORKER", "NOTIFICATION_NOTIFICATION"} {
				if !strings.Contains(readNotificationInventoryArtifact(t, root, NotificationManagerPath), `"`+prefix+`":`) {
					t.Fatalf("missing scope %s", prefix)
				}
			}
		})
	}
}

// TestNotificationResourceWordAppValidation preserves driver validation within the longest owning App scope.
func TestNotificationResourceWordAppValidation(t *testing.T) {
	root := notificationInventoryProject(t, "", "notification", "notification-notification")
	writeGenerationEnvironmentFile(t, root, ".env", "NOTIFICATION_NOTIFICATION_DRIVER=webhook\nNOTIFICATION_NOTIFICATION_WEBHOOK_URL=https://example.com/hook\nNOTIFICATION_NOTIFICATION_NOTIFICATION_CHILD_DRIVER=unknown\n")
	input, err := loadProjectGenerationInput(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := generateNotificationFiles(input); err == nil || !strings.Contains(err.Error(), `NOTIFICATION_NOTIFICATION_NOTIFICATION_CHILD_DRIVER selects unsupported driver "unknown"`) {
		t.Fatalf("validation = %v", err)
	}
}

// TestGeneratedNotificationResourceWordAppRuntime keeps App-owned webhook settings out of default sender fallback.
func TestGeneratedNotificationResourceWordAppRuntime(t *testing.T) {
	root := notificationInventoryProject(t, "apps:\n  notification-disabled:\n    components: [cli]\n", "notification", "notification-notification", "notification-disabled", "billing")
	writeGenerationEnvironmentFile(t, root, ".env", "NOTIFICATION_SUPPORTED_DRIVERS=log,webhook\nNOTIFICATION_NOTIFICATION_DRIVER=webhook\nNOTIFICATION_NOTIFICATION_WEBHOOK_URL=https://example.com/app\nNOTIFICATION_NOTIFICATION_NOTIFICATION_DRIVER=log\nNOTIFICATION_DISABLED_NOTIFICATION_DRIVER=hosted\nBILLING_NOTIFICATION_NOTIFICATION_DRIVER=log\n")
	input, err := loadProjectGenerationInput(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := generateNotificationFiles(input); err != nil {
		t.Fatal(err)
	}
	writeFixtureGoMod(t, root, fixtureModuleSpec("example.com/notificationownership", []string{"github.com/goforj/notification", "github.com/goforj/env/v2"}, nil, nil))
	if err := os.WriteFile(filepath.Join(root, "internal/notifications/ownership_test.go"), []byte(notificationOwnershipRuntimeTests), 0600); err != nil {
		t.Fatal(err)
	}
	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/notifications", "", nil)
}

const notificationOwnershipRuntimeTests = `package notifications
import "testing"
// TestAppOwnership checks isolation with invalid and ambiguous App-owned transport settings.
func TestAppOwnership(t *testing.T) {
 t.Setenv("NOTIFICATION_FEATURE_DRIVER","file")
 t.Setenv("NOTIFICATION_CACHE_DRIVER","redis")
 t.Setenv("NOTIFICATION_NOTIFICATION_DRIVER","webhook")
 t.Setenv("NOTIFICATION_NOTIFICATION_WEBHOOK_URL","https://example.com/app")
 t.Setenv("NOTIFICATION_NOTIFICATION_NOTIFICATION_DRIVER","log")
 t.Setenv("NOTIFICATION_NOTIFICATION_NOTIFICATION_WEBHOOK_URL","invalid nested url")
 t.Setenv("NOTIFICATION_DISABLED_NOTIFICATION_DRIVER","hosted")
 t.Setenv("BILLING_NOTIFICATION_NOTIFICATION_DRIVER","log")
 for _,prefix := range []string{"","NOTIFICATION","NOTIFICATION_NOTIFICATION"} {
  manager,err := NewManagerForApp(prefix)
  if err != nil { t.Fatal(err) }
  if len(manager.Instances()) != 1 { t.Fatalf("%s instances = %#v",prefix,manager.Instances()) }
  wantClosers := 0
  if prefix == "NOTIFICATION" { wantClosers = 1 }
  if len(manager.closers) != wantClosers { t.Fatalf("%s transport ownership = %d want=%d",prefix,len(manager.closers),wantClosers) }
  if err := manager.Close();err != nil { t.Fatal(err) }
 }
 billing,err := NewManagerForApp("BILLING")
 if err != nil { t.Fatal(err) }
 if len(billing.Instances()) != 2 || billing.Named("notification") == nil { t.Fatal("billing named sender missing") }
 if err := billing.Close();err != nil { t.Fatal(err) }
 for _,name := range []string{"notification","disabled_notification","notification_notification","feature","cache"} {
  _,closer,err := newSender("",name)
  if err != nil || closer != nil { t.Fatalf("default %s inherited App transport: %v",name,err) }
 }
 if got := notificationValue("NOTIFICATION","notification","WEBHOOK_URL","fallback"); got != "fallback" { t.Fatalf("nested App settings leaked: %q",got) }
 if manager,err := NewManagerForApp("NOTIFICATION_DISABLED");err == nil { _=manager.Close();t.Fatal("disabled App owns resources") }
}
`

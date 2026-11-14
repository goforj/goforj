package localproviders

import (
	"reflect"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestNotificationProviderFamilyOwnership keeps provider choices and validation aligned with generated App ownership.
func TestNotificationProviderFamilyOwnership(t *testing.T) {
	for _, test := range []struct {
		name, components, app, appComponents, environment, key, scope, portable string
		family                                                                  project.ResourceKey
	}{
		{"Cache App Notification", "cli, cache", "cache", "cli, notification", "CACHE_DRIVER=memory\nNOTIFICATION_SUPPORTED_DRIVERS=log,webhook\nCACHE_NOTIFICATION_DRIVER=webhook\nCACHE_NOTIFICATION_WEBHOOK_URL=https://example.com/hook\n", "CACHE_NOTIFICATION_DRIVER", "CACHE_", "log", project.ResourceNotification},
		{"Notification App Cache", "cli, notification", "notification", "cli, notification, cache", "NOTIFICATION_DRIVER=log\nNOTIFICATION_CACHE_DRIVER=redis\nCACHE_SUPPORTED_DRIVERS=memory,redis\n", "NOTIFICATION_CACHE_DRIVER", "NOTIFICATION_", "memory", project.ResourceCache},
		{"database App Notification", "cli, database_sqlite", "db", "cli, notification", "DB_DRIVER=sqlite\nNOTIFICATION_SUPPORTED_DRIVERS=log,webhook\nDB_NOTIFICATION_DRIVER=webhook\nDB_NOTIFICATION_WEBHOOK_URL=https://example.com/hook\n", "DB_NOTIFICATION_DRIVER", "DB_", "log", project.ResourceNotification},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/scopes\nrender:\n  components: ["+test.components+"]\napps:\n  "+test.app+":\n    components: ["+test.appComponents+"]\n")
			put(t, root, ".env", test.environment)
			session := openTest(t, root)
			definitions := resourceDefinitions(session.config, test.key)
			if len(definitions) != 1 || definitions[0].Key != test.family {
				t.Fatalf("definitions=%#v want=%s", definitions, test.family)
			}
			if scopes := session.resourceScopes(test.key, definitions[0]); !reflect.DeepEqual(scopes, []string{test.scope}) {
				t.Fatalf("scopes=%v", scopes)
			}
			found := false
			for _, resource := range session.Resources {
				if resource.Key == test.key {
					found = true
					if resource.Definition.Key != test.family || len(resource.Definition.Drivers) == 0 {
						t.Fatalf("picker=%#v", resource)
					}
				}
			}
			if !found {
				t.Fatal("owned App resource missing")
			}
			if test.family == project.ResourceNotification && len(session.databaseScopes(test.key)) != 0 {
				t.Fatal("Notification claimed a database target")
			}
			portable, err := session.Portable()
			if err != nil {
				t.Fatal(err)
			}
			if portable[test.key] != test.portable {
				t.Fatalf("portable=%q want=%q", portable[test.key], test.portable)
			}
			if err := session.Validate(portable); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestDisabledNotificationAppRetainsLegacyProviderInventory preserves active legacy named resources across all six families.
func TestDisabledNotificationAppRetainsLegacyProviderInventory(t *testing.T) {
	for _, test := range []struct{ component, app, prefix, driver string }{
		{"cache", "cache", "CACHE", "memory"}, {"database_sqlite", "db", "DB", "sqlite"}, {"jobs", "queue", "QUEUE", "workerpool"},
		{"events", "events", "EVENTS", "inproc"}, {"storage", "storage", "STORAGE", "local"}, {"mail", "mail", "MAIL", "log"},
	} {
		t.Run(test.component, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/legacy\nrender:\n  components: [cli, "+test.component+"]\napps:\n  "+test.app+":\n    components: [cli]\n")
			key := test.prefix + "_NOTIFICATION_DRIVER"
			put(t, root, ".env", key+"="+test.driver+"\n")
			session := openTest(t, root)
			found := false
			for _, resource := range session.Resources {
				if resource.Key == key {
					found = true
					if resource.Definition.Key == project.ResourceNotification {
						t.Fatal("disabled Notification claimed legacy resource")
					}
				}
			}
			if !found {
				t.Fatal("legacy resource missing")
			}
			if _, err := session.Portable(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestNotificationProviderSettingsRespectOwnership matches runtime empty inheritance and whitespace defaults.
func TestNotificationProviderSettingsRespectOwnership(t *testing.T) {
	config := &project.Config{Render: project.RenderConfig{Components: project.Components{CLI: true, Notification: true}}, Apps: map[string]project.AppConfig{
		"notification":              {Components: project.Components{CLI: true, Notification: true, Cache: true}},
		"billing":                   {Components: project.Components{CLI: true, Notification: true}},
		"notification-notification": {Components: project.Components{CLI: true}},
	}}
	session := &Session{config: config}
	definition, _ := project.ResourceDefinitionByKey(project.ResourceNotification)
	for _, suffix := range []string{"", "_CHECKOUT"} {
		for _, test := range []struct{ name, base, overlay, want string }{{"empty", "webhook", "", "webhook"}, {"whitespace", "webhook", " \t ", "log"}, {"explicit", "log", " WeBhOoK ", "webhook"}} {
			t.Run(suffix+test.name, func(t *testing.T) {
				base := "NOTIFICATION" + suffix + "_DRIVER"
				key := "BILLING_" + base
				if got := session.resourceSetting(map[string]string{base: test.base, key: test.overlay}, key, "DRIVER", "BILLING_", definition); got != test.want {
					t.Fatalf("driver=%q want=%q", got, test.want)
				}
			})
		}
	}
	if got := session.resourceSetting(map[string]string{"NOTIFICATION_CACHE_DRIVER": "redis"}, "BILLING_NOTIFICATION_CACHE_DRIVER", "DRIVER", "BILLING_", definition); got != "log" {
		t.Fatalf("other App driver inherited: %q", got)
	}
	if scopes := session.resourceScopes("NOTIFICATION_NOTIFICATION_NOTIFICATION_DRIVER", definition); len(scopes) != 0 {
		t.Fatalf("disabled longer App scopes=%v", scopes)
	}
}

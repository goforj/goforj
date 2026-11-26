package localproviders

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestNotificationProviderFamilyOwnership keeps resource-word Apps aligned with generated manager ownership.
func TestNotificationProviderFamilyOwnership(t *testing.T) {
	for _, family := range []struct{ component, app, prefix, driver string }{
		{"cache", "cache", "CACHE", "file"},
		{"database_sqlite", "db", "DB", "sqlite"},
		{"jobs", "queue", "QUEUE", "workerpool"},
		{"events", "events", "EVENTS", "inproc"},
		{"storage", "storage", "STORAGE", "local"},
		{"mail", "mail", "MAIL", "log"},
		{"feature", "feature", "FEATURE", "memory"},
		{"notification", "notification", "NOTIFICATION", "log"},
	} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/rootNotification=%t", family.app, enabled), func(t *testing.T) {
				root := fixture(t)
				components := "cli, " + family.component
				if enabled && family.component != "notification" {
					components += ", notification"
				}
				put(t, root, ".goforj.yml", "module_name: example.org/ownership\nrender:\n  components: ["+components+"]\napps:\n  "+family.app+":\n    components: [cli, notification]\n")
				key := family.prefix + "_NOTIFICATION_DRIVER"
				environment := family.prefix + "_DRIVER=" + family.driver + "\nNOTIFICATION_SUPPORTED_DRIVERS=log,webhook\n" + key + "=webhook\n" + strings.TrimSuffix(key, "DRIVER") + "WEBHOOK_URL=https://example.com/hook\n"
				put(t, root, ".env", environment)
				session := openTest(t, root)
				definitions := resourceDefinitions(session.config, key)
				if len(definitions) != 1 || definitions[0].Key != project.ResourceNotification {
					t.Fatalf("definitions=%#v", definitions)
				}
				if got := session.resourceScopes(key, definitions[0]); !reflect.DeepEqual(got, []string{family.prefix + "_"}) {
					t.Fatalf("scopes=%v", got)
				}
				portable, err := session.Portable()
				if err != nil {
					t.Fatal(err)
				}
				if portable[key] != "log" {
					t.Fatalf("portable %s=%q", key, portable[key])
				}
				if err := session.Apply(portable); err != nil {
					t.Fatal(err)
				}
				session = openTest(t, root)
				if err := session.Restore(); err != nil {
					t.Fatal(err)
				}
				session = openTest(t, root)
				if session.Current[key] != "webhook" {
					t.Fatalf("restored %s=%q", key, session.Current[key])
				}
			})
		}
	}
}

// TestDisabledNotificationAppRetainsLegacyProviderInventory preserves active legacy meanings when Notification is disabled in the owning App.
func TestDisabledNotificationAppRetainsLegacyProviderInventory(t *testing.T) {
	for _, family := range []struct{ component, app, prefix, driver string }{
		{"cache", "cache", "CACHE", "file"}, {"database_sqlite", "db", "DB", "sqlite"},
		{"jobs", "queue", "QUEUE", "workerpool"}, {"events", "events", "EVENTS", "inproc"},
		{"storage", "storage", "STORAGE", "local"}, {"mail", "mail", "MAIL", "log"},
	} {
		t.Run(family.app, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/legacy\nrender:\n  components: [cli, "+family.component+"]\napps:\n  "+family.app+":\n    components: [cli]\n")
			key := family.prefix + "_NOTIFICATION_DRIVER"
			put(t, root, ".env", key+"="+family.driver+"\n")
			session := openTest(t, root)
			definitions := resourceDefinitions(session.config, key)
			if len(definitions) != 1 || definitions[0].Key == project.ResourceNotification {
				t.Fatalf("legacy definitions=%#v", definitions)
			}
			portable, err := session.Portable()
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := portable[key]; !ok {
				t.Fatal("lost legacy resource")
			}
		})
	}
}

// TestDisabledNotificationDefinitionsRemainApplicationOwned keeps fallback validation from claiming an inactive namespace.
func TestDisabledNotificationDefinitionsRemainApplicationOwned(t *testing.T) {
	root := fixture(t)
	put(t, root, ".goforj.yml", "module_name: example.org/disabled\nrender:\n  components: [cli, cache]\n")
	put(t, root, ".env", "CACHE_DRIVER=memory\nNOTIFICATION_DRIVER=application-owned\n")
	session := openTest(t, root)
	if definitions := resourceDefinitions(session.config, "NOTIFICATION_DRIVER"); len(definitions) != 0 {
		t.Fatalf("disabled definitions=%#v", definitions)
	}
}

// TestNotificationProviderSettingsRespectOwnership mirrors env.Get's raw empty fallback for root and named App settings.
func TestNotificationProviderSettingsRespectOwnership(t *testing.T) {
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true, Notification: true}},
		Apps: map[string]project.AppConfig{
			"notification":              {Components: project.Components{CLI: true, Notification: true, Cache: true}},
			"billing":                   {Components: project.Components{CLI: true, Notification: true}},
			"notification-notification": {Components: project.Components{CLI: true}},
		},
	}
	session := &Session{config: config}
	definition, _ := project.ResourceDefinitionByKey(project.ResourceNotification)
	for _, scope := range []string{"", "_CHECKOUT"} {
		for _, test := range []struct{ name, base, overlay, want string }{
			{"empty inherits", "webhook", "", "webhook"},
			{"whitespace defaults", "webhook", " \t ", "log"},
			{"explicit normalized", "log", " WeBhOoK ", "webhook"},
			{"explicit log", "webhook", "log", "log"},
			{"empty defaults", "", "", "log"},
			{"whitespace base defaults", " \t ", "", "log"},
		} {
			t.Run(scope+"/"+test.name, func(t *testing.T) {
				base := "NOTIFICATION" + scope + "_DRIVER"
				key := "BILLING_" + base
				values := map[string]string{base: test.base, key: test.overlay}
				if got := session.resourceSetting(values, key, "DRIVER", "BILLING_", definition); got != test.want {
					t.Fatalf("driver=%q, want %q", got, test.want)
				}
			})
		}
	}
	if got := session.resourceSetting(map[string]string{"NOTIFICATION_CACHE_DRIVER": "redis"}, "BILLING_NOTIFICATION_CACHE_DRIVER", "DRIVER", "BILLING_", definition); got != "log" {
		t.Fatalf("other App leaked driver=%q", got)
	}
	if scopes := session.resourceScopes("NOTIFICATION_NOTIFICATION_NOTIFICATION_DRIVER", definition); len(scopes) != 0 {
		t.Fatalf("disabled longer App scopes=%v", scopes)
	}
}

// TestNotificationAppOtherResourceFamilies keeps App-owned legacy families out of root Notification validation.
func TestNotificationAppOtherResourceFamilies(t *testing.T) {
	for _, family := range []struct{ component, prefix, driver, portable string }{
		{"cache", "CACHE", "file", "memory"}, {"database_sqlite", "DB", "sqlite", "sqlite"},
		{"jobs", "QUEUE", "workerpool", "workerpool"}, {"events", "EVENTS", "inproc", "inproc"},
		{"storage", "STORAGE", "local", "local"}, {"mail", "MAIL", "log", "log"},
		{"feature", "FEATURE", "memory", "memory"},
	} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/rootNotification=%t", family.prefix, enabled), func(t *testing.T) {
				root := fixture(t)
				components := "cli"
				if enabled {
					components += ", notification"
				}
				put(t, root, ".goforj.yml", "module_name: example.org/notificationapp\nrender:\n  components: ["+components+"]\napps:\n  notification:\n    components: [cli, "+family.component+"]\n")
				key := "NOTIFICATION_" + family.prefix + "_DRIVER"
				put(t, root, ".env", key+"="+family.driver+"\n")
				session := openTest(t, root)
				definitions := resourceDefinitions(session.config, key)
				if len(definitions) != 1 || definitions[0].Key == project.ResourceNotification {
					t.Fatalf("App definitions=%#v", definitions)
				}
				portable, err := session.Portable()
				if err != nil {
					t.Fatal(err)
				}
				if portable[key] != family.portable {
					t.Fatalf("portable %s=%q", key, portable[key])
				}
				selected, err := session.Drivers(map[string]string{key: family.portable})
				if err != nil {
					t.Fatal(err)
				}
				if err := session.Apply(selected); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

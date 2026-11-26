package resourceenv

import (
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestConsumersKnownNotificationAppBoundaries keeps resource words inside known App names before inferring legacy scopes.
func TestConsumersKnownNotificationAppBoundaries(t *testing.T) {
	for _, inventory := range []string{"configured", "conventional", "configless", "dev-apps", "dev-run"} {
		for _, app := range []string{"notification", "notification-child", "billing-notification", "billing-notification-child", "billing-cache", "billing-cache-child"} {
			for _, notification := range []bool{false, true} {
				t.Run(inventory+"/"+app+"/"+map[bool]string{false: "cache-only", true: "notification"}[notification], func(t *testing.T) {
					defaults := project.Components{CLI: true, Cache: true, Notification: true}
					selected := project.Components{CLI: true, Cache: true, Notification: notification}
					config := &project.Config{Render: project.RenderConfig{Components: defaults}}
					var names []string
					switch inventory {
					case "configured":
						config.Apps = map[string]project.AppConfig{app: {Components: selected}}
					case "conventional":
						config.Render.Components = selected
						defaults = selected
						names = []string{app}
					case "configless":
						config = nil
						defaults = selected
						names = []string{app}
					case "dev-apps":
						config.Render.Components = selected
						defaults = selected
						config.Dev.Apps = map[string]project.DevApp{app: {}}
					case "dev-run":
						config.Render.Components = selected
						defaults = selected
						config.Dev.Run = map[string]string{app: "go run ."}
					}
					components := defaults
					if config != nil {
						components = project.ProjectComponents(config)
					}
					family, driver := "CACHE", "memory"
					if notification {
						family, driver = "NOTIFICATION", "log"
					}
					source := []byte("CACHE_DRIVER=memory\nNOTIFICATION_DRIVER=log\n" + project.AppEnvironmentPrefix(app) + "_" + family + "_DRIVER=" + driver + "\n")
					consumers, err := ResolveConsumers(source, defaultResourcePlanForTest(t, components), components, config, names...)
					if err != nil {
						t.Fatal(err)
					}
					want := map[string]string{"cache": "memory", app + ":cache": "memory"}
					if defaults.Notification {
						want["notification"] = "log"
					}
					if notification {
						want[app+":notification"] = "log"
					}
					got := map[string]string{}
					for _, consumer := range consumers {
						got[consumer.Consumer] = consumer.Driver
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("known App boundary consumers = %#v, want %#v", got, want)
					}
				})
			}
		}
	}
}

// TestConsumersKnownNotificationAppBoundaryPreservesLegacyRootNames retains shared legacy stores when inference uses known boundaries.
func TestConsumersKnownNotificationAppBoundaryPreservesLegacyRootNames(t *testing.T) {
	components := project.Components{CLI: true, Cache: true, Notification: true}
	config := &project.Config{
		Render: project.RenderConfig{Components: components},
		Apps: map[string]project.AppConfig{
			"cache-notification": {Components: project.Components{CLI: true, Cache: true}},
		},
	}
	consumers, err := ResolveConsumers([]byte("CACHE_DRIVER=memory\nCACHE_NOTIFICATION_CACHE_DRIVER=memory\n"), defaultResourcePlanForTest(t, components), components, config)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, consumer := range consumers {
		got[consumer.Consumer] = consumer.Driver
	}
	want := map[string]string{
		"cache": "memory", "notification": "log", "cache:notification_cache": "memory",
		"cache-notification:cache": "memory",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy root named consumers = %#v, want %#v", got, want)
	}
}

// TestConsumersKnownNestedNotificationAppBoundary keeps a disabled child independent of its enabled parent.
func TestConsumersKnownNestedNotificationAppBoundary(t *testing.T) {
	components := project.Components{CLI: true, Cache: true, Notification: true}
	config := &project.Config{
		Render: project.RenderConfig{Components: components},
		Apps: map[string]project.AppConfig{
			"billing-notification":       {Components: components},
			"billing-notification-child": {Components: project.Components{CLI: true, Cache: true}},
		},
	}
	consumers, err := ResolveConsumers([]byte("CACHE_DRIVER=memory\nNOTIFICATION_DRIVER=webhook\nBILLING_NOTIFICATION_NOTIFICATION_DRIVER=log\nBILLING_NOTIFICATION_CHILD_CACHE_DRIVER=memory\n"), defaultResourcePlanForTest(t, components), components, config)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, consumer := range consumers {
		got[consumer.Consumer] = consumer.Driver
	}
	want := map[string]string{
		"cache": "memory", "notification": "webhook",
		"billing-notification:cache": "memory", "billing-notification:notification": "log",
		"billing-notification-child:cache": "memory",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nested known App consumers = %#v, want %#v", got, want)
	}
}

// TestConsumersKnownNotificationInfixLegacyFamilies keeps active and stale legacy settings inside known Notification App names.
func TestConsumersKnownNotificationInfixLegacyFamilies(t *testing.T) {
	for _, family := range []struct {
		resource   project.ResourceKey
		components project.Components
		prefix     string
		driver     string
	}{
		{project.ResourceDatabase, project.Components{CLI: true, DatabaseSQLite: true}, "DB", "sqlite"},
		{project.ResourceCache, project.Components{CLI: true, Cache: true}, "CACHE", "memory"},
		{project.ResourceQueue, project.Components{CLI: true, Jobs: true}, "QUEUE", "workerpool"},
		{project.ResourceEvents, project.Components{CLI: true, Events: true}, "EVENTS", "inproc"},
		{project.ResourceStorage, project.Components{CLI: true, Storage: true}, "STORAGE", "local"},
		{project.ResourceMail, project.Components{CLI: true, Mail: true}, "MAIL", "log"},
	} {
		for _, enabled := range []bool{false, true} {
			t.Run(string(family.resource)+"/"+map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
				components := family.components
				components.Notification = true
				selected := family.components
				if !enabled {
					selected = project.Components{CLI: true, Notification: true}
				}
				config := &project.Config{
					Render: project.RenderConfig{Components: components},
					Apps:   map[string]project.AppConfig{"billing-notification": {Components: selected}},
				}
				driver := family.driver
				if family.resource == project.ResourceCache && !enabled {
					driver = "redis"
				}
				source := []byte("BILLING_NOTIFICATION_" + family.prefix + "_DRIVER=" + driver + "\n")
				consumers, err := ResolveConsumers(source, defaultResourcePlanForTest(t, components), components, config)
				if err != nil {
					t.Fatal(err)
				}
				got := map[string]string{}
				for _, consumer := range consumers {
					got[consumer.Consumer] = consumer.Driver
				}
				want := map[string]string{string(family.resource): family.driver, "notification": "log"}
				if enabled {
					want["billing-notification:"+string(family.resource)] = family.driver
				} else {
					want["billing-notification:notification"] = "log"
				}
				if family.resource == project.ResourceStorage {
					want["storage:public"] = "local"
					if enabled {
						want["billing-notification:storage:public"] = "local"
					}
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("legacy family consumers = %#v, want %#v", got, want)
				}
			})
		}
	}
}

// TestConsumersNotificationRawDriverFallback keeps consumer drivers aligned with generated root and named senders.
func TestConsumersNotificationRawDriverFallback(t *testing.T) {
	components := project.Components{Notification: true}
	for _, app := range []string{"billing", "notification"} {
		for _, configured := range []bool{false, true} {
			for _, suffix := range []string{"", "_CHECKOUT"} {
				for _, test := range []struct{ name, overlay, want string }{{"empty", "", "webhook"}, {"whitespace", " \t ", "log"}, {"explicit", " LoG ", "log"}} {
					t.Run(app+"/"+map[bool]string{false: "conventional", true: "configured"}[configured]+"/"+suffix+test.name, func(t *testing.T) {
						key := "NOTIFICATION" + suffix + "_DRIVER"
						source := []byte(key + "=webhook\n" + project.AppEnvironmentPrefix(app) + "_" + key + "=\"" + test.overlay + "\"\n")
						var config *project.Config
						apps := []string{app}
						if configured {
							config = &project.Config{Render: project.RenderConfig{Components: components}, Apps: map[string]project.AppConfig{app: {Components: components}}}
							apps = nil
						}
						consumers, err := ResolveConsumers(source, defaultResourcePlanForTest(t, components), components, config, apps...)
						if err != nil {
							t.Fatal(err)
						}
						name := app + ":notification"
						if suffix != "" {
							name += ":checkout"
						}
						for _, consumer := range consumers {
							if consumer.Consumer == name {
								if consumer.Driver != test.want {
									t.Fatalf("driver=%q want=%q", consumer.Driver, test.want)
								}
								return
							}
						}
						t.Fatalf("missing %s: %#v", name, consumers)
					})
				}
			}
		}
	}
}

// TestConsumersNotificationCrossFamilyAppOwnership prevents other resource families from becoming root Notification names.
func TestConsumersNotificationCrossFamilyAppOwnership(t *testing.T) {
	components := project.Components{CLI: true, Notification: true, Feature: true, Cache: true}
	consumers, err := ResolveConsumers([]byte("NOTIFICATION_FEATURE_DRIVER=file\nNOTIFICATION_CACHE_DRIVER=redis\n"), defaultResourcePlanForTest(t, components), components, nil, "notification")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, consumer := range consumers {
		got[consumer.Consumer] = consumer.Driver
	}
	want := map[string]string{"notification": "log", "feature": "memory", "cache": "memory", "notification:notification": "log", "notification:feature": "file", "notification:cache": "redis"}
	if len(got) != len(want) {
		t.Fatalf("consumers=%#v want=%#v", got, want)
	}
	for name, driver := range want {
		if got[name] != driver {
			t.Errorf("%s=%s want=%s", name, got[name], driver)
		}
	}
}

// TestConsumersNotificationAppExcludesLegacyCollision keeps enabled App Notification settings out of root Cache stores.
func TestConsumersNotificationAppExcludesLegacyCollision(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			config := &project.Config{Render: project.RenderConfig{Components: project.Components{CLI: true, Cache: true}}, Apps: map[string]project.AppConfig{"cache": {Components: project.Components{CLI: true, Notification: enabled}}}}
			components := project.ProjectComponents(config)
			driver := "memory"
			if enabled {
				driver = "webhook"
			}
			consumers, err := ResolveConsumers([]byte("CACHE_DRIVER=memory\nCACHE_NOTIFICATION_DRIVER="+driver+"\nCACHE_NOTIFICATION_WEBHOOK_URL=https://example.com/hook\n"), defaultResourcePlanForTest(t, components), components, config)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, consumer := range consumers {
				if consumer.Resource == project.ResourceCache && consumer.Consumer == "cache:notification" {
					count++
				}
			}
			if enabled && count != 0 || !enabled && count != 1 {
				t.Fatalf("enabled=%t consumers=%#v", enabled, consumers)
			}
		})
	}
}

// TestConsumersReserveNotificationNamespaceWhenEnabled preserves legacy App inference until Notification defaults become active.
func TestConsumersReserveNotificationNamespaceWhenEnabled(t *testing.T) {
	for _, test := range []struct {
		name                string
		defaultNotification bool
		namedNotification   bool
		inventory           string
		wantLegacy          bool
	}{
		{name: "disabled-without-config", wantLegacy: true},
		{name: "disabled-with-config", inventory: "empty", wantLegacy: true},
		{name: "enabled-default", defaultNotification: true},
		{name: "enabled-named-app", namedNotification: true},
		{name: "inherited-default", defaultNotification: true, inventory: "inherited"},
		{name: "configured-resource-names", defaultNotification: true, inventory: "configured", wantLegacy: true},
		{name: "conventional-resource-names", defaultNotification: true, inventory: "conventional", wantLegacy: true},
		{name: "dev-resource-names", defaultNotification: true, inventory: "dev", wantLegacy: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			components := project.Components{CLI: true, Cache: true, Notification: test.defaultNotification}
			var config *project.Config
			var knownApps []string
			if test.inventory != "" || test.namedNotification {
				config = &project.Config{Render: project.RenderConfig{Components: components}, Apps: map[string]project.AppConfig{}}
			}
			if test.namedNotification {
				config.Apps["billing"] = project.AppConfig{Components: project.Components{CLI: true, Notification: true}}
			}
			switch test.inventory {
			case "inherited":
				knownApps = []string{"billing"}
			case "configured":
				for _, name := range []string{"notification", "notification-child"} {
					config.Apps[name] = project.AppConfig{Components: project.Components{CLI: true, Cache: true}}
				}
			case "conventional":
				knownApps = []string{"notification", "notification-child"}
			case "dev":
				config.Dev.Apps = map[string]project.DevApp{"notification": {}, "notification-child": {}}
			}
			if config != nil {
				components = project.ProjectComponents(config)
			}
			driver := "redis"
			if !test.wantLegacy {
				// Active shared Notification settings must select a Notification driver.
				driver = "webhook"
			}
			source := []byte("CACHE_DRIVER=memory\nNOTIFICATION_CACHE_DRIVER=" + driver + "\nNOTIFICATION_CACHE_WEBHOOK_URL=https://example.com/cache\nNOTIFICATION_CHILD_CACHE_DRIVER=" + driver + "\nNOTIFICATION_CHILD_CACHE_WEBHOOK_URL=https://example.com/child\nUNKNOWN_NOTIFICATION_DRIVER=log\n")
			consumers, err := ResolveConsumers(source, defaultResourcePlanForTest(t, components), components, config, knownApps...)
			if err != nil {
				t.Fatal(err)
			}
			found := map[string]bool{}
			shared := map[string]bool{}
			inherited := false
			for _, consumer := range consumers {
				if consumer.Resource == project.ResourceCache {
					for _, name := range []string{"notification", "notification-child"} {
						if consumer.Consumer == name+":cache" && consumer.Driver == "redis" {
							found[name] = true
						}
					}
					if !test.wantLegacy && consumer.Consumer != "cache" && consumer.Consumer != "billing:cache" && consumer.Consumer != "unknown:cache" {
						t.Fatalf("Notification defaults invented a legacy App: %#v", consumers)
					}
				}
				if consumer.Resource == project.ResourceNotification && consumer.Driver == "webhook" {
					shared[consumer.Consumer] = true
				}
				if consumer.Consumer == "unknown:notification" {
					t.Fatalf("unknown App participates in Notification: %#v", consumers)
				}
				if consumer.Consumer == "billing:notification:cache" && consumer.Driver == "webhook" {
					inherited = true
				}
			}
			for _, name := range []string{"notification", "notification-child"} {
				if found[name] != test.wantLegacy {
					t.Errorf("%s legacy App present = %t, want %t; consumers = %#v", name, found[name], test.wantLegacy, consumers)
				}
			}
			if test.defaultNotification && !test.wantLegacy && (!shared["notification:cache"] || !shared["notification:child_cache"]) {
				t.Fatalf("active root Notification namespace lost shared senders: %#v", consumers)
			}
			if (test.inventory == "inherited" || test.namedNotification) && !inherited {
				t.Fatalf("known App did not inherit shared Notification settings: %#v", consumers)
			}
		})
	}
}

// TestConsumersDisabledNotificationPreservesLegacyAppMarkers keeps inactive Notification words inside legacy App names and endpoints.
func TestConsumersDisabledNotificationPreservesLegacyAppMarkers(t *testing.T) {
	for _, family := range []struct {
		resource                                      project.ResourceKey
		components                                    project.Components
		prefix, rootDriver, driver, endpoint, address string
		service                                       project.ServiceKey
	}{
		{project.ResourceDatabase, project.Components{CLI: true, DatabaseSQLite: true}, "DB", "sqlite", "mysql", "HOST", "legacy.example:3306", project.ServiceMySQL},
		{project.ResourceCache, project.Components{CLI: true, Cache: true}, "CACHE", "memory", "redis", "ADDR", "legacy.example:6379", project.ServiceRedis},
		{project.ResourceQueue, project.Components{CLI: true, Jobs: true}, "QUEUE", "workerpool", "redis", "ADDR", "legacy.example:6379", project.ServiceRedis},
		{project.ResourceEvents, project.Components{CLI: true, Events: true}, "EVENTS", "inproc", "redis", "ADDR", "legacy.example:6379", project.ServiceRedis},
		{project.ResourceStorage, project.Components{CLI: true, Storage: true}, "STORAGE", "local", "redis", "ADDR", "legacy.example:6379", project.ServiceRedis},
		{project.ResourceMail, project.Components{CLI: true, Mail: true}, "MAIL", "log", "smtp", "SMTP_HOST", "legacy.example:1025", project.ServiceMailSMTP},
	} {
		for _, app := range []string{"notification", "notification-child", "billing-notification", "missing-notification", "missing-notification-child"} {
			for _, configured := range []bool{false, true} {
				t.Run(string(family.resource)+"/"+app+"/"+map[bool]string{false: "configless", true: "config"}[configured], func(t *testing.T) {
					var config *project.Config
					if configured {
						config = &project.Config{Render: project.RenderConfig{Components: family.components}}
					}
					prefix := project.AppEnvironmentPrefix(app) + "_" + family.prefix
					endpoint := family.address
					if family.endpoint != "ADDR" {
						endpoint = "legacy.example"
					}
					source := []byte(family.prefix + "_DRIVER=" + family.rootDriver + "\n" + prefix + "_DRIVER=" + family.driver + "\n" + prefix + "_" + family.endpoint + "=" + endpoint + "\n")
					consumers, err := ResolveConsumers(source, defaultResourcePlanForTest(t, family.components), family.components, config)
					if err != nil {
						t.Fatal(err)
					}
					found := false
					for _, consumer := range consumers {
						if strings.HasPrefix(consumer.Consumer, "billing:") || strings.HasPrefix(consumer.Consumer, "missing:") {
							t.Fatalf("inactive Notification marker shortened the legacy App: %#v", consumers)
						}
						if consumer.Consumer == app+":"+string(family.resource) {
							found = true
							wantAffinity := opaqueEndpointAffinity(family.service, []string{"addr=" + family.address})
							if consumer.Driver != family.driver || consumer.EndpointAffinity != wantAffinity || consumer.LocalService {
								t.Errorf("legacy consumer = %#v, want driver %s and external endpoint %s", consumer, family.driver, family.address)
							}
						}
					}
					if !found {
						t.Fatalf("missing legacy App %s: %#v", app, consumers)
					}
				})
			}
		}
	}
}

// TestConsumersDisabledNotificationIgnoresNativeMarkers prevents inactive SDK settings from inventing legacy consumers.
func TestConsumersDisabledNotificationIgnoresNativeMarkers(t *testing.T) {
	components := project.Components{CLI: true, Cache: true}
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "configless", true: "config"}[configured], func(t *testing.T) {
			var config *project.Config
			if configured {
				config = &project.Config{Render: project.RenderConfig{Components: components}}
			}
			consumers, err := ResolveConsumers([]byte("CACHE_DRIVER=memory\nMISSING_NOTIFICATION_DRIVER=log\n"), defaultResourcePlanForTest(t, components), components, config)
			if err != nil {
				t.Fatal(err)
			}
			if len(consumers) != 1 || consumers[0].Consumer != "cache" || consumers[0].Driver != "memory" {
				t.Fatalf("inactive Notification setting invented consumers: %#v", consumers)
			}
		})
	}
}

package resourceenv

import (
	"testing"

	"github.com/goforj/goforj/project"
)

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

package generate

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestNotificationOverlayOwnership keeps sibling resource settings out of driver fallback and manifest validation.
func TestNotificationOverlayOwnership(t *testing.T) {
	for _, test := range []struct{ name, components, child, base string }{
		{"active sibling cache", "cli, cache, notification", "CACHE", "redis"},
		{"disabled sibling cache", "cli, cache", "CACHE", "redis"},
		{"disabled sibling notification", "cli", "NOTIFICATION", "hosted"},
		{"active sibling notification", "cli, notification", "NOTIFICATION", "webhook"},
	} {
		for _, override := range []string{"", " \t ", " LoG "} {
			for _, urlOnly := range []bool{false, true} {
				t.Run(test.name+"/"+override+map[bool]string{true: "/URL only"}[urlOnly], func(t *testing.T) {
					root := notificationInventoryProject(t, "apps:\n  notification:\n    components: ["+test.components+"]\n  billing:\n    components: [cli, notification]\n", "notification", "billing")
					values := map[string]string{"NOTIFICATION_SUPPORTED_DRIVERS": "log", "NOTIFICATION_" + test.child + "_DRIVER": test.base}
					if test.components == "cli, notification" || test.components == "cli, cache, notification" {
						values["NOTIFICATION_NOTIFICATION_DRIVER"] = "log"
					}
					key := "BILLING_NOTIFICATION_" + test.child + "_DRIVER"
					if urlOnly {
						values["BILLING_NOTIFICATION_"+test.child+"_WEBHOOK_URL"] = "https://example.com/hook"
					} else {
						values[key] = override
					}
					input := fixtureGenerationInput(root, values)
					prefixes := generationKnownAppEnvPrefixes(input)
					if got := effectiveRawOverlayDriver(input.environment, "NOTIFICATION", "BILLING_NOTIFICATION", test.child, "log", prefixes); got != "log" {
						t.Fatalf("driver = %q", got)
					}
					for _, driver := range appPrefixedActiveDrivers(input, "NOTIFICATION", "log", false) {
						if driver.driver != "log" {
							t.Fatalf("active driver = %#v", driver)
						}
					}
					if _, err := generateNotificationFiles(input); err != nil {
						t.Fatal(err)
					}
					if count, err := generateNotificationFiles(input); err != nil || count != 0 {
						t.Fatalf("regenerate = %d, %v", count, err)
					}
				})
			}
		}
	}
}

// TestNotificationOverlayLongestOwner mirrors runtime ownership before normalizing raw driver fallbacks.
func TestNotificationOverlayLongestOwner(t *testing.T) {
	for _, test := range []struct{ name, base, overlay, want string }{
		{"empty inherits raw base", " WeBhOoK ", "", "webhook"},
		{"whitespace selects default", "webhook", " \t ", "log"},
		{"explicit normalized driver", "log", " WeBhOoK ", "webhook"},
	} {
		t.Run(test.name, func(t *testing.T) {
			environment := generationEnvironment{values: map[string]string{"NOTIFICATION_NOTIFICATION_DRIVER": test.base, "WRAP_NOTIFICATION_NOTIFICATION_DRIVER": test.overlay}}
			for _, prefixes := range [][]string{{"WRAP"}, {"WRAP", "WRAP_NOTIFICATION"}, {"NOTIFICATION", "WRAP", "WRAP_NOTIFICATION"}} {
				want := test.want
				if len(prefixes) == 2 {
					want = effectivePrimitiveDriver(test.base, "log")
				}
				if len(prefixes) == 3 {
					want = "log"
				}
				if got := effectiveRawOverlayDriver(environment, "NOTIFICATION", "WRAP_NOTIFICATION", "NOTIFICATION", "log", prefixes); got != want {
					t.Fatalf("prefixes=%v got=%s want=%s", prefixes, got, want)
				}
			}
		})
	}
}

// TestGeneratedNotificationOverlayOwnership proves validation and runtime share longest App ownership and raw fallback semantics.
func TestGeneratedNotificationOverlayOwnership(t *testing.T) {
	root := notificationInventoryProject(t, "apps:\n  notification:\n    components: [cli, cache]\n  wrap-notification:\n    components: [cli]\n", "notification", "billing", "wrap", "wrap-notification")
	input := fixtureGenerationInput(root, map[string]string{"NOTIFICATION_SUPPORTED_DRIVERS": "log,webhook", "NOTIFICATION_CACHE_DRIVER": "redis", "NOTIFICATION_NOTIFICATION_DRIVER": "hosted", "BILLING_NOTIFICATION_CACHE_DRIVER": "", "BILLING_NOTIFICATION_NOTIFICATION_WEBHOOK_URL": "https://example.com/hook", "NOTIFICATION_SHARED_DRIVER": "webhook", "NOTIFICATION_SHARED_WEBHOOK_URL": "https://example.com/hook", "WRAP_NOTIFICATION_NOTIFICATION_DRIVER": "hosted", "WRAP_NOTIFICATION_NOTIFICATION_WEBHOOK_URL": "invalid"})
	if _, err := generateNotificationFiles(input); err != nil {
		t.Fatal(err)
	}
	if names := discoverNotificationNames(input); !reflect.DeepEqual(names, []string{"cache", "notification", "shared"}) {
		t.Fatalf("names=%v", names)
	}
	writeFixtureGoMod(t, root, fixtureModuleSpec("example.com/notificationoverlayownership", []string{"github.com/goforj/notification", "github.com/goforj/env/v2"}, nil, nil))
	if err := os.WriteFile(filepath.Join(root, "internal/notifications/overlay_ownership_test.go"), []byte(notificationOverlayOwnershipRuntimeTests), 0600); err != nil {
		t.Fatal(err)
	}
	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/notifications", "", nil)
	source := readNotificationInventoryArtifact(t, root, NotificationManagerPath)
	if !strings.Contains(source, `"WRAP":`) || strings.Contains(source, `"WRAP_NOTIFICATION":`) {
		t.Fatal("incorrect active App scopes")
	}
}

const notificationOverlayOwnershipRuntimeTests = `package notifications
import ("strings"; "testing")
// TestOwnershipFallback exercises ignored sibling values and raw normalization without sending traffic.
func TestOwnershipFallback(t *testing.T) {
 t.Setenv("NOTIFICATION_CACHE_DRIVER","redis")
 t.Setenv("NOTIFICATION_NOTIFICATION_DRIVER","hosted")
 t.Setenv("WRAP_NOTIFICATION_NOTIFICATION_DRIVER","hosted")
 t.Setenv("WRAP_NOTIFICATION_NOTIFICATION_WEBHOOK_URL","invalid")
 for _,prefix:=range []string{"BILLING","WRAP"} {
  for _,name:=range []string{"default","cache","notification"} {
   for _,override:=range []string{""," \t "," LoG "} {
    key:=prefix+"_NOTIFICATION"
    if name!="default" { key+="_"+strings.ToUpper(name) }
    if prefix!="WRAP" || name!="notification" { t.Setenv(key+"_DRIVER",override) }
    _,closer,err:=newSender(prefix,name)
    if err!=nil || closer!=nil { t.Fatalf("%s/%s override=%q closer=%v err=%v",prefix,name,override,closer,err) }
   }
  }
 }
 t.Setenv("NOTIFICATION_SHARED_DRIVER"," WeBhOoK ")
 t.Setenv("NOTIFICATION_SHARED_WEBHOOK_URL","https://example.com/hook")
 for _,test:=range []struct{override,want string}{{"","webhook"},{" \t ","log"},{" LoG ","log"}} {
  t.Setenv("BILLING_NOTIFICATION_SHARED_DRIVER",test.override)
  value:=strings.ToLower(strings.TrimSpace(notificationValue("BILLING","shared","DRIVER","log")))
  if value=="" { value="log" }
  if value!=test.want { t.Fatalf("value=%s want=%s",value,test.want) }
  _,closer,err:=newSender("BILLING","shared")
  if err!=nil { t.Fatal(err) }
  if closer!=nil { if err:=closer.Close();err!=nil { t.Fatal(err) } }
 }
}
`

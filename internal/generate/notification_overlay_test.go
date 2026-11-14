package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNotificationOverlayDrivers aligns validation and active-driver discovery with the raw environment fallback contract.
func TestNotificationOverlayDrivers(t *testing.T) {
	for _, scope := range []string{"", "_CHECKOUT"} {
		for _, test := range []struct {
			name, base, overlay, want string
		}{
			{"empty inherits webhook", "webhook", "", "webhook"},
			{"whitespace selects log", "webhook", " \t ", "log"},
			{"explicit normalized webhook", "log", " WeBhOoK ", "webhook"},
			{"explicit log", "webhook", "log", "log"},
			{"blank base defaults", "", "", "log"},
			{"whitespace base defaults", " \t ", "", "log"},
		} {
			t.Run(scope+"/"+test.name, func(t *testing.T) {
				root := t.TempDir()
				key := "NOTIFICATION" + scope + "_DRIVER"
				appKey := "BILLING_" + key
				values := map[string]string{key: test.base, appKey: test.overlay, "NOTIFICATION_SUPPORTED_DRIVERS": "log,webhook"}
				input := fixtureGenerationInput(root, values)
				drivers := appPrefixedActiveDrivers(input, "NOTIFICATION", "log", false)
				if len(drivers) != 1 || drivers[0].key != appKey || drivers[0].driver != test.want {
					t.Fatalf("active drivers = %#v, want %s=%s", drivers, appKey, test.want)
				}
				if count, err := generateNotificationFiles(input); err != nil || count != 2 {
					t.Fatalf("generate: count=%d err=%v", count, err)
				}
				manager, err := os.ReadFile(filepath.Join(root, NotificationManagerPath))
				if err != nil {
					t.Fatal(err)
				}
				_, manifest, found := strings.Cut(string(manager), "var compiledNotificationDrivers = []string{")
				if !found {
					t.Fatal("generated manager does not declare compiled Notification drivers")
				}
				manifest, _, _ = strings.Cut(manifest, "}")
				if !strings.Contains(manifest, `"log"`) || !strings.Contains(manifest, `"webhook"`) {
					t.Fatalf("wrong manifest: %s", manifest)
				}
				if count, err := generateNotificationFiles(input); err != nil || count != 0 {
					t.Fatalf("regenerate metadata: count=%d err=%v", count, err)
				}
			})
		}
	}
}

// TestNotificationOverlayRejectsInvalidDrivers keeps inherited and explicit drivers inside the compiled contract.
func TestNotificationOverlayRejectsInvalidDrivers(t *testing.T) {
	for _, scope := range []string{"", "_CHECKOUT"} {
		for _, test := range []struct{ name, base, overlay, supported, want string }{
			{"inherited gated webhook", "webhook", "", "log", "not enabled"},
			{"explicit gated webhook", "log", "webhook", "log", "not enabled"},
			{"unknown provider", "log", "hosted", "", "unsupported driver"},
			{"unknown manifest", "log", "log", "log,hosted", "unsupported driver"},
		} {
			t.Run(scope+"/"+test.name, func(t *testing.T) {
				input := fixtureGenerationInput(t.TempDir(), map[string]string{"NOTIFICATION" + scope + "_DRIVER": test.base, "BILLING_NOTIFICATION" + scope + "_DRIVER": test.overlay, "NOTIFICATION_SUPPORTED_DRIVERS": test.supported})
				if count, err := generateNotificationFiles(input); err == nil || count != 0 || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("generate = %d %v, want %s", count, err, test.want)
				}
			})
		}
	}
}

// TestGeneratedNotificationOverlayRuntime checks raw fallback and compiled guards against the released env SDK.
func TestGeneratedNotificationOverlayRuntime(t *testing.T) {
	root := t.TempDir()
	input := fixtureGenerationInput(root, map[string]string{"NOTIFICATION_SUPPORTED_DRIVERS": "log,webhook"})
	if _, err := generateNotificationFiles(input); err != nil {
		t.Fatal(err)
	}
	writeFixtureGoMod(t, root, fixtureModuleSpec("example.com/notificationoverlay", []string{"github.com/goforj/env/v2", "github.com/goforj/notification"}, nil, nil))
	if err := os.WriteFile(filepath.Join(root, "internal/notifications/overlay_test.go"), []byte(notificationOverlayRuntimeTests), 0600); err != nil {
		t.Fatal(err)
	}
	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/notifications", "", nil)
}

const notificationOverlayRuntimeTests = `package notifications
import ("strings"; "testing")
// TestOverlayRuntime exercises both resource scopes without sending traffic.
func TestOverlayRuntime(t *testing.T) {
 for _,name := range []string{"default","checkout"} {
  t.Run(name,func(t *testing.T) {
   key := "NOTIFICATION"
   if name != "default" { key += "_CHECKOUT" }
   t.Setenv(key+"_WEBHOOK_URL","https://example.com/hook")
   for _,test := range []struct { base,overlay,want string }{
    {"webhook","","webhook"},{"webhook"," \t ","log"},{"log"," WeBhOoK ","webhook"},
    {"webhook","log","log"},{"","","log"},{" \t ","","log"},
   } {
    t.Setenv(key+"_DRIVER",test.base)
    t.Setenv("BILLING_"+key+"_DRIVER",test.overlay)
    got := strings.ToLower(strings.TrimSpace(notificationValue("BILLING",name,"DRIVER","log")))
    if got == "" { got = "log" }
    if got != test.want { t.Fatalf("driver=%s want=%s",got,test.want) }
    _,closer,err := newSender("BILLING",name)
    if err != nil { t.Fatal(err) }
    if closer != nil { if err := closer.Close(); err != nil { t.Fatal(err) } }
   }
   t.Setenv(key+"_DRIVER","webhook")
   t.Setenv("BILLING_"+key+"_DRIVER","")
   saved := compiledNotificationDrivers
   compiledNotificationDrivers = []string{"log"}
   defer func(){ compiledNotificationDrivers = saved }()
   if _,_,err := newSender("BILLING",name); err == nil || !strings.Contains(err.Error(),"not built in") { t.Fatalf("inherited uncompiled driver: %v",err) }
  })
 }
}
`

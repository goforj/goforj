package localproviders

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/goforj/goforj/internal/coredeps"
	"github.com/goforj/goforj/internal/generate"
)

// TestNotificationExcludedOverlayLifecycle preserves raw App fallback through provider publication and restoration.
func TestNotificationExcludedOverlayLifecycle(t *testing.T) {
	for _, scope := range []string{"", "_CHECKOUT"} {
		for _, overlay := range []struct{ name, assignment, want string }{
			{"absent", "", "webhook"},
			{"empty", "=", "webhook"},
			{"whitespace", "=' \t '", "log"},
			{"explicit", "=' WeBhOoK '", "webhook"},
		} {
			for _, method := range []string{"Drivers", "PortableFor"} {
				t.Run(scope+"/"+overlay.name+"/"+method, func(t *testing.T) {
					root := fixture(t)
					put(t, root, ".goforj.yml", "module_name: example.org/excluded\nrender:\n  components: [cli, notification]\napps:\n  billing:\n    components: [cli, notification]\n")
					parent := "NOTIFICATION" + scope + "_DRIVER"
					key := "BILLING_" + parent
					contents := "NOTIFICATION_SUPPORTED_DRIVERS=log,webhook\n" + parent + "=webhook\nNOTIFICATION" + scope + "_WEBHOOK_URL=https://example.com/hook\n"
					if overlay.assignment != "" {
						contents += key + overlay.assignment + "\n"
					}
					put(t, root, ".env", contents)
					session := openTest(t, root)
					original := clone(session.Current)
					var values map[string]string
					var err error
					if method == "Drivers" {
						values, err = session.Drivers(map[string]string{parent: "log"})
					} else {
						values, err = session.PortableFor(map[string]bool{key: true})
					}
					if err != nil {
						t.Fatal(err)
					}
					if (overlay.name == "empty" || overlay.name == "absent") && values[key] != "webhook" {
						t.Fatalf("excluded driver=%q", values[key])
					}
					if err := session.Apply(values); err != nil {
						t.Fatal(err)
					}
					for pass := 0; pass < 2; pass++ {
						result, err := generate.GenerateProjectFiles(root, generate.GenerationSelection{Notification: true})
						if err != nil {
							t.Fatal(err)
						}
						if pass == 1 && result.ChangedFiles != 0 {
							t.Fatalf("regeneration drift=%#v", result)
						}
					}
					session = openTest(t, root)
					definition := resourceDefinitions(session.config, key)[0]
					if got := session.resourceSetting(session.Current, key, "DRIVER", "BILLING_", definition); got != overlay.want {
						t.Fatalf("excluded effective driver=%q, want %q", got, overlay.want)
					}
					if overlay.name == "empty" && method == "Drivers" {
						name := "default"
						if scope != "" {
							name = "checkout"
						}
						testNotificationExcludedRuntime(t, root, session.Current, name)
					}
					if err := session.Restore(); err != nil {
						t.Fatal(err)
					}
					session = openTest(t, root)
					for _, setting := range []string{parent, key} {
						got, present := session.Current[setting]
						want, existed := original[setting]
						if got != want || present != existed {
							t.Fatalf("restored %s=%q,%t want %q,%t", setting, got, present, want, existed)
						}
					}
				})
			}
		}
	}
}

// testNotificationExcludedRuntime checks the published plan against generated senders and the released environment SDK.
func testNotificationExcludedRuntime(t *testing.T, root string, values map[string]string, name string) {
	t.Helper()
	put(t, root, "go.mod", fmt.Sprintf("module example.org/excluded\n\ngo 1.26.0\n\nrequire (\n github.com/goforj/env/v2 %s\n github.com/goforj/notification %s\n)\n", coredeps.MustVersionFor("github.com/goforj/env/v2"), coredeps.MustVersionFor("github.com/goforj/notification")))
	source := fmt.Sprintf(`package notifications
import ("strings";"testing")
// TestExcludedRuntime verifies publication retains the App's sender without sending traffic.
func TestExcludedRuntime(t *testing.T) {
 got:=strings.ToLower(strings.TrimSpace(notificationValue("BILLING",%q,"DRIVER","log")))
 if got!="webhook" {t.Fatalf("excluded runtime driver=%%q",got)}
 sender,closer,err:=newSender("BILLING",%q)
 if err!=nil {t.Fatal(err)}
 if sender==nil {t.Fatal("missing sender")}
 if closer!=nil {if err:=closer.Close();err!=nil {t.Fatal(err)}}
}
`, name, name)
	if err := os.WriteFile(filepath.Join(root, "internal/notifications/excluded_test.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "test", "-mod=mod", "./internal/notifications")
	command.Dir = root
	environment := os.Environ()
	for key, value := range values {
		environment = append(environment, key+"="+value)
	}
	command.Env = append(environment, "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated runtime: %v\n%s", err, output)
	}
}

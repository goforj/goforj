package localproviders

import (
	"testing"

	"github.com/goforj/goforj/internal/generate"
)

// TestNotificationPortableRegeneration retains deployment settings while named senders use log.
func TestNotificationPortableRegeneration(t *testing.T) {
	root := fixture(t)
	put(t, root, ".env", "NOTIFICATION_DRIVER=log\nNOTIFICATION_SUPPORTED_DRIVERS=log,webhook\nNOTIFICATION_ALERTS_DRIVER=webhook\nNOTIFICATION_ALERTS_WEBHOOK_URL=https://example.com/root-hook\nADMIN_NOTIFICATION_ONLY_DRIVER=webhook\nADMIN_NOTIFICATION_ONLY_WEBHOOK_URL=https://example.com/admin-hook\n")
	session := openTest(t, root)
	portable, err := session.Portable()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"NOTIFICATION_ALERTS_DRIVER", "ADMIN_NOTIFICATION_ONLY_DRIVER"} {
		if portable[key] != "log" {
			t.Fatalf("%s = %s", key, portable[key])
		}
	}
	if err := session.Apply(portable); err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		result, err := generate.GenerateProjectFiles(root, generate.GenerationSelection{Notification: true})
		if err != nil {
			t.Fatal(err)
		}
		if pass == 1 && result.ChangedFiles != 0 {
			t.Fatalf("regeneration drift: %#v", result)
		}
	}
	session = openTest(t, root)
	for key, want := range map[string]string{"NOTIFICATION_ALERTS_WEBHOOK_URL": "https://example.com/root-hook", "ADMIN_NOTIFICATION_ONLY_WEBHOOK_URL": "https://example.com/admin-hook"} {
		if session.Current[key] != want {
			t.Fatalf("%s lost deployment setting", key)
		}
	}
	restored := clone(session.Current)
	restored["NOTIFICATION_ALERTS_DRIVER"] = "webhook"
	restored["ADMIN_NOTIFICATION_ONLY_DRIVER"] = "webhook"
	if err := session.Apply(restored); err != nil {
		t.Fatal(err)
	}
	if _, err := generate.GenerateProjectFiles(root, generate.GenerationSelection{Notification: true}); err != nil {
		t.Fatal(err)
	}
	session = openTest(t, root)
	if session.Current["NOTIFICATION_ALERTS_WEBHOOK_URL"] != "https://example.com/root-hook" || session.Current["ADMIN_NOTIFICATION_ONLY_WEBHOOK_URL"] != "https://example.com/admin-hook" {
		t.Fatal("switching back lost webhook settings")
	}
}

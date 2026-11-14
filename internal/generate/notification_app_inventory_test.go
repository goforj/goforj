package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNotificationGenerationIncludesConventionalApps keeps compiled scopes aligned with Apps the renderer discovers on disk.
func TestNotificationGenerationIncludesConventionalApps(t *testing.T) {
	for _, test := range []struct {
		name        string
		app         string
		prefix      string
		environment string
		accessor    string
	}{
		{name: "no overlay", app: "billing", prefix: "BILLING"},
		{name: "App-only evaluator", app: "billing", prefix: "BILLING", environment: "BILLING_NOTIFICATION_CHECKOUT_DRIVER=log\n", accessor: "Checkout()"},
		{name: "resource word in App name", app: "billing-notification", prefix: "BILLING_NOTIFICATION", environment: "BILLING_NOTIFICATION_NOTIFICATION_CHECKOUT_DRIVER=log\n", accessor: "Checkout()"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := notificationInventoryProject(t, "", test.app)
			writeGenerationEnvironmentFile(t, root, ".env", test.environment)
			input, err := loadProjectGenerationInput(root)
			if err != nil {
				t.Fatal(err)
			}
			if count, err := generateNotificationFiles(input); err != nil || count != 2 {
				t.Fatalf("generate: count=%d, err=%v", count, err)
			}
			manager := readNotificationInventoryArtifact(t, root, NotificationManagerPath)
			if !strings.Contains(manager, `"`+test.prefix+`":`) {
				t.Fatalf("conventional App %s missing from generated manager", test.prefix)
			}
			if test.accessor != "" && !strings.Contains(readNotificationInventoryArtifact(t, root, NotificationAccessorsPath), test.accessor) {
				t.Fatalf("App-only accessor %s missing", test.accessor)
			}
			if count, err := generateNotificationFiles(input); err != nil || count != 0 {
				t.Fatalf("regenerate drift: count=%d, err=%v", count, err)
			}
		})
	}
}

// TestNotificationGenerationValidatesConventionalAppOverlays prevents filesystem-discovered App declarations from bypassing validation.
func TestNotificationGenerationValidatesConventionalAppOverlays(t *testing.T) {
	for _, environment := range []string{
		"BILLING_NOTIFICATION_DRIVER=hosted\n",
		"BILLING_NOTIFICATION_CHECKOUT_DRIVER=unknown\n",
	} {
		t.Run(strings.Split(environment, "\n")[0], func(t *testing.T) {
			root := notificationInventoryProject(t, "", "billing")
			writeGenerationEnvironmentFile(t, root, ".env", environment)
			input, err := loadProjectGenerationInput(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := generateNotificationFiles(input); err == nil || !strings.Contains(err.Error(), "BILLING_NOTIFICATION_") {
				t.Fatalf("invalid conventional App overlay: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, NotificationManagerPath)); !os.IsNotExist(err) {
				t.Fatalf("invalid overlay published generated manager: %v", err)
			}
		})
	}
}

// TestNotificationGenerationOmitsDisabledAndRemovedConventionalApps preserves explicit ownership and drops stale environment overlays.
func TestNotificationGenerationOmitsDisabledAndRemovedConventionalApps(t *testing.T) {
	root := notificationInventoryProject(t, "apps:\n  reports:\n    components: [cli]\n", "billing", "reports")
	writeGenerationEnvironmentFile(t, root, ".env", "BILLING_NOTIFICATION_CHECKOUT_DRIVER=log\nREPORTS_NOTIFICATION_DISABLED_DRIVER=hosted\nMISSING_NOTIFICATION_STALE_DRIVER=hosted\n")
	generate := func() {
		t.Helper()
		input, err := loadProjectGenerationInput(root)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := generateNotificationFiles(input); err != nil {
			t.Fatal(err)
		}
	}
	generate()
	manager := readNotificationInventoryArtifact(t, root, NotificationManagerPath)
	accessors := readNotificationInventoryArtifact(t, root, NotificationAccessorsPath)
	if !strings.Contains(manager, `"BILLING":`) || !strings.Contains(accessors, "Checkout()") {
		t.Fatal("active conventional App missing from generated inventory")
	}
	if strings.Contains(manager, `"REPORTS":`) || strings.Contains(manager, `"MISSING":`) || strings.Contains(accessors, "Disabled()") || strings.Contains(accessors, "Stale()") {
		t.Fatal("disabled or nonexistent App overlay retained")
	}
	if err := os.RemoveAll(filepath.Join(root, "cmd", "billing")); err != nil {
		t.Fatal(err)
	}
	generate()
	if strings.Contains(readNotificationInventoryArtifact(t, root, NotificationManagerPath), `"BILLING":`) || strings.Contains(readNotificationInventoryArtifact(t, root, NotificationAccessorsPath), "Checkout()") {
		t.Fatal("removed conventional App retained by stale environment overlay")
	}
}

// notificationInventoryProject supplies renderer-compatible App markers without rendering into the repository.
func notificationInventoryProject(t *testing.T, appConfig string, apps ...string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("render:\n  components: [cli, notification]\n"+appConfig), 0600); err != nil {
		t.Fatal(err)
	}
	for _, app := range apps {
		dir := filepath.Join(root, "cmd", app)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// readNotificationInventoryArtifact keeps inventory assertions focused on the generated contract.
func readNotificationInventoryArtifact(t *testing.T, root, path string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

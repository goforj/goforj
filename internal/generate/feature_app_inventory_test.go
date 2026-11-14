package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFeatureGenerationIncludesConventionalApps keeps compiled scopes aligned with Apps the renderer discovers on disk.
func TestFeatureGenerationIncludesConventionalApps(t *testing.T) {
	for _, test := range []struct {
		name        string
		app         string
		prefix      string
		environment string
		accessor    string
	}{
		{name: "no overlay", app: "billing", prefix: "BILLING"},
		{name: "App-only evaluator", app: "billing", prefix: "BILLING", environment: "BILLING_FEATURE_CHECKOUT_DRIVER=memory\n", accessor: "Checkout()"},
		{name: "resource word in App name", app: "billing-feature", prefix: "BILLING_FEATURE", environment: "BILLING_FEATURE_FEATURE_CHECKOUT_DRIVER=memory\n", accessor: "Checkout()"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := featureInventoryProject(t, "", test.app)
			writeGenerationEnvironmentFile(t, root, ".env", test.environment)
			input, err := loadProjectGenerationInput(root)
			if err != nil {
				t.Fatal(err)
			}
			if count, err := generateFeatureFiles(input); err != nil || count != 2 {
				t.Fatalf("generate: count=%d, err=%v", count, err)
			}
			manager := readFeatureInventoryArtifact(t, root, FeatureManagerPath)
			if !strings.Contains(manager, `"`+test.prefix+`":`) {
				t.Fatalf("conventional App %s missing from generated manager", test.prefix)
			}
			if test.accessor != "" && !strings.Contains(readFeatureInventoryArtifact(t, root, FeatureAccessorsPath), test.accessor) {
				t.Fatalf("App-only accessor %s missing", test.accessor)
			}
			if count, err := generateFeatureFiles(input); err != nil || count != 0 {
				t.Fatalf("regenerate drift: count=%d, err=%v", count, err)
			}
		})
	}
}

// TestFeatureGenerationValidatesConventionalAppOverlays prevents filesystem-discovered App declarations from bypassing validation.
func TestFeatureGenerationValidatesConventionalAppOverlays(t *testing.T) {
	for _, environment := range []string{
		"BILLING_FEATURE_DRIVER=hosted\n",
		"BILLING_FEATURE_CHECKOUT_DRIVER=memory\nBILLING_FEATURE_CHECKOUT_PATH=flags.json\n",
	} {
		t.Run(strings.Split(environment, "\n")[0], func(t *testing.T) {
			root := featureInventoryProject(t, "", "billing")
			writeGenerationEnvironmentFile(t, root, ".env", environment)
			input, err := loadProjectGenerationInput(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := generateFeatureFiles(input); err == nil || !strings.Contains(err.Error(), "BILLING_FEATURE_") {
				t.Fatalf("invalid conventional App overlay: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, FeatureManagerPath)); !os.IsNotExist(err) {
				t.Fatalf("invalid overlay published generated manager: %v", err)
			}
		})
	}
}

// TestFeatureGenerationOmitsDisabledAndRemovedConventionalApps preserves explicit ownership and drops stale environment overlays.
func TestFeatureGenerationOmitsDisabledAndRemovedConventionalApps(t *testing.T) {
	root := featureInventoryProject(t, "apps:\n  reports:\n    components: [cli]\n", "billing", "reports")
	writeGenerationEnvironmentFile(t, root, ".env", "BILLING_FEATURE_CHECKOUT_DRIVER=memory\nREPORTS_FEATURE_DISABLED_DRIVER=hosted\nMISSING_FEATURE_STALE_DRIVER=hosted\n")
	generate := func() {
		t.Helper()
		input, err := loadProjectGenerationInput(root)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := generateFeatureFiles(input); err != nil {
			t.Fatal(err)
		}
	}
	generate()
	manager := readFeatureInventoryArtifact(t, root, FeatureManagerPath)
	accessors := readFeatureInventoryArtifact(t, root, FeatureAccessorsPath)
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
	if strings.Contains(readFeatureInventoryArtifact(t, root, FeatureManagerPath), `"BILLING":`) || strings.Contains(readFeatureInventoryArtifact(t, root, FeatureAccessorsPath), "Checkout()") {
		t.Fatal("removed conventional App retained by stale environment overlay")
	}
}

// featureInventoryProject supplies renderer-compatible App markers without rendering into the repository.
func featureInventoryProject(t *testing.T, appConfig string, apps ...string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), []byte("render:\n  components: [cli, feature]\n"+appConfig), 0600); err != nil {
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

// readFeatureInventoryArtifact keeps inventory assertions focused on the generated contract.
func readFeatureInventoryArtifact(t *testing.T, root, path string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

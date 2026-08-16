// Package envcontract_test verifies environment contracts through their public boundary.
package envcontract_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/envcontract"
	"github.com/goforj/goforj/internal/envfile"
)

// TestInitializeCreatesPrivateLocalEnvironment verifies clones receive fresh framework secrets without filling app credentials.
func TestInitializeCreatesPrivateLocalEnvironment(t *testing.T) {
	root := t.TempDir()
	example := "APP_ENV=local\nAPP_KEY=\nAPP_DIAG_TOKEN=\nAPI_JWT_SECRET_KEY=\nCUSTOM_TOKEN=\n"
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte(example), 0o644); err != nil {
		t.Fatalf("write example: %v", err)
	}
	created, err := envcontract.Initialize(root)
	if err != nil || !created {
		t.Fatalf("Initialize() = %t, %v; want true, nil", created, err)
	}
	content, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatalf("read local environment: %v", err)
	}
	text := string(content)
	for _, key := range []string{"APP_KEY", "APP_DIAG_TOKEN", "API_JWT_SECRET_KEY"} {
		if strings.Contains(text, key+"=\n") {
			t.Errorf("%s was not generated:\n%s", key, text)
		}
	}
	if !strings.Contains(text, "CUSTOM_TOKEN=\n") {
		t.Fatalf("custom credential was unexpectedly generated:\n%s", text)
	}
	info, err := os.Stat(filepath.Join(root, ".env"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("local environment mode = %v, %v; want 0600", info, err)
	}
	created, err = envcontract.Initialize(root)
	if err != nil || created {
		t.Fatalf("second Initialize() = %t, %v; want false, nil", created, err)
	}
}

// TestSyncPublishesSafeExampleAndTestingContracts verifies local credentials never cross the committed boundary.
func TestSyncPublishesSafeExampleAndTestingContracts(t *testing.T) {
	root := t.TempDir()
	local := "APP_ENV=local\nAPP_KEY=local-key\nDB_HOST=mysql\nDB_DATABASE=app\nDB_USERNAME=owner\nDB_PASSWORD=owner-secret\nCUSTOM_TOKEN=private\nFEATURE_FLAG=true\n"
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(local), 0o600); err != nil {
		t.Fatalf("write local environment: %v", err)
	}
	result, err := envcontract.Sync(root)
	if err != nil || !result.ExampleChanged || !result.TestingChanged {
		t.Fatalf("Sync() = %#v, %v; want both contracts changed", result, err)
	}
	example, err := os.ReadFile(filepath.Join(root, ".env.example"))
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	testingEnvironment, err := os.ReadFile(filepath.Join(root, ".env.testing"))
	if err != nil {
		t.Fatalf("read testing environment: %v", err)
	}
	combined := string(example) + string(testingEnvironment)
	for _, secret := range []string{"local-key", "owner-secret", "private"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("committed contracts exposed %q:\n%s", secret, combined)
		}
	}
	for _, want := range []string{"DB_PASSWORD=test", "DB_DATABASE=app_testing", "FEATURE_FLAG=true"} {
		if !strings.Contains(string(testingEnvironment), want) {
			t.Errorf("testing environment omitted %q:\n%s", want, testingEnvironment)
		}
	}
	if err := envcontract.Check(root); err != nil {
		t.Fatalf("Check() after sync: %v", err)
	}
}

// TestSetLocalUsesDotenvEncodingWithoutPublishingContracts verifies prompted values remain private until the generation lifecycle runs.
func TestSetLocalUsesDotenvEncodingWithoutPublishingContracts(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte("APP_ENV=local\nCUSTOM_TOKEN=\n"), 0o644); err != nil {
		t.Fatalf("write example: %v", err)
	}
	secret := "line one\nINJECTED=true"
	if err := envcontract.SetLocal(root, "CUSTOM_TOKEN", secret); err != nil {
		t.Fatalf("SetLocal() error: %v", err)
	}
	local, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatalf("read local environment: %v", err)
	}
	if strings.Count(string(local), "INJECTED=") != 1 || !strings.Contains(string(local), `CUSTOM_TOKEN="line one\nINJECTED=true"`) {
		t.Fatalf("local environment did not encode the value as one assignment:\n%s", local)
	}
	example, err := os.ReadFile(filepath.Join(root, ".env.example"))
	if err != nil || string(example) != "APP_ENV=local\nCUSTOM_TOKEN=\n" {
		t.Fatalf("SetLocal changed committed example: %q, %v", example, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".env.testing")); !os.IsNotExist(err) {
		t.Fatalf("SetLocal published .env.testing: %v", err)
	}
}

// TestCheckUsesCommittedExampleWithoutLocalEnvironment keeps CI verification read-only on clean checkouts.
func TestCheckUsesCommittedExampleWithoutLocalEnvironment(t *testing.T) {
	root := t.TempDir()
	example := []byte("APP_ENV=local\nAPP_KEY=\nDB_DATABASE=app\n")
	if err := os.WriteFile(filepath.Join(root, ".env.example"), example, 0o644); err != nil {
		t.Fatalf("write example: %v", err)
	}
	testingEnvironment := envfile.MergeTesting(nil, example)
	if err := os.WriteFile(filepath.Join(root, ".env.testing"), testingEnvironment, 0o644); err != nil {
		t.Fatalf("write testing environment: %v", err)
	}
	if err := envcontract.Check(root); err != nil {
		t.Fatalf("Check() without .env: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".env")); !os.IsNotExist(err) {
		t.Fatalf("Check() created .env: %v", err)
	}
}

// TestCheckReportsMissingTestingContract verifies CI receives an actionable drift failure without a write.
func TestCheckReportsMissingTestingContract(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte("APP_ENV=local\n"), 0o644); err != nil {
		t.Fatalf("write example: %v", err)
	}
	err := envcontract.Check(root)
	if err == nil || !strings.Contains(err.Error(), ".env.testing") || !strings.Contains(err.Error(), "run forj generate") {
		t.Fatalf("Check() error = %v, want actionable .env.testing drift", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".env.testing")); !os.IsNotExist(statErr) {
		t.Fatalf("Check() created .env.testing: %v", statErr)
	}
}

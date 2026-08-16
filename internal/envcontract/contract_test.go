// Package envcontract_test verifies environment contracts through their public boundary.
package envcontract_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// TestInitializeReplacesCommittedFrameworkSecrets verifies cloned projects never trust repository-known signing material.
func TestInitializeReplacesCommittedFrameworkSecrets(t *testing.T) {
	root := t.TempDir()
	example := "APP_KEY=known-app-key\nAPP_DIAG_TOKEN=known-diag-token\nAPI_JWT_SECRET_KEY=known-jwt-key\n"
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte(example), 0o644); err != nil {
		t.Fatalf("write example: %v", err)
	}
	if _, err := envcontract.Initialize(root); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}
	local, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatalf("read local environment: %v", err)
	}
	for _, known := range []string{"known-app-key", "known-diag-token", "known-jwt-key"} {
		if strings.Contains(string(local), known) {
			t.Fatalf("local environment trusted committed secret %q:\n%s", known, local)
		}
	}
}

// TestInitializeTightensExistingLocalPermissions verifies upgrades repair historically public local environment files.
func TestInitializeTightensExistingLocalPermissions(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".env")
	if err := os.WriteFile(path, []byte("APP_ENV=local\n"), 0o644); err != nil {
		t.Fatalf("write local environment: %v", err)
	}
	if _, err := envcontract.Initialize(root); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("local environment mode = %v, %v; want 0600", info, err)
	}
}

// TestSyncRejectsEnvironmentSymlinks prevents repository paths from importing files outside the project contract.
func TestSyncRejectsEnvironmentSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "outside.env")
	if err := os.WriteFile(target, []byte("CUSTOM_TOKEN=outside-secret\n"), 0o600); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(root, ".env")); err != nil {
		t.Skipf("create environment symlink: %v", err)
	}
	if _, err := envcontract.Sync(root); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("Sync() error = %v, want regular-file rejection", err)
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
	for _, want := range []string{"DB_PASSWORD=test", "DB_DATABASE=app_testing", "FEATURE_FLAG="} {
		if !strings.Contains(string(testingEnvironment), want) {
			t.Errorf("testing environment omitted %q:\n%s", want, testingEnvironment)
		}
	}
	if err := envcontract.Check(root); err != nil {
		t.Fatalf("Check() after sync: %v", err)
	}
}

// TestSyncRejectsNonPortableAssignments verifies valid-but-ambiguous dotenv grammar never reaches committed contracts.
func TestSyncRejectsNonPortableAssignments(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SERVICE.API_KEY=private\n"), 0o600); err != nil {
		t.Fatalf("write local environment: %v", err)
	}
	if _, err := envcontract.Sync(root); err == nil || !strings.Contains(err.Error(), "portable") {
		t.Fatalf("Sync() error = %v, want portable-key rejection", err)
	}
	for _, name := range []string{".env.example", ".env.testing"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("Sync() published %s after validation failure: %v", name, err)
		}
	}
}

// TestConcurrentSetLocalPreservesEveryUpdate verifies project locking covers the full read-modify-write lifecycle.
func TestConcurrentSetLocalPreservesEveryUpdate(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte("APP_ENV=local\n"), 0o644); err != nil {
		t.Fatalf("write example: %v", err)
	}
	const writers = 12
	var wait sync.WaitGroup
	errorsByWriter := make(chan error, writers)
	for index := 0; index < writers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			errorsByWriter <- envcontract.SetLocal(root, fmt.Sprintf("CUSTOM_%02d", index), fmt.Sprintf("value-%02d", index))
		}(index)
	}
	wait.Wait()
	close(errorsByWriter)
	for err := range errorsByWriter {
		if err != nil {
			t.Fatalf("SetLocal() error: %v", err)
		}
	}
	content, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatalf("read local environment: %v", err)
	}
	for index := 0; index < writers; index++ {
		want := fmt.Sprintf("CUSTOM_%02d=value-%02d", index, index)
		if !strings.Contains(string(content), want) {
			t.Fatalf("concurrent local environment omitted %q:\n%s", want, content)
		}
	}
}

// TestSyncRejectsUnmanagedTestingContract protects ignored legacy credentials from silent destructive migration.
func TestSyncRejectsUnmanagedTestingContract(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("APP_ENV=local\n"), 0o600); err != nil {
		t.Fatalf("write local environment: %v", err)
	}
	legacy := []byte("CUSTOM_TOKEN=private-test-token\n")
	path := filepath.Join(root, ".env.testing")
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatalf("write legacy testing environment: %v", err)
	}
	_, err := envcontract.Sync(root)
	if !errors.Is(err, envcontract.ErrUnmanagedTestingContract) {
		t.Fatalf("Sync() error = %v, want ErrUnmanagedTestingContract", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil || !bytes.Equal(after, legacy) {
		t.Fatalf("legacy testing environment changed: %q, %v", after, readErr)
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

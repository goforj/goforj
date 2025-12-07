package env

import (
	"os"
	"testing"
)

func TestLoadEnvFileIfExists_testingEnv(t *testing.T) {
	tempDir := t.TempDir()
	dotEnvFile := tempDir + "/.env.testing"

	// Write mock .env.testing
	err := os.WriteFile(dotEnvFile, []byte("FAKE_ENV_TESTING=testing_value\nAPP_DEBUG=0"), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp .env.testing: %v", err)
	}

	// Save original working dir to restore later
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(originalDir) // restore after test

	// Move to the temp directory where our mock .env.testing exists
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	// Set environment to "testing" to trigger .env.testing logic
	_ = os.Setenv("APP_ENV", "testing")

	// Reset internal state
	envLoaded = false

	err = LoadEnvFileIfExists()
	if err != nil {
		t.Fatalf("LoadEnvFileIfExists failed: %v", err)
	}

	val := os.Getenv("FAKE_ENV_TESTING")
	if val != "testing_value" {
		t.Errorf("Expected FAKE_ENV_TESTING to be 'testing_value', got %s", val)
	}

	if !IsEnvLoaded() {
		t.Error("Expected IsEnvLoaded to return true")
	}
}

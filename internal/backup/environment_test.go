package backup

import "testing"

// TestDefaultPathUsesConfiguredValueAndFallback verifies empty configuration keeps the documented backup location.
func TestDefaultPathUsesConfiguredValueAndFallback(t *testing.T) {
	t.Setenv("BACKUP_PATH", "")
	if got := DefaultPath(); got != ".goforj/backups" {
		t.Fatalf("DefaultPath() = %q, want fallback", got)
	}

	t.Setenv("BACKUP_PATH", "var/backups")
	if got := DefaultPath(); got != "var/backups" {
		t.Fatalf("DefaultPath() = %q, want configured path", got)
	}
}

// TestRetentionValuePreservesFallbackAndZero verifies typed parsing retains the existing invalid and negative policies.
func TestRetentionValuePreservesFallbackAndZero(t *testing.T) {
	const key = "APP_BACKUP_KEEP_DAILY"
	t.Setenv(key, "invalid")
	if got := retentionValue(key, 14); got != 14 {
		t.Fatalf("retentionValue() = %d, want invalid fallback", got)
	}

	t.Setenv(key, "-1")
	if got := retentionValue(key, 14); got != 14 {
		t.Fatalf("retentionValue() = %d, want negative fallback", got)
	}

	t.Setenv(key, "0")
	if got := retentionValue(key, 14); got != 0 {
		t.Fatalf("retentionValue() = %d, want explicit zero", got)
	}
}

// TestStorageEnvValuePreservesNamedFallback verifies a named disk still inherits root configuration only when its key is empty.
func TestStorageEnvValuePreservesNamedFallback(t *testing.T) {
	t.Setenv("STORAGE_PREFIX", "shared")
	t.Setenv("STORAGE_ARCHIVE_LOGS_PREFIX", "")
	if got := storageEnvValue("archive.logs", "PREFIX"); got != "shared" {
		t.Fatalf("storageEnvValue() = %q, want root fallback", got)
	}

	t.Setenv("STORAGE_ARCHIVE_LOGS_PREFIX", "archive")
	if got := storageEnvValue("archive.logs", "PREFIX"); got != "archive" {
		t.Fatalf("storageEnvValue() = %q, want named value", got)
	}
}

// TestDiscoverStorageResourcesPreservesDefaultPrivateRoot verifies scoped reads retain the established default disk location.
func TestDiscoverStorageResourcesPreservesDefaultPrivateRoot(t *testing.T) {
	t.Setenv("STORAGE_DRIVER", "local")
	t.Setenv("STORAGE_ROOT", "")

	for _, resource := range discoverStorageResources() {
		if resource.Name != "default" {
			continue
		}
		if resource.Root != "storage/app/private" {
			t.Fatalf("default storage root = %q, want storage/app/private", resource.Root)
		}
		return
	}
	t.Fatalf("storage resources = %#v, want default resource", discoverStorageResources())
}

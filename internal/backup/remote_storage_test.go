package backup

import (
	"strings"
	"testing"
)

// TestConfiguredStorageRejectsInvalidBooleanFlags verifies malformed infrastructure flags fail before a storage client is opened.
func TestConfiguredStorageRejectsInvalidBooleanFlags(t *testing.T) {
	tests := []struct {
		name string
		key  string
		open func(*testing.T) error
	}{
		{
			name: "App storage",
			key:  "STORAGE_USE_PATH_STYLE",
			open: func(*testing.T) error {
				_, err := ConfiguredObjectStorage("default")
				return err
			},
		},
		{
			name: "backup repository",
			key:  "APP_BACKUP_S3_USE_PATH_STYLE",
			open: func(t *testing.T) error {
				t.Setenv("APP_BACKUP_DRIVER", "s3")
				_, err := ConfiguredBackupRepository()
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(test.key, "sometimes")
			err := test.open(t)
			if err == nil || !strings.Contains(err.Error(), test.key) {
				t.Fatalf("configured storage error = %v, want invalid %s diagnostic", err, test.key)
			}
		})
	}
}

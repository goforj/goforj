package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MigrationNames returns the ordered SQL migration identities in a project directory.
func MigrationNames(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, "migrations"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		names = append(names, strings.TrimSuffix(entry.Name(), ".up.sql"))
	}
	sort.Strings(names)
	return names, nil
}

// ProjectMigrationFingerprint returns the migration contract fingerprint for a project directory.
func ProjectMigrationFingerprint(root string) (string, error) {
	names, err := MigrationNames(root)
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", nil
	}
	return MigrationFingerprint(names), nil
}

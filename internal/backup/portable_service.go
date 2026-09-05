package backup

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
)

// PortableService orchestrates database-neutral archive creation and restore.
type PortableService struct{}

// NewPortableService creates a portable transfer service.
func NewPortableService() *PortableService { return &PortableService{} }

// Create exports selected tables from a SQL database into a portable backup set.
func (s *PortableService) Create(ctx context.Context, dir string, db *sql.DB, dialect SQLDialect, tables []string, migrationFingerprints ...string) (PortableArchive, error) {
	if err := createExclusivePrivateDirectory(dir); err != nil {
		return PortableArchive{}, fmt.Errorf("create exclusive portable backup set: %w", err)
	}
	archive, err := ExportPortable(ctx, db, dialect, tables)
	if err != nil {
		return PortableArchive{}, err
	}
	if len(migrationFingerprints) > 0 {
		archive.MigrationFingerprint = migrationFingerprints[0]
	}
	if err := WritePortableArchive(dir, archive); err != nil {
		return PortableArchive{}, err
	}
	return archive, nil
}

// Restore imports a portable backup set into a target SQL database transaction.
func (s *PortableService) Restore(ctx context.Context, dir string, db *sql.DB, dialect SQLDialect, migrationFingerprints ...string) error {
	if db == nil {
		return fmt.Errorf("database connection is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	archive, err := ReadPortableArchive(filepath.Clean(dir))
	if err != nil {
		return err
	}
	if len(migrationFingerprints) > 0 {
		if err := ValidateMigrationFingerprint(archive, migrationFingerprints[0]); err != nil {
			return err
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin portable restore: %w", err)
	}
	if err := ImportPortable(ctx, tx, dialect, archive); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit portable restore: %w", err)
	}
	return nil
}

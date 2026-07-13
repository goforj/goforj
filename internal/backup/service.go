package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Service orchestrates native backup creation and verification.
type Service struct{ Hooks HookRegistry }

// NewService creates a native backup service.
func NewService() *Service {
	return &Service{}
}

// Create writes a native backup set under root and returns its manifest.
func (s *Service) Create(ctx context.Context, root string, resourceName string) (string, Manifest, error) {
	if err := s.Hooks.Run(ctx, HookBeforeCreate); err != nil {
		return "", Manifest{}, fmt.Errorf("before backup hook: %w", err)
	}
	if strings.TrimSpace(root) == "" {
		return "", Manifest{}, fmt.Errorf("backup root is required")
	}
	plan, err := BuildPlan()
	if err != nil {
		return "", Manifest{}, err
	}
	if err := plan.Validate(); err != nil {
		return "", Manifest{}, err
	}
	dir := filepath.Join(root, "backup-"+time.Now().UTC().Format("20060102T150405Z"))
	if err := os.MkdirAll(filepath.Join(dir, "databases"), 0o755); err != nil {
		return "", Manifest{}, fmt.Errorf("create backup set: %w", err)
	}
	manifest := Manifest{Version: 1, CreatedAt: time.Now().UTC()}
	resourceName = normalizeResourceName(resourceName)
	storageSelector := strings.HasPrefix(resourceName, "storage.")
	for _, planned := range plan.Resources {
		if storageSelector {
			continue
		}
		if resourceName != "" && resourceName != planned.Connection.Name {
			continue
		}
		strategy, err := NativeStrategy(planned.Connection.Driver)
		if err != nil {
			return "", Manifest{}, err
		}
		artifactName := planned.Connection.Name + "." + normalizeDriver(planned.Connection.Driver) + ".backup"
		artifact := filepath.Join(dir, "databases", artifactName)
		if err := strategy.Backup(ctx, planned.Connection, artifact); err != nil {
			return "", Manifest{}, fmt.Errorf("backup db.%s: %w", planned.Connection.Name, err)
		}
		hash, size, err := Checksum(artifact)
		if err != nil {
			return "", Manifest{}, err
		}
		manifest.Resources = append(manifest.Resources, Resource{
			ID: "db." + planned.Connection.Name, Kind: "database", Name: planned.Connection.Name,
			Driver: normalizeDriver(planned.Connection.Driver), Strategy: strategy.Name(),
			Artifact: filepath.ToSlash(filepath.Join("databases", artifactName)), Checksum: hash, Size: size,
		})
	}
	for _, storage := range plan.Storage {
		selector := "storage." + storage.Name
		if resourceName != "" && resourceName != selector && resourceName != storage.Name {
			continue
		}
		if storage.Driver == "s3" {
			disk, prefix, err := ConfiguredObjectStorage(storage.Name)
			if err != nil {
				return "", Manifest{}, fmt.Errorf("backup %s: %w", selector, err)
			}
			if storage.Prefix != "" {
				prefix = storage.Prefix
			}
			objectManifest, err := BuildObjectManifest(ctx, StorageObjectLister{Disk: disk}, prefix)
			if err != nil {
				return "", Manifest{}, fmt.Errorf("backup %s: %w", selector, err)
			}
			data, err := MarshalObjectManifest(objectManifest)
			if err != nil {
				return "", Manifest{}, err
			}
			artifactName := storage.Name + ".objects.json"
			artifact := filepath.Join(dir, "storage", artifactName)
			if err := os.MkdirAll(filepath.Dir(artifact), 0o755); err != nil {
				return "", Manifest{}, err
			}
			if err := os.WriteFile(artifact, data, 0o644); err != nil {
				return "", Manifest{}, fmt.Errorf("write %s object manifest: %w", selector, err)
			}
			hash, size, err := Checksum(artifact)
			if err != nil {
				return "", Manifest{}, err
			}
			manifest.Resources = append(manifest.Resources, Resource{ID: selector, Kind: "storage", Name: storage.Name, Driver: storage.Driver, Strategy: "object-manifest", Artifact: filepath.ToSlash(filepath.Join("storage", artifactName)), Checksum: hash, Size: size})
			continue
		}
		if storage.Driver != "local" {
			if resourceName == selector || resourceName == storage.Name {
				return "", Manifest{}, fmt.Errorf("backup %s is managed by unsupported external driver %s", selector, storage.Driver)
			}
			continue
		}
		artifactName := storage.Name + ".local.tar.zst"
		artifact := filepath.Join(dir, "storage", artifactName)
		if err := ArchiveDirectory(storage.Root, artifact); err != nil {
			return "", Manifest{}, fmt.Errorf("backup %s: %w", selector, err)
		}
		hash, size, err := Checksum(artifact)
		if err != nil {
			return "", Manifest{}, err
		}
		manifest.Resources = append(manifest.Resources, Resource{ID: selector, Kind: "storage", Name: storage.Name, Driver: storage.Driver, Strategy: "local-archive", Artifact: filepath.ToSlash(filepath.Join("storage", artifactName)), Checksum: hash, Size: size})
	}
	if len(manifest.Resources) == 0 {
		return "", Manifest{}, fmt.Errorf("backup resource %q was not found", resourceName)
	}
	if err := WriteManifest(dir, manifest); err != nil {
		return "", Manifest{}, err
	}
	if err := s.Hooks.Run(ctx, HookAfterCreate); err != nil {
		return "", Manifest{}, fmt.Errorf("after backup hook: %w", err)
	}
	return dir, manifest, nil
}

// Verify checks every artifact referenced by a backup manifest.
func (s *Service) Verify(dir string) (Manifest, error) {
	manifest, err := ReadManifest(dir)
	if err != nil {
		return Manifest{}, err
	}
	for _, resource := range manifest.Resources {
		artifact, err := safeArtifactPath(dir, resource.Artifact)
		if err != nil {
			return Manifest{}, fmt.Errorf("verify %s: %w", resource.ID, err)
		}
		if err := VerifyChecksum(artifact, resource.Checksum); err != nil {
			return Manifest{}, fmt.Errorf("verify %s: %w", resource.ID, err)
		}
	}
	return manifest, nil
}

// Restore restores one or all native database artifacts from a backup set.
func (s *Service) Restore(ctx context.Context, dir string, resourceName string, confirmation string) error {
	if confirmation != "restore-production" {
		return fmt.Errorf("restore requires --confirm restore-production")
	}
	if err := s.Hooks.Run(ctx, HookBeforeRestore); err != nil {
		return fmt.Errorf("before restore hook: %w", err)
	}
	manifest, err := s.Verify(dir)
	if err != nil {
		return err
	}
	plan, err := BuildPlan()
	if err != nil {
		return err
	}
	resourceName = normalizeResourceName(resourceName)
	for _, resource := range manifest.Resources {
		if resourceName != "" && resource.Name != resourceName && resource.ID != resourceName {
			continue
		}
		if resource.Kind == "storage" {
			if resource.Strategy == "object-manifest" {
				return fmt.Errorf("restore %s requires object materialization; this backup contains metadata only", resource.ID)
			}
			storage := findStoragePlan(plan.Storage, resource.Name)
			if storage == nil {
				return fmt.Errorf("restore %s: storage configuration not found", resource.ID)
			}
			if storage.Driver != resource.Driver {
				return fmt.Errorf("restore %s requires storage driver %s, target is %s", resource.ID, resource.Driver, storage.Driver)
			}
			artifact, err := safeArtifactPath(dir, resource.Artifact)
			if err != nil {
				return fmt.Errorf("restore %s: %w", resource.ID, err)
			}
			if err := RestoreDirectoryArchive(artifact, storage.Root); err != nil {
				return fmt.Errorf("restore %s: %w", resource.ID, err)
			}
			continue
		}
		connection := findPlanConnection(plan.Resources, resource.Name)
		if connection == nil {
			return fmt.Errorf("restore %s: database resource not found in App resource contract", resource.ID)
		}
		if normalizeDriver(connection.Connection.Driver) != resource.Driver {
			return fmt.Errorf("restore %s requires %s, target is %s; use --portable for cross-driver restore", resource.ID, resource.Driver, normalizeDriver(connection.Connection.Driver))
		}
		strategy, err := NativeStrategy(connection.Connection.Driver)
		if err != nil {
			return err
		}
		artifact, err := safeArtifactPath(dir, resource.Artifact)
		if err != nil {
			return fmt.Errorf("restore %s: %w", resource.ID, err)
		}
		if err := strategy.Restore(ctx, connection.Connection, artifact); err != nil {
			return fmt.Errorf("restore %s: %w", resource.ID, err)
		}
	}
	return s.Hooks.Run(ctx, HookAfterRestore)
}

// findPlanConnection returns the contract-resolved database resource by name.
func findPlanConnection(resources []PlanResource, name string) *PlanResource {
	for i := range resources {
		if resources[i].Connection.Name == name {
			return &resources[i]
		}
	}
	return nil
}

// findStoragePlan returns one named storage configuration.
func findStoragePlan(resources []StoragePlanResource, name string) *StoragePlanResource {
	for i := range resources {
		if resources[i].Name == name {
			return &resources[i]
		}
	}
	return nil
}

// normalizeResourceName accepts both `reporting` and the stable `db.reporting` selector.
func normalizeResourceName(name string) string {
	name = strings.TrimSpace(name)
	return strings.TrimPrefix(name, "db.")
}

// normalizeDriver maps driver aliases to stable manifest names.
func normalizeDriver(driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "sqlite3":
		return "sqlite"
	case "mariadb":
		return "mysql"
	case "postgresql", "pgx":
		return "postgres"
	default:
		return strings.ToLower(strings.TrimSpace(driver))
	}
}

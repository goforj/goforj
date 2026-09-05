// Package backup contains framework-owned database backup and restore behavior.
package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Manifest describes one verified backup set.
type Manifest struct {
	Version   int         `json:"version"`
	CreatedAt time.Time   `json:"created_at"`
	App       AppMetadata `json:"app,omitempty"`
	Resources []Resource  `json:"resources"`
}

// AppMetadata identifies the application and framework context that created a backup.
type AppMetadata struct {
	Name          string `json:"name,omitempty"`
	Binary        string `json:"binary,omitempty"`
	GoForjVersion string `json:"goforj_version,omitempty"`
	GitSHA        string `json:"git_sha,omitempty"`
}

// Resource describes one database or storage artifact in a backup set.
type Resource struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Driver   string `json:"driver"`
	Strategy string `json:"strategy"`
	Tool     string `json:"tool,omitempty"`
	Artifact string `json:"artifact"`
	Checksum string `json:"checksum"`
	Size     int64  `json:"size"`
}

// WriteManifest writes a deterministic manifest into a backup directory.
func WriteManifest(dir string, manifest Manifest) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("backup directory is required")
	}
	if manifest.Version == 0 {
		manifest.Version = 1
	}
	if manifest.CreatedAt.IsZero() {
		manifest.CreatedAt = time.Now().UTC()
	}
	for _, resource := range manifest.Resources {
		if _, err := safeArtifactPath(dir, resource.Artifact); err != nil {
			return fmt.Errorf("invalid artifact for %s: %w", resource.ID, err)
		}
	}
	sort.SliceStable(manifest.Resources, func(i, j int) bool {
		return manifest.Resources[i].ID < manifest.Resources[j].ID
	})
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode backup manifest: %w", err)
	}
	data = append(data, '\n')
	if err := ensurePrivateDirectory(dir); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}
	if err := writePrivateFile(filepath.Join(dir, "manifest.json"), data); err != nil {
		return fmt.Errorf("write backup manifest: %w", err)
	}
	checksums := strings.Builder{}
	for _, resource := range manifest.Resources {
		if resource.Checksum == "" {
			continue
		}
		fmt.Fprintf(&checksums, "%s  %s\n", resource.Checksum, resource.Artifact)
	}
	if err := writePrivateFile(filepath.Join(dir, "checksums.txt"), []byte(checksums.String())); err != nil {
		return fmt.Errorf("write backup checksums: %w", err)
	}
	return nil
}

// ReadManifest reads and validates a backup manifest from a directory.
func ReadManifest(dir string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return Manifest{}, fmt.Errorf("read backup manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode backup manifest: %w", err)
	}
	if manifest.Version != 1 {
		return Manifest{}, fmt.Errorf("unsupported backup manifest version %d", manifest.Version)
	}
	if len(manifest.Resources) == 0 {
		return Manifest{}, fmt.Errorf("backup manifest contains no resources")
	}
	for _, resource := range manifest.Resources {
		if _, err := safeArtifactPath(dir, resource.Artifact); err != nil {
			return Manifest{}, fmt.Errorf("invalid artifact for %s: %w", resource.ID, err)
		}
	}
	return manifest, nil
}

// safeArtifactPath prevents a manifest from escaping its backup directory.
func safeArtifactPath(dir string, artifact string) (string, error) {
	if filepath.IsAbs(artifact) || strings.TrimSpace(artifact) == "" {
		return "", fmt.Errorf("artifact path must be relative")
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(dir, filepath.FromSlash(artifact)))
	if err != nil {
		return "", err
	}
	if path != root && !strings.HasPrefix(path, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("artifact path escapes backup directory")
	}
	return path, nil
}

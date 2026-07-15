package backup

import (
	"fmt"
	"os"
	"path/filepath"
)

// WritePortableArchive writes a portable archive and its checksum into a directory.
func WritePortableArchive(dir string, archive PortableArchive) error {
	data, err := MarshalArchive(archive)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create portable backup directory: %w", err)
	}
	path := filepath.Join(dir, "portable.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write portable archive: %w", err)
	}
	fingerprint, err := Checksum(path)
	if err != nil {
		return err
	}
	manifest := Manifest{Version: 1, Resources: []Resource{{
		ID: "portable.database", Kind: "database", Name: "portable", Driver: "portable",
		Strategy: "goforj-portable", Artifact: "portable.json", Checksum: fingerprint.Checksum, Size: fingerprint.Size,
	}}}
	return WriteManifest(dir, manifest)
}

// ReadPortableArchive reads and verifies a portable archive directory.
func ReadPortableArchive(dir string) (PortableArchive, error) {
	manifest, err := ReadManifest(dir)
	if err != nil {
		return PortableArchive{}, err
	}
	if len(manifest.Resources) != 1 || manifest.Resources[0].Strategy != "goforj-portable" {
		return PortableArchive{}, fmt.Errorf("backup is not a portable archive")
	}
	path, err := safeArtifactPath(dir, "portable.json")
	if err != nil {
		return PortableArchive{}, err
	}
	if err := VerifyChecksum(path, manifest.Resources[0].Checksum); err != nil {
		return PortableArchive{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return PortableArchive{}, fmt.Errorf("read portable archive: %w", err)
	}
	return UnmarshalArchive(data)
}

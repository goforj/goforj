package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ObjectInfo describes one object in a remote storage disk without provider-specific types.
type ObjectInfo struct {
	Key        string    `json:"key"`
	Size       int64     `json:"size"`
	ETag       string    `json:"etag,omitempty"`
	Version    string    `json:"version,omitempty"`
	ModifiedAt time.Time `json:"modified_at,omitempty"`
	Checksum   string    `json:"checksum,omitempty"`
}

// ObjectLister provides the minimal remote storage surface needed for an inventory backup.
type ObjectLister interface {
	// ListObjects defines the list objects behavior required from implementations.
	ListObjects(context.Context, string) ([]ObjectInfo, error)
}

// ObjectManifest is a provider-neutral remote object inventory.
type ObjectManifest struct {
	Version int          `json:"version"`
	Prefix  string       `json:"prefix,omitempty"`
	Objects []ObjectInfo `json:"objects"`
}

// BuildObjectManifest returns a deterministic inventory of objects under a prefix.
func BuildObjectManifest(ctx context.Context, lister ObjectLister, prefix string) (ObjectManifest, error) {
	if lister == nil {
		return ObjectManifest{}, fmt.Errorf("object lister is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	objects, err := lister.ListObjects(ctx, prefix)
	if err != nil {
		return ObjectManifest{}, fmt.Errorf("list remote objects: %w", err)
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	return ObjectManifest{Version: 1, Prefix: prefix, Objects: objects}, nil
}

// MarshalObjectManifest encodes a remote object inventory deterministically.
func MarshalObjectManifest(manifest ObjectManifest) ([]byte, error) {
	if manifest.Version == 0 {
		manifest.Version = 1
	}
	sort.Slice(manifest.Objects, func(i, j int) bool { return manifest.Objects[i].Key < manifest.Objects[j].Key })
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode object manifest: %w", err)
	}
	return append(data, '\n'), nil
}

// WriteObjectManifest writes and checksums a remote object inventory artifact.
func WriteObjectManifest(ctx context.Context, lister ObjectLister, prefix string, dir string) (ObjectManifest, error) {
	manifest, err := BuildObjectManifest(ctx, lister, prefix)
	if err != nil {
		return ObjectManifest{}, err
	}
	data, err := MarshalObjectManifest(manifest)
	if err != nil {
		return ObjectManifest{}, err
	}
	if err := ensurePrivateDirectory(dir); err != nil {
		return ObjectManifest{}, err
	}
	path := filepath.Join(dir, "objects.json")
	if err := writePrivateFile(path, data); err != nil {
		return ObjectManifest{}, fmt.Errorf("write object manifest: %w", err)
	}
	fingerprint, err := Checksum(path)
	if err != nil {
		return ObjectManifest{}, err
	}
	if err := WriteManifest(dir, Manifest{Version: 1, Resources: []Resource{{ID: "storage.remote", Kind: "storage", Name: "remote", Driver: "s3", Strategy: "object-manifest", Artifact: "objects.json", Checksum: fingerprint.Checksum, Size: fingerprint.Size}}}); err != nil {
		return ObjectManifest{}, err
	}
	return manifest, nil
}

// ReadObjectManifest reads and verifies a remote object inventory artifact.
func ReadObjectManifest(dir string) (ObjectManifest, error) {
	manifest, err := ReadManifest(dir)
	if err != nil {
		return ObjectManifest{}, err
	}
	if len(manifest.Resources) != 1 || manifest.Resources[0].Strategy != "object-manifest" {
		return ObjectManifest{}, fmt.Errorf("backup is not an object manifest")
	}
	path, err := safeArtifactPath(dir, manifest.Resources[0].Artifact)
	if err != nil {
		return ObjectManifest{}, err
	}
	if err := VerifyChecksum(path, manifest.Resources[0].Checksum); err != nil {
		return ObjectManifest{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ObjectManifest{}, err
	}
	var objects ObjectManifest
	if err := json.Unmarshal(data, &objects); err != nil {
		return ObjectManifest{}, fmt.Errorf("decode object manifest: %w", err)
	}
	if objects.Version != 1 {
		return ObjectManifest{}, fmt.Errorf("unsupported object manifest version %d", objects.Version)
	}
	return objects, nil
}

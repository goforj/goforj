package backup

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/goforj/storage"
	"github.com/goforj/storage/driver/s3storage"
	"github.com/goforj/storage/storagecore"
	"github.com/klauspost/compress/zstd"
)

// objectArchiveEntry maps an opaque tar member to its original bucket-relative key.
type objectArchiveEntry struct {
	ContentType        string            `json:"content_type,omitempty"`
	ContentEncoding    string            `json:"content_encoding,omitempty"`
	ContentDisposition string            `json:"content_disposition,omitempty"`
	CacheControl       string            `json:"cache_control,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Artifact           string            `json:"artifact"`
	ObjectInfo
}

// objectArchiveManifest records independent object copies rather than references to mutable source data.
type objectArchiveManifest struct {
	Version int                  `json:"version"`
	Prefix  string               `json:"prefix"`
	Objects []objectArchiveEntry `json:"objects"`
}

// objectArchiveStore delegates provider I/O to the App storage library.
type objectArchiveStore struct {
	objects storagecore.ObjectStorage
	prefix  string
}

// configuredObjectArchiveConfig preserves the archive adapter's default region while reusing App disk configuration.
func configuredObjectArchiveConfig(name string) (s3storage.Config, error) {
	cfg, err := configuredObjectStorageConfig(name)
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	cfg.Prefix = strings.Trim(cfg.Prefix, "/")
	return cfg, err
}

// configuredObjectArchiveStore resolves the optional streaming capability through goforj/storage.
func configuredObjectArchiveStore(ctx context.Context, name string) (*objectArchiveStore, error) {
	cfg, err := configuredObjectArchiveConfig(name)
	if err != nil {
		return nil, err
	}
	disk, err := storage.BuildContext(ctx, cfg)
	if err != nil {
		return nil, err
	}
	objects, err := storage.Objects(disk)
	if err != nil {
		return nil, err
	}
	return &objectArchiveStore{objects: objects, prefix: cfg.Prefix}, nil
}

// objectPrefix retains the resource boundary when a neighbouring prefix shares its leading characters.
func objectPrefix(prefix string) string {
	if prefix == "" {
		return ""
	}
	return prefix + "/"
}

// BackupObjectArchive streams actual S3 object contents into a self-contained native storage artifact.
func BackupObjectArchive(ctx context.Context, name, artifact string, maxBytes int64) error {
	if maxBytes < 0 {
		return fmt.Errorf("object backup size limit cannot be negative")
	}
	store, err := configuredObjectArchiveStore(ctx, name)
	if err != nil {
		return err
	}
	if _, err := artifactPath(artifact); err != nil {
		return err
	}
	file, err := os.OpenFile(artifact, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("create object archive: %w", err)
	}
	success := false
	defer func() {
		file.Close()
		if !success {
			os.Remove(artifact)
		}
	}()
	encoder, err := zstd.NewWriter(file, zstd.WithEncoderConcurrency(1))
	if err != nil {
		return err
	}
	defer encoder.Close()
	writer := tar.NewWriter(encoder)
	defer writer.Close()
	manifest := objectArchiveManifest{Version: 1, Prefix: store.prefix, Objects: []objectArchiveEntry{}}
	total := int64(0)
	err = store.objects.WalkObjectsContext(ctx, "", func(object storagecore.ObjectInfo) error {
		if len(manifest.Objects) >= 100000 {
			return fmt.Errorf("object backup exceeds 100000 objects")
		}
		size := object.Size
		if size < 0 || (maxBytes > 0 && size > maxBytes-total) {
			return fmt.Errorf("object backup exceeds its size limit")
		}
		key := objectPrefix(store.prefix) + object.Key
		source, err := store.objects.OpenObjectContext(ctx, object.Key, storagecore.OpenObjectOptions{IfMatch: object.ETag})
		if err != nil {
			return fmt.Errorf("read source object failed")
		}
		if source.Info.Size != size {
			source.Close()
			return fmt.Errorf("source object changed during backup")
		}
		member := fmt.Sprintf("objects/%08d", len(manifest.Objects))
		if err = writer.WriteHeader(&tar.Header{Name: member, Mode: 0600, Size: size, Typeflag: tar.TypeReg}); err != nil {
			source.Close()
			return err
		}
		hash := sha256.New()
		n, copyErr := io.Copy(io.MultiWriter(writer, hash), io.LimitReader(source, size+1))
		closeErr := source.Close()
		if copyErr != nil || closeErr != nil || n != size {
			return fmt.Errorf("copy source object failed")
		}
		manifest.Objects = append(manifest.Objects, objectArchiveEntry{Artifact: member, ContentType: source.Info.Metadata.ContentType, ContentEncoding: source.Info.Metadata.ContentEncoding, ContentDisposition: source.Info.Metadata.ContentDisposition, CacheControl: source.Info.Metadata.CacheControl, Metadata: source.Info.Metadata.Values, ObjectInfo: ObjectInfo{Key: key, Size: size, ETag: source.Info.ETag, Version: source.Info.Version, ModifiedAt: source.Info.ModifiedAt, Checksum: hex.EncodeToString(hash.Sum(nil))}})
		total += size
		return nil
	})
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if len(metadata) > 64<<20 {
		return fmt.Errorf("object backup manifest exceeds its size limit")
	}
	if err = writer.WriteHeader(&tar.Header{Name: "objects.json", Mode: 0600, Size: int64(len(metadata)), Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	if _, err = writer.Write(metadata); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	if err = encoder.Close(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	success = true
	return nil
}

// inspectObjectArchive validates every object before a restore can write to the target bucket.
func inspectObjectArchive(artifact string) (objectArchiveManifest, error) {
	file, err := os.Open(artifact)
	if err != nil {
		return objectArchiveManifest{}, err
	}
	defer file.Close()
	decoder, err := zstd.NewReader(file, zstd.WithDecoderMaxMemory(64<<20), zstd.WithDecoderConcurrency(1))
	if err != nil {
		return objectArchiveManifest{}, err
	}
	defer decoder.Close()
	reader := tar.NewReader(decoder)
	actual := map[string]ObjectInfo{}
	var manifest objectArchiveManifest
	seenManifest := false
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return manifest, fmt.Errorf("invalid object archive")
		}
		if header.Typeflag != tar.TypeReg || header.Size < 0 {
			return manifest, fmt.Errorf("invalid object archive member")
		}
		if header.Name == "objects.json" {
			if seenManifest || header.Size > 64<<20 {
				return manifest, fmt.Errorf("invalid object archive manifest")
			}
			if err = json.NewDecoder(io.LimitReader(reader, header.Size)).Decode(&manifest); err != nil {
				return manifest, fmt.Errorf("invalid object archive manifest")
			}
			seenManifest = true
			continue
		}
		if seenManifest || len(actual) >= 100000 || header.Name != fmt.Sprintf("objects/%08d", len(actual)) {
			return manifest, fmt.Errorf("invalid or duplicate object archive member")
		}
		hash := sha256.New()
		n, err := io.Copy(hash, reader)
		if err != nil || n != header.Size {
			return manifest, fmt.Errorf("truncated object archive member")
		}
		actual[header.Name] = ObjectInfo{Size: n, Checksum: hex.EncodeToString(hash.Sum(nil))}
	}
	if !seenManifest || manifest.Version != 1 || len(manifest.Objects) != len(actual) {
		return manifest, fmt.Errorf("object archive manifest does not match its members")
	}
	seen := map[string]bool{}
	for index, object := range manifest.Objects {
		stored, ok := actual[object.Artifact]
		if !ok || object.Artifact != fmt.Sprintf("objects/%08d", index) || seen[object.Key] || !strings.HasPrefix(object.Key, objectPrefix(manifest.Prefix)) || object.Key == "" || stored.Size != object.Size || stored.Checksum != object.Checksum {
			return manifest, fmt.Errorf("object archive checksum or key boundary is invalid")
		}
		seen[object.Key] = true
	}
	return manifest, nil
}

// VerifyObjectArchive checks embedded metadata and every retained object checksum.
func VerifyObjectArchive(artifact string) error { _, err := inspectObjectArchive(artifact); return err }

// RestoreObjectArchive writes verified independent object copies into the selected target resource prefix.
func RestoreObjectArchive(ctx context.Context, name, artifact string) error {
	manifest, err := inspectObjectArchive(artifact)
	if err != nil {
		return err
	}
	store, err := configuredObjectArchiveStore(ctx, name)
	if err != nil {
		return err
	}
	file, err := os.Open(artifact)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder, err := zstd.NewReader(file, zstd.WithDecoderMaxMemory(64<<20), zstd.WithDecoderConcurrency(1))
	if err != nil {
		return err
	}
	defer decoder.Close()
	reader := tar.NewReader(decoder)
	for _, object := range manifest.Objects {
		header, err := reader.Next()
		if err != nil || header.Name != object.Artifact {
			return fmt.Errorf("object archive changed during restore")
		}
		relative := strings.TrimPrefix(object.Key, objectPrefix(manifest.Prefix))
		key := relative
		if relative == "" && store.prefix == "" {
			if object.Size == 0 {
				continue
			}
			return fmt.Errorf("object restore key is empty")
		}
		// S3 uploads need a seekable body for retries; spool one bounded object to private temporary storage.
		temporary, err := os.CreateTemp("", "goforj-object-restore-")
		if err != nil {
			return err
		}
		n, copyErr := io.Copy(temporary, reader)
		if copyErr != nil || n != object.Size {
			temporary.Close()
			os.Remove(temporary.Name())
			return fmt.Errorf("object archive changed during restore")
		}
		if _, err = temporary.Seek(0, io.SeekStart); err != nil {
			temporary.Close()
			os.Remove(temporary.Name())
			return err
		}
		_, putErr := store.objects.PutObjectContext(ctx, key, temporary, storagecore.PutObjectOptions{Size: object.Size, Metadata: storagecore.ObjectMetadata{ContentType: object.ContentType, ContentEncoding: object.ContentEncoding, ContentDisposition: object.ContentDisposition, CacheControl: object.CacheControl, Values: object.Metadata}})
		temporary.Close()
		os.Remove(temporary.Name())
		if putErr != nil {
			return fmt.Errorf("restore object upload failed")
		}
		restored, err := store.objects.OpenObjectContext(ctx, key, storagecore.OpenObjectOptions{})
		if err != nil {
			return fmt.Errorf("read restored object failed")
		}
		verified := sha256.New()
		copied, readErr := io.Copy(verified, restored)
		closeErr := restored.Close()
		if readErr != nil || closeErr != nil || copied != object.Size || hex.EncodeToString(verified.Sum(nil)) != object.Checksum {
			return fmt.Errorf("restored object checksum did not match")
		}

	}
	return nil
}

// PrepareObjectRestore creates a missing target bucket only when the caller explicitly requests target preparation.
func PrepareObjectRestore(ctx context.Context, name string) error {
	cfg, err := configuredObjectArchiveConfig(name)
	if err != nil {
		return err
	}
	return s3storage.PrepareBucketContext(ctx, cfg)
}

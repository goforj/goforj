package backup

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
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

// objectArchiveStore keeps streaming SDK access in the framework backup adapter.
type objectArchiveStore struct {
	client *s3.Client
	bucket string
	prefix string
}

// configuredObjectArchiveStore uses the same named resource environment contract as App storage.
func configuredObjectArchiveStore(name string) *objectArchiveStore {
	scope := storageEnvScope(name)
	endpoint := scope.Get("ENDPOINT", "")
	pathStyle, _ := parseEnvBool(scope.Key("USE_PATH_STYLE"))
	return &objectArchiveStore{client: s3.New(s3.Options{BaseEndpoint: optionalEndpoint(endpoint), Region: scope.Get("REGION", "us-east-1"), Credentials: credentials.NewStaticCredentialsProvider(scope.Get("ACCESS_KEY_ID", ""), scope.Get("SECRET_ACCESS_KEY", ""), ""), UsePathStyle: pathStyle, HTTPClient: &http.Client{Timeout: 5 * time.Minute}, RetryMaxAttempts: 2, RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired}), bucket: scope.Get("BUCKET", ""), prefix: strings.Trim(scope.Get("PREFIX", ""), "/")}
}

// optionalEndpoint preserves AWS endpoint resolution when no compatible endpoint is configured.
func optionalEndpoint(endpoint string) *string {
	if endpoint == "" {
		return nil
	}
	return aws.String(endpoint)
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
	store := configuredObjectArchiveStore(name)
	if store.bucket == "" {
		return fmt.Errorf("object backup bucket is required")
	}
	if _, err := parseEnvBool(storageEnvScope(name).Key("USE_PATH_STYLE")); err != nil {
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
	pages := s3.NewListObjectsV2Paginator(store.client, &s3.ListObjectsV2Input{Bucket: aws.String(store.bucket), Prefix: aws.String(objectPrefix(store.prefix))})
	total := int64(0)
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list source objects failed")
		}
		for _, object := range page.Contents {
			if len(manifest.Objects) >= 100000 {
				return fmt.Errorf("object backup exceeds 100000 objects")
			}
			size := aws.ToInt64(object.Size)
			if size < 0 || (maxBytes > 0 && size > maxBytes-total) {
				return fmt.Errorf("object backup exceeds its size limit")
			}
			key := aws.ToString(object.Key)
			if !strings.HasPrefix(key, objectPrefix(store.prefix)) {
				return fmt.Errorf("object is outside the configured storage prefix")
			}
			source, err := store.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(store.bucket), Key: object.Key, IfMatch: object.ETag})
			if err != nil {
				return fmt.Errorf("read source object failed")
			}
			if aws.ToInt64(source.ContentLength) != size {
				source.Body.Close()
				return fmt.Errorf("source object changed during backup")
			}
			member := fmt.Sprintf("objects/%08d", len(manifest.Objects))
			if err = writer.WriteHeader(&tar.Header{Name: member, Mode: 0600, Size: size, Typeflag: tar.TypeReg}); err != nil {
				source.Body.Close()
				return err
			}
			hash := sha256.New()
			n, copyErr := io.Copy(io.MultiWriter(writer, hash), io.LimitReader(source.Body, size+1))
			closeErr := source.Body.Close()
			if copyErr != nil || closeErr != nil || n != size {
				return fmt.Errorf("copy source object failed")
			}
			modified := time.Time{}
			if source.LastModified != nil {
				modified = *source.LastModified
			}
			manifest.Objects = append(manifest.Objects, objectArchiveEntry{Artifact: member, ContentType: aws.ToString(source.ContentType), ContentEncoding: aws.ToString(source.ContentEncoding), ContentDisposition: aws.ToString(source.ContentDisposition), CacheControl: aws.ToString(source.CacheControl), Metadata: source.Metadata, ObjectInfo: ObjectInfo{Key: key, Size: size, ETag: aws.ToString(source.ETag), Version: aws.ToString(source.VersionId), ModifiedAt: modified, Checksum: hex.EncodeToString(hash.Sum(nil))}})
			total += size
		}
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
	store := configuredObjectArchiveStore(name)
	if store.bucket == "" {
		return fmt.Errorf("restore bucket is required")
	}
	if _, err = parseEnvBool(storageEnvScope(name).Key("USE_PATH_STYLE")); err != nil {
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
		key := objectPrefix(store.prefix) + relative
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
		_, putErr := store.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(store.bucket), Key: aws.String(key), Body: temporary, ContentLength: aws.Int64(object.Size), ContentType: optionalEndpoint(object.ContentType), ContentEncoding: optionalEndpoint(object.ContentEncoding), ContentDisposition: optionalEndpoint(object.ContentDisposition), CacheControl: optionalEndpoint(object.CacheControl), Metadata: object.Metadata})
		temporary.Close()
		os.Remove(temporary.Name())
		if putErr != nil {
			return fmt.Errorf("restore object upload failed")
		}
		restored, err := store.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(store.bucket), Key: aws.String(key)})
		if err != nil {
			return fmt.Errorf("read restored object failed")
		}
		verified := sha256.New()
		copied, readErr := io.Copy(verified, restored.Body)
		closeErr := restored.Body.Close()
		if readErr != nil || closeErr != nil || copied != object.Size || hex.EncodeToString(verified.Sum(nil)) != object.Checksum {
			return fmt.Errorf("restored object checksum did not match")
		}

	}
	return nil
}

// PrepareObjectRestore creates a missing target bucket only when the caller explicitly requests target preparation.
func PrepareObjectRestore(ctx context.Context, name string) error {
	store := configuredObjectArchiveStore(name)
	if store.bucket == "" {
		return fmt.Errorf("restore bucket is required")
	}
	if _, err := store.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(store.bucket)}); err == nil {
		return nil
	} else {
		var api smithy.APIError
		if !errors.As(err, &api) || (api.ErrorCode() != "NotFound" && api.ErrorCode() != "NoSuchBucket") {
			return fmt.Errorf("cannot inspect target object bucket")
		}
	}
	_, err := store.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(store.bucket)})
	if err != nil {
		return fmt.Errorf("cannot prepare target object bucket")
	}
	return nil
}

package backup

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// TestObjectArchiveExpandedLimitRejectsBeforePayload proves advertised sizes are checked before decompression.
func TestObjectArchiveExpandedLimitRejectsBeforePayload(t *testing.T) {
	artifact := filepath.Join(t.TempDir(), "oversized.tar.zst")
	file, err := os.Create(artifact)
	if err != nil {
		t.Fatal(err)
	}
	encoder, err := zstd.NewWriter(file)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(encoder)
	if err := writer.WriteHeader(&tar.Header{Name: "objects/00000000", Size: storageArchiveMaxExpandedBytes + 1, Typeflag: tar.TypeReg, Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	// No payload is needed: the size guard must reject the header before attempting a read.
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := VerifyObjectArchive(artifact); err == nil || !strings.Contains(err.Error(), "expanded bytes") {
		t.Fatalf("verify error = %v, want expanded-byte rejection", err)
	}
}

// TestObjectArchiveExpandedLimitAccumulates enforces a shared budget across individually small members.
func TestObjectArchiveExpandedLimitAccumulates(t *testing.T) {
	artifact := filepath.Join(t.TempDir(), "objects.tar.zst")
	file, err := os.Create(artifact)
	if err != nil {
		t.Fatal(err)
	}
	encoder, err := zstd.NewWriter(file)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(encoder)
	for i := range 2 {
		if err := writer.WriteHeader(&tar.Header{Name: fmt.Sprintf("objects/%08d", i), Size: 3, Typeflag: tar.TypeReg, Mode: 0600}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(writer, "abc"); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectObjectArchive(context.Background(), artifact, 5); err == nil || !strings.Contains(err.Error(), "expanded bytes") {
		t.Fatalf("inspect error = %v, want accumulated-byte rejection", err)
	}
}

// TestObjectRestoreCanceledBeforeInspection prevents canceled restores from reading even the archive file.
func TestObjectRestoreCanceledBeforeInspection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := RestoreObjectArchive(ctx, "default", filepath.Join(t.TempDir(), "absent.tar.zst")); !errors.Is(err, context.Canceled) {
		t.Fatalf("restore error = %v, want canceled", err)
	}
}

// cancelObjectArchiveReader triggers cancellation after delivering one chunk to model buffered decompression.
type cancelObjectArchiveReader struct {
	cancel context.CancelFunc
}

// Read cancels after the first bytes so the wrapper must stop a following local copy read.
func (r cancelObjectArchiveReader) Read(data []byte) (int, error) {
	r.cancel()
	return copy(data, "chunk"), nil
}

// TestObjectArchiveReaderStopsCopyOnCancellation bounds work during buffered decompression and disk spooling.
func TestObjectArchiveReaderStopsCopyOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n, err := io.Copy(io.Discard, objectArchiveReader{ctx: ctx, reader: cancelObjectArchiveReader{cancel: cancel}})
	if n != 5 || !errors.Is(err, context.Canceled) {
		t.Fatalf("copied %d bytes, error = %v; want one chunk then cancellation", n, err)
	}
}

// TestObjectArchiveCaptureRespectsRestoreLimit prevents creating object archives that exceed the reader's safety budget.
func TestObjectArchiveCaptureRespectsRestoreLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list-type") != "2" {
			t.Error("oversized source object was opened")
			w.WriteHeader(403)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>huge</Key><Size>%d</Size></Contents></ListBucketResult>`, storageArchiveMaxExpandedBytes+1)
	}))
	defer server.Close()
	for key, value := range map[string]string{"STORAGE_BUCKET": "fixture", "STORAGE_ENDPOINT": server.URL, "STORAGE_REGION": "us-east-1", "STORAGE_ACCESS_KEY_ID": "fixture", "STORAGE_SECRET_ACCESS_KEY": "fixture", "STORAGE_USE_PATH_STYLE": "true", "STORAGE_PREFIX": ""} {
		t.Setenv(key, value)
	}
	for _, limit := range []int64{0, storageArchiveMaxExpandedBytes + 100} {
		artifact := filepath.Join(t.TempDir(), "objects.tar.zst")
		if err := BackupObjectArchive(context.Background(), "default", artifact, limit); err == nil || !strings.Contains(err.Error(), "size limit") {
			t.Fatalf("capture limit %d: error=%v", limit, err)
		}
		if _, err := os.Stat(artifact); !os.IsNotExist(err) {
			t.Fatal("partial oversized archive retained", err)
		}
	}
}

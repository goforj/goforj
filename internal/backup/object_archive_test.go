package backup

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestObjectArchiveRestoresIndependentContents proves object data survives source deletion and prefix rebasing.
func TestObjectArchiveRestoresIndependentContents(t *testing.T) {
	source := map[string]string{"tenant/a.txt": "first object", "tenant/path/../odd": "opaque key"}
	restored := map[string]string{}
	failRead := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2" {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<ListBucketResult><IsTruncated>false</IsTruncated>`)
			for _, key := range []string{"tenant/a.txt", "tenant/path/../odd"} {
				fmt.Fprintf(w, "<Contents><Key>%s</Key><Size>%d</Size><ETag>fixture</ETag></Contents>", key, len(source[key]))
			}
			fmt.Fprint(w, `</ListBucketResult>`)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/fixture/")
		if r.Method == http.MethodPut {
			data, _ := io.ReadAll(r.Body)
			restored[key] = string(data)
			if r.Header.Get("Content-Type") != "text/plain" {
				t.Error("object content type was not retained")
			}
			return
		}
		if value, ok := restored[key]; ok {
			io.WriteString(w, value)
			return
		}
		if failRead {
			w.WriteHeader(404)
			return
		}
		if value, ok := source[key]; ok {
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("ETag", "fixture")
			w.Header().Set("Content-Length", fmt.Sprint(len(value)))
			io.WriteString(w, value)
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()
	for key, value := range map[string]string{"STORAGE_PUBLIC_BUCKET": "fixture", "STORAGE_PUBLIC_ENDPOINT": server.URL, "STORAGE_PUBLIC_REGION": "fixture", "STORAGE_PUBLIC_ACCESS_KEY_ID": "fixture-key", "STORAGE_PUBLIC_SECRET_ACCESS_KEY": "fixture-secret", "STORAGE_PUBLIC_USE_PATH_STYLE": "true", "STORAGE_PUBLIC_PREFIX": "tenant"} {
		t.Setenv(key, value)
	}
	artifact := filepath.Join(t.TempDir(), "objects.tar.zst")
	if err := BackupObjectArchive(context.Background(), "public", artifact, 1000); err != nil {
		t.Fatal(err)
	}
	if err := VerifyObjectArchive(artifact); err != nil {
		t.Fatal(err)
	}
	failRead = true
	t.Setenv("STORAGE_PUBLIC_PREFIX", "restored")
	if err := RestoreObjectArchive(context.Background(), "public", artifact); err != nil {
		t.Fatal(err)
	}
	if restored["restored/a.txt"] != "first object" || restored["restored/path/../odd"] != "opaque key" {
		t.Fatal("object data or opaque keys changed", restored)
	}
}

// TestObjectArchiveRejectsOversizeAndIncompleteCopies prevents partial artifacts being reported as recoverable.
func TestObjectArchiveRejectsOversizeAndIncompleteCopies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list-type") == "2" {
			io.WriteString(w, `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>prefix/file</Key><Size>100</Size></Contents></ListBucketResult>`)
			return
		}
		w.WriteHeader(403)
	}))
	defer server.Close()
	for key, value := range map[string]string{"STORAGE_BUCKET": "fixture", "STORAGE_ENDPOINT": server.URL, "STORAGE_REGION": "fixture", "STORAGE_ACCESS_KEY_ID": "fixture", "STORAGE_SECRET_ACCESS_KEY": "fixture", "STORAGE_USE_PATH_STYLE": "true", "STORAGE_PREFIX": "prefix"} {
		t.Setenv(key, value)
	}
	for _, limit := range []int64{-1, 10, 1000} {
		artifact := filepath.Join(t.TempDir(), "objects.tar.zst")
		if err := BackupObjectArchive(context.Background(), "default", artifact, limit); err == nil {
			t.Fatal("accepted incomplete or over-limit backup", limit)
		}
		if _, err := os.Stat(artifact); !os.IsNotExist(err) {
			t.Fatal("partial artifact left behind", err)
		}
	}
}

// TestSelectedPlanRejectsMissingResources prevents quietly omitted selections.
func TestSelectedPlanRejectsMissingResources(t *testing.T) {
	plan := Plan{Resources: []PlanResource{{Connection: Connection{Name: "default", Driver: "sqlite"}, Strategy: "sqlite-vacuum-into"}}, Storage: []StoragePlanResource{{Name: "public", Driver: "s3", Status: "backupable"}, {Name: "other", Driver: "unsupported", Status: "external-managed"}}}
	selected, err := selectBackupPlan(plan, &CreateOptions{Resources: []string{"db.default", "storage.public"}})
	if err != nil || len(selected.Resources) != 1 || len(selected.Storage) != 1 {
		t.Fatal("incorrect resource selection", err)
	}
	for _, id := range []string{"db.missing", "storage.other"} {
		if _, err = selectBackupPlan(plan, &CreateOptions{Resources: []string{id}}); err == nil {
			t.Fatal("accepted missing or unsupported resource", id)
		}
	}
	for _, options := range []CreateOptions{{}, {Resources: []string{"default"}}, {Resources: []string{"db.default", "db.default"}}, {Resources: []string{"db.default"}, MaxBytes: -1}} {
		if _, err = NewService().CreateSelected(context.Background(), t.TempDir(), options); err == nil {
			t.Fatal("invalid selection accepted")
		}
	}
}

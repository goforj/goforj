package backup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestPrepareObjectRestoreDoesNotProvisionWhenReadAccessIsDenied distinguishes missing targets from forbidden ones.
func TestPrepareObjectRestoreDoesNotProvisionWhenReadAccessIsDenied(t *testing.T) {
	for _, status := range []int{200, 403, 404} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			created := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodHead {
					w.WriteHeader(status)
				} else if r.Method == http.MethodPut {
					created++
					w.WriteHeader(200)
				} else {
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			for key, value := range map[string]string{"STORAGE_BUCKET": "fixture", "STORAGE_ENDPOINT": server.URL, "STORAGE_REGION": "us-east-1", "STORAGE_ACCESS_KEY_ID": "fixture", "STORAGE_SECRET_ACCESS_KEY": "fixture", "STORAGE_USE_PATH_STYLE": "true"} {
				t.Setenv(key, value)
			}
			err := PrepareObjectRestore(context.Background(), "default")
			if status == 403 {
				if err == nil || created != 0 {
					t.Fatal("provisioned a forbidden target", created, err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if status == 404 && created != 1 {
				t.Fatal("missing target not prepared", created)
			}
		})
	}
}

// TestCreateCmdRejectsAmbiguousNativeSelections fails before any database or storage is accessed.
func TestCreateCmdRejectsAmbiguousNativeSelections(t *testing.T) {
	for _, cmd := range []CreateCmd{{Portable: true, Resources: []string{"db.default"}}, {Resources: []string{"db.default"}, Resource: "default"}, {CopyObjects: true}, {MaxBytes: 100}} {
		if err := cmd.Run(); err == nil {
			t.Fatal("ambiguous backup command accepted")
		}
	}
}

// TestObjectRestoreRejectsFailedTargetPreparation keeps an unavailable provider from becoming a successful restore.
func TestObjectRestoreRejectsFailedTargetPreparation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(404)
		} else {
			w.WriteHeader(403)
		}
	}))
	defer server.Close()
	for key, value := range map[string]string{"STORAGE_BUCKET": "fixture", "STORAGE_ENDPOINT": server.URL, "STORAGE_REGION": "us-east-1", "STORAGE_ACCESS_KEY_ID": "fixture", "STORAGE_SECRET_ACCESS_KEY": "fixture", "STORAGE_USE_PATH_STYLE": "true"} {
		t.Setenv(key, value)
	}
	if err := PrepareObjectRestore(context.Background(), "default"); err == nil {
		t.Fatal("failed target preparation accepted")
	}
}

// TestPrepareObjectRestoreTargetsValidatesContractBeforeProvisioning prevents stale provider settings from creating unrelated buckets.
func TestPrepareObjectRestoreTargetsValidatesContractBeforeProvisioning(t *testing.T) {
	for _, driver := range []string{"local", "missing", "s3"} {
		t.Run(driver, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.Mkdir("bin", 0700); err != nil {
				t.Fatal(err)
			}
			resources := `{"id":"storage.public","kind":"storage","name":"public","driver":"` + driver + `"}`
			if driver == "missing" {
				resources = `{"id":"storage.other","kind":"storage","name":"other","driver":"local"}`
			}
			script := "#!/bin/sh\nprintf '%s\n' '{\"version\":1,\"app\":\"app\",\"resources\":[" + resources + "]}'\n"
			if err := os.WriteFile(filepath.Join("bin", "app"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("FORJ_APP", "app")
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method == http.MethodHead {
					w.WriteHeader(404)
				} else {
					w.WriteHeader(200)
				}
			}))
			defer server.Close()
			for key, value := range map[string]string{"STORAGE_PUBLIC_BUCKET": "fixture", "STORAGE_PUBLIC_ENDPOINT": server.URL, "STORAGE_PUBLIC_REGION": "us-east-1", "STORAGE_PUBLIC_ACCESS_KEY_ID": "fixture", "STORAGE_PUBLIC_SECRET_ACCESS_KEY": "fixture", "STORAGE_PUBLIC_USE_PATH_STYLE": "true"} {
				t.Setenv(key, value)
			}
			manifest := Manifest{Resources: []Resource{{ID: "storage.public", Name: "public", Kind: "storage", Driver: "s3", Strategy: "object-archive"}}}
			err := prepareObjectRestoreTargets(context.Background(), manifest, "storage.public")
			if driver == "s3" {
				if err != nil || requests != 2 {
					t.Fatalf("valid target: requests=%d, error=%v", requests, err)
				}
			} else if err == nil || requests != 0 {
				t.Fatalf("incompatible target: requests=%d, error=%v", requests, err)
			}
		})
	}
}

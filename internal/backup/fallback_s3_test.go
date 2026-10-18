package backup

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestFallbackS3SelectedBackupRoundTrip exercises object capture and rebased recovery without an App resource contract.
func TestFallbackS3SelectedBackupRoundTrip(t *testing.T) {
	for _, name := range []string{"default", "public"} {
		t.Run(name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("FORJ_APP", "legacy")
			t.Setenv("STORAGE_DRIVER", "")
			t.Setenv("STORAGE_ROOT", "")
			var mutex sync.Mutex
			restored := ""
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mutex.Lock()
				defer mutex.Unlock()
				switch {
				case r.URL.Query().Get("list-type") == "2":
					w.Header().Set("Content-Type", "application/xml")
					io.WriteString(w, `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>source/note.txt</Key><Size>8</Size><ETag>fixture</ETag></Contents></ListBucketResult>`)
				case r.Method == http.MethodGet && r.URL.Path == "/fixture/source/note.txt":
					w.Header().Set("Content-Type", "text/plain")
					w.Header().Set("Content-Length", "8")
					w.Header().Set("ETag", "fixture")
					io.WriteString(w, "retained")
				case r.Method == http.MethodPut && r.URL.Path == "/fixture/restored/note.txt":
					data, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
						w.WriteHeader(500)
						return
					}
					restored = string(data)
				case r.Method == http.MethodGet && r.URL.Path == "/fixture/restored/note.txt":
					io.WriteString(w, restored)
				default:
					t.Errorf("unexpected provider request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			scope := "STORAGE"
			if name != "default" {
				scope += "_" + strings.ToUpper(name)
			}
			for field, value := range map[string]string{"DRIVER": "s3", "BUCKET": "fixture", "ENDPOINT": server.URL, "REGION": "us-east-1", "ACCESS_KEY_ID": "fixture", "SECRET_ACCESS_KEY": "fixture", "USE_PATH_STYLE": "true", "PREFIX": "source"} {
				t.Setenv(scope+"_"+field, value)
			}
			service := NewService()
			created, err := service.CreateSelected(context.Background(), t.TempDir(), CreateOptions{Resources: []string{"storage." + name}, CopyObjects: true, MaxBytes: 1 << 20})
			if err != nil {
				t.Fatal(err)
			}
			if len(created.Manifest.Resources) != 1 || created.Manifest.Resources[0].Strategy != "object-archive" {
				t.Fatalf("unexpected selected set: %#v", created.Manifest.Resources)
			}
			if _, err := service.Verify(created.Directory); err != nil {
				t.Fatal(err)
			}
			t.Setenv(scope+"_PREFIX", "restored")
			if err := service.Restore(context.Background(), created.Directory, "storage."+name, "restore-production"); err != nil {
				t.Fatal(err)
			}
			mutex.Lock()
			defer mutex.Unlock()
			if restored != "retained" {
				t.Fatalf("restored contents = %q", restored)
			}
		})
	}
}

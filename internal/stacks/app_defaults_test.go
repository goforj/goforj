package stacks

import (
	"strings"
	"testing"
)

// TestAppDefaultsPreserveResourceNamespaceCollisions keeps named resources available independently of which configured App shares their prefix.
func TestAppDefaultsPreserveResourceNamespaceCollisions(t *testing.T) {
	for _, tc := range []struct{ app, root, named string }{
		{"db", "sqlite", "mysql"}, {"cache", "memory", "redis"}, {"queue", "workerpool", "redis"},
		{"events", "inproc", "redis"}, {"storage", "local", "s3"}, {"mail", "log", "smtp"},
	} {
		t.Run(tc.app, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "project_name: stacks\nmodule_name: example.org/stacks\nrender:\n  components: [cli]\napps:\n  "+tc.app+":\n    components: [cli]\n  admin:\n    components: [cli]\n")
			prefix := strings.ToUpper(tc.app) + "_"
			rootKey := prefix + "DRIVER"
			namedKey := prefix + rootKey
			values := map[string]string{rootKey: tc.root, namedKey: tc.named, "ADMIN_" + rootKey: tc.root}
			for _, app := range []string{"app", "admin", tc.app} {
				baked, err := AppDefaults(root, values, app)
				if err != nil {
					t.Fatal(err)
				}
				want := tc.root
				if app == tc.app {
					want = tc.named
				}
				if baked[rootKey] != want || baked[namedKey] != tc.named {
					t.Fatalf("App %s lost the root or named driver: %v", app, baked)
				}
				if _, present := baked["ADMIN_"+rootKey]; present {
					t.Fatal("ordinary App overlay was left unfolded")
				}
			}
		})
	}
}

// TestSavedStackKeepsNamedSQLiteTargetWithDBApp covers the complete definition-to-binary projection that previously selected the root database instead.
func TestSavedStackKeepsNamedSQLiteTargetWithDBApp(t *testing.T) {
	root := fixture(t)
	put(t, root, ".goforj.yml", "project_name: stacks\nmodule_name: example.org/stacks\nrender:\n  components: [database_sqlite]\napps:\n  db:\n    components: [database_sqlite]\n")
	values := map[string]string{"DB_DRIVER": "sqlite", "DB_SQLITE_DATABASE": "./data/root.db", "DB_DB_DRIVER": "sqlite", "DB_DB_SQLITE_DATABASE": "./data/named-db.db"}
	put(t, root, ".env", string(encodeDocument(values)))
	s := openTest(t, root)
	if err := s.Save("saved", s.Current); err != nil {
		t.Fatal(err)
	}
	defaults, err := BuildDefaults(root, "saved")
	if err != nil {
		t.Fatal(err)
	}
	for _, app := range []string{"app", "db"} {
		baked, err := AppDefaults(root, defaults, app)
		if err != nil {
			t.Fatal(err)
		}
		wantRoot := values["DB_SQLITE_DATABASE"]
		if app == "db" {
			wantRoot = values["DB_DB_SQLITE_DATABASE"]
		}
		if baked["DB_SQLITE_DATABASE"] != wantRoot || baked["DB_DB_SQLITE_DATABASE"] != values["DB_DB_SQLITE_DATABASE"] {
			t.Fatalf("App %s changed a database target: %v", app, baked)
		}
	}
}

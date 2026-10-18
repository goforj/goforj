package localproviders

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fixture models a services-first project with a second App and named resources.
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	put(t, root, ".goforj.yml", "project_name: local\nmodule_name: example.org/local\napps:\n  app:\n    components:\n      database_mysql: true\n      cache: true\n      jobs: true\n      events: true\n      storage: true\n      mail: true\n  admin:\n    components:\n      database_mysql: true\n      cache: true\n")
	put(t, root, ".env", "# Keep my comments\nAPP_KEY='unrelated-secret'\nAPP_NAME=Local\nDB_DRIVER=mysql\nDB_SUPPORTED_DRIVERS=mysql\nDB_DATABASE=production\nDB_PASSWORD='line one\nline two'\nCACHE_DRIVER=redis\nCACHE_SUPPORTED_DRIVERS=memory,redis\nCACHE_SESSIONS_DRIVER=redis\nADMIN_CACHE_DRIVER=redis\nQUEUE_DRIVER=redis\nEVENTS_DRIVER=redis\nSTORAGE_DRIVER=s3\nMAIL_DRIVER=smtp\nCOMPOSE_PROFILES=mysql,redis,grafana,custom\n")
	return root
}

// put creates private fixture files.
func put(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

// openTest fails close to the project that could not be inspected.
func openTest(t *testing.T, root string) *Session {
	t.Helper()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// encodeDocument keeps test configuration literals deterministic without adding a profile writer to production.
func encodeDocument(values map[string]string) []byte {
	data := document{}
	return data.replace(func(string) bool { return true }, values)
}

// TestLocalProvidersRoundTrip preserves service settings and unrelated text while consuming one restore point.
func TestLocalProvidersRoundTrip(t *testing.T) {
	root := fixture(t)
	s := openTest(t, root)
	original := clone(s.Current)
	values, err := s.Portable()
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"DB_DRIVER": "sqlite", "CACHE_DRIVER": "memory", "QUEUE_DRIVER": "workerpool", "EVENTS_DRIVER": "inproc", "STORAGE_DRIVER": "local", "MAIL_DRIVER": "log", "COMPOSE_PROFILES": "grafana,custom"} {
		if values[key] != want {
			t.Errorf("%s=%s, want %s", key, values[key], want)
		}
	}
	if err := s.Apply(values); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, ".env"))
	if !bytes.Contains(raw, []byte("# Keep my comments\nAPP_KEY='unrelated-secret'\nAPP_NAME=Local\n")) || !bytes.Contains(raw, []byte("DB_PASSWORD='line one\nline two'")) {
		t.Fatal("changed untouched text")
	}
	s = openTest(t, root)
	if !s.HasPrevious() {
		t.Fatal("missing restore point")
	}
	assertFileMode(t, root, stateName, 0600)
	if err := s.Restore(); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, root)
	if s.HasPrevious() || !reflect.DeepEqual(s.Current, original) {
		t.Fatal("restore changed original values")
	}
	backup, _ := os.ReadFile(filepath.Join(root, stateName))
	if bytes.Contains(backup, []byte("line one")) {
		t.Fatal("consumed restore point retained credentials")
	}
}

// TestRestorePreservesNewSettingsAndRejectsConflictingEdits protects edits made after switching.
func TestRestorePreservesNewSettingsAndRejectsConflictingEdits(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{true: "conflict", false: "unrelated"}[conflict], func(t *testing.T) {
			root := fixture(t)
			s := openTest(t, root)
			values, err := s.Portable()
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Apply(values); err != nil {
				t.Fatal(err)
			}
			raw, _ := os.ReadFile(filepath.Join(root, ".env"))
			raw = append(raw, []byte("\nCACHE_NEW_SETTING=keep\nNEW_SECRET=keep\n")...)
			if conflict {
				raw = bytes.ReplaceAll(raw, []byte("DB_DRIVER=sqlite"), []byte("DB_DRIVER=mysql"))
			}
			put(t, root, ".env", string(raw))
			s = openTest(t, root)
			err = s.Restore()
			if conflict {
				if err == nil || !strings.Contains(err.Error(), "DB_DRIVER changed") {
					t.Fatalf("expected conflict, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(filepath.Join(root, ".env"))
			if !bytes.Contains(after, []byte("CACHE_NEW_SETTING=keep\nNEW_SECRET=keep")) {
				t.Fatal("lost later settings")
			}
		})
	}
}

// TestApplyPreservesExistingRestorePoint prevents a repeated switch from losing service credentials.
func TestApplyPreservesExistingRestorePoint(t *testing.T) {
	root := fixture(t)
	s := openTest(t, root)
	values, err := s.Portable()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(values); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, root)
	before, _ := os.ReadFile(filepath.Join(root, stateName))
	if err := s.Apply(s.Current); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(root, stateName))
	if !bytes.Equal(before, after) {
		t.Fatal("no-op changed restore point")
	}
	changed := clone(s.Current)
	changed["CACHE_DRIVER"] = "redis"
	if err := s.Apply(changed); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, root)
	previous, err := s.Previous()
	if err != nil {
		t.Fatal(err)
	}
	if previous["DB_DRIVER"] != "mysql" || previous["CACHE_DRIVER"] != "redis" {
		t.Fatal("second switch lost original settings")
	}
	if err := s.Restore(); err != nil {
		t.Fatal(err)
	}

}

// TestSessionRejectsStaleUnsafeAndInvalidFiles covers publication preflight without disclosing private values.
func TestSessionRejectsStaleUnsafeAndInvalidFiles(t *testing.T) {
	for _, scenario := range []string{"changed", "readonly", "lock", "symlink", "tracked", "bad-json", "bad-version", "bad-setting", "missing-env", "bad-env"} {
		t.Run(scenario, func(t *testing.T) {
			root := fixture(t)
			if scenario == "bad-json" {
				put(t, root, stateName, "not-json")
			}
			if scenario == "bad-version" {
				put(t, root, stateName, `{"version":2}`)
			}
			if scenario == "bad-setting" {
				put(t, root, stateName, `{"version":1,"previous":{"APP_KEY":"synthetic-secret"}}`)
			}
			if scenario == "missing-env" {
				_ = os.Remove(filepath.Join(root, ".env"))
			}
			if scenario == "bad-env" {
				put(t, root, ".env", "DB_PASSWORD='synthetic-secret")
			}
			s, err := Open(root)
			if strings.HasPrefix(scenario, "bad-") || scenario == "missing-env" {
				if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
					t.Fatalf("unsafe error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			values, err := s.Portable()
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "changed":
				put(t, root, ".env", "DB_DRIVER=postgres\n")
			case "readonly":
				_ = os.Chmod(filepath.Join(root, ".env"), 0400)
			case "lock":
				put(t, root, lockName, "")
			case "symlink":
				put(t, root, "other", "")
				_ = os.Symlink(filepath.Join(root, "other"), filepath.Join(root, stateName))
			case "tracked":
				runLocalGit(t, root, "init", "-q")
				put(t, root, stateName, "")
				runLocalGit(t, root, "add", stateName)
			}
			if err := s.Apply(values); err == nil {
				t.Fatal("accepted unsafe mutation")
			}
		})
	}
}

// TestRestoreEmptyAndAbsentValuesRetainsTheirOriginalMeaning guards against conflating empty values with inherited settings.
func TestRestoreEmptyAndAbsentValuesRetainsTheirOriginalMeaning(t *testing.T) {
	root := fixture(t)
	s := openTest(t, root)
	if _, err := s.Previous(); err == nil {
		t.Fatal("restored without backup")
	}
	original := clone(s.Current)
	values, err := s.Portable()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(values); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, root)
	restored, err := s.Previous()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored, original) {
		t.Fatal("restore changed presence or emptiness")
	}
	var saved state
	data, _ := os.ReadFile(filepath.Join(root, stateName))
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if _, retained := saved.Previous["DB_PASSWORD"]; retained {
		t.Fatal("backup retained an untouched credential")
	}
	if len(changedKeys(saved.Previous, saved.Applied)) == 0 {
		t.Fatal("snapshot changed original values")
	}
}

// selectDriver exercises single-resource transitions against the same original values used by the local preset.
func (s *Session) selectDriver(values map[string]string, resource Resource, driver string) error {
	return s.setDriver(values, resource, driver, values)
}

// TestLocalProviderWritesProtectActiveSettingsInGit covers projects without existing environment ignore rules.
func TestLocalProviderWritesProtectActiveSettingsInGit(t *testing.T) {
	root := fixture(t)
	runLocalGit(t, root, "init", "-q")
	s := openTest(t, root)
	values, err := s.Portable()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(values); err != nil {
		t.Fatal(err)
	}
	runLocalGit(t, root, "add", "-A")
	for _, name := range []string{".env", stateName} {
		runLocalGit(t, root, "check-ignore", "-q", name)
	}
}

// TestInvalidCurrentResourceValuesAreRejectedWithoutDisclosure avoids creating a restore point that cannot represent the original input.
func TestInvalidCurrentResourceValuesAreRejectedWithoutDisclosure(t *testing.T) {
	root := fixture(t)
	put(t, root, ".env", "CACHE_DRIVER=synthetic-private-value\n")
	_, err := Open(root)
	if err == nil || strings.Contains(err.Error(), "synthetic-private-value") {
		t.Fatalf("unsafe validation error: %v", err)
	}
}

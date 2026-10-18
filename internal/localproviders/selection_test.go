package localproviders

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestExcludedResourcesKeepRequiredComposeProfiles checks shared services and unrelated developer tools.
func TestExcludedResourcesKeepRequiredComposeProfiles(t *testing.T) {
	for _, key := range []string{"CACHE_SESSIONS_DRIVER", "ADMIN_CACHE_DRIVER", "QUEUE_DRIVER", "DB_DRIVER"} {
		t.Run(key, func(t *testing.T) {
			s := openTest(t, fixture(t))
			values, err := s.PortableFor(map[string]bool{key: true})
			if err != nil {
				t.Fatal(err)
			}
			if values[key] != s.Current[key] {
				t.Fatalf("excluded %s changed", key)
			}
			profiles := values["COMPOSE_PROFILES"]
			if !strings.Contains(profiles, "grafana,custom") {
				t.Fatalf("removed unrelated profiles: %s", profiles)
			}
			needed := "redis"
			if key == "DB_DRIVER" {
				needed = "mysql"
			}
			if !strings.Contains(profiles, needed) {
				t.Fatalf("removed required %s: %s", needed, profiles)
			}
		})
	}
}

// TestExcludedInheritedDatabaseKeepsItsTarget pins an excluded App's inherited service before converting the root.
func TestExcludedInheritedDatabaseKeepsItsTarget(t *testing.T) {
	root := fixture(t)
	put(t, root, ".env", "DB_DRIVER=mysql\nDB_DSN=synthetic-service-dsn\nDB_DATABASE=service\nCOMPOSE_PROFILES=mysql,grafana\n")
	s := openTest(t, root)
	before := databaseTargetInScope(s.Current, "ADMIN_DB_DRIVER", "ADMIN_")
	values, err := s.PortableFor(map[string]bool{"ADMIN_DB_DRIVER": true})
	if err != nil {
		t.Fatal(err)
	}
	if values["DB_DRIVER"] != "sqlite" || databaseTargetInScope(values, "ADMIN_DB_DRIVER", "ADMIN_") != before {
		t.Fatal("excluded database lost its service target")
	}
	if values["COMPOSE_PROFILES"] != "mysql,grafana" {
		t.Fatal("removed service used by excluded App")
	}
	if err := s.Apply(values); err != nil {
		t.Fatal(err)
	}
	if err := openTest(t, root).Restore(); err != nil {
		t.Fatal(err)
	}
	restored := openTest(t, root)
	if !reflect.DeepEqual(restored.Current, s.Current) {
		t.Fatal("partial switch did not restore")
	}
}

// TestExcludeEverythingIsANoOp keeps the recovery point and active environment untouched.
func TestExcludeEverythingIsANoOp(t *testing.T) {
	s := openTest(t, fixture(t))
	excluded := map[string]bool{}
	for _, resource := range s.Resources {
		excluded[resource.Key] = true
	}
	values, err := s.PortableFor(excluded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(values, s.Current) {
		t.Fatal("excluding everything changed configuration")
	}
	if err := s.Apply(values); err != nil {
		t.Fatal(err)
	}
	if openTest(t, s.Root).HasPrevious() {
		t.Fatal("no-op created restore point")
	}
}

// TestUnusedServiceProfilesRemainExplicit preserves services selected for purposes outside GoForj's resource inventory.
func TestUnusedServiceProfilesRemainExplicit(t *testing.T) {
	root := fixture(t)
	put(t, root, ".env", "DB_DRIVER=sqlite\nCACHE_DRIVER=memory\nCOMPOSE_PROFILES=mysql,redis,opensearch,custom\n")
	s := openTest(t, root)
	values, err := s.Portable()
	if err != nil {
		t.Fatal(err)
	}
	if values["COMPOSE_PROFILES"] != s.Current["COMPOSE_PROFILES"] {
		t.Fatal("removed explicitly selected unrelated services")
	}
}

// TestExcludedConflictingScopesFailBeforePublication refuses a partial switch that cannot retain separate inherited drivers.
func TestExcludedConflictingScopesFailBeforePublication(t *testing.T) {
	s := overlappingDatabaseSession(t, map[string]string{"DB_DRIVER": "mysql", "FOO_DB_DRIVER": "postgres"})
	_, err := s.PortableFor(map[string]bool{"FOO_DB_DB_DRIVER": true})
	if err == nil || !strings.Contains(err.Error(), "conflicting inherited") {
		t.Fatalf("expected scope conflict, got %v", err)
	}
}

// TestDriversRepeatedSwitchRetainsOriginalRecovery covers provider edits in both directions and a third combination.
func TestDriversRepeatedSwitchRetainsOriginalRecovery(t *testing.T) {
	root := fixture(t)
	s := openTest(t, root)
	original := clone(s.Current)
	local, err := s.Portable()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(local); err != nil {
		t.Fatal(err)
	}
	for _, choices := range []map[string]string{
		{"DB_DRIVER": "mysql", "CACHE_DRIVER": "redis", "QUEUE_DRIVER": "redis"},
		{"DB_DRIVER": "sqlite", "CACHE_DRIVER": "memory"},
		{"CACHE_DRIVER": "redis"},
	} {
		s = openTest(t, root)
		values, err := s.Drivers(choices)
		if err != nil {
			t.Fatal(err)
		}
		for key, want := range choices {
			if values[key] != want {
				t.Fatalf("%s=%s, want %s", key, values[key], want)
			}
		}
		if err := s.Apply(values); err != nil {
			t.Fatal(err)
		}
	}
	s = openTest(t, root)
	if err := s.Restore(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(openTest(t, root).Current, original) {
		t.Fatal("repeated selections lost the original configuration")
	}
}

// TestDriversPreserveUnselectedInheritedTargets pins other Apps while editing root database drivers.
func TestDriversPreserveUnselectedInheritedTargets(t *testing.T) {
	root := fixture(t)
	put(t, root, ".env", "DB_DRIVER=mysql\nDB_DSN=synthetic-service-dsn\nCOMPOSE_PROFILES=mysql,grafana\n")
	s := openTest(t, root)
	before := databaseTargetInScope(s.Current, "ADMIN_DB_DRIVER", "ADMIN_")
	values, err := s.Drivers(map[string]string{"DB_DRIVER": "sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	if databaseTargetInScope(values, "ADMIN_DB_DRIVER", "ADMIN_") != before {
		t.Fatal("picker changed unselected inherited target")
	}
	if values["COMPOSE_PROFILES"] != "mysql,grafana" {
		t.Fatal("picker removed a service required by another App")
	}
}

// TestDriversRejectInvalidChoicesAndConflictingEdits stops invalid and stale plans before publication.
func TestDriversRejectInvalidChoicesAndConflictingEdits(t *testing.T) {
	for _, selections := range []map[string]string{{"UNKNOWN_DRIVER": "redis"}, {"CACHE_DRIVER": "invalid"}} {
		s := openTest(t, fixture(t))
		original := clone(s.Current)
		if _, err := s.Drivers(selections); err == nil {
			t.Fatal("accepted invalid selection")
		}
		if !reflect.DeepEqual(s.Current, original) {
			t.Fatal("failed plan changed current settings")
		}
	}
	root := fixture(t)
	s := openTest(t, root)
	values, err := s.Portable()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(values); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	put(t, root, ".env", strings.ReplaceAll(string(raw), "CACHE_DRIVER=memory", "CACHE_DRIVER=redis"))
	s = openTest(t, root)
	if _, err := s.Drivers(map[string]string{"DB_DRIVER": "mysql"}); err == nil {
		t.Fatal("picker accepted changed recovery settings")
	}
	values = clone(s.Current)
	values["DB_DRIVER"] = "mysql"
	if err := s.Apply(values); err == nil {
		t.Fatal("apply overwrote changed recovery settings")
	}
}

// TestDriversRecoverOriginalDSNButNeverReuseSQLiteDSN keeps connection syntax tied to its driver.
func TestDriversRecoverOriginalDSNButNeverReuseSQLiteDSN(t *testing.T) {
	for _, initial := range []string{"DB_DRIVER=mysql\nDB_DSN=synthetic-private-dsn\n", "DB_DRIVER=sqlite\nDB_DSN=file:existing.db\n"} {
		t.Run(initial, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".env", initial)
			s := openTest(t, root)
			if s.Current["DB_DRIVER"] == "mysql" {
				values, err := s.Portable()
				if err != nil {
					t.Fatal(err)
				}
				if err := s.Apply(values); err != nil {
					t.Fatal(err)
				}
				s = openTest(t, root)
			}
			values, err := s.Drivers(map[string]string{"DB_DRIVER": "mysql"})
			if err != nil {
				t.Fatal(err)
			}
			want := ""
			if strings.Contains(initial, "synthetic-private-dsn") {
				want = "synthetic-private-dsn"
			}
			if values["DB_DSN"] != want {
				t.Fatal("service driver received the wrong DSN")
			}
			if !strings.Contains(values["COMPOSE_PROFILES"], "mysql") {
				t.Fatal("new service profile not enabled")
			}
		})
	}
}

// TestDriversKeepCurrentIsANoOp avoids adding explicit settings just by visiting the picker.
func TestDriversKeepCurrentIsANoOp(t *testing.T) {
	s := openTest(t, fixture(t))
	values, err := s.Drivers(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(values, s.Current) {
		t.Fatal("unchanged picker changed configuration")
	}
}

// TestDriversRejectOverlappingInheritedTargets keeps the same scope guarantees as the local preset.
func TestDriversRejectOverlappingInheritedTargets(t *testing.T) {
	s := overlappingDatabaseSession(t, map[string]string{"DB_DRIVER": "mysql", "FOO_DB_DRIVER": "postgres"})
	if _, err := s.Drivers(map[string]string{"DB_DRIVER": "sqlite"}); err == nil {
		t.Fatal("accepted conflicting inherited targets")
	}
}

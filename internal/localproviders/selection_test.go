package localproviders

import (
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

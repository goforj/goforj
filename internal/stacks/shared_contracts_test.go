package stacks

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/generate"
)

// sharedDatabaseCacheSession models a valid key consumed by both an App cache and a named database.
func sharedDatabaseCacheSession(t *testing.T, overlapping bool) (*Session, string) {
	t.Helper()
	root := fixture(t)
	apps := "  db:\n    components: [database_mysql, cache]\n"
	key := "DB_CACHE_DRIVER"
	if overlapping {
		apps = "  admin:\n    components: [database_mysql]\n  admin-db:\n    components: [cache]\n"
		key = "ADMIN_DB_CACHE_DRIVER"
	}
	put(t, root, ".goforj.yml", "project_name: stacks\nmodule_name: example.org/stacks\nrender:\n  components: [database_mysql, cache]\napps:\n"+apps)
	put(t, root, ".env", "DB_DRIVER=mysql\nDB_SUPPORTED_DRIVERS=mysql\n"+key+"=mysql\nCACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory,mysql\n")
	return openTest(t, root), key
}

// TestSharedResourceContractsRejectIncompatibleDrivers protects edits, saved profiles, and activation before an invalid database driver reaches generation.
func TestSharedResourceContractsRejectIncompatibleDrivers(t *testing.T) {
	for _, overlapping := range []bool{false, true} {
		s, key := sharedDatabaseCacheSession(t, overlapping)
		resource := databaseResource(t, s, key)
		if _, allowed := resource.Definition.Driver("memory"); allowed {
			t.Fatal("picker offered a driver that the database cannot consume")
		}
		values := clone(s.Current)
		if err := s.SetDriver(values, resource, "memory"); err == nil || !equalValues(values, s.Current) {
			t.Fatal("invalid shared driver edit succeeded or changed values")
		}
		if err := s.SetValue(values, key, "memory"); err == nil || !equalValues(values, s.Current) {
			t.Fatal("generic editor accepted an incompatible shared driver")
		}
		portable, err := s.Portable()
		if err != nil || portable[key] != "sqlite" {
			t.Fatalf("portable did not select the shared local driver: %v", err)
		}
		if err := s.Validate(portable); err != nil {
			t.Fatal(err)
		}
		values[key] = "memory"
		if err := s.Validate(values); err == nil {
			t.Fatal("validation ignored a database consumer")
		}
		if err := s.Save("invalid", values); err == nil {
			t.Fatal("invalid definition was saved")
		}
		if err := s.Activate("", values, false); err == nil {
			t.Fatal("invalid shared driver was activated")
		}
		if _, err := os.Stat(filepath.Join(s.Root, DefinitionPath("invalid"))); !os.IsNotExist(err) {
			t.Fatal("failed save created a definition")
		}
		if !equalValues(openTest(t, s.Root).Current, s.Current) {
			t.Fatal("failed activation changed .env")
		}
	}
}

// TestSharedResourceChoicesCompileForEveryConsumer verifies common choices and every affected driver manifest against the real generators.
func TestSharedResourceChoicesCompileForEveryConsumer(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		for _, overlapping := range []bool{false, true} {
			t.Run(driver+map[bool]string{false: "/root", true: "/apps"}[overlapping], func(t *testing.T) {
				s, key := sharedDatabaseCacheSession(t, overlapping)
				values := clone(s.Current)
				if err := s.SetDriver(values, databaseResource(t, s, key), driver); err != nil {
					t.Fatal(err)
				}
				for _, manifest := range []string{"DB_SUPPORTED_DRIVERS", "CACHE_SUPPORTED_DRIVERS"} {
					if !slices.Contains(strings.Split(values[manifest], ","), driver) {
						t.Fatalf("%s omitted the selected shared driver", manifest)
					}
				}
				if err := s.Validate(values); err != nil {
					t.Fatal(err)
				}
				for key, value := range values {
					t.Setenv(key, value)
				}
				if _, err := generate.GenerateDBFiles(s.Root); err != nil {
					t.Fatalf("database generator: %v", err)
				}
				if _, err := generate.GenerateCacheFiles(s.Root); err != nil {
					t.Fatalf("cache generator: %v", err)
				}
				if driver == "postgres" {
					values["DB_SUPPORTED_DRIVERS"] = "mysql"
					if err := s.Validate(values); err == nil {
						t.Fatal("validation ignored a shared consumer's compiled support")
					}
				}
			})
		}
	}
}

// TestSharedServiceDatabaseNamesRemainPrivate gives service configuration precedence over automatic legacy SQLite path publication.
func TestSharedServiceDatabaseNamesRemainPrivate(t *testing.T) {
	for _, tc := range []struct {
		name, privateKey string
		values           map[string]string
	}{
		{"overlapping", "FOO_DB_DB_DATABASE", map[string]string{"DB_DRIVER": "mysql", "FOO_DB_DRIVER": "sqlite", "FOO_DB_DB_DATABASE": "tenant-private-name"}},
		{"inherited", "DB_DATABASE", map[string]string{"DB_DRIVER": "sqlite", "FOO_DB_DRIVER": "mysql", "DB_DATABASE": "tenant-private-name"}},
		{"explicit-path", "FOO_DB_DB_DATABASE", map[string]string{"DB_DRIVER": "mysql", "FOO_DB_DRIVER": "sqlite", "FOO_DB_DB_DATABASE": "tenant-private-name", "FOO_DB_DB_SQLITE_DATABASE": "./data/portable.db"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := overlappingDatabaseSession(t, tc.values)
			if err := s.Save("saved", s.Current); err != nil {
				t.Fatal(err)
			}
			defaults, err := BuildDefaults(s.Root, "saved")
			if err != nil {
				t.Fatal(err)
			}
			if _, present := defaults[tc.privateKey]; present {
				t.Fatal("service database name entered shareable defaults")
			}
			if path := tc.values["FOO_DB_DB_SQLITE_DATABASE"]; path != "" && defaults["FOO_DB_DB_SQLITE_DATABASE"] != path {
				t.Fatal("explicit portable path was lost")
			}
			loaded, err := openTest(t, s.Root).Load("saved", true)
			if err != nil || !equalValues(loaded, tc.values) {
				t.Fatalf("private round trip changed values: %v", err)
			}
			put(t, s.Root, DefinitionPath("manual"), string(encodeDocument(tc.values)))
			if _, err := BuildDefaults(s.Root, "manual"); err == nil || strings.Contains(err.Error(), "tenant-private-name") {
				t.Fatal("manually shared service settings were accepted or echoed")
			}
		})
	}
}

// TestSetValueRejectsInvalidEditsWithoutMutation keeps the generic editor transactional and its diagnostics private.
func TestSetValueRejectsInvalidEditsWithoutMutation(t *testing.T) {
	s := openTest(t, fixture(t))
	for _, tc := range []struct{ key, value string }{
		{"DB_DRIVER", "private-secret"}, {"DB_SUPPORTED_DRIVERS", "private-secret"}, {"APP_NAME", "private-secret"},
	} {
		values := clone(s.Current)
		err := s.SetValue(values, tc.key, tc.value)
		if err == nil || strings.Contains(err.Error(), "private-secret") || !equalValues(values, s.Current) {
			t.Fatalf("invalid edit was applied or exposed: %v", err)
		}
	}
	values := clone(s.Current)
	if err := s.SetValue(values, "DB_PASSWORD", "private-secret"); err != nil || values["DB_PASSWORD"] != "private-secret" {
		t.Fatalf("private connection edit failed: %v", err)
	}
}

// TestSharedResourceWithoutCommonDriverReportsConflict keeps impossible explicit selections out of the picker.
func TestSharedResourceWithoutCommonDriverReportsConflict(t *testing.T) {
	root := fixture(t)
	put(t, root, ".goforj.yml", "project_name: stacks\nmodule_name: example.org/stacks\nrender:\n  components: [database_mysql, events]\napps:\n  db:\n    components: [events]\n")
	put(t, root, ".env", "DB_DRIVER=mysql\nDB_EVENTS_DRIVER=\n")
	s := openTest(t, root)
	resource := databaseResource(t, s, "DB_EVENTS_DRIVER")
	if len(resource.Definition.Drivers) != 0 {
		t.Fatal("picker advertised incompatible shared drivers")
	}
	if _, err := s.Portable(); err == nil || !strings.Contains(err.Error(), "no portable driver") {
		t.Fatalf("missing portable conflict: %v", err)
	}
}

// TestSetValueClearsDatabaseOverridesSafely retains blank fallback semantics while preparing newly selected SQLite targets.
func TestSetValueClearsDatabaseOverridesSafely(t *testing.T) {
	for _, key := range []string{"DB_DRIVER", "DB_REPORTS_DRIVER", "ADMIN_DB_DRIVER", "ADMIN_DB_REPORTS_DRIVER"} {
		t.Run(key, func(t *testing.T) {
			root := fixture(t)
			prefix := strings.TrimSuffix(key, "DRIVER")
			put(t, root, ".env", "DB_DRIVER=sqlite\nDB_SUPPORTED_DRIVERS=mysql\n"+key+"=mysql\n"+prefix+"DSN=private-service-dsn\n")
			s := openTest(t, root)
			values := clone(s.Current)
			if err := s.SetValue(values, key, ""); err != nil {
				t.Fatal(err)
			}
			if values[key] != "" || values[prefix+"DSN"] != "" || values[prefix+"SQLITE_DATABASE"] == "" {
				t.Fatal("clearing override lost fallback or left service DSN selected")
			}
			if !slices.Contains(strings.Split(values["DB_SUPPORTED_DRIVERS"], ","), "mysql") {
				t.Fatal("clearing override discarded existing driver support")
			}
		})
	}
}

// TestSetValueClearsSharedOverrideForIndependentFallbacks exercises the generators when blank has different meanings for shared consumers.
func TestSetValueClearsSharedOverrideForIndependentFallbacks(t *testing.T) {
	for _, overlapping := range []bool{false, true} {
		t.Run(map[bool]string{false: "root", true: "apps"}[overlapping], func(t *testing.T) {
			s, key := sharedDatabaseCacheSession(t, overlapping)
			values := clone(s.Current)
			if err := s.SetValue(values, key, ""); err != nil {
				t.Fatal(err)
			}
			if values[key] != "" {
				t.Fatal("blank override became explicit")
			}
			for key, value := range values {
				t.Setenv(key, value)
			}
			if _, err := generate.GenerateDBFiles(s.Root); err != nil {
				t.Fatal(err)
			}
			if _, err := generate.GenerateCacheFiles(s.Root); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestSetValueRejectsConflictingInheritedDatabaseDrivers leaves shared overrides untouched when clearing them requires distinct transitions.
func TestSetValueRejectsConflictingInheritedDatabaseDrivers(t *testing.T) {
	s := overlappingDatabaseSession(t, map[string]string{"DB_DRIVER": "mysql", "FOO_DB_DRIVER": "mysql", "FOO_DB_DB_DRIVER": "postgres"})
	values := clone(s.Current)
	if err := s.SetValue(values, "FOO_DB_DB_DRIVER", ""); err == nil || !equalValues(values, s.Current) {
		t.Fatal("conflicting inherited drivers were accepted or changed values")
	}
}

// TestSharedDatabaseDriverUsesDatabaseInheritance clears inherited service DSNs even when the picker displays the cache family.
func TestSharedDatabaseDriverUsesDatabaseInheritance(t *testing.T) {
	s, key := sharedDatabaseCacheSession(t, false)
	s.Current["DB_DSN"] = "private-service-dsn"
	values := clone(s.Current)
	if err := s.SetValue(values, key, "sqlite"); err != nil {
		t.Fatal(err)
	}
	if (values["DB_CACHE_DSN"] != "" && values["DB_CACHE_DSN"] != values["DB_CACHE_SQLITE_DATABASE"]) || values["DB_CACHE_SQLITE_DATABASE"] == "" {
		t.Fatal("shared driver did not prepare the database target")
	}
}

// TestSetValueClearingRetainsInferredSupport keeps prior providers compiled when the owner restores default or inherited selection.
func TestSetValueClearingRetainsInferredSupport(t *testing.T) {
	for _, key := range []string{"DB_DRIVER", "ADMIN_DB_DRIVER", "CACHE_DRIVER"} {
		t.Run(key, func(t *testing.T) {
			root := fixture(t)
			driver, supportKey := "mysql", "DB_SUPPORTED_DRIVERS"
			if key == "CACHE_DRIVER" {
				driver, supportKey = "redis", "CACHE_SUPPORTED_DRIVERS"
			}
			put(t, root, ".env", key+"="+driver+"\n")
			s := openTest(t, root)
			values := clone(s.Current)
			if err := s.SetValue(values, key, ""); err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(strings.Split(values[supportKey], ","), driver) {
				t.Fatal("clearing override discarded inferred driver support")
			}
		})
	}
}

// TestSetValueClearingUnsetDriverRetainsInferredDefaults avoids introducing an empty compiled-driver contract.
func TestSetValueClearingUnsetDriverRetainsInferredDefaults(t *testing.T) {
	root := fixture(t)
	put(t, root, ".env", "")
	s := openTest(t, root)
	values := clone(s.Current)
	if err := s.SetValue(values, "DB_DRIVER", ""); err != nil {
		t.Fatal(err)
	}
	if _, set := values["DB_SUPPORTED_DRIVERS"]; set {
		t.Fatal("clearing an unset driver changed inferred support")
	}
}

// TestSharedDriverRepairDoesNotCompileInvalidPreviousSelections lets owners repair shared values accepted before every consumer was validated.
func TestSharedDriverRepairDoesNotCompileInvalidPreviousSelections(t *testing.T) {
	for _, generic := range []bool{false, true} {
		for _, overlapping := range []bool{false, true} {
			s, key := sharedDatabaseCacheSession(t, overlapping)
			values := clone(s.Current)
			values[key] = "memory"
			var err error
			if generic {
				err = s.SetValue(values, key, "sqlite")
			} else {
				err = s.SetDriver(values, databaseResource(t, s, key), "sqlite")
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Validate(values); err != nil {
				t.Fatal(err)
			}
			if slices.Contains(strings.Split(values["DB_SUPPORTED_DRIVERS"], ","), "memory") || !slices.Contains(strings.Split(values["CACHE_SUPPORTED_DRIVERS"], ","), "memory") {
				t.Fatal("repair did not preserve prior support only for compatible consumers")
			}
		}
	}
}

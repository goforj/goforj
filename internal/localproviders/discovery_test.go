package localproviders

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/project"
)

// TestResourceSingleDriverEditPreservesSiblingSupport keeps inferred contracts valid when only one root, named resource, or App changes.
func TestResourceSingleDriverEditPreservesSiblingSupport(t *testing.T) {
	for _, definition := range project.ResourceCatalog() {
		for index, scope := range []string{"", "REPORTS_", "ADMIN_", "ADMIN_REPORTS_"} {
			t.Run(string(definition.Key)+"/"+scope, func(t *testing.T) {
				s := openTest(t, fixture(t))
				prefix := definition.EnvironmentPrefix + "_"
				resourceKeys := []string{prefix + "DRIVER", prefix + "REPORTS_DRIVER", "ADMIN_" + prefix + "DRIVER", "ADMIN_" + prefix + "REPORTS_DRIVER"}
				values := map[string]string{}
				for i, key := range resourceKeys {
					values[key] = definition.Drivers[(i+1)%len(definition.Drivers)].Name
				}
				before := clone(values)
				if err := s.Validate(values); err != nil {
					t.Fatal(err)
				}
				for _, resource := range resources(s.Root, s.config, values) {
					if resource.Key == resourceKeys[index] {
						if err := s.selectDriver(values, resource, definition.DefaultDriver); err != nil {
							t.Fatal(err)
						}
					}
				}
				if values[resourceKeys[index]] != definition.DefaultDriver {
					t.Fatal("selected resource was not edited")
				}
				if err := s.Validate(values); err != nil {
					t.Fatal(err)
				}
				supported := strings.Split(values[definition.EnvironmentKey("SUPPORTED_DRIVERS")], ",")
				for key, driver := range before {
					if !slices.Contains(supported, project.CanonicalResourceDriver(definition.Key, driver)) {
						t.Fatalf("lost support for %s=%s: %v", key, driver, supported)
					}
					if key != resourceKeys[index] && values[key] != driver {
						t.Fatalf("changed sibling %s", key)
					}
				}
			})
		}
	}
}

// TestResourceInvalidDriverEditLeavesValuesUntouched prevents failed wizard choices from changing the working configuration.
func TestResourceInvalidDriverEditLeavesValuesUntouched(t *testing.T) {
	s := openTest(t, fixture(t))
	values := clone(s.Current)
	if err := s.selectDriver(values, s.Resources[0], "invalid"); err == nil {
		t.Fatal("accepted an unknown driver")
	}
	if !reflect.DeepEqual(values, s.Current) {
		t.Fatal("failed driver edit changed configuration")
	}
}

// TestPortableNamedDatabaseDiscoveryStaysStable protects repeated conversion and real names containing sqlite.
func TestPortableNamedDatabaseDiscoveryStaysStable(t *testing.T) {
	root := fixture(t)
	put(t, root, ".env", "DB_DRIVER=mysql\nDB_SUPPORTED_DRIVERS=mysql\nDB_ANALYTICS_DATABASE=analytics\nDB_REPORTING_SQLITE_DRIVER=mysql\nADMIN_DB_REPORTS_DATABASE=reports\n")
	want := []string{"analytics", "reporting_sqlite", "reports"}
	for i := 0; i < 4; i++ {
		s := openTest(t, root)
		if got := generate.ResourceNames(root, s.Current)[project.ResourceDatabase]; !reflect.DeepEqual(got, want) {
			t.Fatalf("pass %d: names=%v, want %v", i, got, want)
		}
		portable, err := s.Portable()
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Apply(portable); err != nil {
			t.Fatal(err)
		}
	}
}

// TestResourceStillRejectsDriversOutsideTheCompiledContract distinguishes built-in baseline providers from optional local providers.
func TestResourceStillRejectsDriversOutsideTheCompiledContract(t *testing.T) {
	s := openTest(t, fixture(t))
	for _, values := range []map[string]string{
		{"CACHE_DRIVER": "sqlite", "CACHE_SUPPORTED_DRIVERS": "redis"},
		{"STORAGE_DRIVER": "memory", "STORAGE_SUPPORTED_DRIVERS": "s3"},
		{"DB_DRIVER": "postgres", "DB_SUPPORTED_DRIVERS": "mysql"},
		{"DB_DRIVER": "sqlite", "DB_SUPPORTED_DRIVERS": ","},
		{"CACHE_DRIVER": "memory", "CACHE_SUPPORTED_DRIVERS": "unknown"},
	} {
		if err := s.Validate(values); err == nil {
			t.Fatalf("accepted invalid contract: %v", values)
		}
	}
	if err := s.Validate(map[string]string{"CACHE_DRIVER": "memory", "CACHE_SUPPORTED_DRIVERS": " "}); err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(map[string]string{"CACHE_DRIVER": "redis", "CACHE_SUPPORTED_DRIVERS": ","}); err != nil {
		t.Fatal(err)
	}
}

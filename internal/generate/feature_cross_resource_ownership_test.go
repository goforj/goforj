package generate

import (
	"reflect"
	"testing"
)

// TestFeatureDiscoveryExcludesOtherAppResourceFamilies keeps App component settings out of default evaluator discovery and validation.
func TestFeatureDiscoveryExcludesOtherAppResourceFamilies(t *testing.T) {
	root := featureInventoryProject(t, "", "feature", "feature-child")
	for _, resource := range generationDriverResourcePrefixes {
		if resource == "FEATURE" {
			continue
		}
		t.Run(resource, func(t *testing.T) {
			input := fixtureGenerationInput(root, map[string]string{
				"FEATURE_SHARED_DRIVER":                 "memory",
				"FEATURE_FEATURE_DRIVER":                "memory",
				"FEATURE_" + resource + "_DRIVER":       "other-component-driver",
				"FEATURE_CHILD_" + resource + "_DRIVER": "other-component-driver",
			})
			if names := discoverFeatureNames(input); !reflect.DeepEqual(names, []string{"shared"}) {
				t.Fatalf("names = %v", names)
			}
			if _, err := generateFeatureFiles(input); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestFeatureDiscoveryPreservesUnownedResourceWordNames retains default evaluator names when no App owns the same environment key.
func TestFeatureDiscoveryPreservesUnownedResourceWordNames(t *testing.T) {
	root := featureInventoryProject(t, "")
	input := fixtureGenerationInput(root, map[string]string{"FEATURE_CACHE_DRIVER": "memory", "FEATURE_SHARED_DRIVER": "memory"})
	if names := discoverFeatureNames(input); !reflect.DeepEqual(names, []string{"cache", "shared"}) {
		t.Fatalf("names = %v", names)
	}
	if _, err := generateFeatureFiles(input); err != nil {
		t.Fatal(err)
	}
}

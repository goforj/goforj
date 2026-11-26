package localproviders

import (
	"testing"

	"github.com/goforj/goforj/internal/generate"
)

// TestFeaturePortableRegeneration preserves source paths through publication, regeneration, and restoration.
func TestFeaturePortableRegeneration(t *testing.T) {
	root := fixture(t)
	put(t, root, ".goforj.yml", "module_name: example.org/featureportable\nrender:\n  components: [cli, feature]\napps:\n  billing:\n    components: [cli, feature]\n")
	paths := map[string]string{
		"FEATURE_PATH": "flags/default.json", "FEATURE_CHECKOUT_PATH": "flags/checkout.json",
		"BILLING_FEATURE_PATH": "flags/billing.json", "BILLING_FEATURE_ONLY_PATH": "flags/billing-only.json",
	}
	drivers := []string{"FEATURE_DRIVER", "FEATURE_CHECKOUT_DRIVER", "BILLING_FEATURE_DRIVER", "BILLING_FEATURE_ONLY_DRIVER"}
	contents := "FEATURE_SUPPORTED_DRIVERS=memory,file\n"
	for _, key := range drivers {
		contents += key + "=file\n"
	}
	for key, value := range paths {
		contents += key + "=" + value + "\n"
	}
	put(t, root, ".env", contents)
	session := openTest(t, root)
	portable, err := session.Portable()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range drivers {
		if portable[key] != "memory" {
			t.Fatalf("%s=%q", key, portable[key])
		}
	}
	if err := session.Apply(portable); err != nil {
		t.Fatal(err)
	}
	for _, driver := range []string{"memory", "file"} {
		if driver == "file" {
			session = openTest(t, root)
			restored := clone(session.Current)
			for _, key := range drivers {
				restored[key] = driver
			}
			if err := session.Apply(restored); err != nil {
				t.Fatal(err)
			}
		}
		for pass := 0; pass < 2; pass++ {
			result, err := generate.GenerateProjectFiles(root, generate.GenerationSelection{Feature: true})
			if err != nil {
				t.Fatalf("%s generation: %v", driver, err)
			}
			if pass == 1 && result.ChangedFiles != 0 {
				t.Fatalf("%s regeneration drift: %#v", driver, result)
			}
		}
		session = openTest(t, root)
		for key, want := range paths {
			if session.Current[key] != want {
				t.Fatalf("%s changed %s", driver, key)
			}
		}
		for _, key := range drivers {
			if session.Current[key] != driver {
				t.Fatalf("%s=%q, want %s", key, session.Current[key], driver)
			}
		}
	}
}

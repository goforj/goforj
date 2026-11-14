package localproviders

import (
	"testing"

	"github.com/goforj/goforj/project"
)

// TestResourceInventoriesKeepProjectedResourceWordNames prevents App ownership from being reapplied after scope projection.
func TestResourceInventoriesKeepProjectedResourceWordNames(t *testing.T) {
	for _, test := range []struct{ component, prefix, name, setting, driver string }{
		{"feature", "FEATURE", "CACHE", "PATH", "memory"},
		{"notification", "NOTIFICATION", "FEATURE", "WEBHOOK_URL", "log"},
		{"search", "SEARCH", "CACHE", "PATH", "bleve"},
	} {
		t.Run(test.component, func(t *testing.T) {
			root := fixture(t)
			put(t, root, ".goforj.yml", "module_name: example.org/scopes\nrender:\n  components: [cli, "+test.component+"]\napps:\n  "+test.component+":\n    components: [cli, "+test.component+"]\n  admin:\n    components: [cli, "+test.component+"]\n  reports:\n    components: [cli, "+test.component+"]\n")
			ownKey := "ADMIN_" + test.prefix + "_" + test.name + "_DRIVER"
			ownEndpoint := "ADMIN_" + test.prefix + "_" + test.name + "_" + test.setting
			exampleKey := "ADMIN_" + test.prefix + "_" + test.name + "_EXAMPLE_DRIVER"
			previewKey := "ADMIN_" + test.prefix + "_" + test.name + "_PREVIEW_DRIVER"
			put(t, root, ".env", test.prefix+"_DRIVER="+test.driver+"\n"+test.prefix+"_SHARED_DRIVER="+test.driver+"\n"+ownEndpoint+"=fixture-value\n")
			put(t, root, ".env.example", "ADMIN_"+test.prefix+"_"+test.name+"_EXAMPLE_"+test.setting+"=example-value\n")
			session := openTest(t, root)
			preview := clone(session.Current)
			preview["ADMIN_"+test.prefix+"_"+test.name+"_PREVIEW_"+test.setting] = "preview-value"
			inventory := map[string]bool{}
			for _, resource := range session.ResourcesFor(preview) {
				inventory[resource.Key] = true
			}
			for _, key := range []string{ownKey, exampleKey, previewKey, test.prefix + "_SHARED_DRIVER", "ADMIN_" + test.prefix + "_SHARED_DRIVER", "REPORTS_" + test.prefix + "_SHARED_DRIVER", project.AppEnvironmentPrefix(test.component) + "_" + test.prefix + "_SHARED_DRIVER"} {
				if !inventory[key] {
					t.Errorf("missing %s", key)
				}
			}
			for _, scope := range []string{"", "REPORTS_"} {
				if key := scope + test.prefix + "_" + test.name + "_DRIVER"; inventory[key] {
					t.Errorf("App declaration leaked into %s", key)
				}
			}
			portable, err := session.Portable()
			if err != nil {
				t.Fatal(err)
			}
			if portable[ownKey] != test.driver || portable[exampleKey] != test.driver {
				t.Fatalf("portable omitted projected names: %#v", portable)
			}
		})
	}
}

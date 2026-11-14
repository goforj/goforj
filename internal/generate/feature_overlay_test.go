package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFeatureOverlayDrivers aligns validation and active-driver discovery with the raw environment fallback contract.
func TestFeatureOverlayDrivers(t *testing.T) {
	for _, scope := range []string{"", "_CHECKOUT"} {
		for _, test := range []struct {
			name, base, overlay, want string
		}{
			{"empty inherits file", "file", "", "file"},
			{"whitespace selects memory", "file", " \t ", "memory"},
			{"explicit normalized file", "memory", " FiLe ", "file"},
			{"explicit memory", "file", "memory", "memory"},
			{"blank base defaults", "", "", "memory"},
			{"whitespace base defaults", " \t ", "", "memory"},
		} {
			t.Run(scope+"/"+test.name, func(t *testing.T) {
				root := t.TempDir()
				key := "FEATURE" + scope + "_DRIVER"
				appKey := "BILLING_" + key
				values := map[string]string{key: test.base, appKey: test.overlay, "FEATURE_SUPPORTED_DRIVERS": "memory"}
				if scope != "" {
					if strings.EqualFold(strings.TrimSpace(test.base), "file") {
						values["FEATURE"+scope+"_PATH"] = "base.json"
					}
					if test.want == "file" {
						values["BILLING_FEATURE"+scope+"_PATH"] = "billing.json"
					} else {
						values["BILLING_FEATURE"+scope+"_DOCUMENT"] = `{"schema":1,"flags":{}}`
					}
				}
				input := fixtureGenerationInput(root, values)
				drivers := appPrefixedActiveDrivers(input, "FEATURE", "memory", false)
				if len(drivers) != 1 || drivers[0].key != appKey || drivers[0].driver != test.want {
					t.Fatalf("active drivers = %#v, want %s=%s", drivers, appKey, test.want)
				}
				if count, err := generateFeatureFiles(input); err != nil || count != 2 {
					t.Fatalf("generate: count=%d err=%v", count, err)
				}
				manager, err := os.ReadFile(filepath.Join(root, FeatureManagerPath))
				if err != nil {
					t.Fatal(err)
				}
				_, manifest, found := strings.Cut(string(manager), "var compiledFeatureDrivers = map[string]bool{")
				if !found {
					t.Fatal("generated manager does not declare compiled Feature drivers")
				}
				manifest, _, _ = strings.Cut(manifest, "}")
				assertManifestIncludes(t, manifest, "memory", "file")
				if count, err := generateFeatureFiles(input); err != nil || count != 0 {
					t.Fatalf("regenerate metadata: count=%d err=%v", count, err)
				}
			})
		}
	}
}

// TestFeatureOverlayRejectsIncompatibleDrivers preserves errors for explicitly selected incompatible or unsupported drivers.
func TestFeatureOverlayRejectsIncompatibleDrivers(t *testing.T) {
	for _, test := range []struct {
		name, driver, option, want string
	}{
		{"memory with file path", "memory", "PATH", `not supported for feature driver "memory"`},
		{"whitespace with file path", " \t ", "PATH", `not supported for feature driver "memory"`},
		{"inherited file with document", "", "DOCUMENT", `not supported for feature driver "file"`},
		{"unknown driver", "hosted", "PATH", `unsupported driver "hosted"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			input := fixtureGenerationInput(root, map[string]string{
				"FEATURE_CHECKOUT_DRIVER": "file", "FEATURE_CHECKOUT_PATH": "base.json",
				"BILLING_FEATURE_CHECKOUT_DRIVER": test.driver, "BILLING_FEATURE_CHECKOUT_" + test.option: "value",
			})
			if count, err := generateFeatureFiles(input); err == nil || count != 0 || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("generate: count=%d err=%v, want %s", count, err, test.want)
			}
		})
	}
}

// TestGeneratedFeatureOverlayRuntime checks the generated lookup against the released env SDK and compiled-driver guard.
func TestGeneratedFeatureOverlayRuntime(t *testing.T) {
	root := t.TempDir()
	input := fixtureGenerationInput(root, map[string]string{"FEATURE_CHECKOUT_DRIVER": "file", "BILLING_FEATURE_CHECKOUT_DRIVER": ""})
	if _, err := generateFeatureFiles(input); err != nil {
		t.Fatal(err)
	}
	writeFixtureGoMod(t, root, fixtureGoModSpec{module: "example.com/featureoverlay", required: []string{"github.com/goforj/feature", "github.com/goforj/env/v2", "github.com/fsnotify/fsnotify", "golang.org/x/text"}})
	if err := os.WriteFile(filepath.Join(root, "internal/features/overlay_test.go"), []byte(featureOverlayRuntimeTests), 0600); err != nil {
		t.Fatal(err)
	}
	runFixtureGoModTidy(t, root, nil)
	runFixtureGoTest(t, root, "./internal/features", "", nil)
}

const featureOverlayRuntimeTests = `package features

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

// TestFeatureOverlayRuntime verifies normalization happens only after the App has inherited a raw base value.
func TestFeatureOverlayRuntime(t *testing.T) {
    path := filepath.Join(t.TempDir(), "flags.json")
    if err := os.WriteFile(path, []byte("{\"schema\":1,\"flags\":{}}"), 0600); err != nil { t.Fatal(err) }
    for _, name := range []string{"default", "checkout"} {
        t.Run(name, func(t *testing.T) {
            suffix := ""
            if name != "default" { suffix = "_CHECKOUT" }
            key := "FEATURE"+suffix
            t.Setenv("FEATURE_DRIVER", "file")
            t.Setenv(key+"_PATH", path)
            t.Setenv(key+"_DOCUMENT", "{\"schema\":1,\"flags\":{}}")
            for _, test := range []struct { base, overlay, want string }{
                {"file", "", "file"}, {"file", " \t ", "memory"},
                {"memory", " FiLe ", "file"}, {"file", "memory", "memory"},
                {"", "", "memory"}, {" \t ", "", "memory"},
            } {
                t.Setenv(key+"_DRIVER", test.base)
                t.Setenv("BILLING_"+key+"_DRIVER", test.overlay)
                resource, err := openEvaluator("BILLING", name)
                if err != nil { t.Fatal(err) }
                if resource.driver != test.want { t.Fatalf("base=%q overlay=%q: driver=%s want=%s", test.base,test.overlay,resource.driver,test.want) }
            }
            t.Setenv(key+"_DRIVER", "file")
            t.Setenv("BILLING_"+key+"_DRIVER", "")
            compiledFeatureDrivers["file"] = false
            defer func() { compiledFeatureDrivers["file"] = true }()
            if _, err := openEvaluator("BILLING", name); err == nil || !strings.Contains(err.Error(), "not compiled") { t.Fatalf("uncompiled inherited driver: %v",err) }
        })
    }
}
`

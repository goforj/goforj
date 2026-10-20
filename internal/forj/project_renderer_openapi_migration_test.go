package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestRenderAutomaticallyUpgradesOpenAPI exercises the complete rerender path rather than only its cleanup helper.
func TestRenderAutomaticallyUpgradesOpenAPI(t *testing.T) {
	for _, legacyKey := range []string{"API_SWAGGER_ENABLED", "SWAGGER_ENABLED"} {
		t.Run(legacyKey, func(t *testing.T) {
			root := t.TempDir()
			config := &project.Config{
				ProjectName:  "OpenAPI Upgrade",
				GoModuleName: "example.com/openapi-upgrade",
				Render:       project.RenderConfig{Components: project.Components{CLI: true, WebAPI: true}},
			}
			data, err := yaml.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			writeProjectRendererTestFile(t, filepath.Join(root, ".goforj.yml"), string(data))
			renderer := unitProjectRenderer(t)
			input := ComponentRenderInput{renderAll: true, root: root}
			if err := renderer.Render(input); err != nil {
				t.Fatalf("prepare generated project: %v", err)
			}

			// Recreate the previous generated layout alongside owner configuration and source.
			for _, suffix := range []string{".go", "_test.go"} {
				if err := os.Rename(filepath.Join(root, "internal", "http", "openapi"+suffix), filepath.Join(root, "internal", "http", "swagger"+suffix)); err != nil {
					t.Fatal(err)
				}
			}
			serverPath := filepath.Join(root, "internal", "http", "server.go")
			server, err := os.ReadFile(serverPath)
			if err != nil {
				t.Fatal(err)
			}
			writeProjectRendererTestFile(t, serverPath, strings.ReplaceAll(string(server), "s.openapiRoutes()", "s.swaggerRoutes()"))
			environmentPath := filepath.Join(root, ".env")
			environment, err := os.ReadFile(environmentPath)
			if err != nil {
				t.Fatal(err)
			}
			ownerFiles := map[string]string{
				".env":                   strings.ReplaceAll(string(environment), "API_OPENAPI_ENABLED=true\n", "") + "\n# Keep API documentation private\nexport " + legacyKey + " = 'false'\nBILLING_" + legacyKey + "=false\n",
				".env.host":              "# Host-specific settings\n" + legacyKey + "=false\n",
				"app/http/owner_docs.go": "package http\n\n// ownerDocsURL is an application-owned documentation bookmark.\nconst ownerDocsURL = \"/swagger\"\n",
			}
			for path, contents := range ownerFiles {
				writeProjectRendererTestFile(t, filepath.Join(root, path), contents)
			}

			generated := map[string]string{}
			for attempt := range 2 {
				if err := renderer.Render(input); err != nil {
					t.Fatalf("upgrade render %d: %v", attempt+1, err)
				}
				for _, suffix := range []string{".go", "_test.go"} {
					assertProjectRendererTestFileMissing(t, filepath.Join(root, "internal", "http", "swagger"+suffix))
				}
				for _, path := range []string{"internal/http/openapi.go", "internal/http/openapi_test.go", "internal/http/server.go"} {
					contents, err := os.ReadFile(filepath.Join(root, path))
					if err != nil {
						t.Fatal(err)
					}
					if attempt == 0 {
						generated[path] = string(contents)
					} else if string(contents) != generated[path] {
						t.Fatalf("second upgrade changed %s", path)
					}
				}
				for path, want := range ownerFiles {
					got, err := os.ReadFile(filepath.Join(root, path))
					if err != nil {
						t.Fatal(err)
					}
					if string(got) != want {
						t.Fatalf("upgrade changed owner file %s", path)
					}
				}
				assertProjectRendererFileContains(t, serverPath, "s.openapiRoutes()")
				assertProjectRendererFileNotContains(t, serverPath, "s.swaggerRoutes()")
				assertProjectRendererFileContains(t, filepath.Join(root, "internal", "http", "openapi.go"), `"/openapi"`, `"/swagger"`, `"API_OPENAPI_ENABLED"`, `"API_SWAGGER_ENABLED"`, `"SWAGGER_ENABLED"`)
				assertProjectRendererFileNotContains(t, environmentPath, "API_OPENAPI_ENABLED=")
				assertProjectRendererFileContains(t, filepath.Join(root, ".env.example"), legacyKey)
			}
		})
	}
}

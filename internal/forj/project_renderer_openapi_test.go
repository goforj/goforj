package forj

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// openAPIUpgradeFixture retains unrelated generated and owner files so a broader render cannot pass unnoticed.
func openAPIUpgradeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	config := &project.Config{
		ProjectName: "Scoped upgrade", GoModuleName: "example.com/scoped",
		Render: project.RenderConfig{Components: project.Components{CLI: true, WebAPI: true}},
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		".goforj.yml":                   string(data),
		"go.mod":                        "module example.com/scoped\n\ngo 1.26.0\n",
		"go.sum":                        "owner dependency pins\n",
		".env":                          "API_SWAGGER_ENABLED=false\nBILLING_API_SWAGGER_ENABLED=false\n",
		".env.host":                     "API_SWAGGER_ENABLED=false\n",
		".env.example":                  "API_SWAGGER_ENABLED=false\n",
		".env.testing":                  "# Existing unmanaged testing profile\nAPI_SWAGGER_ENABLED=false\n",
		".gitignore":                    ".env.testing\n",
		"app/http/custom.go":            "package http\n\nconst ownerDocsURL = \"/swagger\"\n",
		"wire/wire_gen.go":              "package wire\n",
		"internal/http/runtime.go":      "package http\n// Existing runtime implementation.\n",
		"internal/http/README.md":       "Owner documentation\n",
		"internal/http/swagger.go":      "package http\nfunc (s *Server) swaggerRoutes() {}\n",
		"internal/http/swagger_test.go": "package http\n",
		"internal/http/server.go":       "package http\n\ntype Server struct{}\n\nfunc (s *Server) frameworkRouteGroups() {\n\t// Keep the old s.swaggerRoutes() reference in this comment.\n\t_ = \"s.swaggerRoutes()\"\n\ts.swaggerRoutes()\n}\n",
	}
	for path, contents := range files {
		writeProjectRendererTestFile(t, filepath.Join(root, path), contents)
	}
	return root
}

// openAPIUpgradeSnapshot records regular files without following symlinks used by boundary tests.
func openAPIUpgradeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(relative)] = string(data)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

// TestRenderOnlyOpenAPIChangesOnlyDocumentation verifies command parsing, exact file scope, and repeatability without full-render dependencies.
func TestRenderOnlyOpenAPIChangesOnlyDocumentation(t *testing.T) {
	root := openAPIUpgradeFixture(t)
	t.Chdir(root)
	before := openAPIUpgradeSnapshot(t, root)
	serverPath := filepath.Join(root, "internal/http/server.go")
	if err := os.Chmod(serverPath, 0o600); err != nil {
		t.Fatal(err)
	}
	command := RootCmd{RenderCmd: *NewCmd(logger.NewSilentLogger(), unitProjectRenderer(t))}
	cli, err := kong.New(&command)
	if err != nil {
		t.Fatal(err)
	}
	context, err := cli.Parse([]string{"render", "--only", "openapi"})
	if err != nil {
		t.Fatal(err)
	}
	if err := context.Run(); err != nil {
		t.Fatal(err)
	}
	after := openAPIUpgradeSnapshot(t, root)
	allowed := map[string]bool{
		"internal/http/server.go": true, "internal/http/openapi.go": true, "internal/http/openapi_test.go": true,
		"internal/http/swagger.go": true, "internal/http/swagger_test.go": true,
	}
	for path, contents := range before {
		if !allowed[path] && after[path] != contents {
			t.Fatalf("scoped render changed %s", path)
		}
	}
	for path := range after {
		if _, existed := before[path]; !existed && !allowed[path] {
			t.Fatalf("scoped render created %s", path)
		}
	}
	for _, path := range []string{"internal/http/swagger.go", "internal/http/swagger_test.go"} {
		if _, exists := after[path]; exists {
			t.Fatalf("legacy file remains: %s", path)
		}
	}
	wantServer := strings.Replace(before["internal/http/server.go"], "\ts.swaggerRoutes()\n", "\ts.openapiRoutes()\n", 1)
	if after["internal/http/server.go"] != wantServer {
		t.Fatal("server changes exceeded the route call")
	}
	assertProjectRendererFileContains(t, filepath.Join(root, "internal/http/openapi.go"), `"/openapi"`, `"/swagger"`, `"API_OPENAPI_ENABLED"`, `"API_SWAGGER_ENABLED"`)
	assertProjectRendererFileContains(t, filepath.Join(root, "internal/http/openapi_test.go"), "func TestOpenAPIUIAndSpecRoutes(")
	info, err := os.Stat(serverPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 && os.PathSeparator != '\\' {
		t.Fatalf("server mode changed: %v", info.Mode())
	}
	if err := renderOpenAPI(root); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, openAPIUpgradeSnapshot(t, root)) {
		t.Fatal("second scoped render changed files")
	}
}

// TestRenderOnlyOpenAPIRejectsInvalidFlags prevents conflicting CLI selections from falling through to a full render.
func TestRenderOnlyOpenAPIRejectsInvalidFlags(t *testing.T) {
	for _, command := range []RenderCmd{
		{Only: "unknown"},
		{Only: "openapi", Components: []string{"web_api"}},
	} {
		if err := command.Run(); err == nil {
			t.Fatal("invalid flags were accepted")
		}
	}
}

// TestRenderOnlyOpenAPIPreflightPreservesFiles rejects unsupported projects before publishing any documentation files.
func TestRenderOnlyOpenAPIPreflightPreservesFiles(t *testing.T) {
	for _, test := range []struct {
		name, path, contents string
		remove               bool
	}{
		{name: "missing configuration", path: ".goforj.yml", remove: true},
		{name: "invalid configuration", path: ".goforj.yml", contents: "[invalid"},
		{name: "no HTTP App", path: ".goforj.yml", contents: "module_name: example.com/scoped\nrender:\n  components:\n    cli: true\n"},
		{name: "no module", path: ".goforj.yml", contents: "render:\n  components:\n    web_api: true\n"},
		{name: "missing server", path: "internal/http/server.go", remove: true},
		{name: "invalid server", path: "internal/http/server.go", contents: "not Go"},
		{name: "unrecognized server", path: "internal/http/server.go", contents: "package http\ntype Server struct{}\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := openAPIUpgradeFixture(t)
			if test.remove {
				if err := os.Remove(filepath.Join(root, test.path)); err != nil {
					t.Fatal(err)
				}
			} else {
				writeProjectRendererTestFile(t, filepath.Join(root, test.path), test.contents)
			}
			before := openAPIUpgradeSnapshot(t, root)
			if err := renderOpenAPI(root); err == nil {
				t.Fatal("unsupported upgrade succeeded")
			}
			if !reflect.DeepEqual(before, openAPIUpgradeSnapshot(t, root)) {
				t.Fatal("failed preflight changed files")
			}
		})
	}
}

// TestRenderOnlyOpenAPIRejectsSpecialPaths prevents redirects through generated paths before existing files can be removed.
func TestRenderOnlyOpenAPIRejectsSpecialPaths(t *testing.T) {
	for _, path := range []string{"internal", "internal/http", "internal/http/server.go", "internal/http/swagger.go", "internal/http/swagger_test.go", "internal/http/openapi.go", "internal/http/openapi_test.go"} {
		t.Run(path, func(t *testing.T) {
			root := openAPIUpgradeFixture(t)
			fullPath := filepath.Join(root, path)
			if err := os.RemoveAll(fullPath); err != nil {
				t.Fatal(err)
			}
			if path == "internal" || path == "internal/http" {
				writeProjectRendererTestFile(t, fullPath, "not a directory\n")
			} else if err := os.MkdirAll(fullPath, 0o755); err != nil {
				t.Fatal(err)
			}
			before := openAPIUpgradeSnapshot(t, root)
			if err := renderOpenAPI(root); err == nil {
				t.Fatal("special path was accepted")
			}
			if !reflect.DeepEqual(before, openAPIUpgradeSnapshot(t, root)) {
				t.Fatal("failed preflight changed files")
			}
		})
	}
	t.Run("symlinked HTTP directory", func(t *testing.T) {
		root := openAPIUpgradeFixture(t)
		if err := os.Rename(filepath.Join(root, "internal/http"), filepath.Join(root, "original-http")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(root, "original-http"), filepath.Join(root, "internal/http")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		before := openAPIUpgradeSnapshot(t, root)
		if err := renderOpenAPI(root); err == nil {
			t.Fatal("symlinked HTTP directory was accepted")
		}
		if !reflect.DeepEqual(before, openAPIUpgradeSnapshot(t, root)) {
			t.Fatal("symlink rejection changed files")
		}
	})
}

// TestPatchOpenAPIRouteRegistrationRecognizesOnlyServerCalls guards against text replacements inside unrelated source.
func TestPatchOpenAPIRouteRegistrationRecognizesOnlyServerCalls(t *testing.T) {
	for _, body := range []string{
		"func (s *Server) f() { s.swaggerRoutes(); s.openapiRoutes() }",
		"func (s *Other) f() { s.swaggerRoutes() }",
		"func f() { s.swaggerRoutes() }",
		"func (*Server) f()",
		"func (s Server) f() { s.swaggerRoutes() }",
		"func (s *Server) f() { other.swaggerRoutes() }",
		"func (s *Server) f() { s.swaggerRoutes(1) }",
	} {
		if _, err := patchOpenAPIRouteRegistration([]byte("package http\n" + body)); err == nil {
			t.Fatalf("unrecognized registration accepted: %s", body)
		}
	}
}

// TestRenderOnlyOpenAPIUsesSharedCapabilities supports an API enabled only on an additional App without reconciling its resource plan.
func TestRenderOnlyOpenAPIUsesSharedCapabilities(t *testing.T) {
	root := openAPIUpgradeFixture(t)
	config := &project.Config{
		GoModuleName: "example.com/scoped",
		Render:       project.RenderConfig{Components: project.Components{CLI: true, Metrics: true}},
		Apps: map[string]project.AppConfig{
			"billing": {Components: project.Components{CLI: true, WebAPI: true, Storage: true}},
		},
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	writeProjectRendererTestFile(t, filepath.Join(root, ".goforj.yml"), string(data))
	if err := renderOpenAPI(root); err != nil {
		t.Fatal(err)
	}
	assertProjectRendererFileContains(t, filepath.Join(root, "internal/http/openapi_test.go"), `"example.com/scoped/internal/metrics"`, "metrics.NewManager()", "newTestMaintenanceState(t), nil, nil")
}

// TestRenderOnlyOpenAPISupportsWebUI retains the same documentation surface that full rendering gives browser-only Apps.
func TestRenderOnlyOpenAPISupportsWebUI(t *testing.T) {
	root := openAPIUpgradeFixture(t)
	config := &project.Config{GoModuleName: "example.com/scoped", Render: project.RenderConfig{Components: project.Components{CLI: true, WebUI: true}}}
	data, err := yaml.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	writeProjectRendererTestFile(t, filepath.Join(root, ".goforj.yml"), string(data))
	if err := renderOpenAPI(root); err != nil {
		t.Fatal(err)
	}
	assertProjectRendererFileContains(t, filepath.Join(root, "internal/http/openapi.go"), `"/openapi"`)
}

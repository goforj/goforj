package apiindex

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/web/webindex"
)

// TestOpenAPIOptionsUsesProjectAppMetadataAndGeneratedAuth verifies GoForj supplies identity and exact cookie policy.
func TestOpenAPIOptionsUsesProjectAppMetadataAndGeneratedAuth(t *testing.T) {
	root := t.TempDir()
	writeOpenAPIFile(t, filepath.Join(root, ".goforj.yml"), `project_name: Commerce
module_name: example.com/commerce
render:
  components:
    web_api: true
    auth: true
    database_mysql: true
`)
	composition := filepath.Join(root, "app", "routes.go")
	writeOpenAPIFile(t, composition, `package app
import (
	"github.com/goforj/web"
	"example.com/commerce/internal/auth"
)
func ProvideRoutes(authController *auth.Controller, authService *auth.Service) []web.RouteGroup {
	publicRoutes := authController.Routes()
	var groups []web.RouteGroup
	if len(publicRoutes) > 0 {
		groups = append(groups, web.NewRouteGroup("/api/v1", publicRoutes, authService.RequireAuth))
	}
	return groups
}
`)
	writeOpenAPIFile(t, filepath.Join(root, "internal", "auth", "controller.go"), `package auth
import "github.com/goforj/web"
type Controller struct{ auth *Service }
func (c *Controller) Routes() []web.Route {
	return []web.Route{web.NewRoute("GET", "/me", c.Me, c.auth.RequireAuth)}
}
func (*Controller) Me(web.Context) error { return nil }
`)
	writeOpenAPIFile(t, filepath.Join(root, "internal", "auth", "service.go"), "package auth\nimport (\n\t\"net/http\"\n\t\"github.com/goforj/web\"\n)\n"+generatedAuthServiceDeclarations())

	options, err := openAPIOptions(paths{root: root, appName: "app", routeComposition: composition}, nil)
	if err != nil {
		t.Fatalf("resolve default App OpenAPI options: %v", err)
	}
	if options.Info.Title != "Commerce" || options.Info.Version != "1.0.0" || options.Info.Description == "" {
		t.Fatalf("unexpected OpenAPI info: %#v", options.Info)
	}
	if access := options.SecuritySchemes[authAccessSecurityScheme]; access.Type != "apiKey" || access.In != "cookie" || access.Name != "auth_access" {
		t.Fatalf("unexpected access-cookie scheme: %#v", access)
	}
	if refresh := options.SecuritySchemes[authRefreshSecurityScheme]; refresh.In != "cookie" || refresh.Name != "auth_refresh" {
		t.Fatalf("unexpected refresh-cookie scheme: %#v", refresh)
	}
	wantRequirement := []webindex.OpenAPISecurityRequirement{
		{authAccessSecurityScheme: []string{}},
		{authRefreshSecurityScheme: []string{}},
	}
	if len(options.MiddlewareSecurity) != 0 {
		t.Fatalf("generated auth retained global textual mappings: %#v", options.MiddlewareSecurity)
	}
	if len(options.MiddlewareSecurityRules) != 2 {
		t.Fatalf("generated auth scoped rules = %#v, want group and controller rules", options.MiddlewareSecurityRules)
	}
	wantRules := []webindex.OpenAPIMiddlewareSecurityRule{
		{Expression: "authService.RequireAuth", SourceFile: "app/routes.go", Function: "ProvideRoutes", Requirements: wantRequirement},
		{Expression: "c.auth.RequireAuth", SourceFile: "internal/auth/controller.go", Function: "Routes", Receiver: "Controller", Requirements: wantRequirement},
	}
	if !reflect.DeepEqual(options.MiddlewareSecurityRules, wantRules) {
		t.Fatalf("generated auth scoped rules = %#v, want %#v", options.MiddlewareSecurityRules, wantRules)
	}
	if len(options.Operations) != 0 {
		t.Fatalf("non-refresh generated routes received operation overrides: %#v", options.Operations)
	}
}

// TestOpenAPIOptionsRejectsSameNamedCustomAuthMiddleware proves textual similarity cannot advertise generated cookie authentication.
func TestOpenAPIOptionsRejectsSameNamedCustomAuthMiddleware(t *testing.T) {
	tests := map[string]string{
		"custom parameter type": `package app
import (
	"github.com/goforj/web"
	customauth "example.com/security/customauth"
)
func ProvideRoutes(authService *customauth.Service) []web.RouteGroup {
	return []web.RouteGroup{web.NewRouteGroup("/api", []web.Route{}, authService.RequireAuth)}
}
`,
		"nested shadow": `package app
import (
	"github.com/goforj/web"
	"example.com/security/internal/auth"
	customauth "example.com/security/customauth"
)
func ProvideRoutes(authService *auth.Service, custom *customauth.Service) []web.RouteGroup {
	var groups []web.RouteGroup
	if enabled {
		authService := custom
		groups = append(groups, web.NewRouteGroup("/api", []web.Route{}, authService.RequireAuth))
	}
	return groups
}
`,
	}

	for name, compositionSource := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeOpenAPIFile(t, filepath.Join(root, ".goforj.yml"), `project_name: Security
module_name: example.com/security
render:
  components:
    web_api: true
    auth: true
    database_mysql: true
`)
			composition := filepath.Join(root, "app", "routes.go")
			writeOpenAPIFile(t, composition, compositionSource)
			writeOpenAPIFile(t, filepath.Join(root, "internal", "auth", "controller.go"), "package auth\nimport (\n\t\"net/http\"\n\t\"github.com/goforj/web\"\n)\n"+generatedAuthServiceDeclarations()+"type Controller struct{ auth *Service }\n")

			options, err := openAPIOptions(paths{root: root, appName: "app", routeComposition: composition}, nil)
			if err != nil {
				t.Fatalf("resolve custom-auth OpenAPI options: %v", err)
			}
			if len(options.MiddlewareSecurity) != 0 || len(options.MiddlewareSecurityRules) != 0 || len(options.SecuritySchemes) != 0 {
				t.Fatalf("custom middleware inherited generated cookie auth: mappings=%#v rules=%#v schemes=%#v", options.MiddlewareSecurity, options.MiddlewareSecurityRules, options.SecuritySchemes)
			}
		})
	}
}

// TestOpenAPIOptionsIgnoresExcludedAuthProof prevents inactive source from authorizing security for an active custom controller.
func TestOpenAPIOptionsIgnoresExcludedAuthProof(t *testing.T) {
	tests := map[string]struct {
		name   string
		source string
	}{
		"ignored filename": {
			name:   "_generated_proof.go",
			source: "package auth\nimport (\n\t\"net/http\"\n\t\"github.com/goforj/web\"\n)\n" + generatedAuthServiceDeclarations() + "type Controller struct{ auth *Service }\n",
		},
		"inactive build constraint": {
			name: "generated_proof.go",
			source: `//go:build goforj_auth_policy_proof_never

package auth
import (
	"net/http"
	"github.com/goforj/web"
)
` + generatedAuthServiceDeclarations() + `type Controller struct{ auth *Service }
`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeOpenAPIFile(t, filepath.Join(root, ".goforj.yml"), `project_name: Security
module_name: example.com/security
render:
  components:
    web_api: true
    auth: true
    database_mysql: true
`)
			composition := filepath.Join(root, "app", "routes.go")
			writeOpenAPIFile(t, composition, `package app
import (
	"github.com/goforj/web"
	"example.com/security/internal/auth"
)
func ProvideRoutes(authController *auth.Controller) []web.RouteGroup {
	return []web.RouteGroup{web.NewRouteGroup("/api", authController.Routes())}
}
`)
			writeOpenAPIFile(t, filepath.Join(root, "internal", "auth", "controller.go"), `package auth
import "github.com/goforj/web"
type CustomService struct{}
func (*CustomService) RequireAuth(next web.Handler) web.Handler { return next }
type Controller struct{ auth *CustomService }
func (c *Controller) Routes() []web.Route {
	return []web.Route{web.NewRoute("GET", "/me", c.Me, c.auth.RequireAuth)}
}
func (*Controller) Me(web.Context) error { return nil }
`)
			writeOpenAPIFile(t, filepath.Join(root, "internal", "auth", test.name), test.source)

			options, err := openAPIOptions(paths{root: root, appName: "app", routeComposition: composition}, nil)
			if err != nil {
				t.Fatalf("resolve excluded-proof OpenAPI options: %v", err)
			}
			if len(options.Operations) != 0 || len(options.MiddlewareSecurityRules) != 0 || len(options.SecuritySchemes) != 0 {
				t.Fatalf("excluded source authorized generated auth policy: operations=%#v rules=%#v schemes=%#v", options.Operations, options.MiddlewareSecurityRules, options.SecuritySchemes)
			}
		})
	}
}

// TestOpenAPIOptionsRejectsModuleIdentityDrift prevents stale config from authorizing a different module's internal auth package.
func TestOpenAPIOptionsRejectsModuleIdentityDrift(t *testing.T) {
	root := t.TempDir()
	writeOpenAPIFile(t, filepath.Join(root, ".goforj.yml"), `project_name: Security
module_name: example.com/configured
render:
  components:
    web_api: true
    auth: true
    database_mysql: true
`)
	writeOpenAPIFile(t, filepath.Join(root, "go.mod"), "module example.com/active\n\ngo 1.25.0\n")
	composition := filepath.Join(root, "app", "routes.go")
	writeOpenAPIFile(t, composition, "package app\n")

	_, err := openAPIOptions(paths{root: root, appName: "app", routeComposition: composition}, nil)
	if err == nil || !strings.Contains(err.Error(), "does not match active go.mod module") {
		t.Fatalf("module identity drift error = %v", err)
	}
}

// TestOpenAPIOptionsRejectsLookalikeGeneratedAuthContracts ensures names alone cannot authorize generated cookie policy.
func TestOpenAPIOptionsRejectsLookalikeGeneratedAuthContracts(t *testing.T) {
	exact := generatedAuthServiceDeclarations()
	tests := map[string]string{
		"service alias": strings.Replace(exact, "type Service struct{}", "type ConcreteService struct{}\ntype Service = ConcreteService", 1),
		"no-op middleware": strings.Replace(exact, `func (s *Service) RequireAuth(next web.Handler) web.Handler {
	return func(r web.Context) error {
		_, err := s.AuthenticateRequest(r)
		if err != nil {
			return r.JSON(http.StatusUnauthorized, map[string]any{"ok": false})
		}
		return next(r)
	}
}`, `func (*Service) RequireAuth(next web.Handler) web.Handler { return next }`, 1),
		"wrong cookie contract": strings.Replace(exact, `AccessCookieName = "auth_access"`, `AccessCookieName = "custom_access"`, 1),
	}

	for name, serviceSource := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeOpenAPIFile(t, filepath.Join(root, ".goforj.yml"), `project_name: Security
module_name: example.com/security
render:
  components:
    web_api: true
    auth: true
    database_mysql: true
`)
			composition := filepath.Join(root, "app", "routes.go")
			writeOpenAPIFile(t, composition, `package app
import (
	"github.com/goforj/web"
	"example.com/security/internal/auth"
)
func ProvideRoutes(authController *auth.Controller, authService *auth.Service) []web.RouteGroup {
	return []web.RouteGroup{web.NewRouteGroup("/api", authController.Routes(), authService.RequireAuth)}
}
`)
			writeOpenAPIFile(t, filepath.Join(root, "internal", "auth", "controller.go"), `package auth
import "github.com/goforj/web"
type Controller struct{ auth *Service }
func (c *Controller) Routes() []web.Route {
	return []web.Route{web.NewRoute("GET", "/me", c.Me, c.auth.RequireAuth)}
}
func (*Controller) Me(web.Context) error { return nil }
`)
			writeOpenAPIFile(t, filepath.Join(root, "internal", "auth", "service.go"), "package auth\nimport (\n\t\"net/http\"\n\t\"github.com/goforj/web\"\n)\n"+serviceSource)

			options, err := openAPIOptions(paths{root: root, appName: "app", routeComposition: composition}, nil)
			if err != nil {
				t.Fatalf("resolve lookalike auth contract: %v", err)
			}
			if len(options.SecuritySchemes) != 0 || len(options.MiddlewareSecurityRules) != 0 || len(options.Operations) != 0 {
				t.Fatalf("lookalike auth contract authorized policy: schemes=%#v rules=%#v operations=%#v", options.SecuritySchemes, options.MiddlewareSecurityRules, options.Operations)
			}
		})
	}
}

// TestOpenAPIOptionsUsesSelectedBuildTagsForAuthProof keeps generated policy on the same active source set as compilation.
func TestOpenAPIOptionsUsesSelectedBuildTagsForAuthProof(t *testing.T) {
	root := t.TempDir()
	writeOpenAPIFile(t, filepath.Join(root, ".goforj.yml"), `project_name: Security
module_name: example.com/security
render:
  components:
    web_api: true
    auth: true
    database_mysql: true
`)
	composition := filepath.Join(root, "app", "routes.go")
	writeOpenAPIFile(t, composition, `package app
import (
	"github.com/goforj/web"
	"example.com/security/internal/auth"
)
func ProvideRoutes(authController *auth.Controller, authService *auth.Service) []web.RouteGroup {
	return []web.RouteGroup{web.NewRouteGroup("/api", authController.Routes(), authService.RequireAuth)}
}
`)
	writeOpenAPIFile(t, filepath.Join(root, "internal", "auth", "controller.go"), `package auth
import "github.com/goforj/web"
type Controller struct{ auth *Service }
func (c *Controller) Routes() []web.Route {
	return []web.Route{web.NewRoute("GET", "/me", c.Me, c.auth.RequireAuth)}
}
func (*Controller) Me(web.Context) error { return nil }
`)
	writeOpenAPIFile(t, filepath.Join(root, "internal", "auth", "service_secure.go"), "//go:build secure\n\npackage auth\nimport (\n\t\"net/http\"\n\t\"github.com/goforj/web\"\n)\n"+generatedAuthServiceDeclarations())

	withoutTags, err := openAPIOptions(paths{root: root, appName: "app", routeComposition: composition}, nil)
	if err != nil {
		t.Fatalf("resolve auth proof without build tag: %v", err)
	}
	if len(withoutTags.SecuritySchemes) != 0 || len(withoutTags.MiddlewareSecurityRules) != 0 {
		t.Fatalf("inactive tagged proof authorized policy: schemes=%#v rules=%#v", withoutTags.SecuritySchemes, withoutTags.MiddlewareSecurityRules)
	}

	withTags, err := openAPIOptions(paths{root: root, appName: "app", routeComposition: composition}, []string{"secure"})
	if err != nil {
		t.Fatalf("resolve auth proof with selected build tag: %v", err)
	}
	if len(withTags.SecuritySchemes) != 2 || len(withTags.MiddlewareSecurityRules) != 2 {
		t.Fatalf("selected tagged proof policy: schemes=%#v rules=%#v", withTags.SecuritySchemes, withTags.MiddlewareSecurityRules)
	}
}

// TestOpenAPIOptionsRequiresGeneratedRefreshControlFlow prevents dead service calls from authorizing refresh cookies.
func TestOpenAPIOptionsRequiresGeneratedRefreshControlFlow(t *testing.T) {
	tests := map[string]struct {
		body           string
		wantOperations int
	}{
		"generated flow": {
			body: `
	user, _, err := c.auth.RefreshSession(r)
	if err != nil {
		return r.JSON(http.StatusUnauthorized, map[string]any{"ok": false})
	}
	return r.JSON(http.StatusOK, map[string]any{"ok": true, "user": user})`,
			wantOperations: 1,
		},
		"dead branch call": {
			body: `
	if false {
		_, _, _ = c.auth.RefreshSession(r)
	}
	return r.JSON(http.StatusOK, map[string]any{"ok": true})`,
		},
		"ignored refresh error": {
			body: `
	user, _, _ := c.auth.RefreshSession(r)
	return r.JSON(http.StatusOK, map[string]any{"ok": true, "user": user})`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeOpenAPIFile(t, filepath.Join(root, ".goforj.yml"), `project_name: Security
module_name: example.com/security
render:
  components:
    web_api: true
    auth: true
    database_mysql: true
`)
			composition := filepath.Join(root, "app", "routes.go")
			writeOpenAPIFile(t, composition, `package app
import (
	"github.com/goforj/web"
	"example.com/security/internal/auth"
)
func ProvideRoutes(authController *auth.Controller) []web.RouteGroup {
	return []web.RouteGroup{web.NewRouteGroup("/api", authController.Routes())}
}
`)
			writeOpenAPIFile(t, filepath.Join(root, "internal", "auth", "controller.go"), `package auth
import (
	"net/http"
	"github.com/goforj/web"
)
type Controller struct{ auth *Service }
func (c *Controller) Routes() []web.Route {
	return []web.Route{web.NewRoute(http.MethodPost, "/auth/refresh", c.Refresh)}
}
func (c *Controller) Refresh(r web.Context) error {`+test.body+`
}
`)
			writeOpenAPIFile(t, filepath.Join(root, "internal", "auth", "service.go"), "package auth\nimport (\n\t\"net/http\"\n\t\"github.com/goforj/web\"\n)\n"+generatedAuthServiceDeclarations())

			options, err := openAPIOptions(paths{root: root, appName: "app", routeComposition: composition}, nil)
			if err != nil {
				t.Fatalf("resolve refresh auth policy: %v", err)
			}
			if len(options.Operations) != test.wantOperations {
				t.Fatalf("refresh operation overrides = %#v, want %d", options.Operations, test.wantOperations)
			}
			if test.wantOperations == 1 {
				want := []webindex.OpenAPISecurityRequirement{{authRefreshSecurityScheme: []string{}}}
				if options.Operations[0].Security == nil || !reflect.DeepEqual(options.Operations[0].Security.Requirements, want) {
					t.Fatalf("refresh security = %#v, want %#v", options.Operations[0].Security, want)
				}
			}
		})
	}
}

// TestRunnerRejectsSameFileAuthReceiverLookalike ensures generated controller policy cannot move to another Routes method.
func TestRunnerRejectsSameFileAuthReceiverLookalike(t *testing.T) {
	root := t.TempDir()
	writeOpenAPIFile(t, filepath.Join(root, ".goforj.yml"), `project_name: Security
module_name: example.com/security
render:
  components:
    web_api: true
    auth: true
    database_mysql: true
`)
	writeOpenAPIFile(t, filepath.Join(root, "go.mod"), "module example.com/security\n\ngo 1.25.0\n")
	composition := filepath.Join(root, "app", "routes.go")
	writeOpenAPIFile(t, composition, `package app
import (
	"github.com/goforj/web"
	"example.com/security/internal/auth"
)
func ProvideRoutes(authController *auth.Controller, otherController *auth.OtherController) []web.RouteGroup {
	routes := append(authController.Routes(), otherController.Routes()...)
	return []web.RouteGroup{web.NewRouteGroup("/api", routes)}
}
`)
	writeOpenAPIFile(t, filepath.Join(root, "internal", "auth", "controller.go"), `package auth
import (
	"net/http"
	"github.com/goforj/web"
)
type Controller struct{ auth *Service }
func (c *Controller) Routes() []web.Route {
	return []web.Route{web.NewRoute(http.MethodGet, "/me", c.Me)}
}
func (*Controller) Me(r web.Context) error { return r.NoContent(http.StatusNoContent) }
type OtherController struct{ auth *Service }
func (c *OtherController) Routes() []web.Route {
	return []web.Route{web.NewRoute(http.MethodGet, "/other", c.Other, c.auth.RequireAuth)}
}
func (*OtherController) Other(r web.Context) error { return r.NoContent(http.StatusNoContent) }
`)
	writeOpenAPIFile(t, filepath.Join(root, "internal", "auth", "service.go"), "package auth\nimport (\n\t\"net/http\"\n\t\"github.com/goforj/web\"\n)\n"+generatedAuthServiceDeclarations())
	paths := paths{
		root:             root,
		appName:          "app",
		out:              filepath.Join(root, "build", "api_index.json"),
		diagnostics:      filepath.Join(root, "build", "api_index.diagnostics.json"),
		openAPI:          filepath.Join(root, "build", "openapi.json"),
		routeComposition: composition,
	}

	_, err := newTestRunner().runIndex(paths, runOptions{})
	if err == nil || !strings.Contains(err.Error(), "did not match middleware") {
		t.Fatalf("same-file receiver lookalike projection error = %v", err)
	}
	if _, statErr := os.Stat(paths.openAPI); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("unsafe receiver lookalike published OpenAPI: %v", statErr)
	}
}

// TestOpenAPIOptionsRejectsMutableAuthControllerRoutes ensures changed route slices cannot retain stale generated security evidence.
func TestOpenAPIOptionsRejectsMutableAuthControllerRoutes(t *testing.T) {
	tests := map[string]string{
		"branch replacement": `
	routes := []web.Route{web.NewRoute("GET", "/me", c.Me, c.auth.RequireAuth)}
	if public {
		routes = []web.Route{web.NewRoute("GET", "/me", c.Me)}
	}
	return routes`,
		"opaque escape": `
	routes := []web.Route{web.NewRoute("GET", "/me", c.Me, c.auth.RequireAuth)}
	mutate(routes)
	return routes`,
		"indexed mutation": `
	routes := []web.Route{web.NewRoute("GET", "/me", c.Me, c.auth.RequireAuth)}
	routes[0] = web.NewRoute("GET", "/me", c.Me)
	return routes`,
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeOpenAPIFile(t, filepath.Join(root, ".goforj.yml"), `project_name: Security
module_name: example.com/security
render:
  components:
    web_api: true
    auth: true
    database_mysql: true
`)
			composition := filepath.Join(root, "app", "routes.go")
			writeOpenAPIFile(t, composition, `package app
import (
	"github.com/goforj/web"
	"example.com/security/internal/auth"
)
func ProvideRoutes(authController *auth.Controller) []web.RouteGroup {
	return []web.RouteGroup{web.NewRouteGroup("/api", authController.Routes())}
}
`)
			writeOpenAPIFile(t, filepath.Join(root, "internal", "auth", "controller.go"), `package auth
import "github.com/goforj/web"
type Controller struct{ auth *Service }
func (c *Controller) Routes() []web.Route {`+body+`
}
func (*Controller) Me(web.Context) error { return nil }
`)
			writeOpenAPIFile(t, filepath.Join(root, "internal", "auth", "service.go"), "package auth\nimport (\n\t\"net/http\"\n\t\"github.com/goforj/web\"\n)\n"+generatedAuthServiceDeclarations())

			options, err := openAPIOptions(paths{root: root, appName: "app", routeComposition: composition}, nil)
			if err != nil {
				t.Fatalf("resolve mutable-controller OpenAPI options: %v", err)
			}
			if len(options.MiddlewareSecurity) != 0 || len(options.MiddlewareSecurityRules) != 1 || len(options.SecuritySchemes) != 2 {
				t.Fatalf("mutable routes did not retain only source-scoped policy evidence: mappings=%#v rules=%#v schemes=%#v", options.MiddlewareSecurity, options.MiddlewareSecurityRules, options.SecuritySchemes)
			}
			rule := options.MiddlewareSecurityRules[0]
			if rule.Expression != "c.auth.RequireAuth" || rule.SourceFile != "internal/auth/controller.go" || rule.Function != "Routes" || rule.Receiver != "Controller" {
				t.Fatalf("mutable route scoped policy = %#v", rule)
			}
		})
	}
}

// TestOpenAPIOptionsUsesNamedAppComponents verifies project-level Auth does not leak into a CLI/WebAPI App that did not select it.
func TestOpenAPIOptionsUsesNamedAppComponents(t *testing.T) {
	root := t.TempDir()
	writeOpenAPIFile(t, filepath.Join(root, ".goforj.yml"), `project_name: Commerce
module_name: example.com/commerce
render:
  components:
    web_api: true
    auth: true
    database_mysql: true
apps:
  reporting:
    components:
      web_api: true
`)
	composition := filepath.Join(root, "app", "reporting", "routes.go")
	writeOpenAPIFile(t, composition, "package reportingapp\n")

	options, err := openAPIOptions(paths{root: root, appName: "reporting", routeComposition: composition}, nil)
	if err != nil {
		t.Fatalf("resolve named App OpenAPI options: %v", err)
	}
	if options.Info.Title != "Commerce / reporting" {
		t.Fatalf("named App title = %q", options.Info.Title)
	}
	if len(options.SecuritySchemes) != 0 || len(options.MiddlewareSecurity) != 0 || len(options.MiddlewareSecurityRules) != 0 {
		t.Fatalf("project auth leaked into named App policy: schemes=%#v mappings=%#v rules=%#v", options.SecuritySchemes, options.MiddlewareSecurity, options.MiddlewareSecurityRules)
	}
}

// TestOpenAPIOptionsWithoutConfigPreservesWebFallback verifies direct runner fixtures retain webindex metadata discovery.
func TestOpenAPIOptionsWithoutConfigPreservesWebFallback(t *testing.T) {
	options, err := openAPIOptions(paths{root: t.TempDir(), appName: "app"}, nil)
	if err != nil {
		t.Fatalf("resolve config-free OpenAPI options: %v", err)
	}
	if !reflect.DeepEqual(options, webindex.OpenAPIOptions{}) {
		t.Fatalf("config-free options = %#v, want zero value", options)
	}
}

// TestSourceFunctionReturnsExpressionIgnoresDeadAndNonCodeText prevents stale evidence from enabling an unused security mapping.
func TestSourceFunctionReturnsExpressionIgnoresDeadAndNonCodeText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.go")
	writeOpenAPIFile(t, path, `package app
// authService.RequireAuth was removed.
const note = `+"`"+`authService.RequireAuth`+"`"+`
func ProvideRoutes() []any {
	unused := authService.RequireAuth
	_ = unused
	return []any{}
}
`)
	present, err := sourceFunctionReturnsExpression(path, "ProvideRoutes", "authService.RequireAuth")
	if err != nil {
		t.Fatalf("inspect decoy middleware source: %v", err)
	}
	if present {
		t.Fatal("dead call, comment, or string enabled an unused security mapping")
	}
}

// TestSourceFunctionReturnsExpressionIgnoresConditionalShadow verifies a branch-local alias cannot mark an unrelated returned group as protected.
func TestSourceFunctionReturnsExpressionIgnoresConditionalShadow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.go")
	writeOpenAPIFile(t, path, `package app
func ProvideRoutes() []any {
	var groups []any
	if enabled {
		groups := []any{authService.RequireAuth}
		_ = groups
	}
	return groups
}
`)
	present, err := sourceFunctionReturnsExpression(path, "ProvideRoutes", "authService.RequireAuth")
	if err != nil {
		t.Fatalf("inspect shadowed middleware source: %v", err)
	}
	if present {
		t.Fatal("conditional shadow enabled an unused security mapping")
	}
}

// TestOpenAPIOptionsTemplHTMXOmitsUnselectedControllerMiddleware verifies generated but unreturned auth routes do not reject projection.
func TestOpenAPIOptionsTemplHTMXOmitsUnselectedControllerMiddleware(t *testing.T) {
	root := t.TempDir()
	writeOpenAPIFile(t, filepath.Join(root, ".goforj.yml"), `project_name: Portal
module_name: example.com/portal
render:
  starter_kit: templ_htmx
  components:
    web_api: true
    web_ui: true
    auth: true
    database_mysql: true
`)
	composition := filepath.Join(root, "app", "routes.go")
	writeOpenAPIFile(t, composition, `package app
import (
	"github.com/goforj/web"
	"example.com/portal/internal/auth"
	"example.com/portal/internal/hello"
)
func ProvideRoutes(helloController *hello.Controller, authService *auth.Service) []web.RouteGroup {
	deadAuthRoutes := authController.Routes()
	_ = deadAuthRoutes
	protectedRoutes := helloController.Routes()
	return []web.RouteGroup{web.NewRouteGroup("/api/v1", protectedRoutes, authService.RequireAuth)}
}
`)
	writeOpenAPIFile(t, filepath.Join(root, "internal", "auth", "controller.go"), `package auth
import (
	"net/http"
	"github.com/goforj/web"
)
`+generatedAuthServiceDeclarations()+`type Controller struct{ auth *Service }
func (c *Controller) Routes() []web.Route {
	return []web.Route{web.NewRoute("GET", "/me", c.Me, c.auth.RequireAuth)}
}
func (*Controller) Me(web.Context) error { return nil }
`)

	options, err := openAPIOptions(paths{root: root, appName: "app", routeComposition: composition}, nil)
	if err != nil {
		t.Fatalf("resolve templ_htmx OpenAPI options: %v", err)
	}
	if len(options.MiddlewareSecurity) != 0 {
		t.Fatalf("templ auth retained global textual mappings: %#v", options.MiddlewareSecurity)
	}
	if len(options.MiddlewareSecurityRules) != 1 {
		t.Fatalf("selected group middleware rules = %#v, want one", options.MiddlewareSecurityRules)
	}
	rule := options.MiddlewareSecurityRules[0]
	if rule.Expression != "authService.RequireAuth" || rule.SourceFile != "app/routes.go" || rule.Function != "ProvideRoutes" {
		t.Fatalf("selected group middleware rule = %#v", rule)
	}
	if len(options.SecuritySchemes) != 2 {
		t.Fatalf("selected group middleware schemes = %#v, want two generated cookie schemes", options.SecuritySchemes)
	}

	writeOpenAPIFile(t, filepath.Join(root, "go.mod"), "module example.com/portal\n\ngo 1.25.0\n")
	writeOpenAPIFile(t, filepath.Join(root, "internal", "hello", "controller.go"), `package hello
import (
	"net/http"
	"github.com/goforj/web"
)
// Controller owns the protected templ fixture route.
type Controller struct{}
// Routes returns the route selected by the active templ composition.
func (*Controller) Routes() []web.Route {
	return []web.Route{web.NewRoute(http.MethodGet, "/hello", hello)}
}
// hello returns the status used to keep strict handler inference complete.
func hello(ctx web.Context) error { return ctx.NoContent(http.StatusNoContent) }
`)
	paths := paths{
		root:             root,
		appName:          "app",
		out:              filepath.Join(root, "build", "api_index.json"),
		diagnostics:      filepath.Join(root, "build", "api_index.diagnostics.json"),
		openAPI:          filepath.Join(root, "build", "openapi.json"),
		routeComposition: composition,
	}
	if _, err := newTestRunner().runIndex(paths, runOptions{strict: true}); err != nil {
		t.Fatalf("index templ_htmx App without exposed auth controller routes: %v", err)
	}
	documentBytes, err := os.ReadFile(paths.openAPI)
	if err != nil {
		t.Fatalf("read templ_htmx OpenAPI document: %v", err)
	}
	var document struct {
		Paths map[string]map[string]struct {
			Security []webindex.OpenAPISecurityRequirement `json:"security"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(documentBytes, &document); err != nil {
		t.Fatalf("decode templ_htmx OpenAPI document: %v", err)
	}
	if _, exists := document.Paths["/api/v1/auth/me"]; exists {
		t.Fatalf("unselected auth controller route leaked into templ_htmx OpenAPI: %#v", document.Paths)
	}
	if got := document.Paths["/api/v1/hello"]["get"].Security; len(got) != 2 {
		t.Fatalf("templ_htmx protected route security = %#v, want generated cookie alternatives", got)
	}
}

// TestOpenAPIOptionsOmitsUnusedGeneratedAuthSchemes verifies Auth capability alone does not add an unused OpenAPI component.
func TestOpenAPIOptionsOmitsUnusedGeneratedAuthSchemes(t *testing.T) {
	root := t.TempDir()
	writeOpenAPIFile(t, filepath.Join(root, ".goforj.yml"), `project_name: Worker API
render:
  components:
    web_api: true
    auth: true
    database_mysql: true
`)
	composition := filepath.Join(root, "app", "routes.go")
	writeOpenAPIFile(t, composition, "package app\nfunc ProvideRoutes() []any { return []any{} }\n")
	writeOpenAPIFile(t, filepath.Join(root, "internal", "auth", "controller.go"), "package auth\n")

	options, err := openAPIOptions(paths{root: root, appName: "app", routeComposition: composition}, nil)
	if err != nil {
		t.Fatalf("resolve unused Auth OpenAPI options: %v", err)
	}
	if len(options.SecuritySchemes) != 0 || len(options.MiddlewareSecurity) != 0 || len(options.MiddlewareSecurityRules) != 0 || len(options.Operations) != 0 {
		t.Fatalf("unused generated Auth policy remained: schemes=%#v mappings=%#v rules=%#v operations=%#v", options.SecuritySchemes, options.MiddlewareSecurity, options.MiddlewareSecurityRules, options.Operations)
	}
}

// TestRunnerProjectsAppMetadataAndAuth verifies GoForj options survive the complete webindex publication path.
func TestRunnerProjectsAppMetadataAndAuth(t *testing.T) {
	root := t.TempDir()
	writeOpenAPIFile(t, filepath.Join(root, ".goforj.yml"), `project_name: Commerce
module_name: example.com/commerce
render:
  components:
    web_api: true
    auth: true
    database_mysql: true
`)
	writeOpenAPIFile(t, filepath.Join(root, "go.mod"), "module example.com/commerce\n\ngo 1.25.0\n")
	writeOpenAPIFile(t, filepath.Join(root, "internal", "auth", "controller.go"), `package auth
import (
	"net/http"
	"github.com/goforj/web"
)
`+generatedAuthServiceDeclarations()+`
// Controller owns generated session routes.
type Controller struct { auth *Service }
// Routes returns the generated session routes.
func (c *Controller) Routes() []web.Route {
	return []web.Route{
		web.NewRoute(http.MethodPost, "/auth/refresh", c.Refresh),
		web.NewRoute(http.MethodGet, "/auth/me", c.Me, c.auth.RequireAuth),
	}
}
// Refresh rotates the refresh-backed session.
func (c *Controller) Refresh(ctx web.Context) error {
	user, _, err := c.auth.RefreshSession(ctx)
	if err != nil {
		return ctx.JSON(http.StatusUnauthorized, map[string]any{"ok": false})
	}
	return ctx.JSON(http.StatusOK, map[string]any{"ok": true, "user": user})
}
// Me returns the current session without inventing a response contract.
func (*Controller) Me(ctx web.Context) error { return ctx.NoContent(http.StatusNoContent) }
`)
	writeOpenAPIFile(t, filepath.Join(root, "internal", "hello", "controller.go"), `package hello
import (
	"net/http"
	"github.com/goforj/web"
)
// Controller owns the protected hello route.
type Controller struct{}
// Routes returns the protected hello route.
func (*Controller) Routes() []web.Route {
	return []web.Route{web.NewRoute(http.MethodGet, "/hello", hello)}
}
// hello returns the protected fixture response.
func hello(ctx web.Context) error { return ctx.NoContent(http.StatusNoContent) }
`)
	writeOpenAPIFile(t, filepath.Join(root, "internal", "custom", "controller.go"), `package custom
import (
	"net/http"
	"github.com/goforj/web"
)
// Service owns a custom policy whose source spelling intentionally collides with generated auth.
type Service struct{}
// RequireAuth applies the custom policy without accepting GoForj auth cookies.
func (*Service) RequireAuth(next web.Handler) web.Handler { return next }
// Controller owns the custom collision route.
type Controller struct { auth *Service }
// Routes returns the custom route with the same receiver expression as generated auth.
func (c *Controller) Routes() []web.Route {
	return []web.Route{web.NewRoute(http.MethodGet, "/custom/me", c.Me, c.auth.RequireAuth)}
}
// Me returns the custom response.
func (*Controller) Me(ctx web.Context) error { return ctx.NoContent(http.StatusNoContent) }
// ProtectedRoutes returns a custom route whose parameter expression intentionally matches generated group auth spelling.
func ProtectedRoutes(authService *Service) []web.Route {
	return []web.Route{web.NewRoute(http.MethodGet, "/custom/provider", providerHandler, authService.RequireAuth)}
}
// providerHandler returns the custom provider response.
func providerHandler(ctx web.Context) error { return ctx.NoContent(http.StatusNoContent) }
`)
	composition := filepath.Join(root, "app", "routes.go")
	writeOpenAPIFile(t, composition, `package app
import (
	"slices"
	"github.com/goforj/web"
	"example.com/commerce/internal/auth"
	"example.com/commerce/internal/custom"
	"example.com/commerce/internal/hello"
)
// ProvideRoutes composes generated public and protected providers.
func ProvideRoutes(authController *auth.Controller, customController *custom.Controller, customService *custom.Service, helloController *hello.Controller, authService *auth.Service) []web.RouteGroup {
	publicRoutes := slices.Concat(authController.Routes(), customController.Routes(), custom.ProtectedRoutes(customService))
	protectedRoutes := slices.Concat(helloController.Routes())
	return []web.RouteGroup{
		web.NewRouteGroup("/api/v1", publicRoutes),
		web.NewRouteGroup("/api/v1", protectedRoutes, authService.RequireAuth),
	}
}
`)
	paths := paths{
		root:             root,
		appName:          "app",
		out:              filepath.Join(root, "build", "api_index.json"),
		diagnostics:      filepath.Join(root, "build", "api_index.diagnostics.json"),
		openAPI:          filepath.Join(root, "build", "openapi.json"),
		routeComposition: composition,
	}
	if _, err := newTestRunner().runIndex(paths, runOptions{}); err != nil {
		t.Fatalf("run App-scoped API index: %v", err)
	}

	content, err := os.ReadFile(paths.openAPI)
	if err != nil {
		t.Fatalf("read App OpenAPI: %v", err)
	}
	var document struct {
		Info struct {
			Title string `json:"title"`
		} `json:"info"`
		Paths map[string]map[string]struct {
			Security []webindex.OpenAPISecurityRequirement `json:"security"`
		} `json:"paths"`
		Components struct {
			SecuritySchemes map[string]webindex.OpenAPISecurityScheme `json:"securitySchemes"`
		} `json:"components"`
	}
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatalf("decode App OpenAPI: %v", err)
	}
	if document.Info.Title != "Commerce" || len(document.Components.SecuritySchemes) != 2 {
		t.Fatalf("unexpected App metadata/security schemes: title=%q schemes=%#v", document.Info.Title, document.Components.SecuritySchemes)
	}
	wantSecurity := []webindex.OpenAPISecurityRequirement{
		{authAccessSecurityScheme: []string{}},
		{authRefreshSecurityScheme: []string{}},
	}
	for _, path := range []string{"/api/v1/auth/me", "/api/v1/hello"} {
		if got := document.Paths[path]["get"].Security; !reflect.DeepEqual(got, wantSecurity) {
			t.Fatalf("security for %s = %#v, want %#v", path, got, wantSecurity)
		}
	}
	wantRefreshSecurity := []webindex.OpenAPISecurityRequirement{{authRefreshSecurityScheme: []string{}}}
	if got := document.Paths["/api/v1/auth/refresh"]["post"].Security; !reflect.DeepEqual(got, wantRefreshSecurity) {
		t.Fatalf("refresh security = %#v, want %#v", got, wantRefreshSecurity)
	}
	if got := document.Paths["/api/v1/custom/me"]["get"].Security; got != nil {
		t.Fatalf("same-spelled custom controller middleware inherited generated auth: %#v", got)
	}
	if got := document.Paths["/api/v1/custom/provider"]["get"].Security; got != nil {
		t.Fatalf("same-spelled custom provider middleware inherited generated group auth: %#v", got)
	}
}

// generatedAuthServiceDeclarations returns the exact generated policy evidence shared by auth projection fixtures.
func generatedAuthServiceDeclarations() string {
	return `const (
	AccessCookieName = "auth_access"
	RefreshCookieName = "auth_refresh"
)
type Service struct{}
func (s *Service) AuthenticateRequest(web.Context) (any, error) { return nil, nil }
func (s *Service) RefreshSession(web.Context) (any, any, error) { return nil, nil, nil }
func (s *Service) RequireAuth(next web.Handler) web.Handler {
	return func(r web.Context) error {
		_, err := s.AuthenticateRequest(r)
		if err != nil {
			return r.JSON(http.StatusUnauthorized, map[string]any{"ok": false})
		}
		return next(r)
	}
}
`
}

// writeOpenAPIFile creates source/config parents so each metadata test can describe only relevant evidence.
func writeOpenAPIFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create OpenAPI fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write OpenAPI fixture: %v", err)
	}
}

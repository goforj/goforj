package apix

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRunMergesGroupAndRouteMiddlewares(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.24\n",
		"internal/hello/controller.go": `package hello
import "net/http"
type Controller struct{}
func (c *Controller) Routes() []any {
	return []any{
		http.NewRoute(http.MethodGet, "/m", c.Index, middleware.Gzip(), trace),
	}
}
func (c *Controller) Index(ctx any) error { return nil }`,
		"internal/router/routes_registry.go": `package router
func ProvideAppRoutes(helloController *hello.Controller) *AppRoutes {
	var app []any
	app = append(app, helloController.Routes()...)
	return &AppRoutes{app: app}
}
type AppRoutes struct { app []any }
func ProvideRoutes(r *AppRoutes) []any {
	groups := []any{}
	groups = append(groups, http.NewRouteGroup("/api", r.app, middleware.Auth(), trace))
	return groups
}`,
	}
	writeFixtureFiles(t, root, files)

	manifest, err := Run(context.Background(), IndexOptions{Root: root})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(manifest.Operations) != 1 {
		t.Fatalf("expected one operation, got %d", len(manifest.Operations))
	}
	got := manifest.Operations[0].Middleware
	want := []string{"middleware.Auth", "trace", "middleware.Gzip"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected middleware list: got=%v want=%v", got, want)
	}
}

func TestRunUsesDefaultMiddlewaresForSingleGroup(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.24\n",
		"internal/hello/controller.go": `package hello
import "net/http"
type Controller struct{}
func (c *Controller) Routes() []any { return []any{http.NewRoute(http.MethodGet, "/m", c.Index)} }
func (c *Controller) Index(ctx any) error { return nil }`,
		"internal/router/routes_registry.go": `package router
func ProvideRoutes(r *AppRoutes) []any {
	groups := []any{}
	groups = append(groups, http.NewRouteGroup("/api", r.app, middleware.RequireAuth()))
	return groups
}
type AppRoutes struct { app []any }`,
	}
	writeFixtureFiles(t, root, files)

	manifest, err := Run(context.Background(), IndexOptions{Root: root})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(manifest.Operations) != 1 {
		t.Fatalf("expected one operation, got %d", len(manifest.Operations))
	}
	got := manifest.Operations[0].Middleware
	want := []string{"middleware.RequireAuth"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected middleware list: got=%v want=%v", got, want)
	}
}

func TestOwnerFromRoutesArg(t *testing.T) {
	paramOwner := map[string]string{"helloController": "hello.Controller"}

	callExpr, err := parser.ParseExpr("helloController.Routes()")
	if err != nil {
		t.Fatalf("parse expr: %v", err)
	}
	if got := ownerFromRoutesArg(callExpr, paramOwner); got != "hello.Controller" {
		t.Fatalf("unexpected owner for call: %q", got)
	}

	selExpr, err := parser.ParseExpr("helloController.Routes")
	if err != nil {
		t.Fatalf("parse expr: %v", err)
	}
	if got := ownerFromRoutesArg(selExpr, paramOwner); got != "hello.Controller" {
		t.Fatalf("unexpected owner for selector: %q", got)
	}

	invalidExpr, err := parser.ParseExpr("helloController.Other()")
	if err != nil {
		t.Fatalf("parse expr: %v", err)
	}
	if got := ownerFromRoutesArg(invalidExpr, paramOwner); got != "" {
		t.Fatalf("expected empty owner for invalid expression, got %q", got)
	}

	nonIdentSelector, err := parser.ParseExpr("pkg.helloController.Routes")
	if err != nil {
		t.Fatalf("parse expr: %v", err)
	}
	if got := ownerFromRoutesArg(nonIdentSelector, paramOwner); got != "" {
		t.Fatalf("expected empty owner for selector with non-ident receiver, got %q", got)
	}

	nonSelectorCall, err := parser.ParseExpr("Routes()")
	if err != nil {
		t.Fatalf("parse expr: %v", err)
	}
	if got := ownerFromRoutesArg(nonSelectorCall, paramOwner); got != "" {
		t.Fatalf("expected empty owner for non-selector call, got %q", got)
	}
}

func TestMiddlewareExprs(t *testing.T) {
	args := []ast.Expr{
		&ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   &ast.Ident{Name: "middleware"},
				Sel: &ast.Ident{Name: "Auth"},
			},
		},
		&ast.Ident{Name: "trace"},
	}
	got := middlewareExprs(args)
	want := []string{"middleware.Auth", "trace"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected middleware expressions: got=%v want=%v", got, want)
	}
	if middlewareExprs(nil) != nil {
		t.Fatalf("expected nil for empty middleware args")
	}
}

func TestParseStatusCode(t *testing.T) {
	if got := parseStatusCode(&ast.BasicLit{Kind: token.INT, Value: "201"}); got != 201 {
		t.Fatalf("expected 201, got %d", got)
	}
	if got := parseStatusCode(&ast.SelectorExpr{
		X:   &ast.Ident{Name: "http"},
		Sel: &ast.Ident{Name: "StatusBadRequest"},
	}); got != 400 {
		t.Fatalf("expected 400, got %d", got)
	}
	if got := parseStatusCode(&ast.SelectorExpr{
		X:   &ast.Ident{Name: "http"},
		Sel: &ast.Ident{Name: "StatusUnknown"},
	}); got != 0 {
		t.Fatalf("expected 0 for unknown status, got %d", got)
	}
}

func TestSchemaFromTypeExprAndStructHelpers(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{expr: "string", want: "string"},
		{expr: "bool", want: "boolean"},
		{expr: "int64", want: "integer"},
		{expr: "float64", want: "number"},
		{expr: "Custom", want: "object"},
		{expr: "[]string", want: "array"},
		{expr: "map[string]int", want: "object"},
		{expr: "*Custom", want: "object"},
		{expr: "dto.Input", want: "object"},
	}
	for _, tc := range cases {
		expr, err := parser.ParseExpr(tc.expr)
		if err != nil {
			t.Fatalf("parse expr %q: %v", tc.expr, err)
		}
		got := schemaFromTypeExpr(expr)["type"]
		if got != tc.want {
			t.Fatalf("unexpected type for %q: got=%v want=%v", tc.expr, got, tc.want)
		}
	}

	if got := schemaFromTypeExpr(&ast.InterfaceType{}); got["type"] != "string" {
		t.Fatalf("expected default schema type string, got %+v", got)
	}

	if got := componentNameFromType("*dto.User"); got != "User" {
		t.Fatalf("unexpected component name: %q", got)
	}
	if got := componentNameFromType("9-user"); got != "Type9user" {
		t.Fatalf("unexpected sanitized component name: %q", got)
	}
}

func TestStructSchemaAndJSONTags(t *testing.T) {
	src := `package p
type Payload struct {
	Name string ` + "`json:\"name\"`" + `
	Meta *Meta ` + "`json:\"meta,omitempty\"`" + `
	Ignored string ` + "`json:\"-\"`" + `
	Count int
}
type Meta struct {
	Source string
}`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "payload.go", src, 0)
	if err != nil {
		t.Fatalf("parse file: %v", err)
	}
	parsed := []*parsedFile{{Path: "payload.go", PackageName: "p", File: file}}
	index := buildTypeSchemaIndex(parsed)
	raw, ok := index["p.Payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected p.Payload schema in index, got %+v", index)
	}
	props := raw["properties"].(map[string]any)
	if _, ok := props["name"]; !ok {
		t.Fatalf("expected json-tagged name property, got %+v", props)
	}
	if _, ok := props["meta"]; !ok {
		t.Fatalf("expected pointer property meta, got %+v", props)
	}
	if _, ok := props["Ignored"]; ok {
		t.Fatalf("did not expect ignored field to be present")
	}
	requiredAny := raw["required"].([]string)
	if !reflect.DeepEqual(requiredAny, []string{"Count", "name"}) {
		t.Fatalf("unexpected required fields: %+v", requiredAny)
	}
}

func TestInferJSONSchemaExprHelpers(t *testing.T) {
	expr, err := parser.ParseExpr(`Resp{OK: true, Stats: map[string]any{"count": 1}, Items: []any{"a"}}`)
	if err != nil {
		t.Fatalf("parse expr: %v", err)
	}
	schema, ok := inferJSONSchemaExpr(expr).(map[string]any)
	if !ok {
		t.Fatalf("expected object schema, got %+v", schema)
	}
	props := schema["properties"].(map[string]any)
	if _, ok := props["OK"]; !ok {
		t.Fatalf("expected struct field OK in schema")
	}
	if _, ok := props["Stats"]; !ok {
		t.Fatalf("expected struct field Stats in schema")
	}

	if got := extractStructFieldName(&ast.SelectorExpr{
		X:   &ast.Ident{Name: "Resp"},
		Sel: &ast.Ident{Name: "Field"},
	}); got != "Field" {
		t.Fatalf("unexpected selector field name: %q", got)
	}

	unaryExpr, err := parser.ParseExpr(`&map[string]any{"n": 1.5, "ok": true, "none": nil}`)
	if err != nil {
		t.Fatalf("parse unary expr: %v", err)
	}
	unarySchema := inferJSONSchemaExpr(unaryExpr).(map[string]any)
	unaryProps := unarySchema["properties"].(map[string]any)
	if unaryProps["n"].(map[string]any)["type"] != "number" {
		t.Fatalf("expected number type for float literal, got %+v", unaryProps["n"])
	}
	if unaryProps["ok"].(map[string]any)["type"] != "boolean" {
		t.Fatalf("expected boolean type for bool literal, got %+v", unaryProps["ok"])
	}
	if _, ok := unaryProps["none"].(map[string]any)["nullable"]; !ok {
		t.Fatalf("expected nullable schema for nil literal, got %+v", unaryProps["none"])
	}

	arrayExpr, err := parser.ParseExpr(`[]any{map[string]any{"k": "v"}}`)
	if err != nil {
		t.Fatalf("parse array expr: %v", err)
	}
	arraySchema := inferJSONSchemaExpr(arrayExpr).(map[string]any)
	if arraySchema["type"] != "array" {
		t.Fatalf("expected array schema, got %+v", arraySchema)
	}
	itemSchema := arraySchema["items"].(map[string]any)
	if itemSchema["type"] != "object" {
		t.Fatalf("expected object item schema, got %+v", itemSchema)
	}

	typedExpr, err := parser.ParseExpr(`Payload{}`)
	if err != nil {
		t.Fatalf("parse typed expr: %v", err)
	}
	typedSchema := inferJSONSchemaExpr(typedExpr).(map[string]any)
	if typedSchema["x-forj-type"] != "Payload" {
		t.Fatalf("expected typed fallback schema, got %+v", typedSchema)
	}
}

func TestToOpenAPIWrapper(t *testing.T) {
	m := Manifest{
		Operations: []Operation{
			{
				ID:     "GET:/x",
				Method: "GET",
				Path:   "/x",
				Outputs: OutputShape{
					Responses: []ResponseShape{{StatusCode: 200, Source: "echo.NoContent"}},
				},
			},
		},
	}
	doc := toOpenAPI(m)
	if doc.Info["title"] != "Forj Generated API" {
		t.Fatalf("unexpected default title: %q", doc.Info["title"])
	}
}

func TestResponseContentAndOpenAPIMergeHelpers(t *testing.T) {
	if contentType, schema := responseContent(ResponseShape{Schema: map[string]any{"type": "object"}}); contentType != "application/json" || schema == nil {
		t.Fatalf("expected direct schema content for json response")
	}
	for _, tc := range []struct {
		resp        ResponseShape
		contentType string
	}{
		{resp: ResponseShape{Source: "echo.String"}, contentType: "text/plain"},
		{resp: ResponseShape{Source: "echo.HTML"}, contentType: "text/html"},
		{resp: ResponseShape{Source: "echo.XML"}, contentType: "application/xml"},
		{resp: ResponseShape{Source: "echo.Blob"}, contentType: "application/octet-stream"},
	} {
		if got, _ := responseContent(tc.resp); got != tc.contentType {
			t.Fatalf("unexpected content type for %+v: got=%s want=%s", tc.resp, got, tc.contentType)
		}
	}
	if got, _ := responseContent(ResponseShape{TypeName: "map[string]any"}); got != "application/json" {
		t.Fatalf("expected json content type for map response")
	}
	if got, _ := responseContent(ResponseShape{TypeName: "[]Thing"}); got != "application/json" {
		t.Fatalf("expected json content type for array response")
	}
	if got, _ := responseContent(ResponseShape{TypeName: "Thing"}); got != "application/json" {
		t.Fatalf("expected json content type for object response")
	}
	if got, schema := responseContent(ResponseShape{}); got != "" || schema != nil {
		t.Fatalf("expected empty content for empty response shape")
	}

	existing := map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string"}}}}
	incoming := map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"b": map[string]any{"type": "string"}}}}
	merged := mergeOpenAPIContentBody(existing, incoming)
	schema, ok := merged["schema"].(map[string]any)
	if !ok {
		t.Fatalf("expected merged schema body, got %+v", merged)
	}
	oneOf, ok := schema["oneOf"].([]any)
	if !ok || len(oneOf) != 2 {
		t.Fatalf("expected oneOf with 2 schemas, got %+v", schema)
	}

	if out := mergeOpenAPIContentBody(nil, incoming); !reflect.DeepEqual(out, incoming) {
		t.Fatalf("expected incoming when existing empty, got %+v", out)
	}
	if out := mergeOpenAPIContentBody(existing, nil); !reflect.DeepEqual(out, existing) {
		t.Fatalf("expected existing when incoming empty, got %+v", out)
	}
	if out := mergeOpenAPIContentBody(map[string]any{"description": "x"}, incoming); !reflect.DeepEqual(out, incoming) {
		t.Fatalf("expected incoming when existing has no schema, got %+v", out)
	}
	if out := mergeOpenAPIContentBody(existing, map[string]any{"description": "x"}); !reflect.DeepEqual(out, existing) {
		t.Fatalf("expected existing when incoming has no schema, got %+v", out)
	}
	sameSchema := map[string]any{"schema": map[string]any{"type": "string"}}
	if out := mergeOpenAPIContentBody(sameSchema, sameSchema); !reflect.DeepEqual(out, sameSchema) {
		t.Fatalf("expected identical schema merge to preserve existing, got %+v", out)
	}
	if !schemasEquivalent(map[string]any{"type": "string"}, map[string]any{"type": "string"}) {
		t.Fatalf("expected equivalent schemas")
	}
	if schemasEquivalent("x", map[string]any{"type": "string"}) {
		t.Fatalf("expected non-map schemas to be non-equivalent")
	}
	items := dedupeSchemas([]any{
		map[string]any{"type": "string"},
		map[string]any{"type": "string"},
		"non-map",
	})
	if len(items) != 2 {
		t.Fatalf("expected deduped items length 2, got %d (%+v)", len(items), items)
	}
}

func TestSchemaComponentsRefStoreAndNaming(t *testing.T) {
	c := newSchemaComponents()
	first := c.refOrStore(map[string]any{"type": "object", "x-forj-type": "hello.Input"})
	second := c.refOrStore(map[string]any{"type": "object", "x-forj-type": "hello.Input"})
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected same ref for identical schema, got %v and %v", first, second)
	}
	third := c.refOrStore(map[string]any{"type": "object", "x-forj-type": "hello.Other"})
	if reflect.DeepEqual(second, third) {
		t.Fatalf("expected different refs for different schema fingerprints")
	}
	if got := c.refOrStore(123); got != 123 {
		t.Fatalf("non-map schemas should pass through untouched")
	}
	if got := c.refOrStore(map[string]any{}); !reflect.DeepEqual(got, map[string]any{}) {
		t.Fatalf("empty map schema should pass through untouched")
	}
}

func TestAnalyzeTypeInferenceHelpers(t *testing.T) {
	locals := map[string]string{"in": "Input"}
	if got := inferExprTypeName(&ast.Ident{Name: "in"}, locals); got != "Input" {
		t.Fatalf("expected local ident type, got %q", got)
	}
	if got := inferExprTypeName(&ast.CallExpr{Fun: &ast.Ident{Name: "new"}, Args: []ast.Expr{&ast.Ident{Name: "Payload"}}}, locals); got != "Payload" {
		t.Fatalf("expected new(T) type, got %q", got)
	}
	if got := inferExprTypeName(&ast.CallExpr{Fun: &ast.SelectorExpr{X: &ast.Ident{Name: "pkg"}, Sel: &ast.Ident{Name: "NewInput"}}}, locals); got != "pkg.NewInput" {
		t.Fatalf("expected constructor selector type, got %q", got)
	}
	if got := inferExprTypeName(&ast.SelectorExpr{X: &ast.Ident{Name: "dto"}, Sel: &ast.Ident{Name: "Input"}}, locals); got != "dto.Input" {
		t.Fatalf("expected selector type, got %q", got)
	}
	if got := inferArgTypeName(&ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: &ast.Ident{Name: "Request"}}}, locals); got != "Request" {
		t.Fatalf("expected unary arg type, got %q", got)
	}
}

func TestCollectLocalTypesAssignmentKeepsExistingTypedVar(t *testing.T) {
	src := `package p
func f() {
	var req Request
	req = normalize(req)
	payload := NewPayload()
	other := new(Other)
	_, _ = payload, other
}`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatalf("parse file: %v", err)
	}
	fn := file.Decls[0].(*ast.FuncDecl)
	got := collectLocalTypes(fn.Body)
	if got["req"] != "Request" {
		t.Fatalf("expected req to remain Request, got %q", got["req"])
	}
	if got["payload"] != "NewPayload" {
		t.Fatalf("expected inferred payload type NewPayload, got %q", got["payload"])
	}
	if got["other"] != "Other" {
		t.Fatalf("expected inferred other type Other, got %q", got["other"])
	}
}

func TestCollectLocalTypesInferredVarAndNonIdentAssignments(t *testing.T) {
	src := `package p
func f() {
	var inferred = Payload{}
	var explicit Payload
	external.Field = factory()
	_, _ = inferred, explicit
}`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatalf("parse file: %v", err)
	}
	fn := file.Decls[0].(*ast.FuncDecl)
	got := collectLocalTypes(fn.Body)
	if got["inferred"] != "Payload" {
		t.Fatalf("expected inferred var type Payload, got %q", got["inferred"])
	}
	if got["explicit"] != "Payload" {
		t.Fatalf("expected explicit var type Payload, got %q", got["explicit"])
	}
}

func TestParseGoFilesWithSetSkipsTemplatesAndTests(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "templates", "x"), 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "a.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatalf("write a.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "a_test.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatalf("write a_test.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "templates", "x", "ignored.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatalf("write ignored.go: %v", err)
	}
	parsed, _, err := parseGoFilesWithSet(root)
	if err != nil {
		t.Fatalf("parseGoFilesWithSet failed: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected only one parsed file, got %d", len(parsed))
	}
}

func TestParseGoFilesWithSetSkipsUnparseableGoFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "good.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatalf("write good.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "bad.go"), []byte("package pkg\nfunc {"), 0o644); err != nil {
		t.Fatalf("write bad.go: %v", err)
	}
	parsed, _, err := parseGoFilesWithSet(root)
	if err != nil {
		t.Fatalf("parseGoFilesWithSet failed: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected parser to skip invalid go file and keep one valid file, got %d", len(parsed))
	}
}

func TestIndexerHelpers(t *testing.T) {
	if got := normalizeMethodExpr("http.MethodPost"); got != "post" {
		t.Fatalf("unexpected method normalization: %q", got)
	}
	if got := normalizeMethodExpr(`"GET"`); got != "get" {
		t.Fatalf("unexpected quoted method normalization: %q", got)
	}
	if got := methodNameFromHandlerExpr("controller.Handle"); got != "Handle" {
		t.Fatalf("unexpected handler method name: %q", got)
	}

	if got := typeNameFromExpr(nil); got != "" {
		t.Fatalf("expected empty type name for nil expr, got %q", got)
	}
	if got := typeNameFromExpr(&ast.StarExpr{X: &ast.Ident{Name: "Input"}}); got != "Input" {
		t.Fatalf("unexpected star type name: %q", got)
	}
	if got := typeNameFromExpr(&ast.SelectorExpr{X: &ast.Ident{Name: "dto"}, Sel: &ast.Ident{Name: "Input"}}); got != "dto.Input" {
		t.Fatalf("unexpected selector type name: %q", got)
	}
	if got := typeNameFromExpr(&ast.ArrayType{Elt: &ast.Ident{Name: "string"}}); got != "[]string" {
		t.Fatalf("unexpected array type name: %q", got)
	}
	if got := typeNameFromExpr(&ast.MapType{Key: &ast.Ident{Name: "string"}, Value: &ast.Ident{Name: "int"}}); got != "map[string]int" {
		t.Fatalf("unexpected map type name: %q", got)
	}
	if got := typeNameFromExpr(&ast.FuncType{}); got == "" {
		t.Fatalf("expected fallback type name for func type")
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("APP_NAME=Indexer Helper App\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if got := openAPITitleFromRoot(root); got != "Indexer Helper App" {
		t.Fatalf("unexpected openapi title from root: %q", got)
	}
	if got := appNameFromDotEnv(filepath.Join(root, ".missing")); got != "" {
		t.Fatalf("expected empty app name when .env missing, got %q", got)
	}
}

func TestRunReturnsErrorWhenOutputPathIsDirectory(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/test\n\ngo 1.24\n",
		"internal/hello/controller.go": `package hello
import "net/http"
type Controller struct{}
func (c *Controller) Routes() []any { return []any{http.NewRoute(http.MethodGet, "/x", c.X)} }
func (c *Controller) X(ctx any) error { return nil }`,
	}
	writeFixtureFiles(t, root, files)

	outDir := filepath.Join(root, "build")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir build: %v", err)
	}
	_, err := Run(context.Background(), IndexOptions{
		Root:    root,
		OutPath: outDir,
	})
	if err == nil {
		t.Fatal("expected Run to fail when OutPath is a directory")
	}
}

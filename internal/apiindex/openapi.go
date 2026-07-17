package apiindex

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/goforj/str/v2"

	"github.com/goforj/goforj/project"
	"github.com/goforj/web/webindex"
)

const (
	// authAccessSecurityScheme names the generated short-lived access-cookie policy.
	authAccessSecurityScheme = "authAccess"
	// authRefreshSecurityScheme names the generated refresh-cookie policy.
	authRefreshSecurityScheme = "authRefresh"
)

// openAPIOptions projects project/App identity and only the auth policy GoForj explicitly generates.
func openAPIOptions(paths paths, buildTags []string) (webindex.OpenAPIOptions, error) {
	config, err := project.LoadProjectConfigAt(paths.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return webindex.OpenAPIOptions{}, nil
		}
		return webindex.OpenAPIOptions{}, fmt.Errorf("load project metadata for OpenAPI: %w", err)
	}

	options := webindex.OpenAPIOptions{Info: webindex.OpenAPIInfoOptions{
		Title:       openAPITitle(config.ProjectName, paths.appName),
		Version:     "1.0.0",
		Description: openAPIDescription(paths.appName),
	}}
	components, known := componentsForApp(config, paths.appName)
	if !known || !components.Auth {
		return options, nil
	}

	requireAuth := []webindex.OpenAPISecurityRequirement{
		{authAccessSecurityScheme: []string{}},
		{authRefreshSecurityScheme: []string{}},
	}
	refreshAuth := []webindex.OpenAPISecurityRequirement{
		{authRefreshSecurityScheme: []string{}},
	}
	generatedAuthImportPath, err := generatedAuthImportPath(paths.root, config.GoModuleName)
	if err != nil {
		return webindex.OpenAPIOptions{}, err
	}
	authControllerPath := filepath.Join(paths.root, "internal", "auth", "controller.go")
	authFiles, parsed, err := sourcePackageFiles(authControllerPath, buildTags)
	if err != nil {
		return webindex.OpenAPIOptions{}, fmt.Errorf("inspect generated auth package for %q: %w", authControllerPath, err)
	}
	if !parsed || !generatedAuthControllerTypesExact(authFiles) {
		return options, nil
	}
	groupAuthSelected, err := sourceFunctionReturnsTypedParameterExpression(sourceReturnContract{
		sourcePath:   paths.routeComposition,
		functionName: "ProvideRoutes",
		expression:   "authService.RequireAuth",
		parameter: importedPointerParameterContract{
			name:       "authService",
			importPath: generatedAuthImportPath,
			typeName:   "Service",
		},
	})
	if err != nil {
		return webindex.OpenAPIOptions{}, err
	}
	if groupAuthSelected {
		compositionSource, sourceErr := projectSourcePath(paths.root, paths.routeComposition)
		if sourceErr != nil {
			return webindex.OpenAPIOptions{}, sourceErr
		}
		options.MiddlewareSecurityRules = append(options.MiddlewareSecurityRules, webindex.OpenAPIMiddlewareSecurityRule{
			Expression:   "authService.RequireAuth",
			SourceFile:   compositionSource,
			Function:     "ProvideRoutes",
			Requirements: cloneSecurityRequirements(requireAuth),
		})
	}

	authControllerRoutes := sourceReturnContract{
		sourcePath:   paths.routeComposition,
		functionName: "ProvideRoutes",
		expression:   "authController.Routes",
		parameter: importedPointerParameterContract{
			name:       "authController",
			importPath: generatedAuthImportPath,
			typeName:   "Controller",
		},
	}
	authRoutesSelected, err := sourceFunctionReturnsTypedParameterExpression(authControllerRoutes)
	if err != nil {
		return webindex.OpenAPIOptions{}, err
	}
	if !authRoutesSelected {
		authControllerRoutes.functionName = "ProvideAppRoutes"
		authRoutesSelected, err = sourceFunctionReturnsTypedParameterExpression(authControllerRoutes)
		if err != nil {
			return webindex.OpenAPIOptions{}, err
		}
	}
	if authRoutesSelected {
		controllerSource, sourceErr := projectSourcePath(paths.root, authControllerPath)
		if sourceErr != nil {
			return webindex.OpenAPIOptions{}, sourceErr
		}
		options.MiddlewareSecurityRules = append(options.MiddlewareSecurityRules, webindex.OpenAPIMiddlewareSecurityRule{
			Expression:   "c.auth.RequireAuth",
			SourceFile:   controllerSource,
			Function:     "Routes",
			Receiver:     "Controller",
			Requirements: cloneSecurityRequirements(requireAuth),
		})
		if generatedAuthRefreshContractExact(authFiles) {
			options.Operations = append(options.Operations, webindex.OpenAPIOperationOverride{
				Match: webindex.OpenAPIOperationSelector{
					ImportPath: generatedAuthImportPath,
					Receiver:   "Controller",
					Function:   "Refresh",
					Method:     "POST",
				},
				Security: &webindex.OpenAPISecurityPolicy{
					Requirements: cloneSecurityRequirements(refreshAuth),
				},
			})
		}
	}
	if len(options.MiddlewareSecurityRules) == 0 && len(options.Operations) == 0 {
		return options, nil
	}
	options.SecuritySchemes = generatedAuthSecuritySchemes()
	return options, nil
}

// projectSourcePath converts an indexed source file to the portable project-relative path required by scoped middleware policy.
func projectSourcePath(root string, sourcePath string) (string, error) {
	if strings.TrimSpace(sourcePath) == "" {
		return "", errors.New("OpenAPI security source path is empty")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve OpenAPI security project root: %w", err)
	}
	absSource := sourcePath
	if !filepath.IsAbs(absSource) {
		absSource = filepath.Join(absRoot, absSource)
	}
	absSource, err = filepath.Abs(absSource)
	if err != nil {
		return "", fmt.Errorf("resolve OpenAPI security source %q: %w", sourcePath, err)
	}
	relative, err := filepath.Rel(absRoot, absSource)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("OpenAPI security source %q is outside project root %q", sourcePath, root)
	}
	return filepath.ToSlash(relative), nil
}

// generatedAuthImportPath resolves the one project-owned package whose Service middleware GoForj is authorized to document as generated cookie auth.
func generatedAuthImportPath(root string, configuredModule string) (string, error) {
	configuredModule = str.Of(configuredModule).Trim().TrimSuffix("/").String()
	activeModule := ""
	content, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read project module path for OpenAPI security: %w", err)
	}
	if err == nil {
		for _, line := range strings.Split(string(content), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "module" {
				activeModule = strings.Trim(fields[1], `"`)
				break
			}
		}
		activeModule = str.Of(activeModule).Trim().TrimSuffix("/").String()
	}
	if configuredModule != "" && activeModule != "" && configuredModule != activeModule {
		return "", fmt.Errorf("project module_name %q does not match active go.mod module %q", configuredModule, activeModule)
	}
	modulePath := activeModule
	if modulePath == "" {
		modulePath = configuredModule
	}
	if modulePath == "" {
		return "", nil
	}
	return modulePath + "/internal/auth", nil
}

// generatedAuthSecuritySchemes describes only the cookies accepted by generated RequireAuth middleware.
func generatedAuthSecuritySchemes() map[string]webindex.OpenAPISecurityScheme {
	return map[string]webindex.OpenAPISecurityScheme{
		authAccessSecurityScheme: {
			Type:        "apiKey",
			Description: "Short-lived GoForj access token.",
			Name:        "auth_access",
			In:          "cookie",
		},
		authRefreshSecurityScheme: {
			Type:        "apiKey",
			Description: "Refresh-backed GoForj session token.",
			Name:        "auth_refresh",
			In:          "cookie",
		},
	}
}

// sourcePackageFiles parses non-test files from the selected source package and requires the requested file to exist.
func sourcePackageFiles(path string, buildTags []string) ([]*ast.File, bool, error) {
	directory := filepath.Dir(path)
	entries, err := os.ReadDir(directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	files := make([]*ast.File, 0, len(entries))
	targetFound := false
	fileSet := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		matches, matchErr := webindex.MatchActiveSourceFile(directory, name, buildTags...)
		if matchErr != nil {
			return nil, false, matchErr
		}
		if !matches {
			continue
		}
		candidate := filepath.Join(directory, name)
		content, readErr := os.ReadFile(candidate)
		if readErr != nil {
			return nil, false, readErr
		}
		file, parseErr := parser.ParseFile(fileSet, candidate, content, parser.AllErrors)
		if parseErr != nil || file == nil {
			return nil, false, nil
		}
		if filepath.Clean(candidate) == filepath.Clean(path) {
			targetFound = true
		}
		files = append(files, file)
	}
	if !targetFound {
		return nil, false, nil
	}
	for _, file := range files {
		if file.Name == nil || file.Name.Name != "auth" {
			return nil, false, nil
		}
	}
	return files, targetFound, nil
}

// generatedAuthControllerTypesExact proves the generated controller field and middleware method use concrete project-owned types with the framework Handler contract.
func generatedAuthControllerTypesExact(files []*ast.File) bool {
	controllerFields := 0
	services := 0
	serviceMethods := 0
	for _, file := range files {
		imports := sourcePolicyImportPaths(file)
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				if declaration.Tok != token.TYPE {
					continue
				}
				for _, rawSpecification := range declaration.Specs {
					specification, ok := rawSpecification.(*ast.TypeSpec)
					if !ok || specification.Assign != token.NoPos {
						continue
					}
					switch specification.Name.Name {
					case "Controller":
						if sourceStructHasLocalPointerField(specification.Type, "auth", "Service") {
							controllerFields++
						}
					case "Service":
						if _, concrete := specification.Type.(*ast.StructType); concrete {
							services++
						}
					}
				}
			case *ast.FuncDecl:
				if declaration.Name.Name != "RequireAuth" || declaration.Body == nil {
					continue
				}
				receiverObject, receiverExact := exactSourceMethodReceiver(declaration, "Service")
				if receiverExact && sourceFunctionHasFrameworkHandlerSignature(declaration, imports) && sourceGeneratedRequireAuthMethod(declaration, receiverObject, imports) {
					serviceMethods++
				}
			}
		}
	}
	return controllerFields == 1 && services == 1 && serviceMethods == 1 && sourceGeneratedAuthCookieConstants(files)
}

// sourceGeneratedAuthCookieConstants ties projected scheme names to the exact active generated package rather than conventional spelling alone.
func sourceGeneratedAuthCookieConstants(files []*ast.File) bool {
	accessCookies := 0
	refreshCookies := 0
	for _, file := range files {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			for _, rawSpecification := range general.Specs {
				specification, ok := rawSpecification.(*ast.ValueSpec)
				if !ok || len(specification.Names) != len(specification.Values) {
					continue
				}
				for index, name := range specification.Names {
					literal, ok := specification.Values[index].(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						continue
					}
					value, err := strconv.Unquote(literal.Value)
					if err != nil {
						continue
					}
					switch {
					case name.Name == "AccessCookieName" && value == "auth_access":
						accessCookies++
					case name.Name == "RefreshCookieName" && value == "auth_refresh":
						refreshCookies++
					}
				}
			}
		}
	}
	return accessCookies == 1 && refreshCookies == 1
}

// sourceGeneratedRequireAuthMethod verifies the generated middleware executes authentication, returns unauthorized on failure, and invokes the wrapped handler on success.
func sourceGeneratedRequireAuthMethod(function *ast.FuncDecl, receiverObject *ast.Object, imports map[string]string) bool {
	if function == nil || function.Body == nil || len(function.Body.List) != 1 || function.Type == nil || function.Type.Params == nil || len(function.Type.Params.List) != 1 {
		return false
	}
	nextField := function.Type.Params.List[0]
	if len(nextField.Names) != 1 || nextField.Names[0].Obj == nil {
		return false
	}
	nextObject := nextField.Names[0].Obj
	outerReturn, ok := function.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(outerReturn.Results) != 1 {
		return false
	}
	closure, ok := outerReturn.Results[0].(*ast.FuncLit)
	if !ok || closure.Body == nil || len(closure.Body.List) != 3 || !sourceFunctionTypeReturnsError(closure.Type) {
		return false
	}
	contextObject, contextExact := sourceFrameworkContextParameter(closure.Type, imports)
	if !contextExact {
		return false
	}

	authenticate, ok := closure.Body.List[0].(*ast.AssignStmt)
	if !ok || authenticate.Tok != token.DEFINE || len(authenticate.Lhs) != 2 || len(authenticate.Rhs) != 1 {
		return false
	}
	blank, blankOK := authenticate.Lhs[0].(*ast.Ident)
	errorName, errorOK := authenticate.Lhs[1].(*ast.Ident)
	call, callOK := authenticate.Rhs[0].(*ast.CallExpr)
	if !blankOK || blank.Name != "_" || !errorOK || errorName.Obj == nil || !callOK || len(call.Args) != 1 {
		return false
	}
	method, ok := call.Fun.(*ast.SelectorExpr)
	argument, argumentOK := call.Args[0].(*ast.Ident)
	if !ok || method.Sel.Name != "AuthenticateRequest" || !sourceSelectorRootMatches(method.X, receiverObject) || !argumentOK || argument.Obj != contextObject {
		return false
	}

	failure, ok := closure.Body.List[1].(*ast.IfStmt)
	if !ok || failure.Else != nil || failure.Body == nil || len(failure.Body.List) != 1 || !sourceErrorNotNilCondition(failure.Cond, errorName.Obj) {
		return false
	}
	if !sourceGeneratedUnauthorizedJSONReturn(failure.Body.List[0], contextObject, imports) {
		return false
	}

	success, ok := closure.Body.List[2].(*ast.ReturnStmt)
	if !ok || len(success.Results) != 1 {
		return false
	}
	nextCall, ok := success.Results[0].(*ast.CallExpr)
	if !ok || len(nextCall.Args) != 1 {
		return false
	}
	next, nextOK := nextCall.Fun.(*ast.Ident)
	context, contextOK := nextCall.Args[0].(*ast.Ident)
	return nextOK && next.Obj == nextObject && contextOK && context.Obj == contextObject
}

// sourceGeneratedUnauthorizedJSONReturn recognizes the exact generated auth failure response.
func sourceGeneratedUnauthorizedJSONReturn(statement ast.Stmt, contextObject *ast.Object, imports map[string]string) bool {
	result, ok := statement.(*ast.ReturnStmt)
	if !ok || len(result.Results) != 1 {
		return false
	}
	call, ok := result.Results[0].(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		return false
	}
	method, ok := call.Fun.(*ast.SelectorExpr)
	return ok && method.Sel.Name == "JSON" && sourceSelectorRootMatches(method.X, contextObject) &&
		sourcePolicySelectorIs(call.Args[0], imports, "net/http", "StatusUnauthorized")
}

// sourceFrameworkContextParameter returns the exact single framework Context parameter from a function type.
func sourceFrameworkContextParameter(functionType *ast.FuncType, imports map[string]string) (*ast.Object, bool) {
	if functionType == nil || functionType.Params == nil || len(functionType.Params.List) != 1 {
		return nil, false
	}
	field := functionType.Params.List[0]
	if len(field.Names) != 1 || field.Names[0].Obj == nil {
		return nil, false
	}
	selector, ok := field.Type.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Context" {
		return nil, false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	if !ok || qualifier.Obj != nil || imports[qualifier.Name] != "github.com/goforj/web" {
		return nil, false
	}
	return field.Names[0].Obj, true
}

// sourceFunctionTypeReturnsError recognizes the exact single predeclared error result used by generated handlers and middleware closures.
func sourceFunctionTypeReturnsError(functionType *ast.FuncType) bool {
	if functionType == nil || functionType.Results == nil || len(functionType.Results.List) != 1 {
		return false
	}
	field := functionType.Results.List[0]
	if len(field.Names) > 0 {
		return false
	}
	identifier, ok := field.Type.(*ast.Ident)
	return ok && identifier.Name == "error" && identifier.Obj == nil
}

// sourceErrorNotNilCondition recognizes the generated error guard without accepting a same-named shadow.
func sourceErrorNotNilCondition(expression ast.Expr, errorObject *ast.Object) bool {
	condition, ok := expression.(*ast.BinaryExpr)
	if !ok || condition.Op != token.NEQ {
		return false
	}
	left, leftOK := condition.X.(*ast.Ident)
	right, rightOK := condition.Y.(*ast.Ident)
	return leftOK && left.Obj == errorObject && rightOK && right.Name == "nil" && right.Obj == nil
}

// sourceStructHasLocalPointerField recognizes the generated unexported collaborator without accepting interfaces or aliases to custom packages.
func sourceStructHasLocalPointerField(expression ast.Expr, fieldName string, typeName string) bool {
	structure, ok := expression.(*ast.StructType)
	if !ok || structure.Fields == nil {
		return false
	}
	for _, field := range structure.Fields.List {
		pointer, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		identifier, ok := pointer.X.(*ast.Ident)
		if !ok || identifier.Name != typeName {
			continue
		}
		for _, name := range field.Names {
			if name.Name == fieldName {
				return true
			}
		}
	}
	return false
}

// sourceFunctionHasFrameworkHandlerSignature verifies the generated middleware method consumes and returns exactly one framework Handler.
func sourceFunctionHasFrameworkHandlerSignature(function *ast.FuncDecl, imports map[string]string) bool {
	if function == nil || function.Type == nil || function.Type.Params == nil || function.Type.Results == nil {
		return false
	}
	return sourceFieldListHasOneFrameworkHandler(function.Type.Params, imports) &&
		sourceFieldListHasOneFrameworkHandler(function.Type.Results, imports)
}

// sourceFieldListHasOneFrameworkHandler counts declared values rather than syntax fields because grouped parameter names still represent multiple arguments.
func sourceFieldListHasOneFrameworkHandler(fields *ast.FieldList, imports map[string]string) bool {
	if fields == nil || len(fields.List) != 1 {
		return false
	}
	field := fields.List[0]
	count := len(field.Names)
	if count == 0 {
		count = 1
	}
	if count != 1 {
		return false
	}
	selector, ok := field.Type.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Handler" {
		return false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	return ok && qualifier.Obj == nil && imports[qualifier.Name] == "github.com/goforj/web"
}

// exactSourceMethodReceiver returns the parser object used to distinguish method-receiver selectors from same-named lexical shadows.
func exactSourceMethodReceiver(function *ast.FuncDecl, expectedTypeName string) (*ast.Object, bool) {
	if function == nil || function.Recv == nil || len(function.Recv.List) != 1 {
		return nil, false
	}
	field := function.Recv.List[0]
	if len(field.Names) != 1 {
		return nil, false
	}
	pointer, ok := field.Type.(*ast.StarExpr)
	if !ok {
		return nil, false
	}
	typeName, ok := pointer.X.(*ast.Ident)
	if !ok || typeName.Name != expectedTypeName {
		return nil, false
	}
	receiver := field.Names[0]
	if receiver.Obj == nil {
		return nil, false
	}
	return receiver.Obj, true
}

// generatedAuthRefreshContractExact proves the generated refresh route and handler consume the refresh-session service without relying on middleware spelling.
func generatedAuthRefreshContractExact(files []*ast.File) bool {
	routesMethods := 0
	refreshRoutes := 0
	refreshMethods := 0
	for _, file := range files {
		imports := sourcePolicyImportPaths(file)
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			receiverObject, exact := exactSourceMethodReceiver(function, "Controller")
			if !exact {
				continue
			}
			switch function.Name.Name {
			case "Routes":
				routesMethods++
				calls, supported := directGeneratedAuthRouteCalls(function, imports)
				if !supported {
					continue
				}
				for _, call := range calls {
					if sourceGeneratedAuthRefreshRoute(call, receiverObject, imports) {
						refreshRoutes++
					}
				}
			case "Refresh":
				if sourceGeneratedAuthRefreshMethod(function, receiverObject, imports) {
					refreshMethods++
				}
			}
		}
	}
	return routesMethods == 1 && refreshRoutes == 1 && refreshMethods == 1
}

// directGeneratedAuthRouteCalls accepts only the immutable direct-return shape emitted by the generated auth controller.
func directGeneratedAuthRouteCalls(function *ast.FuncDecl, imports map[string]string) ([]*ast.CallExpr, bool) {
	if function == nil || function.Body == nil || len(function.Body.List) != 1 {
		return nil, false
	}
	statement, ok := function.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(statement.Results) != 1 {
		return nil, false
	}
	routes, ok := statement.Results[0].(*ast.CompositeLit)
	if !ok || !sourceFrameworkRouteSlice(routes.Type, imports) {
		return nil, false
	}
	calls := make([]*ast.CallExpr, 0, len(routes.Elts))
	for _, element := range routes.Elts {
		call, ok := element.(*ast.CallExpr)
		if !ok || call.Ellipsis != token.NoPos || !sourcePolicyCallIs(call, imports, "github.com/goforj/web", "NewRoute") {
			return nil, false
		}
		calls = append(calls, call)
	}
	return calls, true
}

// sourceFrameworkRouteSlice recognizes the exact framework route slice returned by generated controllers.
func sourceFrameworkRouteSlice(expression ast.Expr, imports map[string]string) bool {
	array, ok := expression.(*ast.ArrayType)
	if !ok || array.Len != nil {
		return false
	}
	selector, ok := array.Elt.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Route" {
		return false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	return ok && qualifier.Obj == nil && imports[qualifier.Name] == "github.com/goforj/web"
}

// sourceGeneratedAuthRefreshRoute matches the one middleware-free POST route that generated auth reserves for refresh-cookie rotation.
func sourceGeneratedAuthRefreshRoute(call *ast.CallExpr, receiverObject *ast.Object, imports map[string]string) bool {
	if len(call.Args) != 3 || !sourcePolicySelectorIs(call.Args[0], imports, "net/http", "MethodPost") {
		return false
	}
	path, ok := call.Args[1].(*ast.BasicLit)
	if !ok || path.Kind != token.STRING {
		return false
	}
	pathValue, err := strconv.Unquote(path.Value)
	if err != nil || pathValue != "/auth/refresh" {
		return false
	}
	handler, ok := call.Args[2].(*ast.SelectorExpr)
	return ok && handler.Sel.Name == "Refresh" && sourceSelectorRootMatches(handler.X, receiverObject)
}

// sourceGeneratedAuthRefreshMethod verifies Refresh follows the generated success/error flow instead of accepting a dead refresh-session call.
func sourceGeneratedAuthRefreshMethod(function *ast.FuncDecl, receiverObject *ast.Object, imports map[string]string) bool {
	contextObject, signatureExact := sourceFrameworkContextMethodParameter(function, imports)
	if !signatureExact || !sourceFunctionReturnsError(function) || function.Body == nil || len(function.Body.List) != 3 {
		return false
	}

	refresh, ok := function.Body.List[0].(*ast.AssignStmt)
	if !ok || refresh.Tok != token.DEFINE || len(refresh.Lhs) != 3 || len(refresh.Rhs) != 1 {
		return false
	}
	userName, userOK := refresh.Lhs[0].(*ast.Ident)
	blank, blankOK := refresh.Lhs[1].(*ast.Ident)
	errorName, errorOK := refresh.Lhs[2].(*ast.Ident)
	call, callOK := refresh.Rhs[0].(*ast.CallExpr)
	if !userOK || userName.Obj == nil || !blankOK || blank.Name != "_" || !errorOK || errorName.Obj == nil || !callOK || len(call.Args) != 1 {
		return false
	}
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	field, fieldOK := method.X.(*ast.SelectorExpr)
	argument, argumentOK := call.Args[0].(*ast.Ident)
	if method.Sel.Name != "RefreshSession" || !fieldOK || field.Sel.Name != "auth" ||
		!sourceSelectorRootMatches(field.X, receiverObject) || !argumentOK || argument.Obj != contextObject {
		return false
	}

	failure, ok := function.Body.List[1].(*ast.IfStmt)
	if !ok || failure.Else != nil || failure.Body == nil || len(failure.Body.List) != 1 || !sourceErrorNotNilCondition(failure.Cond, errorName.Obj) {
		return false
	}
	if !sourceGeneratedUnauthorizedJSONReturn(failure.Body.List[0], contextObject, imports) {
		return false
	}

	success, ok := function.Body.List[2].(*ast.ReturnStmt)
	if !ok || len(success.Results) != 1 {
		return false
	}
	successCall, ok := success.Results[0].(*ast.CallExpr)
	if !ok || len(successCall.Args) < 2 {
		return false
	}
	successMethod, ok := successCall.Fun.(*ast.SelectorExpr)
	return ok && successMethod.Sel.Name == "JSON" && sourceSelectorRootMatches(successMethod.X, contextObject) &&
		sourcePolicySelectorIs(successCall.Args[0], imports, "net/http", "StatusOK") && sourceNodeUsesObject(successCall.Args[1], userName.Obj)
}

// sourceNodeUsesObject proves the successful response consumes the exact value returned by RefreshSession rather than a same-named shadow.
func sourceNodeUsesObject(node ast.Node, object *ast.Object) bool {
	found := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		identifier, ok := candidate.(*ast.Ident)
		if ok && identifier.Obj == object {
			found = true
			return false
		}
		return !found
	})
	return found
}

// sourceFrameworkContextMethodParameter returns the exact single framework Context parameter used by generated controller methods.
func sourceFrameworkContextMethodParameter(function *ast.FuncDecl, imports map[string]string) (*ast.Object, bool) {
	if function == nil {
		return nil, false
	}
	return sourceFrameworkContextParameter(function.Type, imports)
}

// sourceFunctionReturnsError recognizes the exact single predeclared error result used by generated handlers.
func sourceFunctionReturnsError(function *ast.FuncDecl) bool {
	if function == nil {
		return false
	}
	return sourceFunctionTypeReturnsError(function.Type)
}

// sourceSelectorRootMatches accepts only a direct identifier tied to the parser object for the generated receiver.
func sourceSelectorRootMatches(expression ast.Expr, receiverObject *ast.Object) bool {
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.Obj == receiverObject
}

// sourcePolicyCallIs recognizes imported selector calls by full import identity rather than conventional aliases.
func sourcePolicyCallIs(call *ast.CallExpr, imports map[string]string, importPath string, functionName string) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != functionName {
		return false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	return ok && qualifier.Obj == nil && imports[qualifier.Name] == importPath
}

// sourcePolicySelectorIs recognizes an imported selector constant by full import identity.
func sourcePolicySelectorIs(expression ast.Expr, imports map[string]string, importPath string, name string) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	return ok && qualifier.Obj == nil && imports[qualifier.Name] == importPath
}

// cloneSecurityRequirements prevents operation overrides from sharing mutable scope slices with middleware mappings.
func cloneSecurityRequirements(requirements []webindex.OpenAPISecurityRequirement) []webindex.OpenAPISecurityRequirement {
	cloned := make([]webindex.OpenAPISecurityRequirement, 0, len(requirements))
	for _, requirement := range requirements {
		copyRequirement := make(webindex.OpenAPISecurityRequirement, len(requirement))
		for scheme, scopes := range requirement {
			copyScopes := make([]string, len(scopes))
			copy(copyScopes, scopes)
			copyRequirement[scheme] = copyScopes
		}
		cloned = append(cloned, copyRequirement)
	}
	return cloned
}

// componentsForApp resolves security capability from the same App selection used for index participation.
func componentsForApp(config *project.Config, appName string) (project.Components, bool) {
	if appName != "" && appName != project.DefaultAppName {
		appConfig, exists := config.Apps[appName]
		if !exists {
			return project.Components{}, false
		}
		return project.NormalizeAppComponents(config.Render.Components, appConfig.Components), true
	}
	if config.Render.Components == (project.Components{}) {
		return project.Components{}, false
	}
	return project.NormalizeAppComponents(config.Render.Components, config.Render.Components), true
}

// openAPITitle keeps the default App concise while making named App documents unmistakable.
func openAPITitle(projectName string, appName string) string {
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		projectName = "GoForj"
	}
	appName = strings.TrimSpace(appName)
	if appName == "" || appName == project.DefaultAppName {
		return projectName
	}
	return projectName + " / " + appName
}

// openAPIDescription identifies the owning App without coupling the contract version to framework releases.
func openAPIDescription(appName string) string {
	appName = strings.TrimSpace(appName)
	if appName == "" {
		appName = project.DefaultAppName
	}
	return fmt.Sprintf("OpenAPI contract for the %s GoForj App.", appName)
}

// sourceReturnContract binds returned source evidence to the concrete parameter authorized to provide it.
type sourceReturnContract struct {
	sourcePath   string
	functionName string
	expression   string
	parameter    importedPointerParameterContract
}

// importedPointerParameterContract identifies the exact generated collaborator accepted by a source function.
type importedPointerParameterContract struct {
	name       string
	importPath string
	typeName   string
}

// sourceFunctionReturnsTypedParameterExpression requires returned middleware evidence to originate from one exact imported parameter type.
func sourceFunctionReturnsTypedParameterExpression(contract sourceReturnContract) (bool, error) {
	if strings.TrimSpace(contract.sourcePath) == "" || strings.TrimSpace(contract.parameter.importPath) == "" {
		return false, nil
	}
	content, err := os.ReadFile(contract.sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("inspect OpenAPI security source %q: %w", contract.sourcePath, err)
	}
	file, _ := parser.ParseFile(token.NewFileSet(), contract.sourcePath, content, parser.AllErrors)
	if file == nil {
		// webindex owns parse diagnostics; this policy bridge must not replace them with a second error path.
		return false, nil
	}
	imports := sourcePolicyImportPaths(file)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != contract.functionName || function.Body == nil {
			continue
		}
		parameterObject, exact := exactImportedPointerParameter(function, contract.parameter, imports)
		if !exact || functionShadowsParameterObject(function.Body, contract.parameter.name, parameterObject) {
			return false, nil
		}
		return functionReturnsExpression(function.Body.List, contract.expression), nil
	}
	return false, nil
}

// sourcePolicyImportPaths resolves source aliases without trusting a package's short name as ownership evidence.
func sourcePolicyImportPaths(file *ast.File) map[string]string {
	imports := map[string]string{}
	if file == nil {
		return imports
	}
	for _, specification := range file.Imports {
		importPath, err := strconv.Unquote(specification.Path.Value)
		if err != nil || importPath == "" {
			continue
		}
		alias := filepath.Base(importPath)
		if specification.Name != nil {
			alias = specification.Name.Name
		}
		if alias == "" || alias == "_" || alias == "." {
			continue
		}
		imports[alias] = importPath
	}
	return imports
}

// exactImportedPointerParameter proves that a composition identifier is the generated concrete service rather than a same-named interface or custom package.
func exactImportedPointerParameter(function *ast.FuncDecl, contract importedPointerParameterContract, imports map[string]string) (*ast.Object, bool) {
	if function == nil || function.Type == nil || function.Type.Params == nil {
		return nil, false
	}
	for _, field := range function.Type.Params.List {
		pointer, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		selector, ok := pointer.X.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != contract.typeName {
			continue
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok || qualifier.Obj != nil || imports[qualifier.Name] != contract.importPath {
			continue
		}
		for _, name := range field.Names {
			if name.Name == contract.name && name.Obj != nil {
				return name.Obj, true
			}
		}
	}
	return nil, false
}

// functionShadowsParameterObject rejects ambiguous lexical ownership instead of allowing a nested same-name collaborator to inherit generated auth policy.
func functionShadowsParameterObject(body *ast.BlockStmt, parameterName string, parameterObject *ast.Object) bool {
	shadowed := false
	ast.Inspect(body, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || identifier.Name != parameterName || identifier.Obj == nil {
			return true
		}
		if identifier.Obj != parameterObject {
			shadowed = true
			return false
		}
		return true
	})
	return shadowed
}

// sourceFunctionReturnsExpression follows generated composition values so dead calls cannot enable an unused security mapping.
func sourceFunctionReturnsExpression(path string, functionName string, expression string) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("inspect OpenAPI security source %q: %w", path, err)
	}
	file, _ := parser.ParseFile(token.NewFileSet(), path, content, parser.SkipObjectResolution|parser.AllErrors)
	if file == nil {
		// webindex owns parse diagnostics; this policy bridge must not replace them with a second error path.
		return false, nil
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != functionName || function.Body == nil {
			continue
		}
		if functionReturnsExpression(function.Body.List, expression) {
			return true, nil
		}
	}
	return false, nil
}

// returnedExpressionScopes tracks aliases by lexical scope so a dead shadow cannot enable generated security.
type returnedExpressionScopes []map[string]bool

// newReturnedExpressionScopes creates the function-body scope used for returned-flow analysis.
func newReturnedExpressionScopes() returnedExpressionScopes {
	return returnedExpressionScopes{map[string]bool{}}
}

// clone preserves branch isolation because only values that can reach a later return should escape the branch.
func (s returnedExpressionScopes) clone() returnedExpressionScopes {
	cloned := make(returnedExpressionScopes, len(s))
	for index, scope := range s {
		cloned[index] = make(map[string]bool, len(scope))
		for name, value := range scope {
			cloned[index][name] = value
		}
	}
	return cloned
}

// child adds a lexical block without sharing its declarations with the parent scope.
func (s returnedExpressionScopes) child() returnedExpressionScopes {
	child := make(returnedExpressionScopes, len(s)+1)
	copy(child, s)
	child[len(s)] = map[string]bool{}
	return child
}

// value resolves the nearest declaration so nested short declarations cannot overwrite outer flow state.
func (s returnedExpressionScopes) value(name string) bool {
	for index := len(s) - 1; index >= 0; index-- {
		if value, ok := s[index][name]; ok {
			return value
		}
	}
	return false
}

// declare records a value in the current lexical block.
func (s returnedExpressionScopes) declare(name string, value bool) {
	s[len(s)-1][name] = value
}

// declareShort updates an existing name only in the current block because := shadows outer declarations.
func (s returnedExpressionScopes) declareShort(name string, value bool) {
	s[len(s)-1][name] = value
}

// assign updates the nearest declaration so ordinary assignment inside a generated conditional reaches the function return.
func (s returnedExpressionScopes) assign(name string, value bool) {
	for index := len(s) - 1; index >= 0; index-- {
		if _, ok := s[index][name]; ok {
			s[index][name] = value
			return
		}
	}
	s.declare(name, value)
}

// mergeExisting unions branch outcomes only for bindings that existed before the branch.
func (s returnedExpressionScopes) mergeExisting(branch returnedExpressionScopes) {
	for index, scope := range s {
		for name, value := range scope {
			scope[name] = value || branch[index][name]
		}
	}
}

// functionReturnsExpression evaluates the generated composition vocabulary, including historical conditional groups.
func functionReturnsExpression(statements []ast.Stmt, target string) bool {
	return statementsReturnExpression(statements, target, newReturnedExpressionScopes())
}

// statementsReturnExpression follows aliases until a return while retaining branch-local scope boundaries.
func statementsReturnExpression(statements []ast.Stmt, target string, values returnedExpressionScopes) bool {
	for _, statement := range statements {
		if statementReturnsExpression(statement, target, values) {
			return true
		}
	}
	return false
}

// statementReturnsExpression applies one generated statement to the current returned-flow state.
func statementReturnsExpression(statement ast.Stmt, target string, values returnedExpressionScopes) bool {
	switch current := statement.(type) {
	case *ast.DeclStmt:
		declaration, ok := current.Decl.(*ast.GenDecl)
		if !ok {
			return false
		}
		for _, specification := range declaration.Specs {
			valueSpec, ok := specification.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for index, name := range valueSpec.Names {
				resolved := index < len(valueSpec.Values) && expressionContainsReturnedTarget(valueSpec.Values[index], target, values)
				values.declare(name.Name, resolved)
			}
		}
	case *ast.AssignStmt:
		resolved := make([]bool, len(current.Rhs))
		for index, expression := range current.Rhs {
			resolved[index] = expressionContainsReturnedTarget(expression, target, values)
		}
		for index, left := range current.Lhs {
			name, ok := left.(*ast.Ident)
			if !ok {
				continue
			}
			valueIndex := index
			if len(resolved) == 1 {
				valueIndex = 0
			}
			value := valueIndex < len(resolved) && resolved[valueIndex]
			if current.Tok == token.DEFINE {
				values.declareShort(name.Name, value)
				continue
			}
			values.assign(name.Name, value)
		}
	case *ast.ReturnStmt:
		for _, result := range current.Results {
			if expressionContainsReturnedTarget(result, target, values) {
				return true
			}
		}
	case *ast.BlockStmt:
		branch := values.clone().child()
		if statementsReturnExpression(current.List, target, branch) {
			return true
		}
		values.mergeExisting(branch)
	case *ast.IfStmt:
		conditional := values.clone().child()
		if current.Init != nil && statementReturnsExpression(current.Init, target, conditional) {
			return true
		}
		values.mergeExisting(conditional)

		body := conditional.clone().child()
		if statementsReturnExpression(current.Body.List, target, body) {
			return true
		}
		values.mergeExisting(body)

		if current.Else != nil {
			alternate := conditional.clone()
			if statementReturnsExpression(current.Else, target, alternate) {
				return true
			}
			values.mergeExisting(alternate)
		}
	}
	return false
}

// expressionContainsReturnedTarget recognizes only framework composition operations whose arguments flow into their result.
func expressionContainsReturnedTarget(expression ast.Expr, target string, values returnedExpressionScopes) bool {
	switch current := expression.(type) {
	case *ast.Ident:
		return values.value(current.Name)
	case *ast.ParenExpr:
		return expressionContainsReturnedTarget(current.X, target, values)
	case *ast.UnaryExpr:
		return expressionContainsReturnedTarget(current.X, target, values)
	case *ast.CompositeLit:
		for _, element := range current.Elts {
			if expressionContainsReturnedTarget(element, target, values) {
				return true
			}
		}
	case *ast.KeyValueExpr:
		return expressionContainsReturnedTarget(current.Value, target, values)
	case *ast.SelectorExpr:
		return selectorExpression(current) == target
	case *ast.CallExpr:
		if selectorExpression(current.Fun) == target {
			return true
		}
		if !isGeneratedCompositionCall(current.Fun) {
			return false
		}
		for _, argument := range current.Args {
			if expressionContainsReturnedTarget(argument, target, values) {
				return true
			}
		}
	}
	return false
}

// isGeneratedCompositionCall limits data flow to calls whose result retains route or middleware arguments.
func isGeneratedCompositionCall(expression ast.Expr) bool {
	name := selectorExpression(expression)
	return name == "append" ||
		name == "slices.Concat" ||
		strings.HasSuffix(name, ".NewRoute") ||
		strings.HasSuffix(name, ".NewRouteGroup")
}

// selectorExpression renders only identifier/selector chains because generated middleware mappings never include calls or indexing.
func selectorExpression(expression ast.Expr) string {
	switch current := expression.(type) {
	case *ast.Ident:
		return current.Name
	case *ast.SelectorExpr:
		prefix := selectorExpression(current.X)
		if prefix == "" {
			return ""
		}
		return prefix + "." + current.Sel.Name
	default:
		return ""
	}
}

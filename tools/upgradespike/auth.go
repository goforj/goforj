package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

const authRegression = `//go:build integration

package auth

import (
    "errors"
    "testing"
    "time"
)

// TestUpgradeProbeOAuthStateSingleUse preserves one-time and expiry checks across application-specific OAuth flows.
func TestUpgradeProbeOAuthStateSingleUse(t *testing.T) {
    for _, expired := range []bool{false, true} {
        name := "replayed"
        if expired { name = "expired" }
        t.Run(name, func(t *testing.T) {
            h := newAuthIntegrationHarness(t)
            now := time.Now().UTC()
            expiry := now.Add(time.Minute)
            if expired { expiry = now.Add(-time.Minute) }
            state := &OAuthStateToken{Provider: "fake", State: name, CodeVerifier: "verifier", ExpiresAt: expiry}
            if err := h.oauthStates.Save(state); err != nil { t.Fatal(err) }
            if !expired {
                if err := h.oauthStates.MarkUsed(state.ID, now); err != nil { t.Fatal(err) }
            }
            if err := h.oauthStates.MarkUsed(state.ID, now); !errors.Is(err, ErrInvalidOAuthState) {
                t.Fatalf("one-time or expiry check not enforced: %v", err)
            }
        })
    }
}
`

const authClaimMethod = `func (r *OAuthStateRepo) MarkUsed(id int64, usedAt time.Time) error {
    conn, err := r.connection()
    if err != nil { return err }
    result := conn.Model(&OAuthStateToken{}).
        Where("id = ? AND used_at IS NULL AND expires_at > ?", id, usedAt.UTC()).
        Updates(map[string]any{"used_at": usedAt.UTC(), "updated_at": usedAt.UTC()})
    if result.Error != nil { return result.Error }
    if result.RowsAffected != 1 { return ErrInvalidOAuthState }
    return nil
}`

// authPhase retains package-qualified test evidence for each controlled experiment.
type authPhase struct {
	Phase string `json:"phase"`
	testResult
}

// authMethod locates one concrete pointer-receiver method without matching comments or nested braces.
func authMethod(source []byte, receiver, receiverType, method string) (*ast.FuncDecl, *token.FileSet, error) {
	files := token.NewFileSet()
	file, err := parser.ParseFile(files, "auth.go", source, 0)
	if err != nil {
		return nil, nil, err
	}
	var matches []*ast.FuncDecl
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != method || function.Recv == nil {
			continue
		}
		for _, field := range function.Recv.List {
			if authReceiverType(field.Type) == receiverType {
				matches = append(matches, function)
				break
			}
		}
	}
	if len(matches) != 1 {
		return nil, nil, fmt.Errorf("expected exactly one %s method, got %d", method, len(matches))
	}
	function := matches[0]
	if function.Body == nil || len(function.Recv.List) != 1 {
		return nil, nil, fmt.Errorf("unsupported %s method shape", method)
	}
	field := function.Recv.List[0]
	pointer, ok := field.Type.(*ast.StarExpr)
	if !ok {
		return nil, nil, fmt.Errorf("unsupported %s receiver", method)
	}
	name, ok := pointer.X.(*ast.Ident)
	if !ok || name.Name != receiverType || len(field.Names) != 1 || field.Names[0].Name != receiver {
		return nil, nil, fmt.Errorf("unsupported %s receiver", method)
	}
	return function, files, nil
}

// authReceiverType identifies unsupported value or generic receivers before enforcing the concrete pointer shape.
func authReceiverType(expression ast.Expr) string {
	switch receiver := expression.(type) {
	case *ast.Ident:
		return receiver.Name
	case *ast.StarExpr:
		return authReceiverType(receiver.X)
	case *ast.IndexExpr:
		return authReceiverType(receiver.X)
	case *ast.IndexListExpr:
		return authReceiverType(receiver.X)
	default:
		return ""
	}
}

// replaceAuthMethod requires matching signatures before splicing an exact AST byte range.
func replaceAuthMethod(source []byte, receiver, receiverType, method string, replacement []byte) ([]byte, error) {
	original, files, err := authMethod(source, receiver, receiverType, method)
	if err != nil {
		return nil, err
	}
	incomingSource := append([]byte("package auth\n"), replacement...)
	incoming, incomingFiles, err := authMethod(incomingSource, receiver, receiverType, method)
	if err != nil {
		return nil, err
	}
	var oldSignature, newSignature bytes.Buffer
	if err := format.Node(&oldSignature, files, original.Type); err != nil {
		return nil, err
	}
	if err := format.Node(&newSignature, incomingFiles, incoming.Type); err != nil {
		return nil, err
	}
	if oldSignature.String() != newSignature.String() {
		return nil, fmt.Errorf("unsupported %s signature change", method)
	}
	start, end := files.Position(original.Pos()).Offset, files.Position(original.End()).Offset
	result := append([]byte{}, source[:start]...)
	result = append(result, incomingSource[incomingFiles.Position(incoming.Pos()).Offset:incomingFiles.Position(incoming.End()).Offset]...)
	result = append(result, source[end:]...)
	return format.Source(result)
}

// requireAuthSuite rejects incomplete execution even when the process reports success.
func requireAuthSuite(result testResult, required []string) error {
	if result.ExitCode != 0 || len(result.Passed) == 0 || len(result.Failed) != 0 || len(result.Skipped) != 0 || len(result.PackageFailures) != 0 {
		return errors.New("auth suite does not establish every required passing behavior")
	}
	for _, identity := range required {
		if !containsAuthIdentity(result.Passed, identity) {
			return fmt.Errorf("auth suite lost required behavior %s", identity)
		}
	}
	return nil
}

// requireAuthFailure requires each intended failing identity rather than accepting a compiler failure.
func requireAuthFailure(result testResult, required []string) error {
	if result.ExitCode != 1 || len(result.Skipped) != 0 || len(required) == 0 {
		return errors.New("auth negative control did not fail behaviorally")
	}
	for _, identity := range required {
		if !containsAuthIdentity(result.Failed, identity) {
			return fmt.Errorf("auth negative control did not reject %s", identity)
		}
	}
	return nil
}

// containsAuthIdentity preserves package identity when comparing required test evidence.
func containsAuthIdentity(identities []string, required string) bool {
	for _, identity := range identities {
		if identity == required {
			return true
		}
	}
	return false
}

// authFingerprints records all neighboring auth Go files independently of aggregate test outcomes.
func authFingerprints(root string) (map[string]string, error) {
	paths, err := filepath.Glob(filepath.Join(root, "internal/auth/*.go"))
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(paths))
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("auth source is not regular: %s", filepath.Base(path))
		}
		digest, err := digestFile(path)
		if err != nil {
			return nil, err
		}
		result[filepath.Base(path)] = digest
	}
	if len(result) == 0 {
		return nil, errors.New("no auth Go source found")
	}
	return result, nil
}

// authChangedFiles compares the union so deleting an app file cannot escape the source boundary check.
func authChangedFiles(before, after map[string]string) []string {
	changed := []string{}
	for name, digest := range before {
		if after[name] != digest {
			changed = append(changed, name)
		}
	}
	for name := range after {
		if _, exists := before[name]; !exists {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)
	return changed
}

// withAuthReplacement restores the application workflow on both successful and failed negative controls.
func withAuthReplacement(path string, replacement []byte, trial func() error) (err error) {
	original, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.WriteFile(path, original, info.Mode().Perm())) }()
	if err := os.WriteFile(path, replacement, info.Mode().Perm()); err != nil {
		return err
	}
	return trial()
}

// runAuthTrial measures a real security fix while proving a destructive workflow replacement is rejected.
func runAuthTrial(ctx context.Context, renderer, source string) (any, error) {
	workspace, err := os.MkdirTemp("/tmp", "goforj-auth-reconciliation-")
	if err != nil {
		return nil, err
	}
	root := filepath.Join(workspace, "ditracker")
	if err := os.Mkdir(root, 0700); err != nil {
		return nil, err
	}
	var revisionOutput, diagnostics bytes.Buffer
	code, err := runCommand(ctx, source, []string{"git", "rev-parse", "HEAD"}, trialEnvironment(), &revisionOutput, &diagnostics)
	if err != nil {
		return nil, fmt.Errorf("resolve app revision: %w", err)
	}
	if code != 0 {
		return nil, fmt.Errorf("resolve app revision: exit %d: %s", code, diagnostics.String())
	}
	revision := strings.TrimSpace(revisionOutput.String())
	if err := copyCommittedApp(ctx, source, root, revision); err != nil {
		return nil, err
	}
	before, err := authFingerprints(root)
	if err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(workspace, "auth-before.json"), before); err != nil {
		return nil, err
	}
	phases := []authPhase{}
	run := func(name, expression string) (testResult, error) {
		timeout, cancel := context.WithTimeout(ctx, 180*time.Second)
		defer cancel()
		args := []string{"test", "-mod=readonly", "-json", "-tags=integration", "./internal/auth", "-count=1"}
		if expression != "" {
			args = append(args, "-run", expression)
		}
		result, err := runGo(timeout, root, workspace, name, args)
		phases = append(phases, authPhase{Phase: name, testResult: result})
		return result, err
	}
	baseline, err := run("baseline", "")
	if err != nil {
		return nil, err
	}
	if err := requireAuthSuite(baseline, nil); err != nil {
		return nil, err
	}
	// Derive the package from measured events so equal test names in other packages never qualify.
	packageName := ""
	for _, identity := range baseline.Passed {
		parts := strings.SplitN(identity, "::", 2)
		if len(parts) != 2 || (packageName != "" && packageName != parts[0]) {
			return nil, errors.New("ambiguous auth test package")
		}
		packageName = parts[0]
	}
	regressionTests := []string{packageName + "::TestUpgradeProbeOAuthStateSingleUse", packageName + "::TestUpgradeProbeOAuthStateSingleUse/replayed", packageName + "::TestUpgradeProbeOAuthStateSingleUse/expired"}
	regression := filepath.Join(root, "internal/auth/upgrade_probe_integration_test.go")
	file, err := os.OpenFile(regression, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	_, writeErr := file.WriteString(authRegression)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return nil, err
	}
	red, err := run("regression-before-fix", "^TestUpgradeProbeOAuthStateSingleUse$")
	if err != nil {
		return nil, err
	}
	if err := requireAuthFailure(red, regressionTests); err != nil {
		return nil, err
	}
	state := filepath.Join(root, "internal/auth/oauth_state.go")
	stateSource, err := os.ReadFile(state)
	if err != nil {
		return nil, err
	}
	fixed, err := replaceAuthMethod(stateSource, "r", "OAuthStateRepo", "MarkUsed", []byte(authClaimMethod))
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(state, fixed, 0600); err != nil {
		return nil, err
	}
	green, err := run("regression-after-fix", "^TestUpgradeProbeOAuthStateSingleUse$")
	if err != nil {
		return nil, err
	}
	if err := requireAuthSuite(green, regressionTests); err != nil {
		return nil, err
	}
	var snapshotOutput bytes.Buffer
	snapshotContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	code, err = runCommand(snapshotContext, root, []string{renderer, "render", "--snapshot"}, trialEnvironment(), &snapshotOutput, &diagnostics)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("renderer snapshot: %w", err)
	}
	if code != 0 {
		return nil, fmt.Errorf("renderer snapshot: exit %d: %s", code, diagnostics.String())
	}
	var snapshot struct {
		Artifacts []struct {
			Path   string `json:"path"`
			Source string `json:"source"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(snapshotOutput.Bytes(), &snapshot); err != nil {
		return nil, err
	}
	var incoming []byte
	matches := 0
	for _, artifact := range snapshot.Artifacts {
		if artifact.Path == "internal/auth/service.go" {
			incoming = []byte(artifact.Source)
			matches++
		}
	}
	if matches != 1 {
		return nil, errors.New("expected one incoming auth service artifact")
	}
	method, files, err := authMethod(incoming, "s", "Service", "AuthenticateRequest")
	if err != nil {
		return nil, err
	}
	service := filepath.Join(root, "internal/auth/service.go")
	original, err := os.ReadFile(service)
	if err != nil {
		return nil, err
	}
	destructive, err := replaceAuthMethod(original, "s", "Service", "AuthenticateRequest", incoming[files.Position(method.Pos()).Offset:files.Position(method.End()).Offset])
	if err != nil {
		return nil, err
	}
	err = withAuthReplacement(service, destructive, func() error {
		negative, err := run("destructive-workflow-replacement", "^TestImpersonationRoundTripIsSessionScopedAndPrivate$")
		if err != nil {
			return err
		}
		return requireAuthFailure(negative, []string{packageName + "::TestImpersonationRoundTripIsSessionScopedAndPrivate"})
	})
	if err != nil {
		return nil, err
	}
	final, err := run("reconciled-auth-suite", "")
	if err != nil {
		return nil, err
	}
	required := append(append([]string{}, baseline.Passed...), regressionTests...)
	if err := requireAuthSuite(final, required); err != nil {
		return nil, err
	}
	after, err := authFingerprints(root)
	if err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(workspace, "auth-after.json"), after); err != nil {
		return nil, err
	}
	changed := authChangedFiles(before, after)
	if !reflect.DeepEqual(changed, []string{"oauth_state.go", "upgrade_probe_integration_test.go"}) {
		return nil, fmt.Errorf("candidate changed unexpected auth source: %v", changed)
	}
	summary := struct {
		Workspace string            `json:"workspace"`
		Revision  string            `json:"revision"`
		Changed   []string          `json:"changed_auth_files"`
		Before    map[string]string `json:"auth_before"`
		After     map[string]string `json:"auth_after"`
		Phases    []authPhase       `json:"phases"`
	}{workspace, revision, changed, before, after, phases}
	if err := writeJSON(filepath.Join(workspace, "summary.json"), summary); err != nil {
		return nil, err
	}
	return summary, nil
}

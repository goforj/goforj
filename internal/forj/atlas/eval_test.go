package atlas

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/goforj/atlas/eval"
	"github.com/goforj/goforj/project"
)

// TestLoadOrCreateEvalArtifactKeyReusesOnePrivateKey keeps retained manifests verifiable across invocations.
func TestLoadOrCreateEvalArtifactKeyReusesOnePrivateKey(t *testing.T) {
	root := t.TempDir()
	first, err := loadOrCreateEvalArtifactKey(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadOrCreateEvalArtifactKey(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 32 || !bytes.Equal(first, second) {
		t.Fatalf("artifact keys differ: %x != %x", first, second)
	}
	info, err := os.Stat(filepath.Join(root, ".manifest-key"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("artifact key mode = %o, want 600", info.Mode().Perm())
	}
}

// TestMaterializeEvaluationGuidanceUsesProductionDurablePath keeps evaluation treatments on the same config and marker path used by rendering.
func TestMaterializeEvaluationGuidanceUsesProductionDurablePath(t *testing.T) {
	root := t.TempDir()
	if err := writeProjectGuidanceConfig(filepath.Join(root, ".goforj.yml"), &project.Config{ProjectName: "evaluation-test"}); err != nil {
		t.Fatal(err)
	}
	prepared := evaluationPreparedProject{result: eval.PreparationResult{ProjectRoot: root}}
	agents, err := materializeEvaluationGuidance(context.Background(), prepared, eval.Guidance{Profile: eval.GuidanceProfileAgents}, project.AgentGuidanceBaseline)
	if err != nil {
		t.Fatalf("materializeEvaluationGuidance(agents): %v", err)
	}
	if !strings.Contains(string(agents.Files["AGENTS.md"]), "<!-- goforj-atlas:start -->") {
		t.Fatalf("agents guidance = %q, want production marker", agents.Files["AGENTS.md"])
	}
	config, err := project.LoadProjectConfigAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if config.Render.AgentGuidance != project.AgentGuidanceBaseline {
		t.Fatalf("agent guidance = %q, want baseline", config.Render.AgentGuidance)
	}
	none, err := materializeEvaluationGuidance(context.Background(), prepared, eval.Guidance{Profile: eval.GuidanceProfileNone}, project.AgentGuidanceNone)
	if err != nil {
		t.Fatalf("materializeEvaluationGuidance(none): %v", err)
	}
	if len(none.Files) != 0 {
		t.Fatalf("none guidance files = %#v", none.Files)
	}
	if content, readErr := os.ReadFile(filepath.Join(root, "AGENTS.md")); readErr == nil && strings.Contains(string(content), "<!-- goforj-atlas:start -->") {
		t.Fatalf("none treatment retained managed guidance: %s", content)
	} else if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("read none guidance: %v", readErr)
	}
}

// evaluationPreparedProject supplies the prepared root needed to test the production durable guidance callback.
type evaluationPreparedProject struct {
	result eval.PreparationResult
}

// Result returns the prepared Project identity.
func (project evaluationPreparedProject) Result() eval.PreparationResult {
	return project.result
}

// Close has no resources because this fixture only represents an existing Project root.
func (evaluationPreparedProject) Close(context.Context) error {
	return nil
}

// TestFrozenEvalCredentialRedactsExactAuthority keeps the credential used by both treatments aligned with terminal redaction.
func TestFrozenEvalCredentialRedactsExactAuthority(t *testing.T) {
	credential := filepath.Join(t.TempDir(), "auth.json")
	body := []byte(`{"auth":{"access_token":"access-token-value","token":"token-value"},"provider":{"refresh_token":"refresh-token-value","OPENAI_API_KEY":"api-key-value"}}`)
	if err := os.WriteFile(credential, body, 0o600); err != nil {
		t.Fatal(err)
	}
	frozen, err := eval.LoadCodexCredential(credential)
	if err != nil {
		t.Fatal(err)
	}
	redactor := frozen.Redactor(eval.NewRedactor(nil))
	for _, want := range []string{"access-token-value", "token-value", "refresh-token-value", "api-key-value"} {
		if strings.Contains(redactor.Text(string(body)), want) {
			t.Fatalf("frozen credential redaction retained %q", want)
		}
	}
}

// TestResolveEvalCredentialRequiresExplicitDisposableSource prevents silent use of a developer's normal Codex login.
func TestResolveEvalCredentialRequiresExplicitDisposableSource(t *testing.T) {
	if _, err := resolveEvalCredential(""); err == nil || !strings.Contains(err.Error(), "disposable") {
		t.Fatalf("resolveEvalCredential() error = %v, want explicit disposable credential requirement", err)
	}
}

// TestResolveEvalArtifactRootRejectsUnownedExistingContent prevents broad caller paths from becoming artifact stores.
func TestResolveEvalArtifactRootRejectsUnownedExistingContent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "artifacts")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unrelated.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveEvalArtifactRoot(root); err == nil || !strings.Contains(err.Error(), "not owned") {
		t.Fatalf("resolveEvalArtifactRoot() error = %v, want unowned-root rejection", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("artifact root permissions changed to %o", info.Mode().Perm())
	}
}

// TestMarshalRedactedEvaluationKeepsSecretsOutOfTerminalJSON applies the retained-artifact boundary to command output.
func TestMarshalRedactedEvaluationKeepsSecretsOutOfTerminalJSON(t *testing.T) {
	secret := "terminal-secret-value"
	body, err := marshalRedactedEvaluation(map[string]any{"error": "token=" + secret}, eval.NewRedactor([]string{secret}))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte(secret)) || !bytes.Contains(body, []byte("[REDACTED]")) {
		t.Fatalf("redacted terminal JSON = %s", body)
	}
}

// TestEvaluationEnvironmentUsesPrivateCachesAndCopiedForj keeps host workspaces and the source executable outside agent ownership.
func TestEvaluationEnvironmentUsesPrivateCachesAndCopiedForj(t *testing.T) {
	root := t.TempDir()
	hostTools := filepath.Join(root, "host-tools")
	if err := os.Mkdir(hostTools, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range append([]string{"go"}, evaluationSupportToolNames()...) {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		if err := os.WriteFile(filepath.Join(hostTools, name), []byte(name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", hostTools)
	t.Setenv("ATLAS_EVAL_SECRET", "must-not-leak")
	t.Setenv("GOENV", filepath.Join(root, "host-go-env"))
	t.Setenv("GOFLAGS", "-modfile=host.mod")
	t.Setenv("GOPROXY", "https://credential@private.example.invalid")
	t.Setenv("GOPRIVATE", "private.example.invalid")
	source := filepath.Join(root, "source-forj")
	if err := os.WriteFile(source, []byte("candidate"), 0o700); err != nil {
		t.Fatal(err)
	}
	environment, err := evaluationEnvironment(filepath.Join(root, "attempt"), source)
	if err != nil {
		t.Fatal(err)
	}
	values := environmentValues(environment)
	for key, suffix := range map[string]string{"GOCACHE": "gocache", "GOMODCACHE": "gomodcache", "GOPATH": "gopath", "HOME": "home"} {
		if values[key] != filepath.Join(root, "attempt", suffix) {
			t.Fatalf("%s = %q", key, values[key])
		}
	}
	if values["GOWORK"] != "off" {
		t.Fatalf("GOWORK = %q", values["GOWORK"])
	}
	if !strings.HasPrefix(values["ATLAS_EVAL_TOOLS_DIGEST"], "sha256:") {
		t.Fatalf("ATLAS_EVAL_TOOLS_DIGEST = %q", values["ATLAS_EVAL_TOOLS_DIGEST"])
	}
	for key, want := range map[string]string{"GOENV": "off", "GOPROXY": "https://proxy.golang.org,direct", "GOSUMDB": "sum.golang.org", "GOTOOLCHAIN": "local"} {
		if values[key] != want {
			t.Fatalf("%s = %q, want %q", key, values[key], want)
		}
	}
	for _, key := range []string{"GOFLAGS", "GOPRIVATE"} {
		if _, leaked := values[key]; leaked {
			t.Fatalf("ambient %s reached the evaluation process", key)
		}
	}
	if _, leaked := values["ATLAS_EVAL_SECRET"]; leaked {
		t.Fatal("ambient credential-like environment reached the evaluation process")
	}
	toolsDir := filepath.Join(root, "attempt", "tools")
	if !strings.HasPrefix(values["PATH"], toolsDir+string(os.PathListSeparator)) {
		t.Fatalf("PATH = %q", values["PATH"])
	}
	name := "forj"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	copyPath := filepath.Join(toolsDir, name)
	if err := os.Chmod(copyPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copyPath, []byte("mutated"), 0o500); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "candidate" {
		t.Fatalf("source executable changed through private copy: %q", body)
	}
	for _, tool := range evaluationSupportToolNames() {
		if runtime.GOOS == "windows" {
			tool += ".exe"
		}
		if _, err := os.Stat(filepath.Join(toolsDir, tool)); err != nil {
			t.Fatalf("snapshotted tool %q: %v", tool, err)
		}
	}
}

// TestEvaluationSupportToolsIncludeCodexInterpreter keeps script-based Codex launchers usable inside the fixed tool snapshot.
func TestEvaluationSupportToolsIncludeCodexInterpreter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Windows Codex launcher does not use an env-resolved Node interpreter")
	}
	if !slices.Contains(evaluationSupportToolNames(), "node") {
		t.Fatal("evaluation support tools omit the Codex Node interpreter")
	}
}

// TestRemoveEvaluationWorkRootHandlesReadOnlyModuleDirectories keeps successful diagnostics from failing during cache cleanup.
func TestRemoveEvaluationWorkRootHandlesReadOnlyModuleDirectories(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "owned")
	directory := filepath.Join(root, "gomodcache", "module")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte("module example.com/test\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(directory, 0o555); err != nil {
			t.Fatal(err)
		}
	}
	if err := removeEvaluationWorkRoot(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("evaluation root survived cleanup: %v", err)
	}
	if err := removeEvaluationWorkRoot(root); err != nil {
		t.Fatalf("idempotent cleanup: %v", err)
	}
}

// TestEvaluationRuntimeIdentityRecordsReconstructableComponents keeps retained attempts independent from deleted command workspaces.
func TestEvaluationRuntimeIdentityRecordsReconstructableComponents(t *testing.T) {
	identity := evaluationRuntimeIdentity()
	if identity.Framework.Module != "github.com/goforj/goforj" || identity.Framework.Version == "" || identity.Supervisor.Module != "github.com/goforj/atlas" || identity.Supervisor.Version == "" {
		t.Fatalf("runtime identity is incomplete: %#v", identity)
	}
	if identity.GoVersion == "" || identity.GOOS != runtime.GOOS || identity.GOARCH != runtime.GOARCH {
		t.Fatalf("Go runtime identity is incomplete: %#v", identity)
	}
}

// environmentValues converts one process environment into its effective key/value map for focused assertions.
func environmentValues(environment []string) map[string]string {
	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	return values
}

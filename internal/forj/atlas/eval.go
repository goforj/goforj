package atlas

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/goforj/atlas/eval"
	"github.com/goforj/atlas/eval/agent/codex"
	"github.com/goforj/atlas/eval/guidance"
	"github.com/goforj/atlas/eval/isolate"
	"github.com/goforj/atlas/eval/verify"
	"github.com/goforj/goforj/internal/forj/atlaseval"
	"github.com/goforj/goforj/version"
)

// EvalCmd groups opt-in live-agent evaluation commands.
type EvalCmd struct {
	Compare EvalCompareCmd `cmd:""`
}

// Signature returns the Kong metadata for EvalCmd.
func (*EvalCmd) Signature() string {
	return `name:"atlas:eval" help:"Run opt-in Atlas agent evaluations"`
}

// EvalCompareCmd runs the first diagnostic control-versus-AGENTS comparison.
type EvalCompareCmd struct {
	Evaluation      string `arg:"" help:"Promoted evaluation ID" default:"add-http-controller"`
	Model           string `help:"Exact Codex model identity" required:""`
	ModelProvider   string `name:"model-provider" help:"Codex model provider" default:"openai"`
	CodexExecutable string `name:"codex" help:"Codex executable or PATH name" default:"codex"`
	Credential      string `help:"Codex auth.json source; defaults to the current user configuration" type:"path"`
	Artifacts       string `help:"Supervisor-owned artifact directory" type:"path"`
}

// Signature returns the Kong metadata for EvalCompareCmd.
func (*EvalCompareCmd) Signature() string {
	return `name:"compare" help:"Compare no guidance with canonical AGENTS.md guidance"`
}

// Run executes two fresh diagnostic sessions and prints their machine-readable results.
func (command *EvalCompareCmd) Run() (runErr error) {
	definition, err := eval.LoadPromotedDefinition(command.Evaluation)
	if err != nil {
		return err
	}
	forjExecutable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve current Forj executable: %w", err)
	}
	goExecutable, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("resolve Go executable: %w", err)
	}
	credential, err := resolveEvalCredential(command.Credential)
	if err != nil {
		return err
	}
	artifactRoot, err := resolveEvalArtifactRoot(command.Artifacts)
	if err != nil {
		return err
	}
	artifactKey, err := loadOrCreateEvalArtifactKey(artifactRoot)
	if err != nil {
		return err
	}
	workRoot, err := os.MkdirTemp("", "goforj-atlas-eval-")
	if err != nil {
		return err
	}
	defer func() {
		runErr = errors.Join(runErr, removeEvaluationWorkRoot(workRoot))
	}()
	for _, directory := range []string{"backend", "verifier"} {
		if err := os.MkdirAll(filepath.Join(workRoot, directory), 0o700); err != nil {
			return err
		}
	}
	noneEnvironment, err := evaluationEnvironment(filepath.Join(workRoot, eval.GuidanceProfileNone), forjExecutable)
	if err != nil {
		return err
	}
	agentsEnvironment, err := evaluationEnvironment(filepath.Join(workRoot, eval.GuidanceProfileAgents), forjExecutable)
	if err != nil {
		return err
	}
	verifierEnvironment := append([]string(nil), noneEnvironment...)
	verifierCommands := isolate.VerifierCommands{
		WorkRoot:       filepath.Join(workRoot, "verifier"),
		GoExecutable:   goExecutable,
		ForjExecutable: forjExecutable,
		Environment:    verifierEnvironment,
	}
	registry, err := eval.NewRegistry(eval.PromotedWorkflows(), verify.Promoted(verifierCommands))
	if err != nil {
		return err
	}
	agent, err := codex.New(codex.Options{
		Executable:       command.CodexExecutable,
		Model:            command.Model,
		ModelProvider:    command.ModelProvider,
		CredentialSource: credential,
	})
	if err != nil {
		return err
	}
	artifacts, err := eval.NewArtifactStore(artifactRoot, artifactKey, eval.NewRedactor(nil))
	if err != nil {
		return err
	}
	preparer := atlaseval.NewPreparer(filepath.Join(workRoot, "bases"), nil)
	defer func() {
		runErr = errors.Join(runErr, preparer.Close(context.Background()))
	}()
	runner := eval.Runner{
		Registry:  registry,
		Preparer:  preparer,
		Backend:   isolate.UnconfinedLocal{WorkRoot: filepath.Join(workRoot, "backend")},
		Agent:     agent,
		Guidance:  guidance.ProjectResolver{},
		Artifacts: artifacts,
	}
	trialID, err := newEvaluationTrialID()
	if err != nil {
		return err
	}
	result, diagnosticErr := runner.RunGuidanceDiagnostic(context.Background(), eval.GuidanceDiagnosticRequest{
		LogicalTrialID:  trialID,
		Definition:      definition,
		DestinationRoot: filepath.Join(workRoot, "projects"),
		ForjExecutable:  forjExecutable,
		Environments: map[string][]string{
			eval.GuidanceProfileNone:   noneEnvironment,
			eval.GuidanceProfileAgents: agentsEnvironment,
		},
		Runtime: evaluationRuntimeIdentity(),
	})
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	fmt.Printf("Artifacts: %s\n", artifactRoot)
	var attemptErrors []error
	for _, attempt := range result.Attempts {
		if attempt.Error != "" {
			attemptErrors = append(attemptErrors, fmt.Errorf("%s treatment: %s", attempt.Profile, attempt.Error))
		}
	}
	if diagnosticErr != nil {
		attemptErrors = append(attemptErrors, diagnosticErr)
	}
	return errors.Join(attemptErrors...)
}

// evaluationRuntimeIdentity makes retained diagnostics reconstructable after command-owned binaries and workspaces are removed.
func evaluationRuntimeIdentity() eval.RuntimeIdentity {
	return eval.RuntimeIdentity{
		Supervisor: atlasSoftwareIdentity(),
		Framework: eval.SoftwareIdentity{
			Module:  "github.com/goforj/goforj",
			Version: version.Version,
			Commit:  version.Commit,
			Dirty:   version.Dirty,
		},
		GoVersion: runtime.Version(),
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
	}
}

// atlasSoftwareIdentity reports the exact dependency selected into the current GoForj binary.
func atlasSoftwareIdentity() eval.SoftwareIdentity {
	identity := eval.SoftwareIdentity{Module: "github.com/goforj/atlas", Version: "unknown"}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return identity
	}
	for _, dependency := range info.Deps {
		if dependency.Path != identity.Module {
			continue
		}
		identity.Version = dependency.Version
		if dependency.Replace != nil {
			replacement := dependency.Replace.Version
			if replacement == "" {
				replacement = "local"
			}
			identity.Version += " => " + replacement
		}
		return identity
	}
	return identity
}

// removeEvaluationWorkRoot restores traversal only inside the command-owned root so read-only module caches remain disposable.
func removeEvaluationWorkRoot(root string) error {
	if _, err := os.Lstat(root); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o700)
		}
		return nil
	})
	return errors.Join(walkErr, os.RemoveAll(root))
}

// resolveEvalCredential uses only the caller's existing Codex credential as the source for private per-attempt copies.
func resolveEvalCredential(candidate string) (string, error) {
	if strings.TrimSpace(candidate) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		candidate = filepath.Join(home, ".codex", "auth.json")
	}
	path, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("Codex credential %q is not a regular file", path)
	}
	return path, nil
}

// resolveEvalArtifactRoot selects a retained private directory outside every disposable agent Project.
func resolveEvalArtifactRoot(candidate string) (string, error) {
	if strings.TrimSpace(candidate) == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		candidate = filepath.Join(cache, "goforj", "atlas-evaluations")
	}
	path, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", fmt.Errorf("create evaluation artifact root: %w", err)
	}
	return path, nil
}

// loadOrCreateEvalArtifactKey keeps retained manifests verifiable across diagnostic commands.
func loadOrCreateEvalArtifactKey(root string) ([]byte, error) {
	path := filepath.Join(root, ".manifest-key")
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != 32 {
			return nil, fmt.Errorf("evaluation artifact key has invalid length")
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			existing, readErr := os.ReadFile(path)
			if readErr == nil && len(existing) == 32 {
				return existing, nil
			}
		}
		return nil, err
	}
	_, writeErr := file.Write(key)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return nil, errors.Join(err, os.Remove(path))
	}
	return key, nil
}

// evaluationEnvironment gives both treatments identical private Go caches and disables workspace leakage.
func evaluationEnvironment(workRoot, forjExecutable string) ([]string, error) {
	goCache := filepath.Join(workRoot, "gocache")
	moduleCache := filepath.Join(workRoot, "gomodcache")
	goPath := filepath.Join(workRoot, "gopath")
	home := filepath.Join(workRoot, "home")
	toolsDir := filepath.Join(workRoot, "tools")
	if err := errors.Join(os.MkdirAll(goCache, 0o700), os.MkdirAll(moduleCache, 0o700), os.MkdirAll(goPath, 0o700), os.MkdirAll(home, 0o700), os.MkdirAll(toolsDir, 0o700)); err != nil {
		return nil, err
	}
	if err := installEvaluationForj(toolsDir, forjExecutable); err != nil {
		return nil, err
	}
	overrides := map[string]string{
		"GOCACHE":    goCache,
		"GOMODCACHE": moduleCache,
		"GOPATH":     goPath,
		"GOWORK":     "off",
		"HOME":       home,
		"PATH":       toolsDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	base := baseEvaluationEnvironment()
	environment := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, replaced := overrides[key]; replaced {
				continue
			}
		}
		environment = append(environment, entry)
	}
	for key, value := range overrides {
		environment = append(environment, key+"="+value)
	}
	return environment, nil
}

// baseEvaluationEnvironment retains only process settings needed by portable tools and excludes ambient credentials and user configuration.
func baseEvaluationEnvironment() []string {
	keys := []string{
		"PATH",
		"TMPDIR", "TMP", "TEMP",
		"SystemRoot", "WINDIR", "COMSPEC", "PATHEXT",
		"LANG", "LC_ALL", "LC_CTYPE", "TERM", "COLORTERM",
		"SSL_CERT_FILE", "SSL_CERT_DIR",
		"GOPROXY", "GOSUMDB", "GONOPROXY", "GOPRIVATE", "GOENV", "GOFLAGS", "CGO_ENABLED", "CC", "CXX",
	}
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			environment = append(environment, key+"="+value)
		}
	}
	return environment
}

// installEvaluationForj gives the agent one attempt-private PATH entry for the exact candidate binary.
func installEvaluationForj(toolsDir, source string) error {
	name := "forj"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	destination := filepath.Join(toolsDir, name)
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o500)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	return errors.Join(copyErr, closeErr)
}

// newEvaluationTrialID creates a sortable safe identifier without treating wall-clock time as unique.
func newEvaluationTrialID() (string, error) {
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return fmt.Sprintf("trial-%s-%s", time.Now().UTC().Format("20060102t150405"), hex.EncodeToString(random)), nil
}

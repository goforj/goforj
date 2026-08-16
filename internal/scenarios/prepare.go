package scenarios

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// PrepareOptions controls creation of a live-agent starting workspace.
type PrepareOptions struct {
	Logger             *logger.AppLogger
	SpecDir            string
	WorkDir            string
	Keep               bool
	ScenarioID         string
	ForjExec           string
	Environment        []string
	ExpectedPlanDigest string
	ExpectedToolDigest string
}

// ResolveOptions identifies a live scenario without granting permission to mutate a workspace.
type ResolveOptions struct {
	SpecDir    string
	ScenarioID string
}

// ResolvedPreparation records the immutable scenario prefix and Project configuration selected before execution.
type ResolvedPreparation struct {
	ScenarioID        string
	SchemaVersion     int
	PlanDigest        string
	CatalogDigest     string
	ScenarioDigests   []ScenarioSourceDigest
	ProjectConfig     project.Config
	ProjectConfigYAML []byte
}

// ScenarioSourceDigest identifies one exact scenario source in a prepared dependency closure.
type ScenarioSourceDigest struct {
	ID     string
	Digest string
}

// PreparedScenario owns a live-evaluable Project until the caller closes it.
type PreparedScenario struct {
	Root            string
	ScenarioID      string
	SchemaVersion   int
	ProjectConfig   project.Config
	CatalogDigest   string
	ScenarioDigests []ScenarioSourceDigest
	ForjExecutable  string
	ForjDigest      string
	ToolDigest      string
	PlanDigest      string
	BaselineTree    string
	workspace       scenarioWorkspace
}

// ErrUnsupportedLiveScenario identifies a legacy scenario that has no trustworthy preparation boundary.
var ErrUnsupportedLiveScenario = errors.New("unsupported_live_scenario")

// ResolvePreparation authenticates one live preparation prefix without creating files or running commands.
func ResolvePreparation(options ResolveOptions) (ResolvedPreparation, error) {
	if strings.TrimSpace(options.ScenarioID) == "" {
		return ResolvedPreparation{}, fmt.Errorf("scenario ID is required")
	}
	catalog, err := loadScenarioCatalog(options.SpecDir)
	if err != nil {
		return ResolvedPreparation{}, err
	}
	plan, ok := catalog.plans[options.ScenarioID]
	if !ok {
		return ResolvedPreparation{}, fmt.Errorf("unknown scenario %q", options.ScenarioID)
	}
	if plan.spec.SchemaVersion != liveScenarioSchemaVersion {
		return ResolvedPreparation{}, fmt.Errorf("%w: scenario %q must declare schema_version %d", ErrUnsupportedLiveScenario, plan.spec.ID, liveScenarioSchemaVersion)
	}
	config := scenarioProjectConfig(plan.spec)
	configYAML, err := yaml.Marshal(config)
	if err != nil {
		return ResolvedPreparation{}, fmt.Errorf("marshal scenario Project configuration: %w", err)
	}
	digests := preparationScenarioDigests(catalog, plan)
	return ResolvedPreparation{
		ScenarioID:        plan.spec.ID,
		SchemaVersion:     plan.spec.SchemaVersion,
		PlanDigest:        digestPreparationPlan(plan.spec.ID, plan.spec.SchemaVersion, digests),
		CatalogDigest:     catalog.catalogDigest,
		ScenarioDigests:   digests,
		ProjectConfig:     config,
		ProjectConfigYAML: configYAML,
	}, nil
}

// Prepare materializes and verifies only the starting-state prefix of one v2 scenario.
func Prepare(ctx context.Context, options PrepareOptions) (*PreparedScenario, error) {
	if options.Logger == nil {
		return nil, fmt.Errorf("scenario logger is required")
	}
	if strings.TrimSpace(options.ScenarioID) == "" {
		return nil, fmt.Errorf("scenario ID is required")
	}
	if strings.TrimSpace(options.ForjExec) == "" {
		return nil, fmt.Errorf("forj executable is required")
	}
	if options.Environment == nil {
		return nil, fmt.Errorf("scenario environment is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	forjExecutable, forjDigest, err := resolveScenarioExecutable(options.ForjExec)
	if err != nil {
		return nil, err
	}
	catalog, err := loadScenarioCatalog(options.SpecDir)
	if err != nil {
		return nil, err
	}
	plan, ok := catalog.plans[options.ScenarioID]
	if !ok {
		return nil, fmt.Errorf("unknown scenario %q", options.ScenarioID)
	}
	if plan.spec.SchemaVersion != liveScenarioSchemaVersion {
		return nil, fmt.Errorf("%w: scenario %q must declare schema_version %d", ErrUnsupportedLiveScenario, plan.spec.ID, liveScenarioSchemaVersion)
	}
	tools, toolDigest, err := resolveScenarioPlanTools(forjExecutable, options.Environment, plan, false)
	if err != nil {
		return nil, err
	}
	if options.ExpectedToolDigest != "" && options.ExpectedToolDigest != toolDigest {
		return nil, fmt.Errorf("resolved scenario tools changed: got %s, want %s", toolDigest, options.ExpectedToolDigest)
	}
	digests := preparationScenarioDigests(catalog, plan)
	planDigest := digestPreparationPlan(plan.spec.ID, plan.spec.SchemaVersion, digests)
	if options.ExpectedPlanDigest != "" && options.ExpectedPlanDigest != planDigest {
		return nil, fmt.Errorf("resolved preparation plan changed: got %s, want %s", planDigest, options.ExpectedPlanDigest)
	}

	workspace, err := createScenarioWorkspace(ValidateOptions{WorkDir: options.WorkDir, Keep: options.Keep}, plan.spec)
	if err != nil {
		return nil, err
	}
	execution := scenarioExecution{
		context:     ctx,
		logger:      options.Logger,
		workspace:   workspace,
		forjExec:    forjExecutable,
		tools:       tools,
		toolDigest:  toolDigest,
		environment: append([]string(nil), options.Environment...),
	}
	execution, err = execution.snapshotTools()
	if err != nil {
		return nil, workspace.cleanupAfter(err)
	}
	if err := execution.prepare(plan); err != nil {
		return nil, workspace.cleanupAfter(err)
	}

	config := scenarioProjectConfig(plan.spec)
	baselineTree, err := digestScenarioTree(workspace.root)
	if err != nil {
		return nil, workspace.cleanupAfter(err)
	}
	return &PreparedScenario{
		Root:            workspace.root,
		ScenarioID:      plan.spec.ID,
		SchemaVersion:   plan.spec.SchemaVersion,
		ProjectConfig:   config,
		CatalogDigest:   catalog.catalogDigest,
		ScenarioDigests: digests,
		ForjExecutable:  forjExecutable,
		ForjDigest:      forjDigest,
		ToolDigest:      toolDigest,
		PlanDigest:      planDigest,
		BaselineTree:    baselineTree,
		workspace:       workspace,
	}, nil
}

// ResolveScenarioPreparationTools binds every prefix executable to an identity selected before cached preparation.
func ResolveScenarioPreparationTools(forjExecutable string, environment []string, options ResolveOptions) (map[string]string, string, error) {
	forjPath, _, err := resolveScenarioExecutable(forjExecutable)
	if err != nil {
		return nil, "", err
	}
	catalog, err := loadScenarioCatalog(options.SpecDir)
	if err != nil {
		return nil, "", err
	}
	plan, ok := catalog.plans[options.ScenarioID]
	if !ok {
		return nil, "", fmt.Errorf("unknown scenario %q", options.ScenarioID)
	}
	if plan.spec.SchemaVersion != liveScenarioSchemaVersion {
		return nil, "", fmt.Errorf("%w: scenario %q must declare schema_version %d", ErrUnsupportedLiveScenario, plan.spec.ID, liveScenarioSchemaVersion)
	}
	return resolveScenarioPlanTools(forjPath, environment, plan, false)
}

// resolveScenarioPlanTools fingerprints every executable that the selected plan may invoke.
func resolveScenarioPlanTools(forjPath string, environment []string, plan scenarioPlan, includeTarget bool) (map[string]string, string, error) {
	// GoForj commands can invoke Go and Wire internally, so their bytes must be
	// bound even when those tools do not appear as top-level scenario commands.
	names := map[string]bool{"forj": true, "go": true, "wire": true}
	collect := func(steps []plannedScenarioStep) {
		for _, planned := range steps {
			if planned.step.Run != nil && len(planned.step.Run.Run) > 0 {
				names[planned.step.Run.Run[0]] = true
			}
		}
	}
	collect(plan.dependencySteps)
	collect(plan.preparationSteps)
	for _, command := range plan.startingChecks {
		names[command.Run[0]] = true
	}
	if includeTarget {
		collect(plan.targetSteps)
		for _, command := range plan.finalChecks {
			names[command.Run[0]] = true
		}
	}
	tools := make(map[string]string, len(names))
	digests := make(map[string]string, len(names))
	for name := range names {
		var toolPath, digest string
		var err error
		if name == "forj" {
			toolPath, digest, err = resolveScenarioExecutable(forjPath)
		} else {
			toolPath, digest, err = resolveScenarioTool(name, environment)
		}
		if err != nil {
			return nil, "", err
		}
		tools[name] = toolPath
		digests[name] = digest
	}
	return tools, digestScenarioTools(digests), nil
}

// preparationScenarioDigests records the complete dependency closure followed by the selected target scenario.
func preparationScenarioDigests(catalog scenarioCatalog, plan scenarioPlan) []ScenarioSourceDigest {
	digests := make([]ScenarioSourceDigest, 0, len(plan.dependencyScenarioIDs)+1)
	for _, id := range append(append([]string(nil), plan.dependencyScenarioIDs...), plan.spec.ID) {
		digests = append(digests, ScenarioSourceDigest{ID: id, Digest: catalog.digests[id]})
	}
	return digests
}

// digestPreparationPlan binds the selected scenario and its complete source closure without unrelated catalog entries.
func digestPreparationPlan(scenarioID string, schemaVersion int, digests []ScenarioSourceDigest) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "%s\x00%d\x00", scenarioID, schemaVersion)
	for _, digest := range digests {
		fmt.Fprintf(hash, "%s\x00%s\x00", digest.ID, digest.Digest)
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil))
}

// resolveScenarioExecutable records the exact binary rather than trusting later PATH resolution.
func resolveScenarioExecutable(candidate string) (string, string, error) {
	path, err := exec.LookPath(candidate)
	if err != nil {
		return "", "", fmt.Errorf("resolve forj executable %q: %w", candidate, err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("resolve absolute forj executable path: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", "", fmt.Errorf("stat forj executable %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("forj executable %q is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", "", fmt.Errorf("open forj executable %q: %w", path, err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", "", fmt.Errorf("digest forj executable %q: %w", path, err)
	}
	return path, fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
}

// ResolveExecutable returns the exact regular-file identity used by live scenario preparation.
func ResolveExecutable(candidate string) (string, string, error) {
	return resolveScenarioExecutable(candidate)
}

// resolveScenarioTool resolves one PATH-selected regular executable from the supplied process environment.
func resolveScenarioTool(name string, environment []string) (string, string, error) {
	pathValue := ""
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if ok && scenarioEnvironmentKey(key) == scenarioEnvironmentKey("PATH") {
			pathValue = value
			break
		}
	}
	if pathValue == "" {
		return "", "", fmt.Errorf("resolve tool %q: PATH is required", name)
	}
	for _, directory := range filepath.SplitList(pathValue) {
		for _, fileName := range scenarioToolFileNames(name, environment) {
			candidate := filepath.Join(directory, fileName)
			path, digest, err := resolveScenarioExecutable(candidate)
			if err == nil {
				return path, digest, nil
			}
		}
	}
	return "", "", fmt.Errorf("resolve tool %q from PATH", name)
}

// scenarioToolFileNames applies the supplied Windows executable policy without consulting mutable process state.
func scenarioToolFileNames(name string, environment []string) []string {
	if runtime.GOOS != "windows" || filepath.Ext(name) != "" {
		return []string{name}
	}
	extensions := scenarioEnvironmentMap(environment)[scenarioEnvironmentKey("PATHEXT")]
	if extensions == "" {
		extensions = ".COM;.EXE;.BAT;.CMD"
	}
	names := make([]string, 0, 4)
	for _, extension := range strings.Split(extensions, ";") {
		extension = strings.TrimSpace(extension)
		if extension != "" {
			names = append(names, name+extension)
		}
	}
	return names
}

// Close releases the temporary Project when the caller did not request retention.
func (prepared *PreparedScenario) Close() error {
	if prepared == nil {
		return nil
	}
	return prepared.workspace.cleanupAfter(nil)
}

// ClonePrepared copies one verified starting state into a separately owned trial workspace.
func ClonePrepared(prepared *PreparedScenario, workDir string) (*PreparedScenario, error) {
	if prepared == nil || strings.TrimSpace(prepared.Root) == "" {
		return nil, fmt.Errorf("prepared scenario is required")
	}
	if strings.TrimSpace(workDir) == "" {
		return nil, fmt.Errorf("clone work root is required")
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("create clone work root: %w", err)
	}
	root, err := os.MkdirTemp(workDir, prepared.ScenarioID+"-")
	if err != nil {
		return nil, fmt.Errorf("create cloned scenario workspace: %w", err)
	}
	if err := copyScenarioTree(prepared.Root, root); err != nil {
		return nil, errors.Join(err, removeScenarioTree(root))
	}
	digest, err := digestScenarioTree(root)
	if err != nil {
		return nil, errors.Join(err, removeScenarioTree(root))
	}
	if digest != prepared.BaselineTree {
		return nil, errors.Join(fmt.Errorf("cloned scenario tree digest changed: got %s, want %s", digest, prepared.BaselineTree), removeScenarioTree(root))
	}
	clone := *prepared
	clone.Root = root
	clone.ScenarioDigests = append([]ScenarioSourceDigest(nil), prepared.ScenarioDigests...)
	clone.workspace = scenarioWorkspace{root: root, removeAfter: true}
	return &clone, nil
}

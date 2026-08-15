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
	"strings"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

// PrepareOptions controls creation of a live-agent starting workspace.
type PrepareOptions struct {
	Logger      *logger.AppLogger
	SpecDir     string
	WorkDir     string
	Keep        bool
	ScenarioID  string
	ForjExec    string
	Environment []string
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
	workspace       scenarioWorkspace
}

// ErrUnsupportedLiveScenario identifies a legacy scenario that has no trustworthy preparation boundary.
var ErrUnsupportedLiveScenario = errors.New("unsupported_live_scenario")

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

	workspace, err := createScenarioWorkspace(ValidateOptions{WorkDir: options.WorkDir, Keep: options.Keep}, plan.spec)
	if err != nil {
		return nil, err
	}
	execution := scenarioExecution{
		context:     ctx,
		logger:      options.Logger,
		workspace:   workspace,
		forjExec:    forjExecutable,
		environment: append([]string(nil), options.Environment...),
	}
	if err := execution.prepare(plan); err != nil {
		return nil, workspace.cleanupAfter(err)
	}

	config := scenarioProjectConfig(plan.spec)
	digests := make([]ScenarioSourceDigest, 0, len(plan.dependencyScenarioIDs)+1)
	for _, id := range append(append([]string(nil), plan.dependencyScenarioIDs...), plan.spec.ID) {
		digests = append(digests, ScenarioSourceDigest{ID: id, Digest: catalog.digests[id]})
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
		workspace:       workspace,
	}, nil
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

// Close releases the temporary Project when the caller did not request retention.
func (prepared *PreparedScenario) Close() error {
	if prepared == nil {
		return nil
	}
	return prepared.workspace.cleanupAfter(nil)
}

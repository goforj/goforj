// Package atlaseval adapts GoForj's scenario catalog to Atlas evaluation contracts.
package atlaseval

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/goforj/atlas/eval"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/scenarios"
)

// Preparer resolves and materializes trusted GoForj scenario prefixes for Atlas.
type Preparer struct {
	SpecDir         string
	Logger          *logger.AppLogger
	BaseRoot        string
	BaseEnvironment []string
	cache           *preparationCache
}

// preparationCache owns immutable command-local bases shared only by paired trial clones.
type preparationCache struct {
	mu     sync.Mutex
	bases  map[string]*scenarios.PreparedScenario
	closed bool
}

// NewPreparer enables command-local immutable-base reuse under an explicitly owned root.
func NewPreparer(baseRoot string, baseEnvironment []string, appLogger *logger.AppLogger) *Preparer {
	return &Preparer{
		BaseRoot:        baseRoot,
		BaseEnvironment: append([]string(nil), baseEnvironment...),
		Logger:          appLogger,
		cache:           &preparationCache{bases: map[string]*scenarios.PreparedScenario{}},
	}
}

// Capabilities reports the live scenario schema supported by this GoForj build.
func (preparer Preparer) Capabilities(context.Context) (eval.PreparationCapabilities, error) {
	return eval.PreparationCapabilities{ScenarioSchemaVersions: []int{2}}, nil
}

// Resolve authenticates a scenario prefix without creating a Project or running its commands.
func (preparer Preparer) Resolve(_ context.Context, request eval.PreparationRequest) (eval.ResolvedPreparationPlan, error) {
	if err := validatePreparationRequest(request); err != nil {
		return eval.ResolvedPreparationPlan{}, err
	}
	_, forjDigest, err := scenarios.ResolveExecutable(request.ForjExecutable)
	if err != nil {
		return eval.ResolvedPreparationPlan{}, err
	}
	resolved, err := scenarios.ResolvePreparation(scenarios.ResolveOptions{
		SpecDir:    preparer.SpecDir,
		ScenarioID: request.ScenarioID,
	})
	if err != nil {
		return eval.ResolvedPreparationPlan{}, err
	}
	environmentDigest := preparationEnvironmentDigest(request.Environment)
	return eval.ResolvedPreparationPlan{
		ResolutionID:         request.OrchestrationID,
		ScenarioID:           resolved.ScenarioID,
		ScenarioSchema:       resolved.SchemaVersion,
		PlanDigest:           preparationPlanDigest(resolved.PlanDigest, forjDigest, environmentDigest),
		ScenarioPlanDigest:   resolved.PlanDigest,
		CatalogDigest:        resolved.CatalogDigest,
		ForjDigest:           forjDigest,
		EnvironmentDigest:    environmentDigest,
		DependencyDigests:    scenarioDigestMap(resolved.ScenarioDigests),
		ProjectConfiguration: append([]byte(nil), resolved.ProjectConfigYAML...),
		TargetOmitted:        true,
	}, nil
}

// Prepare materializes the resolved starting state while rejecting request or catalog drift.
func (preparer Preparer) Prepare(ctx context.Context, request eval.PreparationRequest, plan eval.ResolvedPreparationPlan) (eval.PreparedProject, error) {
	if err := validatePreparationRequest(request); err != nil {
		return nil, err
	}
	if err := validatePreparationPlan(request, plan); err != nil {
		return nil, err
	}
	current, err := preparer.Resolve(ctx, request)
	if err != nil {
		return nil, err
	}
	if !samePreparationPlan(plan, current) {
		return nil, fmt.Errorf("resolved preparation inputs changed before Project mutation")
	}
	prepared, err := preparer.prepareScenario(ctx, request, plan)
	if err != nil {
		return nil, err
	}
	result := eval.PreparationResult{
		ResolutionID:   plan.ResolutionID,
		ProjectRoot:    prepared.Root,
		ScenarioID:     prepared.ScenarioID,
		ScenarioSchema: prepared.SchemaVersion,
		PlanDigest:     plan.PlanDigest,
		CatalogDigest:  prepared.CatalogDigest,
		BaselineTree:   prepared.BaselineTree,
		ForjExecutable: prepared.ForjExecutable,
		ForjDigest:     prepared.ForjDigest,
		OwnedPaths:     []string{prepared.Root},
	}
	if prepared.PlanDigest != plan.ScenarioPlanDigest || result.ForjDigest != plan.ForjDigest || result.CatalogDigest != plan.CatalogDigest || !sameScenarioDigests(plan.DependencyDigests, prepared.ScenarioDigests) {
		return nil, closePreparedAfter(prepared, fmt.Errorf("resolved scenario catalog changed before preparation completed"))
	}
	return &preparedProject{prepared: prepared, result: result}, nil
}

// Close removes every immutable base retained for this command.
func (preparer *Preparer) Close(context.Context) error {
	if preparer == nil || preparer.cache == nil {
		return nil
	}
	preparer.cache.mu.Lock()
	defer preparer.cache.mu.Unlock()
	if preparer.cache.closed {
		return nil
	}
	preparer.cache.closed = true
	var cleanupErrors []error
	for _, base := range preparer.cache.bases {
		cleanupErrors = append(cleanupErrors, base.Close())
	}
	preparer.cache.bases = nil
	return errors.Join(cleanupErrors...)
}

// prepareScenario either materializes directly or clones one command-local verified base.
func (preparer Preparer) prepareScenario(ctx context.Context, request eval.PreparationRequest, plan eval.ResolvedPreparationPlan) (*scenarios.PreparedScenario, error) {
	if preparer.cache == nil {
		return preparer.materializeScenario(ctx, request, plan, request.DestinationRoot, false)
	}
	if strings.TrimSpace(preparer.BaseRoot) == "" {
		return nil, fmt.Errorf("evaluation base root is required")
	}
	preparer.cache.mu.Lock()
	defer preparer.cache.mu.Unlock()
	if preparer.cache.closed {
		return nil, fmt.Errorf("evaluation preparer is closed")
	}
	key := plan.PlanDigest
	base := preparer.cache.bases[key]
	if base == nil {
		var err error
		baseRequest := request
		baseRequest.Environment = append([]string(nil), preparer.BaseEnvironment...)
		if baseRequest.Environment == nil {
			return nil, fmt.Errorf("evaluation base environment is required")
		}
		if preparationEnvironmentDigest(baseRequest.Environment) != plan.EnvironmentDigest {
			return nil, fmt.Errorf("evaluation base environment does not match the resolved material environment")
		}
		base, err = preparer.materializeScenario(ctx, baseRequest, plan, preparer.BaseRoot, false)
		if err != nil {
			return nil, err
		}
		preparer.cache.bases[key] = base
	}
	return scenarios.ClonePrepared(base, request.DestinationRoot)
}

// preparationPlanDigest binds the scenario prefix to the exact executable and material environment selected before mutation.
func preparationPlanDigest(scenarioPlanDigest, forjDigest, environmentDigest string) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "scenario\x00%s\x00forj\x00%s\x00environment\x00%s\x00", scenarioPlanDigest, forjDigest, environmentDigest)
	return fmt.Sprintf("sha256:%x", hash.Sum(nil))
}

// preparationEnvironmentDigest excludes attempt-private tool paths while binding every value that can change the rendered base.
func preparationEnvironmentDigest(environment []string) string {
	ignored := map[string]bool{
		"GOCACHE":    true,
		"GOMODCACHE": true,
		"GOPATH":     true,
		"GOWORK":     true,
		"HOME":       true,
		"PATH":       true,
	}
	values := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, ok := strings.Cut(entry, "=")
		if ok && ignored[key] {
			continue
		}
		values = append(values, entry)
	}
	sort.Strings(values)
	hash := sha256.New()
	for _, value := range values {
		fmt.Fprintf(hash, "%s\x00", value)
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil))
}

// samePreparationPlan compares every execution-bearing identity without relying on map iteration order.
func samePreparationPlan(left, right eval.ResolvedPreparationPlan) bool {
	if left.ResolutionID != right.ResolutionID || left.ScenarioID != right.ScenarioID || left.ScenarioSchema != right.ScenarioSchema || left.PlanDigest != right.PlanDigest || left.ScenarioPlanDigest != right.ScenarioPlanDigest || left.CatalogDigest != right.CatalogDigest || left.ForjDigest != right.ForjDigest || left.EnvironmentDigest != right.EnvironmentDigest || left.TargetOmitted != right.TargetOmitted || string(left.ProjectConfiguration) != string(right.ProjectConfiguration) {
		return false
	}
	if len(left.DependencyDigests) != len(right.DependencyDigests) {
		return false
	}
	for id, digest := range left.DependencyDigests {
		if right.DependencyDigests[id] != digest {
			return false
		}
	}
	return true
}

// materializeScenario applies the trusted prefix once before any candidate receives a clone.
func (preparer Preparer) materializeScenario(ctx context.Context, request eval.PreparationRequest, plan eval.ResolvedPreparationPlan, workDir string, keep bool) (*scenarios.PreparedScenario, error) {
	appLogger := preparer.Logger
	if appLogger == nil {
		appLogger = logger.NewSilentLogger()
	}
	return scenarios.Prepare(ctx, scenarios.PrepareOptions{
		Logger:             appLogger,
		SpecDir:            preparer.SpecDir,
		WorkDir:            workDir,
		Keep:               keep,
		ScenarioID:         request.ScenarioID,
		ForjExec:           request.ForjExecutable,
		Environment:        append([]string(nil), request.Environment...),
		ExpectedPlanDigest: plan.ScenarioPlanDigest,
	})
}

// preparedProject binds Atlas result metadata to the GoForj workspace owner.
type preparedProject struct {
	prepared *scenarios.PreparedScenario
	result   eval.PreparationResult
}

// Result returns a defensive copy of the prepared Project identity.
func (project *preparedProject) Result() eval.PreparationResult {
	result := project.result
	result.OwnedPaths = append([]string(nil), project.result.OwnedPaths...)
	return result
}

// Close releases the GoForj-owned scenario workspace.
func (project *preparedProject) Close(context.Context) error {
	if project == nil || project.prepared == nil {
		return nil
	}
	return project.prepared.Close()
}

// validatePreparationRequest rejects ambiguous ownership and uncorrelated attempts before scenario resolution.
func validatePreparationRequest(request eval.PreparationRequest) error {
	if strings.TrimSpace(request.OrchestrationID) == "" {
		return fmt.Errorf("preparation orchestration ID is required")
	}
	if strings.TrimSpace(request.ScenarioID) == "" {
		return fmt.Errorf("preparation scenario ID is required")
	}
	if strings.TrimSpace(request.DestinationRoot) == "" {
		return fmt.Errorf("preparation destination root is required")
	}
	if !filepath.IsAbs(request.DestinationRoot) {
		return fmt.Errorf("preparation destination root must be absolute")
	}
	if strings.TrimSpace(request.ForjExecutable) == "" {
		return fmt.Errorf("preparation Forj executable is required")
	}
	if request.Environment == nil {
		return fmt.Errorf("preparation environment is required")
	}
	return nil
}

// validatePreparationPlan binds execution to the exact request and resolved scenario prefix.
func validatePreparationPlan(request eval.PreparationRequest, plan eval.ResolvedPreparationPlan) error {
	if plan.ResolutionID != request.OrchestrationID {
		return fmt.Errorf("resolved preparation identity does not match the orchestration request")
	}
	if plan.ScenarioID != request.ScenarioID {
		return fmt.Errorf("resolved scenario %q does not match requested scenario %q", plan.ScenarioID, request.ScenarioID)
	}
	if plan.ScenarioSchema != 2 || plan.PlanDigest == "" || plan.ScenarioPlanDigest == "" || plan.CatalogDigest == "" || plan.ForjDigest == "" || plan.EnvironmentDigest == "" {
		return fmt.Errorf("resolved preparation plan is incomplete")
	}
	if !plan.TargetOmitted {
		return fmt.Errorf("resolved preparation plan does not prove target omission")
	}
	return nil
}

// scenarioDigestMap preserves source identity while making closure comparisons order-independent.
func scenarioDigestMap(digests []scenarios.ScenarioSourceDigest) map[string]string {
	result := make(map[string]string, len(digests))
	for _, digest := range digests {
		result[digest.ID] = digest.Digest
	}
	return result
}

// sameScenarioDigests rejects a changed dependency closure after the plan was resolved.
func sameScenarioDigests(expected map[string]string, actual []scenarios.ScenarioSourceDigest) bool {
	if len(expected) != len(actual) {
		return false
	}
	for _, digest := range actual {
		if expected[digest.ID] != digest.Digest {
			return false
		}
	}
	return true
}

// closePreparedAfter retains both provenance and workspace cleanup failures.
func closePreparedAfter(prepared *scenarios.PreparedScenario, cause error) error {
	if err := prepared.Close(); err != nil {
		return errors.Join(cause, fmt.Errorf("cleanup prepared Project: %w", err))
	}
	return cause
}

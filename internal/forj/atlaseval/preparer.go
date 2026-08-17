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
	"github.com/goforj/goforj/project"
)

// Preparer resolves and materializes trusted GoForj scenario prefixes for Atlas.
type Preparer struct {
	SpecDir         string
	Logger          *logger.AppLogger
	BaseRoot        string
	BaseEnvironment []string
	Guidance        GuidanceMaterializer
	cache           *preparationCache
	baseGate        chan struct{}
}

// GuidanceMaterializer applies one durable host-specific guidance selection and returns the exact native files visible to the agent.
type GuidanceMaterializer func(context.Context, eval.PreparedProject, eval.Guidance, project.AgentGuidance) (eval.Guidance, error)

// preparationCache owns immutable command-local bases shared only by paired trial clones.
type preparationCache struct {
	mu          sync.Mutex
	bases       map[string]*scenarios.PreparedScenario
	inflight    map[string]*preparationFlight
	active      int
	idle        chan struct{}
	closed      bool
	cleaning    bool
	cleanupDone chan struct{}
	cleanupErr  error
}

// preparationFlight publishes one plan's immutable base to same-plan waiters.
type preparationFlight struct {
	done chan struct{}
	base *scenarios.PreparedScenario
	err  error
}

// NewPreparer enables command-local immutable-base reuse under an explicitly owned root.
func NewPreparer(baseRoot string, baseEnvironment []string, appLogger *logger.AppLogger, materializer GuidanceMaterializer) *Preparer {
	baseGate := make(chan struct{}, 1)
	baseGate <- struct{}{}
	return &Preparer{
		BaseRoot:        baseRoot,
		BaseEnvironment: append([]string(nil), baseEnvironment...),
		Logger:          appLogger,
		Guidance:        materializer,
		cache:           newPreparationCache(),
		baseGate:        baseGate,
	}
}

// newPreparationCache creates command-local base ownership with independent in-flight plans.
func newPreparationCache() *preparationCache {
	cache := &preparationCache{
		bases:       map[string]*scenarios.PreparedScenario{},
		inflight:    map[string]*preparationFlight{},
		idle:        make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}
	close(cache.idle)
	return cache
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
	_, toolchainDigest, err := scenarios.ResolveScenarioPreparationTools(request.ForjExecutable, request.Environment, scenarios.ResolveOptions{SpecDir: preparer.SpecDir, ScenarioID: request.ScenarioID})
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
		PlanDigest:           preparationPlanDigest(resolved.PlanDigest, forjDigest, toolchainDigest, environmentDigest),
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
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
	if err := ctx.Err(); err != nil {
		return nil, closePreparedAfter(prepared, err)
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

// MaterializeGuidance delegates durable host-native instruction ownership to the injected production materializer.
func (preparer Preparer) MaterializeGuidance(ctx context.Context, prepared eval.PreparedProject, guidance eval.Guidance) (eval.Guidance, error) {
	if prepared == nil || strings.TrimSpace(prepared.Result().ProjectRoot) == "" {
		return eval.Guidance{}, fmt.Errorf("prepared Project is required")
	}
	selection, err := evaluationGuidanceSelection(guidance.Profile)
	if err != nil {
		return eval.Guidance{}, err
	}
	if preparer.Guidance == nil {
		return eval.Guidance{}, fmt.Errorf("durable guidance materializer is required")
	}
	return preparer.Guidance(ctx, prepared, guidance, selection)
}

// evaluationGuidanceSelection binds Atlas treatment names to GoForj's durable render policy.
func evaluationGuidanceSelection(profile string) (project.AgentGuidance, error) {
	switch profile {
	case eval.GuidanceProfileNone:
		return project.AgentGuidanceNone, nil
	case eval.GuidanceProfileAgents:
		return project.AgentGuidanceBaseline, nil
	default:
		return "", fmt.Errorf("unsupported evaluation guidance profile %q", profile)
	}
}

// Close removes every immutable base retained for this command.
func (preparer *Preparer) Close(ctx context.Context) error {
	if preparer == nil || preparer.cache == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var bases map[string]*scenarios.PreparedScenario
	for {
		preparer.cache.mu.Lock()
		preparer.cache.closed = true
		if preparer.cache.cleaning {
			done := preparer.cache.cleanupDone
			preparer.cache.mu.Unlock()
			return waitForPreparationCleanup(ctx, preparer.cache, done)
		}
		if preparer.cache.active == 0 {
			preparer.cache.cleaning = true
			bases = preparer.cache.bases
			preparer.cache.bases = nil
			preparer.cache.mu.Unlock()
			break
		}
		idle := preparer.cache.idle
		preparer.cache.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-idle:
		}
	}
	go finishPreparationCleanup(preparer.cache, bases)
	return waitForPreparationCleanup(ctx, preparer.cache, preparer.cache.cleanupDone)
}

// waitForPreparationCleanup lets concurrent or retried close calls observe the one cleanup result without racing base destruction.
func waitForPreparationCleanup(ctx context.Context, cache *preparationCache, done <-chan struct{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		cache.mu.Lock()
		err := cache.cleanupErr
		cache.mu.Unlock()
		return err
	}
}

// finishPreparationCleanup publishes one durable result even when the caller's shutdown context expires first.
func finishPreparationCleanup(cache *preparationCache, bases map[string]*scenarios.PreparedScenario) {
	var cleanupErrors []error
	for _, base := range bases {
		cleanupErrors = append(cleanupErrors, base.Close())
	}
	cache.mu.Lock()
	cache.cleanupErr = errors.Join(cleanupErrors...)
	close(cache.cleanupDone)
	cache.mu.Unlock()
}

// prepareScenario either materializes directly or clones one command-local verified base.
func (preparer Preparer) prepareScenario(ctx context.Context, request eval.PreparationRequest, plan eval.ResolvedPreparationPlan) (*scenarios.PreparedScenario, error) {
	if preparer.cache == nil {
		return preparer.materializeScenario(ctx, request, plan, request.DestinationRoot, false)
	}
	if strings.TrimSpace(preparer.BaseRoot) == "" {
		return nil, fmt.Errorf("evaluation base root is required")
	}
	key := plan.PlanDigest
	base, release, err := preparer.cache.acquire(ctx, key, func() (*scenarios.PreparedScenario, error) {
		baseRequest := request
		baseRequest.Environment = append([]string(nil), preparer.BaseEnvironment...)
		basePlan, err := preparer.Resolve(ctx, baseRequest)
		if err != nil {
			return nil, err
		}
		if !samePreparationPlan(plan, basePlan) {
			return nil, fmt.Errorf("evaluation base environment does not match the resolved material environment")
		}
		return preparer.materializeBase(ctx, baseRequest, plan)
	})
	if err != nil {
		return nil, err
	}
	defer release()
	return scenarios.ClonePrepared(base, request.DestinationRoot)
}

// materializeBase serializes trusted base construction so every verifier sees the command-warmed module seed.
func (preparer Preparer) materializeBase(ctx context.Context, request eval.PreparationRequest, plan eval.ResolvedPreparationPlan) (*scenarios.PreparedScenario, error) {
	if preparer.baseGate == nil {
		return nil, fmt.Errorf("evaluation preparer base gate is required")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-preparer.baseGate:
	}
	defer func() { preparer.baseGate <- struct{}{} }()
	return preparer.materializeScenario(ctx, request, plan, preparer.BaseRoot, false)
}

// acquire retains one active cache user until release and single-flights materialization for matching plan digests.
func (cache *preparationCache) acquire(ctx context.Context, key string, materialize func() (*scenarios.PreparedScenario, error)) (*scenarios.PreparedScenario, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	cache.mu.Lock()
	if err := ctx.Err(); err != nil {
		cache.mu.Unlock()
		return nil, nil, err
	}
	if cache.closed {
		cache.mu.Unlock()
		return nil, nil, fmt.Errorf("evaluation preparer is closed")
	}
	if cache.active == 0 {
		cache.idle = make(chan struct{})
	}
	cache.active++
	release := sync.OnceFunc(func() {
		cache.mu.Lock()
		cache.active--
		if cache.active == 0 {
			close(cache.idle)
		}
		cache.mu.Unlock()
	})
	if base := cache.bases[key]; base != nil {
		cache.mu.Unlock()
		return base, release, nil
	}
	if flight := cache.inflight[key]; flight != nil {
		cache.mu.Unlock()
		select {
		case <-ctx.Done():
			release()
			return nil, nil, ctx.Err()
		case <-flight.done:
			if err := ctx.Err(); err != nil {
				release()
				return nil, nil, err
			}
			if flight.err != nil {
				release()
				return nil, nil, flight.err
			}
			return flight.base, release, nil
		}
	}
	flight := &preparationFlight{done: make(chan struct{})}
	cache.inflight[key] = flight
	cache.mu.Unlock()
	base, err := materialize()
	cache.mu.Lock()
	if err == nil {
		cache.bases[key] = base
	}
	flight.base, flight.err = base, err
	delete(cache.inflight, key)
	close(flight.done)
	cache.mu.Unlock()
	if err != nil {
		release()
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		release()
		return nil, nil, err
	}
	return base, release, nil
}

// preparationPlanDigest binds the scenario prefix to the exact executable, toolchain, and material environment selected before mutation.
func preparationPlanDigest(scenarioPlanDigest, forjDigest, toolchainDigest, environmentDigest string) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "scenario\x00%s\x00forj\x00%s\x00toolchain\x00%s\x00environment\x00%s\x00", scenarioPlanDigest, forjDigest, toolchainDigest, environmentDigest)
	return fmt.Sprintf("sha256:%x", hash.Sum(nil))
}

// preparationEnvironmentDigest excludes attempt-private tool paths while binding every value that can change the rendered base.
func preparationEnvironmentDigest(environment []string) string {
	ignored := map[string]bool{
		"GOCACHE":    true,
		"GOMODCACHE": true,
		"GOTMPDIR":   true,
		"GOPATH":     true,
		"HOME":       true,
		"PATH":       true,
		"TEMP":       true,
		"TMP":        true,
		"TMPDIR":     true,
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
	_, forjDigest, err := scenarios.ResolveExecutable(request.ForjExecutable)
	if err != nil {
		return nil, err
	}
	_, toolchainDigest, err := scenarios.ResolveScenarioPreparationTools(request.ForjExecutable, request.Environment, scenarios.ResolveOptions{SpecDir: preparer.SpecDir, ScenarioID: request.ScenarioID})
	if err != nil {
		return nil, err
	}
	if preparationPlanDigest(plan.ScenarioPlanDigest, forjDigest, toolchainDigest, preparationEnvironmentDigest(request.Environment)) != plan.PlanDigest {
		return nil, fmt.Errorf("resolved scenario tools changed before preparation")
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
		ExpectedToolDigest: toolchainDigest,
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

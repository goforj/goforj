package apiindex

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/goforj/goforj/project"
	"github.com/goforj/web/webindex"
)

// noRouteCompositionReason explains why a convention-only app produced no contract.
const noRouteCompositionReason = "no route composition"

// noWebAPIReason explains why an explicitly configured CLI-only app does not participate.
const noWebAPIReason = "no web API"

// outcome describes how one transaction affected the active artifact set.
type outcome string

const (
	// outcomeChanged reports that a new artifact generation was published.
	outcomeChanged outcome = "changed"
	// outcomeUnchanged reports that candidate bytes already matched the active generation.
	outcomeUnchanged outcome = "unchanged"
	// outcomeCleaned reports that stale artifacts for a nonparticipating app were removed.
	outcomeCleaned outcome = "cleaned"
	// outcomeSkipped reports that no generation or cleanup work was necessary.
	outcomeSkipped outcome = "skipped"
	// outcomeRejected reports that a candidate could not cross the publication boundary.
	outcomeRejected outcome = "rejected"
)

// runOptions controls diagnostics policy without coupling command flags to web internals.
type runOptions struct {
	root      string
	strict    bool
	buildTags []string
}

// runReport describes the active App and analyzed contract for one lifecycle outcome.
type runReport struct {
	appName     string
	outcome     outcome
	reason      string
	operations  int
	schemas     int
	diagnostics int
}

// preparedRun keeps a staged candidate and its report on the same preparation boundary.
type preparedRun struct {
	candidate *preparedCandidate
	report    runReport
}

// Options controls diagnostics policy and source selection for one indexing transaction.
type Options struct {
	// Root anchors default source discovery and artifacts without changing process working directory.
	Root string
	// Strict rejects candidates that contain analyzer warnings or errors.
	Strict bool
	// BuildTags keeps indexing on the same conditional source surface as compilation.
	BuildTags []string
}

// Candidate is a validated artifact generation that remains isolated until its caller succeeds.
type Candidate interface {
	// Publish atomically promotes the candidate or returns an App-scoped rejection error.
	Publish() error
	// Discard removes staged artifacts without changing the active generation.
	Discard()
}

// Preparation keeps a staged candidate and its user-facing status together as one indexing outcome.
type Preparation struct {
	// Candidate remains unpublished until the caller's surrounding operation succeeds.
	Candidate Candidate
	// Status summarizes analysis even when no candidate is needed or preparation fails.
	Status string
}

// Preparer isolates API-index candidates so build orchestration can share its success boundary.
type Preparer interface {
	// Prepare analyzes the active App without changing its published artifacts.
	Prepare(options Options) (Preparation, error)
}

// AppResolver returns the active App when an indexing operation starts.
type AppResolver func() project.App

// Runner generates API contract artifacts for a project App.
type Runner struct {
	resolveApp AppResolver
}

// NewRunner creates an API index runner whose App selection remains late-bound to CLI dispatch.
func NewRunner(resolveApp AppResolver) *Runner {
	return &Runner{resolveApp: resolveApp}
}

// Prepare creates a validated candidate while leaving publication under the caller's control.
func (r *Runner) Prepare(options Options) (Preparation, error) {
	prepared, err := r.prepareDefault(runOptions{
		root:      options.Root,
		strict:    options.Strict,
		buildTags: append([]string(nil), options.BuildTags...),
	})
	return Preparation{
		Candidate: prepared.candidate,
		Status:    prepared.report.status(),
	}, err
}

// RunDefault generates and immediately publishes the active App's default artifacts.
func (r *Runner) RunDefault(options Options) (string, error) {
	report, err := r.runDefault(runOptions{
		root:      options.Root,
		strict:    options.Strict,
		buildTags: append([]string(nil), options.BuildTags...),
	})
	return report.status(), err
}

// runDefault prepares and immediately publishes one active-app artifact transaction.
func (r *Runner) runDefault(options runOptions) (runReport, error) {
	prepared, err := r.prepareDefault(options)
	if err != nil {
		return prepared.report, err
	}
	if prepared.candidate == nil {
		return prepared.report, nil
	}
	defer prepared.candidate.discard()
	if err := prepared.candidate.publish(); err != nil {
		prepared.report.outcome = outcomeRejected
		return prepared.report, err
	}
	return prepared.report, nil
}

// prepareDefault builds a complete candidate without changing the active artifacts used by a running app.
func (r *Runner) prepareDefault(options runOptions) (prepared preparedRun, err error) {
	defer func() {
		if err != nil {
			prepared.report.outcome = outcomeRejected
		}
	}()
	root := options.root
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return preparedRun{}, fmt.Errorf("resolve API index project root %q: %w", root, err)
	}
	target := r.resolveApp()
	paths := defaultPaths(target)
	expectedRouteComposition := paths.routeComposition
	paths = rootDefaultPaths(absRoot, paths)
	participation, err := resolveParticipation(absRoot, target)
	if err != nil {
		return preparedRun{report: runReport{appName: paths.appName}}, err
	}
	if participation.known && !participation.webAPI {
		active, err := readSnapshots(paths)
		if err != nil {
			return preparedRun{report: runReport{appName: paths.appName}}, err
		}
		outcome := outcomeSkipped
		if active.anyExists() {
			outcome = outcomeCleaned
		}
		report := runReport{appName: paths.appName, outcome: outcome, reason: noWebAPIReason}
		if !active.anyExists() {
			return preparedRun{report: report}, nil
		}
		return preparedRun{
			candidate: &preparedCandidate{paths: paths, active: active, report: report, remove: true},
			report:    report,
		}, nil
	}
	routeComposition, err := existingRouteCompositionPath(target, paths.routeComposition)
	if err != nil {
		return preparedRun{report: runReport{appName: paths.appName}}, err
	}
	paths.routeComposition = routeComposition
	if paths.routeComposition == "" {
		if participation.known && participation.webAPI {
			return preparedRun{report: runReport{appName: paths.appName}}, fmt.Errorf("API index for app %q requires route composition %q", paths.appName, expectedRouteComposition)
		}
		report := runReport{appName: paths.appName, outcome: outcomeSkipped, reason: noRouteCompositionReason}
		return preparedRun{report: report}, nil
	}
	paths.root = absRoot
	return r.prepareDefaultPaths(paths, options)
}

// prepareDefaultPaths generates and validates all candidate artifacts in a directory on the active artifacts' filesystem.
func (r *Runner) prepareDefaultPaths(paths paths, options runOptions) (prepared preparedRun, err error) {
	defer func() {
		if err != nil {
			prepared.report.outcome = outcomeRejected
		}
	}()
	appName := paths.appName
	paths, err = resolvePaths(paths)
	if err != nil {
		return preparedRun{report: runReport{appName: appName}}, err
	}
	before, err := readSnapshots(paths)
	if err != nil {
		return preparedRun{report: runReport{appName: paths.appName}}, err
	}
	stagingDir, err := createStagingDir(paths)
	if err != nil {
		return preparedRun{report: runReport{appName: paths.appName}}, err
	}
	candidate := &preparedCandidate{
		paths:       paths,
		stagedPaths: stagedPaths(paths, stagingDir),
		stagingDir:  stagingDir,
		active:      before,
	}
	manifest, err := r.runIndex(candidate.stagedPaths, options)
	prepared.report = reportFromManifest(paths.appName, outcomeChanged, manifest)
	if err != nil {
		candidate.discard()
		return prepared, err
	}
	candidate.candidates, err = readValidatedSnapshots(candidate.stagedPaths)
	if err != nil {
		candidate.discard()
		return prepared, err
	}
	if before.equal(candidate.candidates) {
		prepared.report.outcome = outcomeUnchanged
	}
	candidate.report = prepared.report
	prepared.candidate = candidate
	return prepared, nil
}

// runIndex applies GoForj's generated and runtime directory exclusions to web's analyzer.
func (r *Runner) runIndex(paths paths, options runOptions) (webindex.Manifest, error) {
	openAPIOptions, err := openAPIOptions(paths, options.buildTags)
	if err != nil {
		return webindex.Manifest{}, err
	}
	indexOptions := webindex.IndexOptions{
		Root:                 paths.root,
		OutPath:              paths.out,
		DiagnosticsPath:      paths.diagnostics,
		OpenAPIPath:          paths.openAPI,
		OpenAPI:              openAPIOptions,
		RouteCompositionPath: paths.routeComposition,
		BuildTags:            options.buildTags,
		Strict:               options.strict,
		SkipDir: func(_ string, name string) bool {
			return shouldSkipSourceDir(name)
		},
	}
	return webindex.Run(context.Background(), indexOptions)
}

// shouldSkipSourceDir avoids generated runtime data that is not project source.
func shouldSkipSourceDir(name string) bool {
	switch name {
	case "_data", "bin":
		return true
	default:
		return false
	}
}

// reportFromManifest captures counts before projection so every command reports the same contract totals.
func reportFromManifest(appName string, outcome outcome, manifest webindex.Manifest) runReport {
	return runReport{
		appName:     appName,
		outcome:     outcome,
		operations:  len(manifest.Operations),
		schemas:     len(manifest.Schemas),
		diagnostics: len(manifest.Diagnostics),
	}
}

// status formats one compact summary suitable for standalone output and optional pipeline timings.
func (r runReport) status() string {
	appName := strings.TrimSpace(r.appName)
	if appName == "" {
		appName = project.DefaultAppName
	}
	parts := []string{"app " + appName}
	if r.outcome != "" {
		outcome := string(r.outcome)
		if reason := strings.TrimSpace(r.reason); reason != "" {
			outcome += " (" + reason + ")"
		}
		parts = append(parts, outcome)
	}
	parts = append(parts,
		countLabel(r.operations, "operation"),
		countLabel(r.schemas, "schema"),
		countLabel(r.diagnostics, "diagnostic"),
	)
	return strings.Join(parts, ", ")
}

// countLabel keeps summary grammar readable without introducing command-specific formatting branches.
func countLabel(count int, noun string) string {
	if count != 1 {
		noun += "s"
	}
	return fmt.Sprintf("%d %s", count, noun)
}

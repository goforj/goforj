package apiindex

import (
	"context"
	"fmt"
	"strings"

	"github.com/goforj/goforj/internal/logger"
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

// Options controls diagnostics policy and source selection for one indexing transaction.
type Options struct {
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

// Preparer isolates API-index candidates so build orchestration can share its success boundary.
type Preparer interface {
	// Prepare analyzes the active App without changing its published artifacts.
	Prepare(options Options) (Candidate, string, error)
}

// AppResolver returns the active App when an indexing operation starts.
type AppResolver func() project.App

// Runner generates API contract artifacts for a project App.
type Runner struct {
	logger     *logger.AppLogger
	resolveApp AppResolver
}

// NewRunner creates an API index runner whose App selection remains late-bound to CLI dispatch.
func NewRunner(appLogger *logger.AppLogger, resolveApp AppResolver) *Runner {
	return &Runner{logger: appLogger, resolveApp: resolveApp}
}

// Run generates API artifacts immediately at caller-provided paths.
func (r *Runner) Run(root string, out string, diagnostics string, openAPI string, emitLog bool) error {
	paths, err := resolvePaths(root, out, diagnostics, openAPI, "", "")
	if err != nil {
		return err
	}

	manifest, err := r.runIndex(paths, runOptions{})
	if err != nil {
		return err
	}

	if emitLog {
		r.logManifestSummary(manifest, paths)
	}
	return nil
}

// Prepare creates a validated candidate while leaving publication under the caller's control.
func (r *Runner) Prepare(options Options) (Candidate, string, error) {
	prepared, report, err := r.prepareDefault(runOptions{
		strict:    options.Strict,
		buildTags: append([]string(nil), options.BuildTags...),
	})
	status := report.status()
	if prepared == nil {
		return nil, status, err
	}
	return prepared, status, err
}

// RunDefault generates and immediately publishes the active App's default artifacts.
func (r *Runner) RunDefault(options Options) (string, error) {
	report, err := r.runDefault(runOptions{
		strict:    options.Strict,
		buildTags: append([]string(nil), options.BuildTags...),
	})
	return report.status(), err
}

// runDefault prepares and immediately publishes one active-app artifact transaction.
func (r *Runner) runDefault(options runOptions) (runReport, error) {
	prepared, report, err := r.prepareDefault(options)
	if err != nil {
		return report, err
	}
	if prepared == nil {
		return report, nil
	}
	defer prepared.discard()
	if err := prepared.publish(); err != nil {
		report.outcome = outcomeRejected
		return report, err
	}
	return report, nil
}

// prepareDefault builds a complete candidate without changing the active artifacts used by a running app.
func (r *Runner) prepareDefault(options runOptions) (prepared *preparedCandidate, report runReport, err error) {
	defer func() {
		if err != nil {
			report.outcome = outcomeRejected
		}
	}()
	target := r.resolveApp()
	paths := defaultPaths(target)
	participation, err := resolveParticipation(target)
	if err != nil {
		return nil, runReport{appName: paths.appName}, err
	}
	if participation.known && !participation.webAPI {
		active, err := readSnapshots(paths)
		if err != nil {
			return nil, runReport{appName: paths.appName}, err
		}
		outcome := outcomeSkipped
		if active.anyExists() {
			outcome = outcomeCleaned
		}
		report := runReport{appName: paths.appName, outcome: outcome, reason: noWebAPIReason}
		if !active.anyExists() {
			return nil, report, nil
		}
		return &preparedCandidate{paths: paths, active: active, report: report, remove: true}, report, nil
	}
	expectedRouteComposition := paths.routeComposition
	routeComposition, err := existingRouteCompositionPath(target, expectedRouteComposition)
	if err != nil {
		return nil, runReport{appName: paths.appName}, err
	}
	paths.routeComposition = routeComposition
	if paths.routeComposition == "" {
		if participation.known && participation.webAPI {
			return nil, runReport{appName: paths.appName}, fmt.Errorf("API index for app %q requires route composition %q", paths.appName, expectedRouteComposition)
		}
		report := runReport{appName: paths.appName, outcome: outcomeSkipped, reason: noRouteCompositionReason}
		return nil, report, nil
	}
	paths.root = "."
	return r.prepareDefaultPaths(paths, options)
}

// prepareDefaultPaths generates and validates all candidate artifacts in a directory on the active artifacts' filesystem.
func (r *Runner) prepareDefaultPaths(paths paths, options runOptions) (prepared *preparedCandidate, report runReport, err error) {
	defer func() {
		if err != nil {
			report.outcome = outcomeRejected
		}
	}()
	appName := paths.appName
	paths, err = resolvePaths(paths.root, paths.out, paths.diagnostics, paths.openAPI, paths.routeComposition, appName)
	if err != nil {
		return nil, runReport{appName: appName}, err
	}
	before, err := readSnapshots(paths)
	if err != nil {
		return nil, runReport{appName: paths.appName}, err
	}
	stagingDir, err := createStagingDir(paths)
	if err != nil {
		return nil, runReport{appName: paths.appName}, err
	}
	prepared = &preparedCandidate{
		paths:       paths,
		stagedPaths: stagedPaths(paths, stagingDir),
		stagingDir:  stagingDir,
		active:      before,
	}
	manifest, err := r.runIndex(prepared.stagedPaths, options)
	report = reportFromManifest(paths.appName, outcomeChanged, manifest)
	if err != nil {
		report.outcome = outcomeRejected
		prepared.discard()
		return nil, report, err
	}
	prepared.candidates, err = readValidatedSnapshots(prepared.stagedPaths)
	if err != nil {
		report.outcome = outcomeRejected
		prepared.discard()
		return nil, report, err
	}
	if before.equal(prepared.candidates) {
		report.outcome = outcomeUnchanged
	}
	prepared.report = report
	return prepared, report, nil
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

// logManifestSummary records artifact counts and destinations without printing the full contract.
func (r *Runner) logManifestSummary(manifest webindex.Manifest, paths paths) {
	r.logger.Info().
		Str("app", paths.appName).
		Any("operations", len(manifest.Operations)).
		Any("schemas", len(manifest.Schemas)).
		Any("diagnostics", len(manifest.Diagnostics)).
		Any("out", paths.out).
		Any("diagnostics_out", paths.diagnostics).
		Any("openapi_out", paths.openAPI).
		Msg("API index generated")
}

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
)

// agentTask keeps the actor's frozen instructions beside the evaluator source.
//
//go:embed testdata/agent-upgrade-task.md
var agentTask string

// agentProbe keeps the independent behavior check outside the candidate tree.
//
//go:embed testdata/agent-upgrade-context_test.go.tmpl
var agentProbe string

const agentHoldout = "TestAgentUpgradeContextCorrelation"

// trialMetadata records immutable inputs for a separately executed evaluation.
type trialMetadata struct {
	Workspace          string               `json:"workspace"`
	SourceRevision     string               `json:"source_revision"`
	Binary             string               `json:"binary"`
	SourceBinary       string               `json:"source_binary"`
	BinarySHA256       string               `json:"binary_sha256"`
	BaselineProvenance string               `json:"baseline_provenance"`
	Files              map[string]fileState `json:"files"`
	PacketSHA256       map[string]string    `json:"packet_sha256"`
}

// trialResults retains package-qualified test identities for both trees.
type trialResults map[string]map[string]testResult

// coverageLoss records behaviors that vanish from the candidate invocation.
type coverageLoss struct {
	Tests    []string `json:"tests"`
	Packages []string `json:"packages"`
}

// prepareTrial freezes committed application source and target inputs before actor work.
func prepareTrial(ctx context.Context, source, renderer, revision string) (any, error) {
	return prepareTrialWithTools(ctx, source, renderer, revision, agentPreparationTools{runCommand, copyFile, copyCommittedApp})
}

// agentPreparationTools makes binary-copy and invocation boundaries independently testable.
type agentPreparationTools struct {
	run     func(context.Context, string, []string, []string, io.Writer, io.Writer) (int, error)
	copy    func(string, string, os.FileMode) error
	archive func(context.Context, string, string, string) error
}

// prepareTrialWithTools freezes inputs using explicit process and copy collaborators.
func prepareTrialWithTools(ctx context.Context, source, renderer, revision string, tools agentPreparationTools) (any, error) {
	source, err := filepath.Abs(source)
	if err != nil {
		return nil, err
	}
	renderer, err = filepath.Abs(renderer)
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp("/tmp", "goforj-agent-upgrade-")
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	baseline, app, packet := filepath.Join(root, "baseline"), filepath.Join(root, "app"), filepath.Join(root, "packet")
	if err = os.MkdirAll(baseline, 0700); err != nil {
		return nil, err
	}
	if err = os.MkdirAll(packet, 0700); err != nil {
		return nil, err
	}
	var output, diagnostic bytes.Buffer
	code, err := tools.run(ctx, source, []string{"git", "rev-parse", "--verify", "--end-of-options", revision + "^{commit}"}, trialEnvironment(), &output, &diagnostic)
	if err != nil || code != 0 {
		return nil, fmt.Errorf("resolve source revision: exit %d: %s: %v", code, diagnostic.String(), err)
	}
	revision = strings.TrimSpace(output.String())
	fingerprint, err := digestFile(renderer)
	if err != nil {
		return nil, err
	}
	frozen := filepath.Join(packet, "forj")
	if err = tools.copy(renderer, frozen, 0500); err != nil {
		return nil, err
	}
	if err = verifyDigest(frozen, fingerprint, "target binary changed while being frozen"); err != nil {
		return nil, err
	}
	if err = verifyDigest(renderer, fingerprint, "target binary changed while being frozen"); err != nil {
		return nil, err
	}
	if err = tools.archive(ctx, source, baseline, revision); err != nil {
		return nil, err
	}
	files, err := inventory(baseline)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(app, 0700); err != nil {
		return nil, err
	}
	publicFiles, err := inventoryTree(baseline, false)
	if err != nil {
		return nil, err
	}
	for name, state := range publicFiles {
		if state.Private || state.Symlink != "" {
			continue
		}
		if err = tools.copy(filepath.Join(baseline, name), filepath.Join(app, name), os.FileMode(state.Mode)); err != nil {
			return nil, err
		}
	}
	for _, args := range [][]string{{"git", "init", "-q"}, {"git", "add", "-f", "--", "."}, {"git", "commit", "-q", "-m", "chore: establish disposable upgrade trial baseline"}} {
		output.Reset()
		diagnostic.Reset()
		code, err = tools.run(ctx, app, args, trialEnvironment(), &output, &diagnostic)
		if err != nil || code != 0 {
			return nil, fmt.Errorf("initialize trial: exit %d: %s: %v", code, diagnostic.String(), err)
		}
	}
	for _, request := range []struct {
		name  string
		flags []string
	}{{"plan.json", []string{"--plan", "--json"}}, {"target-snapshot.json", []string{"--snapshot"}}} {
		output.Reset()
		diagnostic.Reset()
		commandCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		code, err = tools.run(commandCtx, app, append([]string{frozen, "render"}, request.flags...), trialEnvironment(), &output, &diagnostic)
		cancel()
		if err != nil || code != 0 {
			return nil, fmt.Errorf("freeze %s: exit %d: %s: %v", request.name, code, diagnostic.String(), err)
		}
		if err = os.WriteFile(filepath.Join(packet, request.name), output.Bytes(), 0600); err != nil {
			return nil, err
		}
	}
	if err = verifyDigest(frozen, fingerprint, "frozen renderer changed during preparation"); err != nil {
		return nil, err
	}
	task := strings.NewReplacer("{{WORKSPACE}}", root, "{{RENDERER}}", frozen).Replace(agentTask)
	if err = os.WriteFile(filepath.Join(packet, "TASK.md"), []byte(task), 0600); err != nil {
		return nil, err
	}
	metadata := trialMetadata{root, revision, frozen, renderer, fingerprint, "unknown", files, map[string]string{}}
	for _, name := range []string{"TASK.md", "plan.json", "target-snapshot.json"} {
		metadata.PacketSHA256[name], err = digestFile(filepath.Join(packet, name))
		if err != nil {
			return nil, err
		}
	}
	if err = writeJSON(filepath.Join(root, "trial.json"), metadata); err != nil {
		return nil, err
	}
	return map[string]any{"workspace": root, "task": filepath.Join(packet, "TASK.md")}, nil
}

// verifyDigest refuses to continue when a frozen input changes.
func verifyDigest(path, expected, message string) error {
	actual, err := digestFile(path)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("%s", message)
	}
	return nil
}

// protectedChanges identifies rewritten tests and migration history independently of actor claims.
func protectedChanges(before, after map[string]fileState) []string {
	changed := []string{}
	for name, state := range before {
		protected := strings.HasSuffix(name, "_test.go") || strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") || strings.HasPrefix(name, "migrations/") && strings.HasSuffix(name, ".sql")
		actual, ok := after[name]
		if protected && (!ok || actual != state) {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)
	return changed
}

// holdoutOverlay canonicalizes paths so temporary-directory aliases cannot silently omit the probe.
func holdoutOverlay(app, evaluation, name string) (string, error) {
	canonical, err := filepath.EvalSymlinks(app)
	if err != nil {
		return "", err
	}
	virtual, err := canonicalOverlayPath(filepath.Join(canonical, "internal/logger/agent_upgrade_holdout_test.go"))
	if err != nil {
		return "", err
	}
	if _, err = os.Lstat(virtual); err == nil {
		return "", fmt.Errorf("independent probe would replace an existing app test")
	} else if !os.IsNotExist(err) {
		return "", err
	}
	source, err := os.ReadFile(filepath.Join(app, "go.mod"))
	if err != nil {
		return "", err
	}
	module := regexp.MustCompile(`(?m)^module\s+"?([a-zA-Z0-9._~/-]+)"?\s*$`).FindSubmatch(source)
	if module == nil {
		return "", fmt.Errorf("unsupported module declaration for independent probe")
	}
	evaluation, err = filepath.EvalSymlinks(evaluation)
	if err != nil {
		return "", err
	}
	probe := filepath.Join(evaluation, name+"_probe_test.go")
	if err = os.WriteFile(probe, []byte(strings.ReplaceAll(agentProbe, "{{MODULE}}", string(module[1]))), 0600); err != nil {
		return "", err
	}
	overlay := filepath.Join(evaluation, name+"-overlay.json")
	err = writeJSON(overlay, map[string]any{"Replace": map[string]string{virtual: probe}})
	return overlay, err
}

// canonicalOverlayPath resolves existing ancestors while allowing the virtual test to remain absent.
func canonicalOverlayPath(name string) (string, error) {
	suffix := []string{}
	for {
		canonical, err := filepath.EvalSymlinks(name)
		if err == nil {
			for index := len(suffix) - 1; index >= 0; index-- {
				canonical = filepath.Join(canonical, suffix[index])
			}
			return canonical, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(name)
		if parent == name {
			return "", err
		}
		suffix = append(suffix, filepath.Base(name))
		name = parent
	}
}

// difference returns stable evidence of identities absent from the candidate set.
func difference(required, actual []string) []string {
	present := map[string]bool{}
	for _, value := range actual {
		present[value] = true
	}
	missing := map[string]bool{}
	for _, value := range required {
		if !present[value] {
			missing[value] = true
		}
	}
	result := []string{}
	for value := range missing {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// coverageLosses detects failed or skipped tests and packages hidden by new module boundaries.
func coverageLosses(results trialResults) map[string]coverageLoss {
	losses := map[string]coverageLoss{}
	for _, suite := range []string{"unit", "auth"} {
		baseline, candidate := results["baseline"][suite], results["app"][suite]
		tests := difference(append(append([]string{}, baseline.Passed...), baseline.Failed...), append(append([]string{}, candidate.Passed...), candidate.Failed...))
		tests = append(tests, difference(baseline.Skipped, append(append([]string{}, candidate.Passed...), candidate.Skipped...))...)
		losses[suite] = coverageLoss{difference(tests, nil), difference(baseline.Packages, candidate.Packages)}
	}
	return losses
}

// assessResults refuses green claims from empty runs, lost behavior, skips, or ineffective negative controls.
func assessResults(results trialResults, protected []string) (bool, map[string][]string) {
	accepted := len(protected) == 0 && results["baseline"]["holdout"].ExitCode == 1
	regressions := map[string][]string{}
	for _, suite := range []string{"unit", "auth"} {
		baseline, candidate := results["baseline"][suite], results["app"][suite]
		regressions[suite] = difference(baseline.Passed, candidate.Passed)
		accepted = accepted && len(regressions[suite]) == 0 && len(baseline.Passed) > 0 && len(candidate.Passed) > 0 && len(difference(candidate.Skipped, baseline.Skipped)) == 0
	}
	for _, result := range results["app"] {
		accepted = accepted && result.ExitCode == 0 && len(result.Failed) == 0 && len(result.PackageFailures) == 0
	}
	for _, required := range []string{agentHoldout, agentHoldout + "/event", agentHoldout + "/http-access"} {
		accepted = accepted && hasTestName(results["app"]["holdout"].Passed, required) && hasTestName(results["baseline"]["holdout"].Failed, required)
	}
	for _, loss := range coverageLosses(results) {
		accepted = accepted && len(loss.Tests) == 0 && len(loss.Packages) == 0
	}
	return accepted, regressions
}

// hasTestName compares the test portion while retaining package identity in stored evidence.
func hasTestName(identities []string, name string) bool {
	for _, identity := range identities {
		parts := strings.SplitN(identity, "::", 2)
		if len(parts) == 2 && parts[1] == name {
			return true
		}
	}
	return false
}

// trialStatus distinguishes measured regressions from incomplete acceptance and validation writes.
func trialStatus(accepted bool, protected []string, regressions map[string][]string, validationChanges []string, losses map[string]coverageLoss) string {
	if len(validationChanges) > 0 {
		return "invalidated"
	}
	if len(protected) > 0 {
		return "regressed"
	}
	for _, lost := range regressions {
		if len(lost) > 0 {
			return "regressed"
		}
	}
	for _, lost := range losses {
		if len(lost.Tests) > 0 || len(lost.Packages) > 0 {
			return "regressed"
		}
	}
	if accepted {
		return "measured-gates-pass"
	}
	return "blocked"
}

// residualArtifacts exposes unadopted assets without requiring application source to equal defaults.
func residualArtifacts(snapshot []byte, files map[string]fileState) (map[string][]string, error) {
	var document struct {
		Artifacts []struct {
			Path      string `json:"path"`
			Ownership string `json:"ownership"`
			Digest    string `json:"digest"`
			Source    string `json:"source"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(snapshot, &document); err != nil {
		return nil, err
	}
	if document.Artifacts == nil {
		return nil, fmt.Errorf("target snapshot must contain an artifacts array")
	}
	result := map[string][]string{"raw_asset_differences": {}, "source_differences": {}, "missing_managed": {}, "missing_derived": {}}
	for _, artifact := range document.Artifacts {
		file, exists := files[artifact.Path]
		if artifact.Ownership == "app-derived" && !exists {
			found := false
			for name := range files {
				found = found || strings.HasPrefix(name, artifact.Path+"/")
			}
			if !found {
				result["missing_derived"] = append(result["missing_derived"], artifact.Path)
			}
		}
		if artifact.Ownership != "framework-template" && artifact.Ownership != "framework-asset" {
			continue
		}
		if !exists {
			result["missing_managed"] = append(result["missing_managed"], artifact.Path)
			continue
		}
		expected := artifact.Digest
		if expected == "" {
			hash := sha256.Sum256([]byte(artifact.Source))
			expected = hex.EncodeToString(hash[:])
		}
		if file.SHA256 != expected {
			category := "source_differences"
			if artifact.Ownership == "framework-asset" {
				category = "raw_asset_differences"
			}
			result[category] = append(result[category], artifact.Path)
		}
	}
	for _, values := range result {
		sort.Strings(values)
	}
	return result, nil
}

// appBuildArguments requires an executable rather than accepting a library archive.
func appBuildArguments(output string) []string {
	return []string{"build", "-buildmode=exe", "-mod=readonly", "-o", output, "./cmd/app"}
}

// inventoryChanges preserves stable paths for both removed and newly created files.
func inventoryChanges(before, after map[string]fileState) []string {
	names := map[string]bool{}
	for name := range before {
		names[name] = true
	}
	for name := range after {
		names[name] = true
	}
	changes := []string{}
	for name := range names {
		a, aok := before[name]
		b, bok := after[name]
		if aok != bok || a != b {
			changes = append(changes, name)
		}
	}
	sort.Strings(changes)
	return changes
}

// evaluateTrial runs only after actor work stops and retains complete comparison evidence.
func evaluateTrial(ctx context.Context, root string) (any, error) {
	return evaluateTrialWithRunner(ctx, root, runGo)
}

// evaluateTrialWithRunner permits bounded integrity tests without launching external tools.
func evaluateTrialWithRunner(ctx context.Context, root string, runner func(context.Context, string, string, string, []string) (testResult, error)) (any, error) {
	raw, err := os.ReadFile(filepath.Join(root, "trial.json"))
	if err != nil {
		return nil, err
	}
	var metadata trialMetadata
	if err = json.Unmarshal(raw, &metadata); err != nil {
		return nil, err
	}
	if err = verifyDigest(metadata.Binary, metadata.BinarySHA256, "frozen renderer changed; the trial requires a new preparation"); err != nil {
		return nil, err
	}
	for name, expected := range metadata.PacketSHA256 {
		if err = verifyDigest(filepath.Join(root, "packet", name), expected, "actor input packet changed; the trial requires a new preparation"); err != nil {
			return nil, err
		}
	}
	baseline, err := inventory(filepath.Join(root, "baseline"))
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(baseline, metadata.Files) {
		return nil, fmt.Errorf("baseline source changed; the trial cannot be independently compared")
	}
	evaluation := filepath.Join(root, "evaluation")
	if err = os.MkdirAll(evaluation, 0700); err != nil {
		return nil, err
	}
	candidateBefore, err := inventory(filepath.Join(root, "app"))
	if err != nil {
		return nil, err
	}
	results := trialResults{}
	for _, name := range []string{"baseline", "app"} {
		app := filepath.Join(root, name)
		overlay, err := holdoutOverlay(app, evaluation, name)
		if err != nil {
			return nil, err
		}
		results[name] = map[string]testResult{}
		for _, suite := range []struct {
			name string
			args []string
		}{{"unit", []string{"test", "-p", "1", "-mod=readonly", "-json", "./...", "-count=1"}}, {"auth", []string{"test", "-mod=readonly", "-json", "-tags=integration", "./internal/auth", "-count=1"}}, {"holdout", []string{"test", "-mod=readonly", "-json", "-overlay", overlay, "./internal/logger", "-run", "^" + agentHoldout + "$", "-count=1"}}, {"build", appBuildArguments(filepath.Join(evaluation, name+"-binary"))}} {
			result, err := runner(ctx, app, evaluation, name+"-"+suite.name, suite.args)
			if err != nil {
				return nil, err
			}
			results[name][suite.name] = result
		}
	}
	baseline, err = inventory(filepath.Join(root, "baseline"))
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(baseline, metadata.Files) {
		return nil, fmt.Errorf("baseline validation changed source; the trial requires investigation")
	}
	after, err := inventory(filepath.Join(root, "app"))
	if err != nil {
		return nil, err
	}
	validationChanges := inventoryChanges(candidateBefore, after)
	changed := inventoryChanges(metadata.Files, after)
	protected := protectedChanges(metadata.Files, after)
	accepted, regressions := assessResults(results, protected)
	losses := coverageLosses(results)
	accepted = accepted && len(validationChanges) == 0
	status := trialStatus(accepted, protected, regressions, validationChanges, losses)
	snapshot, err := os.ReadFile(filepath.Join(root, "packet", "target-snapshot.json"))
	if err != nil {
		return nil, err
	}
	residual, err := residualArtifacts(snapshot, after)
	if err != nil {
		return nil, err
	}
	summary := map[string]any{"status": status, "manual_upgrade_review_required": true, "source_revision": metadata.SourceRevision, "binary_sha256": metadata.BinarySHA256, "changed_files": changed, "protected_changes": protected, "validation_changes": validationChanges, "lost_baseline_test_passes": regressions, "lost_baseline_coverage": losses, "results": results, "residual_artifacts": residual}
	summaryPath := filepath.Join(evaluation, "summary.json")
	if err = writeJSON(summaryPath, summary); err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for suite, values := range regressions {
		counts[suite] = len(values)
	}
	return map[string]any{"workspace": root, "status": status, "changed_files": len(changed), "protected_changes": protected, "lost_baseline_test_passes": counts, "lost_baseline_coverage": losses, "summary": summaryPath}, nil
}

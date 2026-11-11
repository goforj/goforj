package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// agentWrite creates public fixture source without relying on shell utilities.
func agentWrite(t *testing.T, path, source string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
}

// agentPassingResults supplies distinct baseline and candidate evidence for scoring mutations.
func agentPassingResults() trialResults {
	results := trialResults{}
	for _, side := range []string{"baseline", "app"} {
		results[side] = map[string]testResult{}
		for _, suite := range []string{"unit", "auth", "holdout", "build"} {
			results[side][suite] = testResult{Passed: []string{}, Failed: []string{}, Skipped: []string{}, PackageFailures: []string{}, Packages: []string{}}
		}
		results[side]["unit"] = testResult{Passed: []string{"app::TestPolicy"}}
		results[side]["auth"] = testResult{Passed: []string{"auth::TestImpersonation"}}
	}
	names := []string{"logger::" + agentHoldout, "logger::" + agentHoldout + "/event", "logger::" + agentHoldout + "/http-access"}
	results["baseline"]["holdout"] = testResult{ExitCode: 1, Failed: names}
	results["app"]["holdout"] = testResult{Passed: names}
	return results
}

// TestAgentAssessmentRejectsFalseGreens checks each independent acceptance condition.
func TestAgentAssessmentRejectsFalseGreens(t *testing.T) {
	if accepted, _ := assessResults(agentPassingResults(), nil); !accepted {
		t.Fatal("valid measured gates rejected")
	}
	if accepted, _ := assessResults(agentPassingResults(), []string{"modified_test.go"}); accepted {
		t.Fatal("accepted protected edit")
	}
	for _, name := range []string{"exit", "empty-unit", "failed", "package-failure", "auth-identity", "new-skip", "holdout-empty", "holdout-parent-only", "build", "negative-control", "negative-control-timeout", "negative-control-exit-zero", "baseline-empty", "vanished-failed", "vanished-skipped", "vanished-package"} {
		t.Run(name, func(t *testing.T) {
			results := agentPassingResults()
			unit := results["app"]["unit"]
			auth := results["app"]["auth"]
			holdout := results["app"]["holdout"]
			baseline := results["baseline"]["unit"]
			switch name {
			case "exit":
				unit.ExitCode = 1
			case "empty-unit":
				unit.Passed = nil
			case "failed":
				unit.Failed = []string{"app::TestBroken"}
			case "package-failure":
				unit.PackageFailures = []string{"app"}
			case "auth-identity":
				auth.Passed = []string{"auth::DifferentTest"}
			case "new-skip":
				auth.Skipped = []string{"auth::Skipped"}
			case "holdout-empty":
				holdout.Passed = nil
			case "holdout-parent-only":
				holdout.Passed = []string{"logger::" + agentHoldout}
			case "build":
				results["app"]["build"] = testResult{ExitCode: 1}
			case "negative-control":
				results["baseline"]["holdout"] = testResult{ExitCode: 1}
			case "negative-control-timeout":
				control := results["baseline"]["holdout"]
				control.ExitCode = 124
				results["baseline"]["holdout"] = control
			case "negative-control-exit-zero":
				control := results["baseline"]["holdout"]
				control.ExitCode = 0
				results["baseline"]["holdout"] = control
			case "baseline-empty":
				baseline.Passed = nil
			case "vanished-failed":
				baseline.Failed = []string{"app::PreviouslyBroken"}
			case "vanished-skipped":
				baseline.Skipped = []string{"app::PreviouslySkipped"}
			case "vanished-package":
				baseline.Packages = []string{"app/previously-broken"}
			}
			results["app"]["unit"] = unit
			results["app"]["auth"] = auth
			results["app"]["holdout"] = holdout
			results["baseline"]["unit"] = baseline
			if accepted, _ := assessResults(results, nil); accepted {
				t.Fatal("accepted false green")
			}
		})
	}
	for _, field := range []string{"failed", "skipped", "package"} {
		results := agentPassingResults()
		before, after := results["baseline"]["unit"], results["app"]["unit"]
		switch field {
		case "failed":
			before.Failed = []string{"app::Repaired"}
			after.Passed = append(after.Passed, "app::Repaired")
		case "skipped":
			before.Skipped = []string{"app::Repaired"}
			after.Passed = append(after.Passed, "app::Repaired")
		case "package":
			before.Packages = []string{"app/repaired"}
			after.Packages = []string{"app/repaired"}
		}
		results["baseline"]["unit"] = before
		results["app"]["unit"] = after
		if accepted, _ := assessResults(results, nil); !accepted {
			t.Fatalf("repaired %s rejected", field)
		}
	}
}

// TestAgentStatusSeparatesBlockers checks pre-existing failures and validation writes separately.
func TestAgentStatusSeparatesBlockers(t *testing.T) {
	for _, test := range []struct {
		accepted    bool
		protected   []string
		regressions map[string][]string
		changes     []string
		losses      map[string]coverageLoss
		want        string
	}{{true, nil, nil, nil, nil, "measured-gates-pass"}, {false, nil, nil, nil, nil, "blocked"}, {false, []string{"test.go"}, nil, nil, nil, "regressed"}, {false, nil, map[string][]string{"unit": {"Lost"}}, nil, nil, "regressed"}, {false, nil, nil, []string{"write.go"}, nil, "invalidated"}, {false, nil, nil, nil, map[string]coverageLoss{"unit": {Packages: []string{"hidden"}}}, "regressed"}} {
		if got := trialStatus(test.accepted, test.protected, test.regressions, test.changes, test.losses); got != test.want {
			t.Fatalf("status=%s want=%s", got, test.want)
		}
	}
}

// TestAgentProtectedChangesIncludesModes prevents test weakening through removals or permissions.
func TestAgentProtectedChangesIncludesModes(t *testing.T) {
	before := map[string]fileState{"internal/auth/app_test.go": {SHA256: "old"}, "migrations/one.sql": {SHA256: "history"}, "ui/a.spec.ts": {SHA256: "old", Mode: 0600}, "ui/a.test.js": {SHA256: "old"}, "app/routes.go": {SHA256: "routes"}}
	after := map[string]fileState{"internal/auth/app_test.go": {SHA256: "new"}, "ui/a.spec.ts": {SHA256: "old", Mode: 0400}, "app/routes.go": {SHA256: "adapted"}}
	want := []string{"internal/auth/app_test.go", "migrations/one.sql", "ui/a.spec.ts", "ui/a.test.js"}
	if got := protectedChanges(before, after); !reflect.DeepEqual(got, want) {
		t.Fatalf("protected=%v", got)
	}
	if got := inventoryChanges(before, before); len(got) != 0 {
		t.Fatalf("unstable comparison=%v", got)
	}
}

// TestAgentResidualsSeparateForksAndMissingOutputs retains useful adoption evidence after successful builds.
func TestAgentResidualsSeparateForksAndMissingOutputs(t *testing.T) {
	snapshot := []byte(`{"artifacts":[{"path":"internal/custom.go","ownership":"framework-template","source":"incoming"},{"path":"ui/new.js","ownership":"framework-asset","digest":"incoming"},{"path":"ui/fork.js","ownership":"framework-asset","digest":"incoming"},{"path":"cmd/app/frontend/dist","ownership":"app-derived"},{"path":"build/openapi.json","ownership":"app-derived"}]}`)
	result, err := residualArtifacts(snapshot, map[string]fileState{"internal/custom.go": {SHA256: "app policy"}, "ui/fork.js": {SHA256: "app asset"}, "cmd/app/frontend/dist/index.html": {SHA256: "built"}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"source_differences": {"internal/custom.go"}, "raw_asset_differences": {"ui/fork.js"}, "missing_managed": {"ui/new.js"}, "missing_derived": {"build/openapi.json"}}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("residual=%v", result)
	}
	for _, malformed := range []string{"invalid", "null", "{}", `{"artifacts":null}`, `{"artifacts":{}}`} {
		if _, err = residualArtifacts([]byte(malformed), nil); err == nil {
			t.Fatalf("accepted malformed snapshot %s", malformed)
		}
	}
	if _, err = residualArtifacts([]byte(`{"artifacts":[]}`), nil); err != nil {
		t.Fatalf("rejected empty artifacts array: %v", err)
	}
}

// TestAgentOverlayPreservesAppTests checks collision, module, and canonical-path contracts.
func TestAgentOverlayPreservesAppTests(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "app")
	evaluation := filepath.Join(root, "evaluation")
	if err := os.MkdirAll(evaluation, 0700); err != nil {
		t.Fatal(err)
	}
	for _, declaration := range []string{"no module declaration", "module invalid path"} {
		agentWrite(t, filepath.Join(app, "go.mod"), declaration)
		if _, err := holdoutOverlay(app, evaluation, "invalid"); err == nil {
			t.Fatal("accepted unsupported module")
		}
	}
	agentWrite(t, filepath.Join(app, "go.mod"), "module \"example.com/owner\"\n")
	existing := filepath.Join(app, "internal/logger/agent_upgrade_holdout_test.go")
	agentWrite(t, existing, "owner test")
	if _, err := holdoutOverlay(app, evaluation, "collision"); err == nil {
		t.Fatal("replaced owner test")
	}
	if err := os.Remove(existing); err != nil {
		t.Fatal(err)
	}
	overlay, err := holdoutOverlay(app, evaluation, "valid")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(overlay)
	if err != nil {
		t.Fatal(err)
	}
	var document struct{ Replace map[string]string }
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(app)
	if err != nil {
		t.Fatal(err)
	}
	probe := document.Replace[filepath.Join(canonical, "internal/logger/agent_upgrade_holdout_test.go")]
	if probe == "" {
		t.Fatalf("noncanonical overlay=%s", raw)
	}
	content, err := os.ReadFile(probe)
	if err != nil || !bytes.Contains(content, []byte("example.com/owner/internal")) {
		t.Fatalf("probe imports not substituted: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(filepath.Join(root, "missing"), existing); err != nil {
			t.Fatal(err)
		}
		if _, err := holdoutOverlay(app, evaluation, "symlink"); err == nil {
			t.Fatal("overlay masked dangling symlink")
		}
	}
}

// agentTrialFixture writes metadata in the retained trial's snake_case format.
func agentTrialFixture(t *testing.T) (string, trialMetadata) {
	t.Helper()
	root := t.TempDir()
	agentWrite(t, filepath.Join(root, "baseline", "owner.go"), "package owner")
	agentWrite(t, filepath.Join(root, "packet", "TASK.md"), "frozen task")
	agentWrite(t, filepath.Join(root, "packet", "target-snapshot.json"), `{"artifacts":[]}`)
	agentWrite(t, filepath.Join(root, "renderer"), "frozen renderer")
	for _, side := range []string{"baseline", "app"} {
		agentWrite(t, filepath.Join(root, side, "go.mod"), "module example.com/owner\n")
	}
	files, err := inventory(filepath.Join(root, "baseline"))
	if err != nil {
		t.Fatal(err)
	}
	binary, err := digestFile(filepath.Join(root, "renderer"))
	if err != nil {
		t.Fatal(err)
	}
	packet, err := digestFile(filepath.Join(root, "packet", "TASK.md"))
	if err != nil {
		t.Fatal(err)
	}
	metadata := trialMetadata{Binary: filepath.Join(root, "renderer"), BinarySHA256: binary, Files: files, PacketSHA256: map[string]string{"TASK.md": packet}}
	if err = writeJSON(filepath.Join(root, "trial.json"), metadata); err != nil {
		t.Fatal(err)
	}
	return root, metadata
}

// TestAgentIntegrityChecksPrecedeTools rejects changed packets, renderers, and baselines before invocation.
func TestAgentIntegrityChecksPrecedeTools(t *testing.T) {
	for _, name := range []string{"packet", "baseline", "renderer"} {
		t.Run(name, func(t *testing.T) {
			root, _ := agentTrialFixture(t)
			paths := map[string]string{"packet": "packet/TASK.md", "baseline": "baseline/owner.go", "renderer": "renderer"}
			agentWrite(t, filepath.Join(root, paths[name]), "changed")
			called := false
			_, err := evaluateTrialWithRunner(context.Background(), root, func(context.Context, string, string, string, []string) (testResult, error) {
				called = true
				return testResult{}, nil
			})
			if err == nil || called {
				t.Fatalf("integrity err=%v tools called=%v", err, called)
			}
		})
	}
}

// TestAgentValidationInventoryInvalidatesWrites catches tool side effects in either source tree.
func TestAgentValidationInventoryInvalidatesWrites(t *testing.T) {
	for _, side := range []string{"app", "baseline", "unchanged"} {
		t.Run(side, func(t *testing.T) {
			root, _ := agentTrialFixture(t)
			calls := 0
			value, err := evaluateTrialWithRunner(context.Background(), root, func(ctx context.Context, app, evaluation, name string, args []string) (testResult, error) {
				calls++
				if calls == 1 && side != "unchanged" {
					agentWrite(t, filepath.Join(root, side, "tool-write.go"), "package owner")
				}
				parts := strings.SplitN(name, "-", 2)
				return agentPassingResults()[parts[0]][parts[1]], nil
			})
			if side == "baseline" {
				if err == nil {
					t.Fatal("accepted baseline mutation")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := "measured-gates-pass"
			if side == "app" {
				want = "invalidated"
			}
			if value.(map[string]any)["status"] != want {
				t.Fatalf("result=%v", value)
			}
			if calls != 8 {
				t.Fatalf("runner calls=%d", calls)
			}
		})
	}
}

// TestAgentPreparationRejectsChangingBinary exercises copy and invocation mutation boundaries.
func TestAgentPreparationRejectsChangingBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("trial preparation requires /tmp")
	}
	for _, change := range []string{"copy", "source", "snapshot"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			renderer := filepath.Join(root, "renderer")
			agentWrite(t, renderer, "original renderer")
			frozen := ""
			tools := agentPreparationTools{
				copy: func(source, destination string, mode os.FileMode) error {
					if err := copyFile(source, destination, mode); err != nil {
						return err
					}
					if source == renderer {
						frozen = destination
						defer t.Cleanup(func() { os.RemoveAll(filepath.Dir(filepath.Dir(destination))) })
						if change == "copy" {
							if err := os.Chmod(destination, 0600); err != nil {
								return err
							}
							return os.WriteFile(destination, []byte("partial copy"), 0600)
						}
						if change == "source" {
							return os.WriteFile(source, []byte("replacement"), 0600)
						}
					}
					return nil
				},
				archive: func(ctx context.Context, source, destination, revision string) error {
					if revision != "frozen-revision" {
						return fmt.Errorf("unresolved revision %s", revision)
					}
					return os.WriteFile(filepath.Join(destination, "owner.go"), []byte("package owner"), 0600)
				},
				run: func(ctx context.Context, dir string, args, env []string, stdout, stderr io.Writer) (int, error) {
					if args[0] == "git" && args[1] == "rev-parse" {
						fmt.Fprintln(stdout, "frozen-revision")
						return 0, nil
					}
					if args[0] != "git" {
						if err := os.Chmod(frozen, 0600); err != nil {
							return 1, err
						}
						if err := os.WriteFile(frozen, []byte("changed during snapshot"), 0600); err != nil {
							return 1, err
						}
						fmt.Fprintln(stdout, "{}")
					}
					return 0, nil
				},
			}
			if _, err := prepareTrialWithTools(context.Background(), root, renderer, "HEAD", tools); err == nil {
				t.Fatal("accepted changing renderer")
			}
		})
	}
}

// TestAgentExecutableBuildRejectsArchives proves the real build gate requires a main package.
func TestAgentExecutableBuildRejectsArchives(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	root := t.TempDir()
	agentWrite(t, filepath.Join(root, "go.mod"), "module example.com/buildproof\n\ngo 1.25.0\n")
	source := filepath.Join(root, "cmd/app/main.go")
	agentWrite(t, source, "package main\nfunc main(){}\n")
	output := filepath.Join(root, "application")
	result, err := runGo(ctx, root, root, "executable", appBuildArguments(output))
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("main build=%+v err=%v", result, err)
	}
	executable, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(executable, []byte("!<arch>")) {
		t.Fatal("build produced archive")
	}
	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
		t.Fatal("executable permissions missing")
	}
	agentWrite(t, source, "package app\nfunc main(){}\n")
	result, err = runGo(ctx, root, root, "archive", appBuildArguments(output))
	if err != nil || result.ExitCode == 0 {
		t.Fatalf("library build=%+v err=%v", result, err)
	}
	preserved, err := os.ReadFile(output)
	if err != nil || !bytes.Equal(preserved, executable) {
		t.Fatalf("failed build replaced executable: %v", err)
	}
}

// TestAgentNestedModulesCannotHideCoverage uses the Go tool to expose vanished failing and empty packages.
func TestAgentNestedModulesCannotHideCoverage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	root := t.TempDir()
	agentWrite(t, filepath.Join(root, "go.mod"), "module example.com/coverage\n\ngo 1.25.0\n")
	agentWrite(t, filepath.Join(root, "healthy/policy_test.go"), "package healthy\nimport \"testing\"\nfunc TestPolicy(t *testing.T){}\n")
	agentWrite(t, filepath.Join(root, "broken/policy_test.go"), "package broken\nimport \"testing\"\nfunc TestExisting(t *testing.T){t.Fatal(\"existing defect\")}\n")
	agentWrite(t, filepath.Join(root, "notests/policy.go"), "package notests\n")
	baseline, err := runGo(ctx, root, root, "baseline", []string{"test", "-json", "./...", "-count=1"})
	if err != nil || baseline.ExitCode == 0 {
		t.Fatalf("baseline=%+v err=%v", baseline, err)
	}
	before, err := inventory(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"broken", "notests"} {
		agentWrite(t, filepath.Join(root, name, "go.mod"), "module example.com/coverage/"+name+"\n\ngo 1.25.0\n")
	}
	candidate, err := runGo(ctx, root, root, "candidate", []string{"test", "-json", "./...", "-count=1"})
	if err != nil || candidate.ExitCode != 0 {
		t.Fatalf("candidate=%+v err=%v", candidate, err)
	}
	after, err := inventory(root)
	if err != nil {
		t.Fatal(err)
	}
	if changed := protectedChanges(before, after); len(changed) != 0 {
		t.Fatalf("protected=%v", changed)
	}
	results := trialResults{"baseline": {"unit": baseline, "auth": baseline}, "app": {"unit": candidate, "auth": candidate}}
	losses := coverageLosses(results)
	want := coverageLoss{Tests: []string{"example.com/coverage/broken::TestExisting"}, Packages: []string{"example.com/coverage/broken", "example.com/coverage/notests"}}
	if !reflect.DeepEqual(losses["unit"], want) {
		t.Fatalf("losses=%+v", losses)
	}
	if trialStatus(false, nil, nil, nil, losses) != "regressed" {
		t.Fatal("hidden failures not regression")
	}
	results["app"] = results["baseline"]
	if trialStatus(false, nil, nil, nil, coverageLosses(results)) != "blocked" {
		t.Fatal("existing failures mislabeled regression")
	}
}

// TestAgentOverlayCanonicalizesLinkedAncestors keeps virtual keys aligned with the Go tool's physical paths.
func TestAgentOverlayCanonicalizesLinkedAncestors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture requires POSIX privileges")
	}
	root := t.TempDir()
	actual := filepath.Join(root, "actual")
	if err := os.Mkdir(actual, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(actual, alias); err != nil {
		t.Fatal(err)
	}
	got, err := canonicalOverlayPath(filepath.Join(alias, "missing", "holdout_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	actual, err = filepath.EvalSymlinks(actual)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(actual, "missing", "holdout_test.go") {
		t.Fatalf("canonical path=%s", got)
	}
}

// TestAgentPreparationCopiesCommittedDependencies preserves tracked public files while excluding cache paths from evidence.
func TestAgentPreparationCopiesCommittedDependencies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("trial preparation requires /tmp")
	}
	root := t.TempDir()
	renderer := filepath.Join(root, "renderer")
	agentWrite(t, renderer, "original renderer")
	tools := agentPreparationTools{
		copy: copyFile,
		archive: func(ctx context.Context, source, destination, revision string) error {
			if revision != "frozen-revision" {
				return fmt.Errorf("unresolved revision %s", revision)
			}
			agentWrite(t, filepath.Join(destination, "owner.go"), "package owner")
			dependency := filepath.Join(destination, "node_modules/tracked.js")
			agentWrite(t, dependency, "public tracked dependency")
			return os.Chmod(dependency, 0751)
		},
		run: func(ctx context.Context, dir string, args, env []string, stdout, stderr io.Writer) (int, error) {
			if args[0] == "git" && args[1] == "rev-parse" {
				fmt.Fprintln(stdout, "frozen-revision")
			} else if args[0] != "git" {
				fmt.Fprintln(stdout, `{"artifacts":[]}`)
			}
			return 0, nil
		},
	}
	value, err := prepareTrialWithTools(context.Background(), root, renderer, "HEAD", tools)
	if err != nil {
		t.Fatal(err)
	}
	workspace := value.(map[string]any)["workspace"].(string)
	t.Cleanup(func() { os.RemoveAll(workspace) })
	dependency := filepath.Join(workspace, "app/node_modules/tracked.js")
	content, err := os.ReadFile(dependency)
	if err != nil || string(content) != "public tracked dependency" {
		t.Fatalf("tracked dependency=%q err=%v", content, err)
	}
	info, err := os.Stat(dependency)
	if err != nil || info.Mode().Perm() != 0751 {
		t.Fatalf("dependency mode=%v err=%v", info, err)
	}
	raw, err := os.ReadFile(filepath.Join(workspace, "trial.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata trialMetadata
	if err = json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	if _, exists := metadata.Files["node_modules/tracked.js"]; exists {
		t.Fatal("generated dependency tree entered evidence inventory")
	}
	if _, exists := metadata.Files["owner.go"]; !exists {
		t.Fatal("public application source missing from evidence inventory")
	}
	if metadata.SourceRevision != "frozen-revision" {
		t.Fatalf("source revision=%s", metadata.SourceRevision)
	}
}

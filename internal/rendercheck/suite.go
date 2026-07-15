package rendercheck

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/testkit"
)

// Suite owns the render combinations selected by one coverage profile.
type Suite struct {
	profile string
	combos  []renderCombo
}

// NewSuite selects a stable render matrix while retaining the legacy full-flag precedence.
func NewSuite(profile string, full bool) *Suite {
	selectedProfile := selectedRenderProfile(profile, full)
	return &Suite{
		profile: selectedProfile,
		combos:  buildRenderCombos(selectedProfile),
	}
}

// List prints the selected shard without performing filesystem or toolchain work.
func (suite *Suite) List(writer io.Writer) error {
	combos, shardLabel, err := shardRenderCombos(suite.combos)
	if err != nil {
		return err
	}
	listRenderCombos(writer, suite.profile, combos, shardLabel)
	return nil
}

// Run renders and compiles every combination in the selected shard before returning their aggregate result.
func (suite *Suite) Run(runTests bool) error {
	combos := suite.combos
	totalCombos := len(combos)
	combos, shardLabel, err := shardRenderCombos(combos)
	if err != nil {
		return err
	}
	console.Infof("Testing %d component combinations with %s profile%s", len(combos), suite.profile, shardLabel)
	if len(combos) == 0 {
		console.Warnf("No render combinations selected%s", shardLabel)
		return nil
	}

	modCache, buildCache := testkit.GoCachePaths()
	workspaceRoot := testRenderWorkspaceRoot()
	if err := os.MkdirAll(workspaceRoot, 0755); err != nil {
		return fmt.Errorf("create test render workspace: %w", err)
	}

	workerCount := testRenderWorkerCount()
	console.Infof("Render workers: %d", workerCount)
	forjExec, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}

	jobs := make(chan renderCombo)
	failureResults := make(chan *renderComboFailure, len(combos))
	var wg sync.WaitGroup
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		workerRoot := filepath.Join(workspaceRoot, fmt.Sprintf("worker-%02d", i))
		go func(root string) {
			defer wg.Done()
			worker := renderComboWorker{
				workspaceRoot:  root,
				moduleCache:    modCache,
				buildCache:     buildCache,
				forjExecutable: forjExec,
				runTests:       runTests,
			}
			for combo := range jobs {
				if failure := worker.run(combo); failure != nil {
					failureResults <- failure
				}
			}
		}(workerRoot)
	}

	for _, combo := range combos {
		jobs <- combo
	}
	close(jobs)
	wg.Wait()
	close(failureResults)

	failures := make([]*renderComboFailure, 0, len(failureResults))
	for failure := range failureResults {
		failures = append(failures, failure)
	}
	if len(failures) > 0 {
		aggregate := aggregateRenderComboFailures(failures, len(combos), shardLabel)
		for _, failure := range aggregate.failures {
			reportRenderComboFailure(failure)
		}
		return aggregate
	}

	console.Successf("Rendered %d combinations%s", len(combos), shardLabel)
	if totalCombos != len(combos) {
		console.Infof("Shard completed %d/%d combinations", len(combos), totalCombos)
	}
	return nil
}

// listRenderCombos keeps profile review output stable across local and CI invocations.
func listRenderCombos(writer io.Writer, profile string, combos []renderCombo, shardLabel string) {
	fmt.Fprintf(writer, "profile: %s\n", profile)
	fmt.Fprintf(writer, "combinations: %d%s\n\n", len(combos), shardLabel)

	w := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tComponents")
	for _, combo := range combos {
		fmt.Fprintf(w, "%s\t%s\n", combo.id, strings.Join(combo.enabled, ", "))
	}
	_ = w.Flush()
}

// shardRenderCombos partitions by stable matrix order so CI shards remain deterministic.
func shardRenderCombos(combos []renderCombo) ([]renderCombo, string, error) {
	count := 1
	if v := strings.TrimSpace(os.Getenv("FORJ_TEST_RENDERS_SHARD_COUNT")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, "", fmt.Errorf("invalid FORJ_TEST_RENDERS_SHARD_COUNT=%q (must be integer >= 1)", v)
		}
		count = n
	}
	if count == 1 {
		return combos, "", nil
	}

	index := 0
	if v := strings.TrimSpace(os.Getenv("FORJ_TEST_RENDERS_SHARD_INDEX")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, "", fmt.Errorf("invalid FORJ_TEST_RENDERS_SHARD_INDEX=%q (must be integer >= 0)", v)
		}
		index = n
	}
	if index >= count {
		return nil, "", fmt.Errorf(
			"invalid shard config: FORJ_TEST_RENDERS_SHARD_INDEX=%d must be < FORJ_TEST_RENDERS_SHARD_COUNT=%d",
			index,
			count,
		)
	}

	filtered := make([]renderCombo, 0, len(combos)/count+1)
	for i, combo := range combos {
		if i%count == index {
			filtered = append(filtered, combo)
		}
	}
	label := fmt.Sprintf(" (shard %d/%d · total %d)", index+1, count, len(combos))
	return filtered, label, nil
}

// testRenderWorkspaceRoot keeps generated projects outside the source repository by default.
func testRenderWorkspaceRoot() string {
	if v := strings.TrimSpace(os.Getenv("FORJ_TEST_RENDERS_DIR")); v != "" {
		return v
	}
	return filepath.Join(os.TempDir(), "forj_test_renders")
}

// testRenderWorkerCount caps parallel toolchains to avoid exhausting CI hosts with many logical CPUs.
func testRenderWorkerCount() int {
	if v := strings.TrimSpace(os.Getenv("FORJ_TEST_RENDERS_WORKERS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	workerCount := runtime.NumCPU()
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > 12 {
		workerCount = 12
	}
	return workerCount
}

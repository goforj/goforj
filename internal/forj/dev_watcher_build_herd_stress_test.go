//go:build watcherstress

package forj

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/build"
	"github.com/goforj/goforj/internal/devwatch"
)

const devWatcherBuildHerdTimeout = 30 * time.Second

type devWatcherBuildHerdState struct {
	ActivePrepare int            `json:"active_prepare"`
	ActiveApp     int            `json:"active_app"`
	ActiveSPA     int            `json:"active_spa"`
	MaxPrepare    int            `json:"max_prepare"`
	MaxApp        int            `json:"max_app"`
	MaxSPA        int            `json:"max_spa"`
	Prepare       int            `json:"prepare"`
	App           int            `json:"app"`
	SPA           int            `json:"spa"`
	Completed     int            `json:"completed"`
	Entries       map[string]int `json:"entries"`
	Violations    []string       `json:"violations"`
}

// TestDevWatcherBuildHerdHelper records cross-process phase overlap for the production watcher controller.
func TestDevWatcherBuildHerdHelper(t *testing.T) {
	if os.Getenv("GOFORJ_BUILD_HERD_HELPER") != "1" {
		return
	}
	if err := runDevWatcherBuildHerdHelper(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(17)
	}
}

// TestDevWatcherBuildThunderingHerd exercises staggered mixed build pressure against one shared project.
func TestDevWatcherBuildThunderingHerd(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "phase-state.json")
	command := func(task string, phase string, work time.Duration) devwatch.Command {
		return devwatch.Command{
			Shell: devWatcherBuildHerdHelperCommand(),
			Dir:   root,
			Env: map[string]string{
				"GOFORJ_BUILD_HERD_HELPER":  "1",
				"GOFORJ_BUILD_HERD_STATE":   statePath,
				"GOFORJ_BUILD_HERD_TASK":    task,
				"GOFORJ_BUILD_HERD_PHASE":   phase,
				"GOFORJ_BUILD_HERD_WORK_MS": strconv.FormatInt(work.Milliseconds(), 10),
			},
		}
	}
	compiled := []devCompiledWatcher{
		{
			ID: "herd/app/build", Name: "Build App", Kind: devWatcherAppBuild, App: "app",
			Command: command("app", "", 17*time.Millisecond), PhasedBuild: true, Postpone: true,
		},
		{
			ID: "herd/admin/build", Name: "Build admin", Kind: devWatcherAppBuild, App: "admin",
			Command: command("admin", "", 23*time.Millisecond), PhasedBuild: true, Postpone: true,
		},
		{
			ID: "herd/app/frontend", Name: "Build app SPA frontend", Kind: devWatcherSPABuild, App: "app",
			Command: command("app-frontend", "spa", 19*time.Millisecond), Postpone: true,
		},
		{
			ID: "herd/app/docs", Name: "Build app SPA docs", Kind: devWatcherSPABuild, App: "app",
			Command: command("app-docs", "spa", 11*time.Millisecond), Postpone: true,
		},
		{
			ID: "herd/admin/frontend", Name: "Build admin SPA frontend", Kind: devWatcherSPABuild, App: "admin",
			Command: command("admin-frontend", "spa", 29*time.Millisecond), Postpone: true,
		},
		{
			ID: "herd/custom/build", Name: "Build reports", Kind: devWatcherAppBuild, App: "reports",
			Command: command("custom", "prepare", 13*time.Millisecond), Postpone: true,
		},
	}
	controller, err := newDevWatcherController(compiled, nil, io.Discard, io.Discard, false)
	if err != nil {
		t.Fatalf("start build-herd watcher controller: %v", err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			controller.stop(5 * time.Second)
		}
		if t.Failed() {
			t.Logf("build-herd state: %+v", readDevWatcherBuildHerdState(t, statePath))
		}
	})

	start := time.Now()
	seed := devWatcherBuildHerdSeed(t)
	t.Logf("build-herd seed: %d", seed)
	controller.tasks["herd/app/build"].request()
	controller.tasks["herd/admin/build"].request()
	waitForDevWatcherBuildHerdCondition(t, "managed Apps to enter concurrent compilation", func() bool {
		state, err := loadDevWatcherBuildHerdState(statePath)
		return err == nil && state.App >= 2
	})
	controller.tasks["herd/app/frontend"].request()
	controller.tasks["herd/app/docs"].request()
	waitForDevWatcherBuildHerdCondition(t, "SPAs to enter concurrent builds", func() bool {
		state, err := loadDevWatcherBuildHerdState(statePath)
		return err == nil && state.SPA >= 2
	})
	runDevWatcherBuildHerdTriggers(controller, seed, []string{
		"herd/app/build", "herd/admin/build", "herd/app/frontend", "herd/app/docs", "herd/admin/frontend", "herd/custom/build",
	})
	waitForDevWatcherBuildHerdIdle(t, controller, compiled)
	elapsed := time.Since(start)
	state := readDevWatcherBuildHerdState(t, statePath)
	assertDevWatcherBuildHerdState(t, state)
	if elapsed >= devWatcherBuildHerdTimeout {
		t.Fatalf("mixed build herd took %s, want less than %s", elapsed, devWatcherBuildHerdTimeout)
	}
	assertNoDevWatcherBuildHerdExit(t, controller)

	controller.stop(5 * time.Second)
	stopped = true
}

// assertNoDevWatcherBuildHerdExit verifies coordinator pressure did not terminate a logical watcher.
func assertNoDevWatcherBuildHerdExit(t *testing.T, controller *devWatcherController) {
	t.Helper()
	select {
	case exit := <-controller.exitCh:
		t.Fatalf("build-herd watcher exited unexpectedly: %+v", exit)
	default:
	}
}

// runDevWatcherBuildHerdTriggers sends deterministic staggered bursts while active tasks coalesce repeated requests.
func runDevWatcherBuildHerdTriggers(controller *devWatcherController, seed int64, taskIDs []string) {
	intervals := []time.Duration{0, 1, 2, 3, 5, 8, 13, 21}
	random := rand.New(rand.NewSource(seed))
	tasks := make([]string, 160)
	delays := make([]time.Duration, len(tasks))
	for index := range tasks {
		if index < len(taskIDs) {
			tasks[index] = taskIDs[index]
		} else {
			tasks[index] = taskIDs[random.Intn(len(taskIDs))]
		}
		delays[index] = intervals[random.Intn(len(intervals))] * time.Millisecond
	}
	var wait sync.WaitGroup
	for index := range tasks {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			timer := time.NewTimer(delays[index])
			defer timer.Stop()
			<-timer.C
			controller.tasks[tasks[index]].request()
		}(index)
	}
	wait.Wait()
}

// devWatcherBuildHerdSeed makes failures reproducible while allowing extended CI runs to supply additional schedules.
func devWatcherBuildHerdSeed(t *testing.T) int64 {
	t.Helper()
	const defaultSeed int64 = 8675309
	value := os.Getenv("GOFORJ_WATCHER_SOAK_SEED")
	if value == "" {
		return defaultSeed
	}
	seed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		t.Fatalf("parse GOFORJ_WATCHER_SOAK_SEED: %v", err)
	}
	return seed
}

// waitForDevWatcherBuildHerdIdle requires every task to drain its active and coalesced work.
func waitForDevWatcherBuildHerdIdle(t *testing.T, controller *devWatcherController, compiled []devCompiledWatcher) {
	t.Helper()
	waitForDevWatcherBuildHerdCondition(t, "mixed build herd to become idle", func() bool {
		for _, spec := range compiled {
			task := controller.tasks[spec.ID]
			task.mu.Lock()
			idle := !task.busy && !task.pending
			task.mu.Unlock()
			if !idle {
				return false
			}
		}
		return true
	})
}

// waitForDevWatcherBuildHerdCondition bounds liveness checks without making phase ordering depend on wall-clock sleeps.
func waitForDevWatcherBuildHerdCondition(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(devWatcherBuildHerdTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s", description)
		case <-ticker.C:
		}
	}
}

// assertDevWatcherBuildHerdState verifies safety, useful concurrency, phase completion, and custom-build behavior.
func assertDevWatcherBuildHerdState(t *testing.T, state devWatcherBuildHerdState) {
	t.Helper()
	if len(state.Violations) > 0 {
		t.Fatalf("unsafe project phase overlap: %v", state.Violations)
	}
	if state.ActivePrepare != 0 || state.ActiveApp != 0 || state.ActiveSPA != 0 {
		t.Fatalf("build herd did not quiesce: %+v", state)
	}
	if state.MaxPrepare != 1 {
		t.Fatalf("exclusive preparation concurrency=%d, want 1", state.MaxPrepare)
	}
	if state.MaxApp < 2 {
		t.Fatalf("App compile concurrency=%d, want at least 2", state.MaxApp)
	}
	if state.MaxSPA < 2 {
		t.Fatalf("SPA build concurrency=%d, want at least 2", state.MaxSPA)
	}
	if state.Completed != state.Prepare+state.App+state.SPA {
		t.Fatalf("completed phases=%d, entries=%d", state.Completed, state.Prepare+state.App+state.SPA)
	}
	for _, task := range []string{"app", "admin"} {
		prepares := state.Entries[task+":"+build.DevBuildPhasePrepare]
		compiles := state.Entries[task+":"+build.DevBuildPhaseCompile]
		if prepares == 0 || prepares != compiles || prepares > 3 {
			t.Fatalf("managed App %s phase entries are unbalanced: %#v", task, state.Entries)
		}
	}
	for _, task := range []string{"app-frontend", "app-docs", "admin-frontend"} {
		if entries := state.Entries[task+":spa"]; entries == 0 || entries > 3 {
			t.Fatalf("SPA %s did not coalesce its trigger burst: %#v", task, state.Entries)
		}
	}
	if entries := state.Entries["custom:prepare"]; entries == 0 || entries > 3 || state.Entries["custom:"+build.DevBuildPhaseCompile] != 0 {
		t.Fatalf("custom builder did not remain single-phase: %#v", state.Entries)
	}
	if state.Prepare+state.App+state.SPA >= 164 {
		t.Fatalf("trigger herd was not coalesced: phases=%d triggers=164", state.Prepare+state.App+state.SPA)
	}
}

// devWatcherBuildHerdHelperCommand returns the isolated subprocess used by each configured build task.
func devWatcherBuildHerdHelperCommand() string {
	return devWatcherRunnerShellQuote(os.Args[0]) + " -test.run='^TestDevWatcherBuildHerdHelper$'"
}

// runDevWatcherBuildHerdHelper enters one coordinated phase, performs bounded work, and records its exit.
func runDevWatcherBuildHerdHelper() error {
	statePath := os.Getenv("GOFORJ_BUILD_HERD_STATE")
	task := os.Getenv("GOFORJ_BUILD_HERD_TASK")
	phase := os.Getenv(build.DevBuildPhaseEnvironment)
	if phase == "" {
		phase = os.Getenv("GOFORJ_BUILD_HERD_PHASE")
	}
	workMilliseconds, err := strconv.Atoi(os.Getenv("GOFORJ_BUILD_HERD_WORK_MS"))
	if err != nil {
		return fmt.Errorf("parse herd work duration: %w", err)
	}
	if err := updateDevWatcherBuildHerdState(statePath, func(state *devWatcherBuildHerdState) error {
		return enterDevWatcherBuildHerdPhase(state, task, phase)
	}); err != nil {
		return err
	}
	if err := waitForDevWatcherBuildHerdPeers(statePath, phase); err != nil {
		return err
	}
	timer := time.NewTimer(time.Duration(workMilliseconds) * time.Millisecond)
	<-timer.C
	return updateDevWatcherBuildHerdState(statePath, func(state *devWatcherBuildHerdState) error {
		leaveDevWatcherBuildHerdPhase(state, phase)
		return nil
	})
}

// enterDevWatcherBuildHerdPhase rejects forbidden overlap before recording one active helper.
func enterDevWatcherBuildHerdPhase(state *devWatcherBuildHerdState, task string, phase string) error {
	if state.Entries == nil {
		state.Entries = make(map[string]int)
	}
	state.Entries[task+":"+phase]++
	var violation string
	switch phase {
	case build.DevBuildPhasePrepare:
		if state.ActivePrepare != 0 || state.ActiveApp != 0 || state.ActiveSPA != 0 {
			violation = fmt.Sprintf("prepare entered with prepare=%d app=%d spa=%d", state.ActivePrepare, state.ActiveApp, state.ActiveSPA)
		}
		state.ActivePrepare++
		state.Prepare++
		state.MaxPrepare = max(state.MaxPrepare, state.ActivePrepare)
	case build.DevBuildPhaseCompile:
		if state.ActivePrepare != 0 || state.ActiveSPA != 0 {
			violation = fmt.Sprintf("App compile entered with prepare=%d spa=%d", state.ActivePrepare, state.ActiveSPA)
		}
		state.ActiveApp++
		state.App++
		state.MaxApp = max(state.MaxApp, state.ActiveApp)
	case "spa":
		if state.ActivePrepare != 0 || state.ActiveApp != 0 {
			violation = fmt.Sprintf("SPA entered with prepare=%d app=%d", state.ActivePrepare, state.ActiveApp)
		}
		state.ActiveSPA++
		state.SPA++
		state.MaxSPA = max(state.MaxSPA, state.ActiveSPA)
	default:
		return fmt.Errorf("unknown herd phase %q", phase)
	}
	if violation != "" {
		state.Violations = append(state.Violations, violation)
		return fmt.Errorf("unsafe build overlap: %s", violation)
	}
	return nil
}

// waitForDevWatcherBuildHerdPeers forces the first safe App and SPA waves to demonstrate useful concurrency.
func waitForDevWatcherBuildHerdPeers(statePath string, phase string) error {
	if phase != build.DevBuildPhaseCompile && phase != "spa" {
		return nil
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state, err := loadDevWatcherBuildHerdState(statePath)
		if err != nil {
			return err
		}
		if phase == build.DevBuildPhaseCompile && state.App >= 2 || phase == "spa" && state.SPA >= 2 {
			return nil
		}
		timer := time.NewTimer(time.Millisecond)
		<-timer.C
	}
	return fmt.Errorf("phase %q did not gain a concurrent peer", phase)
}

// leaveDevWatcherBuildHerdPhase records clean phase completion after the helper work finishes.
func leaveDevWatcherBuildHerdPhase(state *devWatcherBuildHerdState, phase string) {
	switch phase {
	case build.DevBuildPhasePrepare:
		state.ActivePrepare--
	case build.DevBuildPhaseCompile:
		state.ActiveApp--
	case "spa":
		state.ActiveSPA--
	}
	state.Completed++
}

// updateDevWatcherBuildHerdState serializes one cross-process state transition with a portable atomic-directory lock.
func updateDevWatcherBuildHerdState(path string, update func(*devWatcherBuildHerdState) error) error {
	unlock, err := acquireDevWatcherBuildHerdStateLock(path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	state, err := decodeDevWatcherBuildHerdState(file)
	if err != nil {
		return err
	}
	updateErr := update(&state)
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return err
	}
	if err := json.NewEncoder(file).Encode(state); err != nil {
		return err
	}
	return updateErr
}

// acquireDevWatcherBuildHerdStateLock uses atomic directory creation because the stress helper must run unchanged on every supported OS.
func acquireDevWatcherBuildHerdStateLock(path string) (func(), error) {
	deadline := time.Now().Add(devWatcherBuildHerdTimeout)
	for {
		err := os.Mkdir(path, 0o700)
		if err == nil {
			return func() { _ = os.Remove(path) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("acquire build-herd state lock: timed out")
		}
		timer := time.NewTimer(time.Millisecond)
		<-timer.C
	}
}

// loadDevWatcherBuildHerdState returns one consistent state snapshot under the same writer lock.
func loadDevWatcherBuildHerdState(path string) (devWatcherBuildHerdState, error) {
	var result devWatcherBuildHerdState
	err := updateDevWatcherBuildHerdState(path, func(state *devWatcherBuildHerdState) error {
		result = *state
		result.Entries = make(map[string]int, len(state.Entries))
		for key, value := range state.Entries {
			result.Entries[key] = value
		}
		result.Violations = append([]string(nil), state.Violations...)
		return nil
	})
	return result, err
}

// decodeDevWatcherBuildHerdState reads an empty or previously persisted helper state.
func decodeDevWatcherBuildHerdState(file *os.File) (devWatcherBuildHerdState, error) {
	var state devWatcherBuildHerdState
	info, err := file.Stat()
	if err != nil {
		return state, err
	}
	if info.Size() == 0 {
		state.Entries = make(map[string]int)
		return state, nil
	}
	if _, err := file.Seek(0, 0); err != nil {
		return state, err
	}
	if err := json.NewDecoder(file).Decode(&state); err != nil {
		return state, err
	}
	return state, nil
}

// readDevWatcherBuildHerdState loads helper state or fails the owning test with context.
func readDevWatcherBuildHerdState(t *testing.T, path string) devWatcherBuildHerdState {
	t.Helper()
	state, err := loadDevWatcherBuildHerdState(path)
	if err != nil {
		t.Fatalf("read build-herd state: %v", err)
	}
	return state
}

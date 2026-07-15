package rendercheck

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// formatCommandFailure retains tool output because generated-project failures are otherwise difficult to reproduce from CI logs.
func formatCommandFailure(command string, err error, stdout, stderr string) error {
	var parts []string
	parts = append(parts, fmt.Sprintf("%s: %v", command, err))
	if trimmed := strings.TrimSpace(stdout); trimmed != "" {
		parts = append(parts, fmt.Sprintf("stdout:\n%s", trimmed))
	}
	if trimmed := strings.TrimSpace(stderr); trimmed != "" {
		parts = append(parts, fmt.Sprintf("stderr:\n%s", trimmed))
	}
	return fmt.Errorf("%s", strings.Join(parts, "\n\n"))
}

// stepTimer keeps per-combination diagnostics useful without coupling render execution to a profiling package.
type stepTimer struct {
	start time.Time
	parts map[string]time.Duration
}

// renderComboFailure retains the context needed to report one failed render without terminating its worker.
type renderComboFailure struct {
	reason  string
	comboID string
	config  *project.Config
	err     error
}

// Error identifies the failed combination while preserving its underlying cause.
func (failure renderComboFailure) Error() string {
	return fmt.Sprintf("%s for combo %s: %v", failure.reason, failure.comboID, failure.err)
}

// Unwrap exposes the command or filesystem error that caused this combination to fail.
func (failure renderComboFailure) Unwrap() error {
	return failure.err
}

// renderComboFailures keeps every worker failure available while presenting one command-level summary.
type renderComboFailures struct {
	failures   []*renderComboFailure
	total      int
	shardLabel string
}

// Error summarizes the failed combinations for the CLI boundary after their detailed reports are printed.
func (failures renderComboFailures) Error() string {
	return fmt.Sprintf("%d of %d render combinations failed%s", len(failures.failures), failures.total, failures.shardLabel)
}

// Unwrap exposes every combination failure to callers that need to inspect the aggregate.
func (failures renderComboFailures) Unwrap() []error {
	causes := make([]error, len(failures.failures))
	for index := range failures.failures {
		causes[index] = failures.failures[index]
	}
	return causes
}

// newStepTimer starts one timing report so failed and slow combinations can be diagnosed independently.
func newStepTimer() *stepTimer {
	return &stepTimer{
		start: time.Now(),
		parts: make(map[string]time.Duration),
	}
}

// Track records a named phase even when that phase returns an error.
func (t *stepTimer) Track(name string, fn func() error) error {
	s := time.Now()
	err := fn()
	t.parts[name] = time.Since(s)
	return err
}

// Report prints phase timings together so parallel render output still identifies its combination.
func (t *stepTimer) Report(label string) {
	total := time.Since(t.start)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n⏱ Timing breakdown for %s\n", label))
	b.WriteString("---------------------------------------------------\n")

	for k, v := range t.parts {
		b.WriteString(fmt.Sprintf("%-15s %s\n", k+":", v))
	}

	b.WriteString("---------------------------------------------------\n")
	b.WriteString(fmt.Sprintf("%-15s %s\n\n", "total:", total))

	fmt.Print(b.String())
}

// newRenderComboFailure packages a worker failure without reporting it concurrently with other combinations.
func newRenderComboFailure(reason, comboID string, cfg *project.Config, err error) *renderComboFailure {
	return &renderComboFailure{
		reason:  reason,
		comboID: comboID,
		config:  cfg,
		err:     err,
	}
}

// aggregateRenderComboFailures orders concurrent results so repeated runs report the same diagnostic sequence.
func aggregateRenderComboFailures(failures []*renderComboFailure, total int, shardLabel string) renderComboFailures {
	sort.SliceStable(failures, func(left, right int) bool {
		return failures[left].comboID < failures[right].comboID
	})
	return renderComboFailures{
		failures:   failures,
		total:      total,
		shardLabel: shardLabel,
	}
}

// reportRenderComboFailure prints the same detailed diagnostics once workers have finished.
func reportRenderComboFailure(failure *renderComboFailure) {
	console.Errorf("Failure")
	console.Infof("reason: %s", failure.reason)
	console.Infof("combo: %s", failure.comboID)
	if failure.err != nil {
		console.Infof("error: %v", failure.err)
	}
	if failure.config != nil {
		if yamlDump, yerr := yaml.Marshal(failure.config); yerr == nil {
			console.Infof("config:\n%s", string(yamlDump))
		}
	}
}

// renderBuildTraceEnabled requires general debug output before emitting the especially noisy Go build trace.
func renderBuildTraceEnabled() bool {
	if !renderDebugEnabled() {
		return false
	}
	for _, key := range []string{"FORJ_RENDER_BUILD_TRACE", "FORJ_RENDER_GO_BUILD_TRACE"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" && value != "0" {
			return true
		}
	}
	return false
}

// renderDebugEnabled follows the renderer's established environment contract for diagnostic output.
func renderDebugEnabled() bool {
	for _, key := range []string{"FORJ_DEBUG", "DEBUG"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" && value != "0" {
			return true
		}
	}
	return false
}

package forj

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/goforj/console"
	"github.com/goforj/goforj/project"
)

const (
	devLifecycleStartupKey  = "lifecycle:startup"
	devLifecycleRestartKey  = "lifecycle:restart"
	devLifecycleReadyPath   = "/-/health"
	devLifecycleReadyLimit  = 10 * time.Second
	devLifecycleReadyEnv    = "GOFORJ_DEV_READINESS_TOKEN"
	devLifecycleReadyHeader = "X-GoForj-Dev-Readiness"
)

type devLifecycleTransactionKind uint8

const (
	devLifecycleStartup devLifecycleTransactionKind = iota + 1
	devLifecycleRestart
)

type devLifecycleTransaction struct {
	Key      string
	Kind     devLifecycleTransactionKind
	Watchers []string
	Ready    string
	Detailed bool
}

type devLifecycleTransactionSummary struct {
	BuildElapsed   time.Duration
	MigrateElapsed time.Duration
}

// installDevLifecycleReadinessToken lets the App distinguish GoForj's private readiness traffic without granting arbitrary health requests a logging bypass.
func installDevLifecycleReadinessToken() (func(), error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, fmt.Errorf("generate dev readiness token: %w", err)
	}
	previous, existed := os.LookupEnv(devLifecycleReadyEnv)
	if err := os.Setenv(devLifecycleReadyEnv, hex.EncodeToString(raw[:])); err != nil {
		return nil, fmt.Errorf("set dev readiness token: %w", err)
	}
	return func() {
		if existed {
			_ = os.Setenv(devLifecycleReadyEnv, previous)
			return
		}
		_ = os.Unsetenv(devLifecycleReadyEnv)
	}, nil
}

// waitDevLifecycleReadiness keeps startup output inside the transaction until a conventional HTTP App is serving.
func waitDevLifecycleReadiness(ctx context.Context, config *project.Config, controller *devWatcherController) error {
	if !devLifecycleNeedsHTTPReadiness(config, controller) {
		return nil
	}
	readinessURL, err := devLifecycleReadinessURL(resolveAPIURL(snapshotProcessEnv()))
	if err != nil {
		return err
	}
	readyContext, cancel := context.WithTimeout(ctx, devLifecycleReadyLimit)
	defer cancel()
	client := &http.Client{Timeout: 500 * time.Millisecond}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, requestErr := http.NewRequestWithContext(readyContext, http.MethodGet, readinessURL, nil)
		if requestErr != nil {
			return fmt.Errorf("prepare App readiness probe: %w", requestErr)
		}
		if token := strings.TrimSpace(os.Getenv(devLifecycleReadyEnv)); token != "" {
			request.Header.Set(devLifecycleReadyHeader, token)
		}
		response, requestErr := client.Do(request)
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
				return nil
			}
		}
		select {
		case <-readyContext.Done():
			return fmt.Errorf("wait for App readiness at %s: %w", readinessURL, readyContext.Err())
		case <-ticker.C:
		}
	}
}

// devLifecycleNeedsHTTPReadiness limits probing to the conventional default App whose URL GoForj owns.
func devLifecycleNeedsHTTPReadiness(config *project.Config, controller *devWatcherController) bool {
	components := appRenderComponents(config, project.DefaultApp())
	if !components.WebAPI && !components.WebUI {
		return false
	}
	if controller == nil {
		return false
	}
	for _, task := range controller.tasks {
		spec := task.spec
		if spec.Kind == devWatcherAppRun && spec.App == project.DefaultAppName && !spec.Legacy && !spec.FullProcessOverride {
			return true
		}
	}
	return false
}

// devLifecycleReadinessURL probes the framework-owned root path even when a public App URL includes a path prefix.
func devLifecycleReadinessURL(appURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(appURL))
	if err != nil {
		return "", fmt.Errorf("parse App URL for readiness: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("parse App URL for readiness: absolute URL required")
	}
	parsed.Path = devLifecycleReadyPath
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

// newDevStartupTransaction describes the first watcher generation without exposing its internal state changes.
func newDevStartupTransaction(config *project.Config, watchers []string, detailed bool) devLifecycleTransaction {
	return devLifecycleTransaction{
		Key:      devLifecycleStartupKey,
		Kind:     devLifecycleStartup,
		Watchers: compactDevWatcherNames(watchers),
		Ready:    formatDevReadyDetail(config, snapshotProcessEnv()),
		Detailed: detailed,
	}
}

// newDevRestartTransaction treats a watcher replacement as one user-visible operation.
func newDevRestartTransaction(watchers []string, detailed bool) devLifecycleTransaction {
	return devLifecycleTransaction{
		Key:      devLifecycleRestartKey,
		Kind:     devLifecycleRestart,
		Watchers: compactDevWatcherNames(watchers),
		Detailed: detailed,
	}
}

// inProgressLine renders the single reserved row shown while lifecycle work is active.
func (t devLifecycleTransaction) inProgressLine() string {
	verb := "Starting"
	if t.Kind == devLifecycleRestart {
		verb = "Restarting"
	}
	return joinDevLifecycleFields(verb, t.Watchers...)
}

// successLine renders the durable result beneath grouped infrastructure output.
func (t devLifecycleTransaction) successLine(elapsed time.Duration, summary devLifecycleTransactionSummary) string {
	verb := "Ready"
	fields := []string{}
	if t.Kind == devLifecycleRestart {
		verb = "Restarted"
		if summary.BuildElapsed > 0 {
			fields = append(fields, "build "+formatDevLifecycleDuration(summary.BuildElapsed))
		}
		if summary.MigrateElapsed > 0 {
			fields = append(fields, "migrate "+formatDevLifecycleDuration(summary.MigrateElapsed))
		}
	} else if strings.TrimSpace(t.Ready) != "" {
		fields = append(fields, t.Ready)
	}
	fields = append(fields, formatDevLifecycleDuration(elapsed))
	return console.SuccessMark() + " " + joinDevLifecycleFields(verb, fields...)
}

// failureLine identifies the failed lifecycle boundary before its retained output is replayed.
func (t devLifecycleTransaction) failureLine(elapsed time.Duration) string {
	label := "Startup failed"
	if t.Kind == devLifecycleRestart {
		label = "Restart failed"
	}
	return console.ErrorMark() + " " + joinDevLifecycleFields(label, formatDevLifecycleDuration(elapsed))
}

// joinDevLifecycleFields keeps lifecycle summaries visually compact without embedding terminal layout concerns in the runner.
func joinDevLifecycleFields(label string, fields ...string) string {
	parts := []string{console.Colorize(console.ColorBoldWhite, strings.TrimSpace(label))}
	for _, field := range fields {
		if field = strings.TrimSpace(field); field != "" {
			parts = append(parts, console.Colorize(console.ColorGray, field))
		}
	}
	return strings.Join(parts, " · ")
}

// formatDevLifecycleDuration favors readable subsecond values while retaining useful precision for longer restarts.
func formatDevLifecycleDuration(elapsed time.Duration) string {
	if elapsed < time.Second {
		return elapsed.Round(time.Millisecond).String()
	}
	return elapsed.Round(10 * time.Millisecond).String()
}

// compactDevWatcherNames translates generated watcher labels into the concepts users configured.
func compactDevWatcherNames(watchers []string) []string {
	compact := make([]string, 0, len(watchers))
	seen := map[string]bool{}
	for _, watcher := range watchers {
		watcher = strings.TrimSpace(watcher)
		label := watcher
		switch {
		case strings.HasPrefix(watcher, "Build ") && strings.Contains(watcher, " SPA "):
			label = "SPA"
		}
		if label == "" || seen[label] {
			continue
		}
		seen[label] = true
		compact = append(compact, label)
	}
	return compact
}

// formatDevReadyDetail summarizes resources already available in the persistent TUI header.
func formatDevReadyDetail(config *project.Config, env map[string]string) string {
	fields := make([]string, 0, 2)
	if appURL := resolveAPIURL(env); appURL != "" {
		if parsed, err := url.Parse(appURL); err == nil {
			port := parsed.Port()
			if port == "" {
				switch parsed.Scheme {
				case "http":
					port = "80"
				case "https":
					port = "443"
				}
			}
			if port != "" {
				fields = append(fields, "App :"+port)
			}
		}
	}
	if count := len(collectDevToolLinks(config, env)); count > 0 {
		label := "resources"
		if count == 1 {
			label = "resource"
		}
		fields = append(fields, strconv.Itoa(count)+" "+label)
	}
	return strings.Join(fields, " · ")
}

// devLifecycleDetailedOutput keeps retained lifecycle diagnostics visible when the developer explicitly asks for runner detail.
func devLifecycleDetailedOutput(watchers []devCompiledWatcher) bool {
	for _, key := range []string{"FORJ_DEBUG", "DEBUG"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" && value != "0" {
			return true
		}
	}
	for _, watcher := range watchers {
		if watcher.Verbose || watcher.ExecLog {
			return true
		}
	}
	return false
}

// formatDevLifecycleFailure appends a cause only when buffered child output did not already explain it.
func formatDevLifecycleFailure(err error) string {
	if err == nil {
		return ""
	}
	return "  " + err.Error()
}

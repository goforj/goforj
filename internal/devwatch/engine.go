package devwatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	// DefaultDebounce is the trailing-edge delay used when a watcher omits its debounce setting.
	DefaultDebounce = 300 * time.Millisecond
	// DefaultPollInterval is the snapshot interval used when polling is enabled without a duration.
	DefaultPollInterval = 250 * time.Millisecond
)

// BackendKind selects the physical filesystem event source.
type BackendKind string

const (
	// BackendAuto uses notifications where directory watches do not retain every child, otherwise filtered polling.
	BackendAuto BackendKind = "auto"
	// BackendNotify requires native filesystem notifications.
	BackendNotify BackendKind = "notify"
	// BackendPoll uses bounded, whole-tree snapshots.
	BackendPoll BackendKind = "poll"
)

// HealthState describes whether the backend provides complete physical watch coverage.
type HealthState string

const (
	// HealthStarting means physical watch coverage is not established yet.
	HealthStarting HealthState = "starting"
	// HealthHealthy means the backend currently covers every configured root.
	HealthHealthy HealthState = "healthy"
	// HealthDegraded means the backend cannot guarantee complete event coverage.
	HealthDegraded HealthState = "degraded"
	// HealthStopped means the engine has completed shutdown.
	HealthStopped HealthState = "stopped"
)

// Op identifies one or more coalesced filesystem operations.
type Op uint8

const (
	// OpCreate reports a newly observed path.
	OpCreate Op = 1 << iota
	// OpWrite reports changed file contents or metadata.
	OpWrite
	// OpRemove reports a path that no longer exists.
	OpRemove
	// OpRename reports a path moved away from its observed name.
	OpRename
)

// Matcher decides whether one normalized, root-relative path belongs to a watcher.
type Matcher interface {
	// Matches reports whether the matcher accepts a normalized, root-relative path.
	Matches(relativePath string) bool
}

type devWatchSimpleMatcherKind uint8

const (
	devWatchSimpleSuffix devWatchSimpleMatcherKind = iota
	devWatchSimpleExactBasename
	devWatchSimpleBasenamePrefix
	devWatchSimpleExactPath
)

type devWatchSimpleMatcher struct {
	kind  devWatchSimpleMatcherKind
	value string
}

// Matches applies the simple matcher's typed path rule.
func (m devWatchSimpleMatcher) Matches(relativePath string) bool {
	relativePath = normalizeDevWatchRelativePath(relativePath)
	switch m.kind {
	case devWatchSimpleSuffix:
		return strings.HasSuffix(path.Base(relativePath), m.value)
	case devWatchSimpleExactBasename:
		return path.Base(relativePath) == m.value
	case devWatchSimpleBasenamePrefix:
		return strings.HasPrefix(path.Base(relativePath), m.value)
	case devWatchSimpleExactPath:
		return relativePath == m.value
	default:
		return false
	}
}

type devWatchRegexMatcher struct {
	expression *regexp.Regexp
}

// Matches applies an explicit regular expression to a normalized relative path.
func (m devWatchRegexMatcher) Matches(relativePath string) bool {
	return m.expression.MatchString(normalizeDevWatchRelativePath(relativePath))
}

// NewMatcher compiles the readable matcher syntax used by native watcher config.
func NewMatcher(value string) (Matcher, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("watch matcher cannot be empty")
	}
	if expression, ok := strings.CutPrefix(value, "re:"); ok {
		if expression == "" {
			return nil, fmt.Errorf("watch regular expression cannot be empty")
		}
		compiled, err := regexp.Compile(expression)
		if err != nil {
			return nil, fmt.Errorf("compile watch regular expression %q: %w", expression, err)
		}
		return devWatchRegexMatcher{expression: compiled}, nil
	}
	return newDevWatchSimpleMatcher(value), nil
}

// NewLegacyRegexpMatcher compiles a wgo-compatible regular expression matcher.
func NewLegacyRegexpMatcher(pattern string) (Matcher, error) {
	compiled, err := compileLegacyRegexp(pattern)
	if err != nil {
		return nil, err
	}
	return devWatchRegexMatcher{expression: compiled}, nil
}

// compileLegacyRegexp preserves wgo's convenience escaping for dots before ASCII letters.
func compileLegacyRegexp(pattern string) (*regexp.Regexp, error) {
	if strings.HasPrefix(pattern, "./") && len(pattern) > 2 {
		pattern = pattern[2:]
	}
	var builder strings.Builder
	builder.Grow(len(pattern) + strings.Count(pattern, "."))
	for offset := 0; offset < len(pattern); {
		previous, _ := utf8.DecodeLastRuneInString(builder.String())
		current, width := utf8.DecodeRuneInString(pattern[offset:])
		next, _ := utf8.DecodeRuneInString(pattern[offset+width:])
		offset += width
		if previous != '\\' && current == '.' && (next >= 'a' && next <= 'z' || next >= 'A' && next <= 'Z') {
			builder.WriteString(`\.`)
			continue
		}
		builder.WriteRune(current)
	}
	return regexp.Compile(builder.String())
}

// newDevWatchSimpleMatcher translates common filenames and paths into typed matching behavior.
func newDevWatchSimpleMatcher(value string) Matcher {
	value = filepath.ToSlash(strings.TrimSpace(value))
	if strings.HasPrefix(value, "./") || strings.Contains(value, "/") {
		return devWatchSimpleMatcher{
			kind:  devWatchSimpleExactPath,
			value: normalizeDevWatchRelativePath(value),
		}
	}
	if prefix, ok := strings.CutSuffix(value, ".*"); ok {
		return devWatchSimpleMatcher{kind: devWatchSimpleBasenamePrefix, value: prefix + "."}
	}
	if value == ".env" {
		return devWatchSimpleMatcher{kind: devWatchSimpleExactBasename, value: value}
	}
	if strings.HasPrefix(value, ".") {
		return devWatchSimpleMatcher{kind: devWatchSimpleSuffix, value: value}
	}
	return devWatchSimpleMatcher{kind: devWatchSimpleExactBasename, value: value}
}

// Spec describes one logical watcher served by the shared physical backend.
type Spec struct {
	// Name uniquely identifies the logical watcher in emitted events.
	Name string
	// Roots contains ordered filesystem paths; every outermost physical root must be an existing non-symlink directory.
	Roots []string
	// Includes contains file matchers, with an empty list accepting every non-excluded file.
	Includes []Matcher
	// Excludes contains file matchers that take precedence over includes.
	Excludes []Matcher
	// DirectoryIncludes limits matching files to accepted directories.
	DirectoryIncludes []Matcher
	// DirectoryExcludes prunes and rejects matching directories.
	DirectoryExcludes []Matcher
	// Debounce controls the trailing-edge coalescing window.
	Debounce time.Duration
	// DebounceSet distinguishes an explicit zero-duration debounce from an omitted default.
	DebounceSet bool
	// LegacyDirectoryRegex preserves wgo full-relative-directory matching and default pruning.
	LegacyDirectoryRegex bool
}

// Matches reports whether a normalized, root-relative file path belongs to the watcher.
func (s Spec) Matches(relativePath string) bool {
	return devWatchSpecMatches(s, relativePath)
}

// EngineConfig configures logical watchers and their shared physical backend.
type EngineConfig struct {
	// Watchers contains the logical subscriptions sharing physical filesystem coverage.
	Watchers []Spec
	// Backend selects automatic, notification, or polling event delivery.
	Backend BackendKind
	// PollInterval controls snapshot frequency for the polling backend.
	PollInterval   time.Duration
	backendForTest devWatchBackend
}

// Change describes one coalesced path change in a logical watcher event.
type Change struct {
	// Path is the absolute changed path.
	Path string
	// RelativePath is resolved against the watcher's first matching root.
	RelativePath string
	// Op contains every filesystem operation coalesced for the path.
	Op Op
}

// Event contains the debounced changes routed to one logical watcher.
type Event struct {
	// Watcher identifies the logical watcher receiving the batch.
	Watcher string
	// Changes contains path-sorted changes coalesced during the debounce window.
	Changes []Change
}

// Health reports whether every configured root currently has complete physical coverage.
type Health struct {
	// State describes current physical coverage.
	State HealthState
	// Backend identifies the active event source, including automatic fallback results.
	Backend BackendKind
	// WatchedDirectories is the number of directories currently covered.
	WatchedDirectories int
	// Err contains the failure responsible for degraded or failed shutdown state.
	Err error
}

type devWatchRawEvent struct {
	path  string
	op    Op
	isDir bool
}

type devWatchBackendUpdate struct {
	healthy            bool
	watchedDirectories int
	err                error
}

type devWatchBackendStart struct {
	events             <-chan devWatchRawEvent
	updates            <-chan devWatchBackendUpdate
	stop               func() error
	watchedDirectories int
}

// devWatchBackend abstracts physical event delivery so all logical watchers share one subscription layer.
type devWatchBackend interface {
	// start defines the start behavior required from implementations.
	start(context.Context, []string, func(string) bool, func(string) bool) (devWatchBackendStart, error)
}

type devWatchPending struct {
	deadline time.Time
	changes  map[string]Change
}

// Engine owns shared filesystem coverage and dispatches debounced logical watcher events.
type Engine struct {
	config        EngineConfig
	watchers      []Spec
	roots         []string
	events        chan Event
	errors        chan error
	healthUpdates chan Health
	done          chan struct{}

	mu        sync.RWMutex
	health    Health
	cancel    context.CancelFunc
	startDone chan struct{}
	starting  bool
	started   bool
	closeErr  error
}

// NewEngine validates and normalizes a shared native watcher engine.
func NewEngine(config EngineConfig) (*Engine, error) {
	watchers, roots, err := normalizeDevWatchSpecs(config.Watchers)
	if err != nil {
		return nil, err
	}
	if config.Backend == "" {
		config.Backend = BackendAuto
	}
	switch config.Backend {
	case BackendAuto, BackendNotify, BackendPoll:
	default:
		return nil, fmt.Errorf("unsupported dev watch backend %q", config.Backend)
	}
	if config.PollInterval <= 0 {
		config.PollInterval = DefaultPollInterval
	}
	return &Engine{
		config:        config,
		watchers:      watchers,
		roots:         roots,
		events:        make(chan Event, 64),
		errors:        make(chan error, 64),
		healthUpdates: make(chan Health, 16),
		done:          make(chan struct{}),
		health: Health{
			State:   HealthStarting,
			Backend: config.Backend,
		},
	}, nil
}

// Start establishes physical watch coverage before returning to the caller.
func (e *Engine) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	e.mu.Lock()
	if e.started || e.starting {
		e.mu.Unlock()
		return fmt.Errorf("dev watch engine already started")
	}
	e.starting = true
	e.startDone = make(chan struct{})
	ctx, e.cancel = context.WithCancel(ctx)
	e.mu.Unlock()

	if err := validateDevWatchRoots(e.roots); err != nil {
		e.cancel()
		e.setHealth(Health{
			State:   HealthDegraded,
			Backend: e.config.Backend,
			Err:     err,
		})
		e.mu.Lock()
		e.starting = false
		e.cancel = nil
		close(e.startDone)
		e.mu.Unlock()
		return fmt.Errorf("validate dev watch roots: %w", err)
	}

	backend, backendKind := e.configuredBackend()
	started, err := backend.start(ctx, e.roots, e.shouldDescend, e.shouldTrackFile)
	if err != nil && e.config.Backend == BackendAuto && backendKind == BackendNotify {
		notifyErr := err
		backendKind = BackendPoll
		backend = &devWatchPollBackend{interval: e.config.PollInterval}
		started, err = backend.start(ctx, e.roots, e.shouldDescend, e.shouldTrackFile)
		if err != nil {
			err = errors.Join(
				fmt.Errorf("start filesystem notification backend: %w", notifyErr),
				fmt.Errorf("start polling fallback backend: %w", err),
			)
		}
	}
	if err != nil {
		e.cancel()
		e.setHealth(Health{
			State:   HealthDegraded,
			Backend: backendKind,
			Err:     err,
		})
		e.mu.Lock()
		e.starting = false
		e.cancel = nil
		close(e.startDone)
		e.mu.Unlock()
		return fmt.Errorf("start dev watch %s backend: %w", backendKind, err)
	}
	e.setHealth(Health{
		State:              HealthHealthy,
		Backend:            backendKind,
		WatchedDirectories: started.watchedDirectories,
	})
	go e.run(ctx, backendKind, started)
	e.mu.Lock()
	e.starting = false
	e.started = true
	close(e.startDone)
	e.mu.Unlock()
	return nil
}

// validateDevWatchRoots ensures every physical root has stable directory semantics before backend setup.
func validateDevWatchRoots(roots []string) error {
	for _, root := range roots {
		if err := validateDevWatchRoot(root); err != nil {
			return err
		}
	}
	return nil
}

// validateDevWatchRoot rejects symlinks because event paths cannot be routed consistently through their lexical alias.
func validateDevWatchRoot(root string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("inspect watched root %q: %w", root, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("watched root %q must not be a symbolic link", root)
	}
	if !info.IsDir() {
		return fmt.Errorf("watched root %q is not a directory", root)
	}
	return nil
}

// configuredBackend selects the requested physical event source.
func (e *Engine) configuredBackend() (devWatchBackend, BackendKind) {
	return e.configuredBackendForPlatform(runtime.GOOS)
}

// configuredBackendForPlatform keeps platform policy testable without changing the process environment.
func (e *Engine) configuredBackendForPlatform(goos string) (devWatchBackend, BackendKind) {
	if e.config.backendForTest != nil {
		backendKind := e.config.Backend
		if backendKind == BackendAuto {
			backendKind = BackendNotify
		}
		return e.config.backendForTest, backendKind
	}
	if e.config.Backend == BackendPoll {
		return &devWatchPollBackend{interval: e.config.PollInterval}, BackendPoll
	}
	if e.config.Backend == BackendAuto && devWatchPlatformUsesFileDescriptorNotifications(goos) {
		return &devWatchPollBackend{interval: e.config.PollInterval}, BackendPoll
	}
	return &devWatchFSNotifyBackend{}, BackendNotify
}

// devWatchPlatformUsesFileDescriptorNotifications identifies native backends that open every child of a watched directory.
func devWatchPlatformUsesFileDescriptorNotifications(goos string) bool {
	switch goos {
	case "darwin", "dragonfly", "freebsd", "netbsd", "openbsd":
		return true
	default:
		return false
	}
}

// Events returns debounced logical watcher batches.
func (e *Engine) Events() <-chan Event {
	return e.events
}

// Errors returns asynchronous backend failures; Health remains the authoritative current state.
func (e *Engine) Errors() <-chan error {
	return e.errors
}

// HealthUpdates returns state changes for physical watcher coverage.
func (e *Engine) HealthUpdates() <-chan Health {
	return e.healthUpdates
}

// Health returns the latest physical watcher coverage state.
func (e *Engine) Health() Health {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.health
}

// Done closes after the engine and its physical backend have stopped.
func (e *Engine) Done() <-chan struct{} {
	return e.done
}

// Close stops event delivery and waits for backend resources to be released.
func (e *Engine) Close() error {
	e.mu.Lock()
	starting := e.starting
	started := e.started
	cancel := e.cancel
	startDone := e.startDone
	if !starting && !started {
		e.mu.Unlock()
		return nil
	}
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if starting {
		<-startDone
		e.mu.RLock()
		started = e.started
		e.mu.RUnlock()
		if !started {
			return nil
		}
	}
	<-e.done
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.closeErr
}

// run dispatches raw events and manages independent trailing-edge debounce windows.
func (e *Engine) run(ctx context.Context, backendKind BackendKind, started devWatchBackendStart) {
	defer close(e.done)
	defer close(e.events)
	defer close(e.errors)
	defer close(e.healthUpdates)
	defer func() {
		stopErr := started.stop()
		if stopErr != nil {
			e.publishError(stopErr)
			e.mu.Lock()
			e.closeErr = stopErr
			e.mu.Unlock()
		}
		e.setHealth(Health{State: HealthStopped, Backend: backendKind, Err: stopErr})
	}()

	pending := make(map[string]devWatchPending, len(e.watchers))
	timer := time.NewTimer(time.Hour)
	stopDevWatchTimer(timer)
	for {
		timerChannel := devWatchTimerChannel(timer, pending)
		select {
		case <-ctx.Done():
			stopDevWatchTimer(timer)
			return
		case rawEvent, ok := <-started.events:
			if !ok {
				started.events = nil
				if started.updates == nil {
					return
				}
				continue
			}
			e.queueRawEvent(pending, rawEvent, time.Now())
			resetDevWatchTimer(timer, pending)
		case update, ok := <-started.updates:
			if !ok {
				started.updates = nil
				if started.events == nil {
					return
				}
				continue
			}
			e.applyBackendUpdate(backendKind, update)
		case now := <-timerChannel:
			e.flushPending(ctx, pending, now)
			resetDevWatchTimer(timer, pending)
		}
	}
}

// queueRawEvent adds a matching physical change to each logical watcher's pending batch.
func (e *Engine) queueRawEvent(pending map[string]devWatchPending, rawEvent devWatchRawEvent, now time.Time) {
	if rawEvent.isDir || rawEvent.op == 0 {
		return
	}
	for _, watcher := range e.watchers {
		relativePath, ok := relativeDevWatchPath(watcher.Roots, rawEvent.path)
		if !ok || !devWatchSpecMatches(watcher, relativePath) {
			continue
		}
		batch := pending[watcher.Name]
		if batch.changes == nil {
			batch.changes = make(map[string]Change)
		}
		change := batch.changes[rawEvent.path]
		change.Path = rawEvent.path
		change.RelativePath = relativePath
		change.Op |= rawEvent.op
		batch.changes[rawEvent.path] = change
		batch.deadline = now.Add(watcher.Debounce)
		pending[watcher.Name] = batch
	}
}

// flushPending emits all watcher batches whose debounce windows have elapsed.
func (e *Engine) flushPending(ctx context.Context, pending map[string]devWatchPending, now time.Time) {
	for _, watcher := range e.watchers {
		batch, ok := pending[watcher.Name]
		if !ok || batch.deadline.After(now) {
			continue
		}
		changes := make([]Change, 0, len(batch.changes))
		for _, change := range batch.changes {
			changes = append(changes, change)
		}
		slices.SortFunc(changes, func(left, right Change) int {
			return strings.Compare(left.Path, right.Path)
		})
		select {
		case e.events <- Event{Watcher: watcher.Name, Changes: changes}:
			delete(pending, watcher.Name)
		case <-ctx.Done():
			return
		}
	}
}

// applyBackendUpdate keeps health truthful when physical event coverage changes.
func (e *Engine) applyBackendUpdate(backendKind BackendKind, update devWatchBackendUpdate) {
	if update.err != nil {
		e.publishError(update.err)
	}
	state := HealthDegraded
	if update.healthy {
		state = HealthHealthy
	}
	e.setHealth(Health{
		State:              state,
		Backend:            backendKind,
		WatchedDirectories: update.watchedDirectories,
		Err:                update.err,
	})
}

// publishError makes errors observable without allowing an abandoned advisory channel to deadlock shutdown.
func (e *Engine) publishError(err error) {
	if err == nil {
		return
	}
	select {
	case e.errors <- err:
	default:
	}
}

// setHealth records every distinct health transition and retains it even when the update channel is full.
func (e *Engine) setHealth(health Health) {
	e.mu.Lock()
	previous := e.health
	e.health = health
	e.mu.Unlock()
	if previous.State == health.State &&
		previous.Backend == health.Backend &&
		previous.WatchedDirectories == health.WatchedDirectories &&
		errors.Is(previous.Err, health.Err) && errors.Is(health.Err, previous.Err) {
		return
	}
	select {
	case e.healthUpdates <- health:
	default:
	}
}

// shouldDescend reports whether at least one logical watcher needs a directory subtree.
func (e *Engine) shouldDescend(directory string) bool {
	for _, watcher := range e.watchers {
		if devWatchSpecNeedsDirectory(watcher, directory) {
			return true
		}
	}
	return false
}

// shouldTrackFile reports whether at least one logical watcher selected a physical file.
func (e *Engine) shouldTrackFile(filePath string) bool {
	for _, watcher := range e.watchers {
		relativePath, ok := relativeDevWatchPath(watcher.Roots, filePath)
		if ok && devWatchSpecMatches(watcher, relativePath) {
			return true
		}
	}
	return false
}

// devWatchSpecNeedsDirectory checks every root because a nested root can intentionally restore an excluded subtree.
func devWatchSpecNeedsDirectory(watcher Spec, directory string) bool {
	for _, root := range watcher.Roots {
		rootFromDirectory, rootErr := filepath.Rel(directory, root)
		if rootErr == nil && rootFromDirectory != ".." && !strings.HasPrefix(rootFromDirectory, ".."+string(filepath.Separator)) {
			return true
		}
		relativePath, err := filepath.Rel(root, directory)
		if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
			continue
		}
		relativePath = normalizeDevWatchRelativePath(relativePath)
		if relativePath == "." {
			return true
		}
		if watcher.LegacyDirectoryRegex {
			if !devWatchLegacyDirectoryPruned(watcher, relativePath) {
				return true
			}
			continue
		}
		if !devWatchDirectoryExcluded(watcher.DirectoryExcludes, relativePath) {
			return true
		}
	}
	return false
}

// normalizeDevWatchSpecs validates logical watchers and produces their minimal physical coverage roots.
func normalizeDevWatchSpecs(specs []Spec) ([]Spec, []string, error) {
	if len(specs) == 0 {
		return nil, nil, fmt.Errorf("dev watch engine requires at least one watcher")
	}
	normalized := make([]Spec, len(specs))
	rootSet := make(map[string]struct{})
	nameSet := make(map[string]struct{})
	var roots []string
	for index, spec := range specs {
		spec.Name = strings.TrimSpace(spec.Name)
		if spec.Name == "" {
			return nil, nil, fmt.Errorf("dev watcher %d has no name", index+1)
		}
		if _, exists := nameSet[spec.Name]; exists {
			return nil, nil, fmt.Errorf("duplicate dev watcher name %q", spec.Name)
		}
		nameSet[spec.Name] = struct{}{}
		if len(spec.Roots) == 0 {
			return nil, nil, fmt.Errorf("dev watcher %q has no roots", spec.Name)
		}
		spec.Roots = make([]string, len(spec.Roots))
		for rootIndex, root := range specs[index].Roots {
			absoluteRoot, err := filepath.Abs(root)
			if err != nil {
				return nil, nil, fmt.Errorf("resolve root %q for dev watcher %q: %w", root, spec.Name, err)
			}
			absoluteRoot = filepath.Clean(absoluteRoot)
			spec.Roots[rootIndex] = absoluteRoot
			if _, exists := rootSet[absoluteRoot]; !exists {
				rootSet[absoluteRoot] = struct{}{}
				roots = append(roots, absoluteRoot)
			}
		}
		if spec.Debounce < 0 {
			return nil, nil, fmt.Errorf("dev watcher %q debounce must not be negative", spec.Name)
		}
		if spec.Debounce == 0 && !spec.DebounceSet {
			spec.Debounce = DefaultDebounce
		}
		normalized[index] = spec
	}
	return normalized, outermostDevWatchRoots(roots), nil
}

// outermostDevWatchRoots avoids registering a nested root separately because its ancestor provides discovery while it is absent.
func outermostDevWatchRoots(roots []string) []string {
	outermost := make([]string, 0, len(roots))
	for _, candidate := range roots {
		covered := false
		for _, possibleAncestor := range roots {
			if candidate == possibleAncestor {
				continue
			}
			if devWatchPathWithinRoot(possibleAncestor, candidate) {
				covered = true
				break
			}
		}
		if !covered {
			outermost = append(outermost, candidate)
		}
	}
	return outermost
}

// devWatchPathWithinRoot uses filepath.Rel so containment remains correct across platform-specific volumes.
func devWatchPathWithinRoot(root string, candidate string) bool {
	relativePath, err := filepath.Rel(root, candidate)
	return err == nil && relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator))
}

// devWatchSpecMatches applies directory rules before file exclusion and inclusion rules.
func devWatchSpecMatches(spec Spec, relativePath string) bool {
	directory := path.Dir(normalizeDevWatchRelativePath(relativePath))
	if spec.LegacyDirectoryRegex {
		if devWatchMatchersMatch(spec.DirectoryExcludes, directory) {
			return false
		}
		if len(spec.DirectoryIncludes) > 0 && !devWatchMatchersMatch(spec.DirectoryIncludes, directory) {
			return false
		}
		return devWatchFileMatches(spec, relativePath)
	}
	if devWatchDirectoryExcluded(spec.DirectoryExcludes, directory) {
		return false
	}
	if len(spec.DirectoryIncludes) > 0 && !devWatchDirectoryIncluded(spec.DirectoryIncludes, directory) {
		return false
	}
	return devWatchFileMatches(spec, relativePath)
}

// devWatchFileMatches applies file exclusions before inclusive matcher rules.
func devWatchFileMatches(spec Spec, relativePath string) bool {
	for _, matcher := range spec.Excludes {
		if matcher.Matches(relativePath) {
			return false
		}
	}
	if len(spec.Includes) == 0 {
		return true
	}
	for _, matcher := range spec.Includes {
		if matcher.Matches(relativePath) {
			return true
		}
	}
	return false
}

// devWatchLegacyDirectoryAccessible simulates wgo's full-path checks at each walked directory.
func devWatchLegacyDirectoryAccessible(spec Spec, relativeDirectory string) bool {
	relativeDirectory = normalizeDevWatchRelativePath(relativeDirectory)
	if relativeDirectory == "." {
		return true
	}
	parts := strings.Split(relativeDirectory, "/")
	for index := range parts {
		ancestor := strings.Join(parts[:index+1], "/")
		if devWatchLegacyDirectoryPruned(spec, ancestor) {
			return false
		}
	}
	return true
}

// devWatchLegacyDirectoryPruned preserves wgo exclusion precedence and default directory filtering.
func devWatchLegacyDirectoryPruned(spec Spec, relativeDirectory string) bool {
	if devWatchMatchersMatch(spec.DirectoryExcludes, relativeDirectory) {
		return true
	}
	if devWatchMatchersMatch(spec.DirectoryIncludes, relativeDirectory) {
		return false
	}
	baseName := path.Base(normalizeDevWatchRelativePath(relativeDirectory))
	switch baseName {
	case ".git", ".hg", ".svn", ".idea", ".vscode", ".settings", "node_modules":
		return true
	default:
		return strings.HasPrefix(baseName, ".")
	}
}

// devWatchMatchersMatch reports whether any matcher accepts the same full relative path.
func devWatchMatchersMatch(matchers []Matcher, relativePath string) bool {
	for _, matcher := range matchers {
		if matcher.Matches(relativePath) {
			return true
		}
	}
	return false
}

// devWatchDirectoryExcluded checks every ancestor so a basename exclusion prunes its entire subtree.
func devWatchDirectoryExcluded(matchers []Matcher, relativeDirectory string) bool {
	return devWatchDirectoryAncestorMatches(matchers, relativeDirectory)
}

// devWatchDirectoryIncluded checks ancestors so an included directory naturally includes descendants.
func devWatchDirectoryIncluded(matchers []Matcher, relativeDirectory string) bool {
	return devWatchDirectoryAncestorMatches(matchers, relativeDirectory)
}

// devWatchDirectoryAncestorMatches applies directory matchers from the root toward the leaf.
func devWatchDirectoryAncestorMatches(matchers []Matcher, relativeDirectory string) bool {
	relativeDirectory = normalizeDevWatchRelativePath(relativeDirectory)
	if relativeDirectory == "." {
		for _, matcher := range matchers {
			if matcher.Matches(relativeDirectory) {
				return true
			}
		}
		return false
	}
	parts := strings.Split(relativeDirectory, "/")
	for index := range parts {
		ancestor := strings.Join(parts[:index+1], "/")
		for _, matcher := range matchers {
			if matcher.Matches(ancestor) {
				return true
			}
		}
	}
	return false
}

// relativeDevWatchPath resolves an absolute path against the first matching root.
func relativeDevWatchPath(roots []string, absolutePath string) (string, bool) {
	absolutePath = filepath.Clean(absolutePath)
	for _, root := range roots {
		relativePath, err := filepath.Rel(root, absolutePath)
		if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
			continue
		}
		return normalizeDevWatchRelativePath(relativePath), true
	}
	return "", false
}

// normalizeDevWatchRelativePath gives every matcher stable forward-slash input.
func normalizeDevWatchRelativePath(relativePath string) string {
	relativePath = filepath.ToSlash(strings.TrimSpace(relativePath))
	relativePath = strings.TrimPrefix(relativePath, "./")
	if relativePath == "" {
		return "."
	}
	return path.Clean(relativePath)
}

// devWatchTimerChannel returns a disabled channel when no watcher has pending changes.
func devWatchTimerChannel(timer *time.Timer, pending map[string]devWatchPending) <-chan time.Time {
	if len(pending) == 0 {
		return nil
	}
	return timer.C
}

// resetDevWatchTimer schedules the earliest pending debounce deadline.
func resetDevWatchTimer(timer *time.Timer, pending map[string]devWatchPending) {
	stopDevWatchTimer(timer)
	var earliest time.Time
	for _, batch := range pending {
		if earliest.IsZero() || batch.deadline.Before(earliest) {
			earliest = batch.deadline
		}
	}
	if earliest.IsZero() {
		return
	}
	delay := time.Until(earliest)
	if delay < 0 {
		delay = 0
	}
	timer.Reset(delay)
}

// stopDevWatchTimer stops and drains a reusable debounce timer.
func stopDevWatchTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

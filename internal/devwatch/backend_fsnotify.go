package devwatch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const devWatchFSNotifyRetryInterval = 100 * time.Millisecond

type devWatchFSNotifyBackend struct{}

type devWatchFSNotifyRuntimeState struct {
	healthy            bool
	watchedDirectories int
	errText            string
	retryNeeded        bool
	notificationFailed bool
}

// start recursively registers the union of roots with one fsnotify watcher.
func (b *devWatchFSNotifyBackend) start(
	ctx context.Context,
	roots []string,
	shouldDescend func(string) bool,
) (devWatchBackendStart, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return devWatchBackendStart{}, fmt.Errorf("create filesystem watcher: %w", err)
	}

	watched := make(map[string]struct{})
	recoveryDirectories := make(map[string]struct{})
	knownDirectories := make(map[string]struct{})
	knownFiles := make(map[string]struct{})
	if err := addDevWatchRecoveryDirectories(watcher, roots, watched, recoveryDirectories); err != nil {
		_ = watcher.Close()
		return devWatchBackendStart{}, err
	}
	for _, root := range roots {
		if err := validateDevWatchRoot(root); err != nil {
			_ = watcher.Close()
			return devWatchBackendStart{}, err
		}
		if err := addDevWatchDirectories(ctx, watcher, root, true, shouldDescend, watched, knownDirectories, knownFiles, nil); err != nil {
			_ = watcher.Close()
			return devWatchBackendStart{}, err
		}
	}
	if err := devWatchFSNotifyCoverageError(roots, watched); err != nil {
		_ = watcher.Close()
		return devWatchBackendStart{}, err
	}

	backendCtx, cancel := context.WithCancel(ctx)
	events := make(chan devWatchRawEvent, 256)
	updates := make(chan devWatchBackendUpdate, 16)
	done := make(chan struct{})
	initialDirectoryCount := len(watched)
	go runDevWatchFSNotifyBackend(
		backendCtx,
		watcher,
		roots,
		shouldDescend,
		watched,
		recoveryDirectories,
		knownDirectories,
		knownFiles,
		events,
		updates,
		done,
	)

	var stopOnce sync.Once
	var stopErr error
	stop := func() error {
		stopOnce.Do(func() {
			cancel()
			if err := watcher.Close(); err != nil && !strings.Contains(err.Error(), "already closed") {
				stopErr = fmt.Errorf("close filesystem watcher: %w", err)
			}
			<-done
		})
		return stopErr
	}
	return devWatchBackendStart{
		events:             events,
		updates:            updates,
		stop:               stop,
		watchedDirectories: initialDirectoryCount,
	}, nil
}

// runDevWatchFSNotifyBackend converts native notifications and registers newly created subtrees.
func runDevWatchFSNotifyBackend(
	ctx context.Context,
	watcher *fsnotify.Watcher,
	roots []string,
	shouldDescend func(string) bool,
	watched map[string]struct{},
	recoveryDirectories map[string]struct{},
	knownDirectories map[string]struct{},
	knownFiles map[string]struct{},
	events chan<- devWatchRawEvent,
	updates chan<- devWatchBackendUpdate,
	done chan<- struct{},
) {
	defer close(done)
	defer close(events)
	defer close(updates)
	retryTicker := time.NewTicker(devWatchFSNotifyRetryInterval)
	defer retryTicker.Stop()
	state := devWatchFSNotifyRuntimeState{
		healthy:            true,
		watchedDirectories: len(watched),
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-retryTicker.C:
			if !state.retryNeeded || state.notificationFailed {
				continue
			}
			retryDevWatchFSNotifyCoverage(
				ctx,
				watcher,
				roots,
				shouldDescend,
				watched,
				recoveryDirectories,
				knownDirectories,
				knownFiles,
				events,
				updates,
				&state,
			)
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			state.notificationFailed = true
			state.retryNeeded = false
			publishDevWatchFSNotifyHealth(
				ctx,
				updates,
				&state,
				false,
				len(watched),
				fmt.Errorf("filesystem watcher reported incomplete coverage: %w", err),
			)
		case nativeEvent, ok := <-watcher.Events:
			if !ok {
				return
			}
			handleDevWatchFSNotifyEvent(
				ctx,
				watcher,
				roots,
				shouldDescend,
				watched,
				recoveryDirectories,
				knownDirectories,
				knownFiles,
				events,
				updates,
				&state,
				nativeEvent,
			)
		}
	}
}

// handleDevWatchFSNotifyEvent maps native operations and closes the new-directory registration race.
func handleDevWatchFSNotifyEvent(
	ctx context.Context,
	watcher *fsnotify.Watcher,
	roots []string,
	shouldDescend func(string) bool,
	watched map[string]struct{},
	recoveryDirectories map[string]struct{},
	knownDirectories map[string]struct{},
	knownFiles map[string]struct{},
	events chan<- devWatchRawEvent,
	updates chan<- devWatchBackendUpdate,
	state *devWatchFSNotifyRuntimeState,
	nativeEvent fsnotify.Event,
) {
	operation := devWatchOperationFromFSNotify(nativeEvent.Op)
	if operation == 0 {
		return
	}
	absolutePath := filepath.Clean(nativeEvent.Name)
	belongsToRoot := pathBelongsToDevWatchRoots(roots, absolutePath)
	_, wasWatchedDirectory := watched[absolutePath]
	_, wasRecoveryDirectory := recoveryDirectories[absolutePath]
	_, wasKnownDirectory := knownDirectories[absolutePath]
	isDirectory := wasWatchedDirectory || wasRecoveryDirectory || wasKnownDirectory || isDevWatchRoot(roots, absolutePath)
	if info, err := os.Stat(absolutePath); err == nil {
		isDirectory = info.IsDir()
	}

	rootCreated := nativeEvent.Has(fsnotify.Create) && isDirectory && isDevWatchRoot(roots, absolutePath)
	if rootCreated {
		removedFiles := removeDevWatchFileTree(knownFiles, absolutePath)
		for _, removedFile := range removedFiles {
			publishDevWatchRawEvent(ctx, events, devWatchRawEvent{path: removedFile, op: OpRemove})
		}
		removeDevWatchRegisteredDirectoryTree(watcher, watched, absolutePath)
		removeDevWatchDirectoryTree(knownDirectories, absolutePath)
	}

	if nativeEvent.Has(fsnotify.Create) && isDirectory && belongsToRoot {
		var discoveredFiles []string
		if err := addDevWatchDirectories(
			ctx,
			watcher,
			absolutePath,
			isDevWatchRoot(roots, absolutePath),
			shouldDescend,
			watched,
			knownDirectories,
			knownFiles,
			&discoveredFiles,
		); err != nil {
			state.retryNeeded = true
			publishDevWatchFSNotifyHealth(ctx, updates, state, false, len(watched), err)
		} else {
			publishDevWatchDiscoveredFiles(ctx, events, discoveredFiles)
			refreshDevWatchFSNotifyCoverage(ctx, roots, watched, updates, state)
		}
	}

	if isDirectory && (nativeEvent.Has(fsnotify.Remove) || nativeEvent.Has(fsnotify.Rename)) {
		removedFiles := removeDevWatchFileTree(knownFiles, absolutePath)
		for _, removedFile := range removedFiles {
			publishDevWatchRawEvent(ctx, events, devWatchRawEvent{
				path: removedFile,
				op:   operation & (OpRemove | OpRename),
			})
		}
		removeDevWatchRegisteredDirectoryTree(watcher, watched, absolutePath)
		removeDevWatchDirectoryTree(recoveryDirectories, absolutePath)
		removeDevWatchDirectoryTree(knownDirectories, absolutePath)
		refreshDevWatchFSNotifyCoverage(ctx, roots, watched, updates, state)
	}
	if !isDirectory && belongsToRoot {
		if operation&(OpCreate|OpWrite) != 0 {
			knownFiles[absolutePath] = struct{}{}
		}
		if operation&(OpRemove|OpRename) != 0 {
			delete(knownFiles, absolutePath)
		}
	}

	if belongsToRoot {
		publishDevWatchRawEvent(ctx, events, devWatchRawEvent{
			path:  absolutePath,
			op:    operation,
			isDir: isDirectory,
		})
	}
}

// retryDevWatchFSNotifyCoverage restores parent guards and recursively registers roots after transient removal.
func retryDevWatchFSNotifyCoverage(
	ctx context.Context,
	watcher *fsnotify.Watcher,
	roots []string,
	shouldDescend func(string) bool,
	watched map[string]struct{},
	recoveryDirectories map[string]struct{},
	knownDirectories map[string]struct{},
	knownFiles map[string]struct{},
	events chan<- devWatchRawEvent,
	updates chan<- devWatchBackendUpdate,
	state *devWatchFSNotifyRuntimeState,
) {
	if err := addDevWatchRecoveryDirectories(watcher, roots, watched, recoveryDirectories); err != nil {
		publishDevWatchFSNotifyHealth(ctx, updates, state, false, len(watched), err)
		return
	}
	var discoveredFiles []string
	defer func() {
		publishDevWatchDiscoveredFiles(ctx, events, discoveredFiles)
	}()
	for _, root := range roots {
		if err := validateDevWatchRoot(root); err != nil {
			publishDevWatchFSNotifyHealth(ctx, updates, state, false, len(watched), err)
			return
		}
		if err := addDevWatchDirectories(
			ctx,
			watcher,
			root,
			true,
			shouldDescend,
			watched,
			knownDirectories,
			knownFiles,
			&discoveredFiles,
		); err != nil {
			publishDevWatchFSNotifyHealth(ctx, updates, state, false, len(watched), err)
			return
		}
	}
	refreshDevWatchFSNotifyCoverage(ctx, roots, watched, updates, state)
}

// addDevWatchRecoveryDirectories watches root parents so replacing a root remains observable outside that root's inode.
func addDevWatchRecoveryDirectories(
	watcher *fsnotify.Watcher,
	roots []string,
	watched map[string]struct{},
	recoveryDirectories map[string]struct{},
) error {
	for _, root := range roots {
		root = filepath.Clean(root)
		parent := filepath.Dir(root)
		if parent == root {
			continue
		}
		if _, covered := watched[parent]; covered {
			continue
		}
		if _, exists := recoveryDirectories[parent]; exists {
			continue
		}
		info, err := os.Stat(parent)
		if err != nil {
			return fmt.Errorf("inspect watch recovery directory %q: %w", parent, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("watch recovery path %q is not a directory", parent)
		}
		if err := watcher.Add(parent); err != nil {
			return fmt.Errorf("watch recovery directory %q: %w", parent, err)
		}
		recoveryDirectories[parent] = struct{}{}
	}
	return nil
}

// refreshDevWatchFSNotifyCoverage derives health from root registrations after one filesystem transition.
func refreshDevWatchFSNotifyCoverage(
	ctx context.Context,
	roots []string,
	watched map[string]struct{},
	updates chan<- devWatchBackendUpdate,
	state *devWatchFSNotifyRuntimeState,
) {
	if state.notificationFailed {
		return
	}
	if coverageErr := devWatchFSNotifyCoverageError(roots, watched); coverageErr != nil {
		state.retryNeeded = true
		publishDevWatchFSNotifyHealth(ctx, updates, state, false, len(watched), coverageErr)
		return
	}
	state.retryNeeded = false
	publishDevWatchFSNotifyHealth(ctx, updates, state, true, len(watched), nil)
}

// devWatchFSNotifyCoverageError identifies the first root whose notification registration disappeared.
func devWatchFSNotifyCoverageError(roots []string, watched map[string]struct{}) error {
	for _, root := range roots {
		root = filepath.Clean(root)
		if _, covered := watched[root]; !covered {
			return fmt.Errorf("watched root %q is no longer available", root)
		}
	}
	return nil
}

// publishDevWatchFSNotifyHealth avoids repeating the same degradation on every recovery attempt.
func publishDevWatchFSNotifyHealth(
	ctx context.Context,
	updates chan<- devWatchBackendUpdate,
	state *devWatchFSNotifyRuntimeState,
	healthy bool,
	watchedDirectories int,
	err error,
) {
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	if state.healthy == healthy &&
		state.watchedDirectories == watchedDirectories &&
		state.errText == errText {
		return
	}
	state.healthy = healthy
	state.watchedDirectories = watchedDirectories
	state.errText = errText
	publishDevWatchBackendUpdate(ctx, updates, devWatchBackendUpdate{
		healthy:            healthy,
		watchedDirectories: watchedDirectories,
		err:                err,
	})
}

// publishDevWatchDiscoveredFiles closes the registration race for files created before a recovered subtree was scanned.
func publishDevWatchDiscoveredFiles(ctx context.Context, events chan<- devWatchRawEvent, discoveredFiles []string) {
	for _, discoveredFile := range discoveredFiles {
		publishDevWatchRawEvent(ctx, events, devWatchRawEvent{
			path: discoveredFile,
			op:   OpCreate,
		})
	}
}

// addDevWatchDirectories registers a subtree atomically from the engine's health perspective.
func addDevWatchDirectories(
	ctx context.Context,
	watcher *fsnotify.Watcher,
	root string,
	requiredRoot bool,
	shouldDescend func(string) bool,
	watched map[string]struct{},
	knownDirectories map[string]struct{},
	knownFiles map[string]struct{},
	discoveredFiles *[]string,
) error {
	root = filepath.Clean(root)
	return filepath.WalkDir(root, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) && (!requiredRoot || filepath.Clean(currentPath) != root) {
				return nil
			}
			return fmt.Errorf("walk watch directory %q: %w", currentPath, walkErr)
		}
		currentPath = filepath.Clean(currentPath)
		if !entry.IsDir() {
			_, alreadyKnown := knownFiles[currentPath]
			knownFiles[currentPath] = struct{}{}
			if discoveredFiles != nil && !alreadyKnown {
				*discoveredFiles = append(*discoveredFiles, currentPath)
			}
			return nil
		}
		knownDirectories[currentPath] = struct{}{}
		if !shouldDescend(currentPath) {
			return filepath.SkipDir
		}
		if _, exists := watched[currentPath]; exists {
			return nil
		}
		if err := watcher.Add(currentPath); err != nil {
			if errors.Is(err, fs.ErrNotExist) && (!requiredRoot || currentPath != root) {
				return filepath.SkipDir
			}
			return fmt.Errorf("watch directory %q: %w", currentPath, err)
		}
		watched[currentPath] = struct{}{}
		return nil
	})
}

// removeDevWatchFileTree returns remembered files because their paths cannot be inspected after a subtree disappears.
func removeDevWatchFileTree(knownFiles map[string]struct{}, removedRoot string) []string {
	removedRoot = filepath.Clean(removedRoot)
	prefix := removedRoot + string(filepath.Separator)
	var removedFiles []string
	for filePath := range knownFiles {
		if strings.HasPrefix(filePath, prefix) {
			removedFiles = append(removedFiles, filePath)
			delete(knownFiles, filePath)
		}
	}
	return removedFiles
}

// removeDevWatchRegisteredDirectoryTree drops stale inode registrations before a root path is reused.
func removeDevWatchRegisteredDirectoryTree(
	watcher *fsnotify.Watcher,
	watched map[string]struct{},
	removedRoot string,
) {
	removedRoot = filepath.Clean(removedRoot)
	prefix := removedRoot + string(filepath.Separator)
	for directory := range watched {
		if directory != removedRoot && !strings.HasPrefix(directory, prefix) {
			continue
		}
		_ = watcher.Remove(directory)
		delete(watched, directory)
	}
}

// removeDevWatchDirectoryTree forgets coverage removed by a directory rename or deletion.
func removeDevWatchDirectoryTree(watched map[string]struct{}, removedRoot string) {
	removedRoot = filepath.Clean(removedRoot)
	prefix := removedRoot + string(filepath.Separator)
	for directory := range watched {
		if directory == removedRoot || strings.HasPrefix(directory, prefix) {
			delete(watched, directory)
		}
	}
}

// devWatchOperationFromFSNotify preserves every lifecycle-relevant filesystem operation.
func devWatchOperationFromFSNotify(operation fsnotify.Op) Op {
	var result Op
	if operation.Has(fsnotify.Create) {
		result |= OpCreate
	}
	if operation.Has(fsnotify.Write) {
		result |= OpWrite
	}
	if operation.Has(fsnotify.Remove) {
		result |= OpRemove
	}
	if operation.Has(fsnotify.Rename) {
		result |= OpRename
	}
	return result
}

// pathBelongsToDevWatchRoots prevents backend bookkeeping paths from leaking into logical dispatch.
func pathBelongsToDevWatchRoots(roots []string, absolutePath string) bool {
	_, ok := relativeDevWatchPath(roots, absolutePath)
	return ok
}

// isDevWatchRoot reports whether a directory event removed configured physical coverage.
func isDevWatchRoot(roots []string, absolutePath string) bool {
	absolutePath = filepath.Clean(absolutePath)
	for _, root := range roots {
		if filepath.Clean(root) == absolutePath {
			return true
		}
	}
	return false
}

// publishDevWatchRawEvent stops backend delivery promptly during engine shutdown.
func publishDevWatchRawEvent(ctx context.Context, events chan<- devWatchRawEvent, event devWatchRawEvent) {
	select {
	case events <- event:
	case <-ctx.Done():
	}
}

// publishDevWatchBackendUpdate stops health delivery promptly during engine shutdown.
func publishDevWatchBackendUpdate(ctx context.Context, updates chan<- devWatchBackendUpdate, update devWatchBackendUpdate) {
	select {
	case updates <- update:
	case <-ctx.Done():
	}
}

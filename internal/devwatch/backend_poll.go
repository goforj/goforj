package devwatch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type devWatchPollBackend struct {
	interval time.Duration
}

type devWatchFileFingerprint struct {
	modTime       time.Time
	size          int64
	mode          fs.FileMode
	info          fs.FileInfo
	identityKnown bool
}

type devWatchSnapshot struct {
	files       map[string]devWatchFileFingerprint
	directories int
}

// start captures a baseline and launches one bounded snapshot loop for every physical root.
func (b *devWatchPollBackend) start(
	ctx context.Context,
	roots []string,
	shouldDescend func(string) bool,
	shouldTrackFile func(string) bool,
) (devWatchBackendStart, error) {
	interval := b.interval
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	previous, err := snapshotDevWatchRoots(ctx, roots, shouldDescend, shouldTrackFile)
	if err != nil {
		return devWatchBackendStart{}, err
	}

	backendCtx, cancel := context.WithCancel(ctx)
	events := make(chan devWatchRawEvent, 256)
	updates := make(chan devWatchBackendUpdate, 16)
	done := make(chan struct{})
	go runDevWatchPollBackend(backendCtx, roots, shouldDescend, shouldTrackFile, interval, previous, events, updates, done)

	var stopOnce sync.Once
	stop := func() error {
		stopOnce.Do(func() {
			cancel()
			<-done
		})
		return nil
	}
	return devWatchBackendStart{
		events:             events,
		updates:            updates,
		stop:               stop,
		watchedDirectories: previous.directories,
	}, nil
}

// runDevWatchPollBackend diffs complete snapshots without creating per-file goroutines.
func runDevWatchPollBackend(
	ctx context.Context,
	roots []string,
	shouldDescend func(string) bool,
	shouldTrackFile func(string) bool,
	interval time.Duration,
	previous devWatchSnapshot,
	events chan<- devWatchRawEvent,
	updates chan<- devWatchBackendUpdate,
	done chan<- struct{},
) {
	defer close(done)
	defer close(events)
	defer close(updates)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	healthy := true
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current, err := snapshotDevWatchRoots(ctx, roots, shouldDescend, shouldTrackFile)
			if err != nil {
				if healthy {
					healthy = false
					publishDevWatchBackendUpdate(ctx, updates, devWatchBackendUpdate{
						healthy:            false,
						watchedDirectories: previous.directories,
						err:                err,
					})
				}
				continue
			}
			if !healthy || current.directories != previous.directories {
				healthy = true
				publishDevWatchBackendUpdate(ctx, updates, devWatchBackendUpdate{
					healthy:            true,
					watchedDirectories: current.directories,
				})
			}
			publishDevWatchSnapshotDiff(ctx, events, previous, current)
			previous = current
		}
	}
}

// snapshotDevWatchRoots captures file metadata while applying shared directory pruning.
func snapshotDevWatchRoots(
	ctx context.Context,
	roots []string,
	shouldDescend func(string) bool,
	shouldTrackFile func(string) bool,
) (devWatchSnapshot, error) {
	snapshot := devWatchSnapshot{files: make(map[string]devWatchFileFingerprint)}
	seenDirectories := make(map[string]struct{})
	for _, root := range roots {
		if err := validateDevWatchRoot(root); err != nil {
			return devWatchSnapshot{}, err
		}
		err := filepath.WalkDir(root, func(currentPath string, entry fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if walkErr != nil {
				return devWatchPollPathError(root, currentPath, walkErr)
			}
			currentPath = filepath.Clean(currentPath)
			if entry.IsDir() {
				if !shouldDescend(currentPath) {
					return filepath.SkipDir
				}
				seenDirectories[currentPath] = struct{}{}
				return nil
			}
			if !shouldTrackFile(currentPath) {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return fmt.Errorf("stat polled file %q: %w", currentPath, err)
			}
			snapshot.files[currentPath] = newDevWatchFileFingerprint(info)
			return nil
		})
		if err != nil {
			return devWatchSnapshot{}, err
		}
		if err := validateDevWatchRoot(root); err != nil {
			return devWatchSnapshot{}, err
		}
	}
	snapshot.directories = len(seenDirectories)
	return snapshot, nil
}

// newDevWatchFileFingerprint captures identity immediately because Windows resolves path-based file IDs lazily.
func newDevWatchFileFingerprint(info fs.FileInfo) devWatchFileFingerprint {
	return devWatchFileFingerprint{
		modTime:       info.ModTime(),
		size:          info.Size(),
		mode:          info.Mode(),
		info:          info,
		identityKnown: os.SameFile(info, info),
	}
}

// matches reports whether metadata and the underlying filesystem object are unchanged.
func (f devWatchFileFingerprint) matches(other devWatchFileFingerprint) bool {
	if f.modTime != other.modTime || f.size != other.size || f.mode != other.mode {
		return false
	}
	// A transient identity lookup failure should cause a harmless rebuild instead of hiding a replacement.
	if !f.identityKnown || !other.identityKnown {
		return false
	}
	return os.SameFile(f.info, other.info)
}

// devWatchPollPathError ignores vanished non-root entries while preserving physical root failures.
func devWatchPollPathError(root string, currentPath string, err error) error {
	if errors.Is(err, fs.ErrNotExist) && filepath.Clean(currentPath) != filepath.Clean(root) {
		return nil
	}
	return fmt.Errorf("poll watch path %q: %w", currentPath, err)
}

// publishDevWatchSnapshotDiff converts one complete polling diff into lifecycle operations.
func publishDevWatchSnapshotDiff(
	ctx context.Context,
	events chan<- devWatchRawEvent,
	previous devWatchSnapshot,
	current devWatchSnapshot,
) {
	for filePath, currentFingerprint := range current.files {
		previousFingerprint, existed := previous.files[filePath]
		if !existed {
			publishDevWatchRawEvent(ctx, events, devWatchRawEvent{path: filePath, op: OpCreate})
			continue
		}
		if !currentFingerprint.matches(previousFingerprint) {
			publishDevWatchRawEvent(ctx, events, devWatchRawEvent{path: filePath, op: OpWrite})
		}
	}
	for filePath := range previous.files {
		if _, exists := current.files[filePath]; !exists {
			publishDevWatchRawEvent(ctx, events, devWatchRawEvent{path: filePath, op: OpRemove})
		}
	}
}

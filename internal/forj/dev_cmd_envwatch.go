package forj

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"
)

type devEnvFileFingerprint struct {
	modTimeUnixNano int64
	size            int64
}

type devAppTargetFingerprint struct {
	names []string
}

var suppressedDevEnvTriggerCount atomic.Int32

func suppressNextDevEnvTrigger() {
	suppressedDevEnvTriggerCount.Add(1)
}

func consumeSuppressedDevEnvTrigger() bool {
	for {
		current := suppressedDevEnvTriggerCount.Load()
		if current <= 0 {
			return false
		}
		if suppressedDevEnvTriggerCount.CompareAndSwap(current, current-1) {
			return true
		}
	}
}

func startDevEnvFileWatcher(ctx context.Context, trigger func(), interval time.Duration) func() {
	if trigger == nil {
		return func() {}
	}
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	stopCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		prev, _ := snapshotDevEnvFiles()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stopCh:
				return
			case <-ticker.C:
				current, err := snapshotDevEnvFiles()
				if err != nil {
					continue
				}
				if devEnvFilesChanged(prev, current) {
					prev = current
					if consumeSuppressedDevEnvTrigger() {
						continue
					}
					trigger()
				}
			}
		}
	}()
	return func() {
		close(stopCh)
	}
}

func startDevAppTargetWatcher(ctx context.Context, trigger func(), interval time.Duration) func() {
	if trigger == nil {
		return func() {}
	}
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	stopCh := make(chan struct{})
	prev := snapshotDevAppTargets()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		pending := devAppTargetFingerprint{}
		pendingTicks := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-stopCh:
				return
			case <-ticker.C:
				current := snapshotDevAppTargets()
				if !devAppTargetsChanged(prev, current) {
					pending = devAppTargetFingerprint{}
					pendingTicks = 0
					continue
				}
				if !devAppTargetsChanged(pending, current) {
					pendingTicks++
				} else {
					pending = current
					pendingTicks = 1
				}
				if pendingTicks < 2 {
					continue
				}
				prev = current
				pending = devAppTargetFingerprint{}
				pendingTicks = 0
				trigger()
			}
		}
	}()
	return func() {
		close(stopCh)
	}
}

func snapshotDevEnvFiles() (map[string]devEnvFileFingerprint, error) {
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil, err
	}
	out := make(map[string]devEnvFileFingerprint)
	for _, entry := range entries {
		name := entry.Name()
		if name != ".env" && !strings.HasPrefix(name, ".env.") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		path := filepath.Clean(name)
		out[path] = devEnvFileFingerprint{
			modTimeUnixNano: info.ModTime().UnixNano(),
			size:            info.Size(),
		}
	}
	return out, nil
}

func devEnvFilesChanged(prev, current map[string]devEnvFileFingerprint) bool {
	if len(prev) != len(current) {
		return true
	}
	prevKeys := make([]string, 0, len(prev))
	currentKeys := make([]string, 0, len(current))
	for key := range prev {
		prevKeys = append(prevKeys, key)
	}
	for key := range current {
		currentKeys = append(currentKeys, key)
	}
	slices.Sort(prevKeys)
	slices.Sort(currentKeys)
	if !slices.Equal(prevKeys, currentKeys) {
		return true
	}
	for _, key := range prevKeys {
		if prev[key] != current[key] {
			return true
		}
	}
	return false
}

func snapshotDevAppTargets() devAppTargetFingerprint {
	return devAppTargetFingerprint{names: devTargetNames(activeDevTargets())}
}

func devAppTargetsChanged(prev, current devAppTargetFingerprint) bool {
	return !slices.Equal(prev.names, current.names)
}

package forj

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/goforj/goforj/project"
)

const devStartupProgressCapability = "dev-startup-progress.v1"

// devStartupProgress owns an optional diagnostic stream, never lifecycle authority.
type devStartupProgress struct {
	mu        sync.Mutex
	file      *os.File
	session   string
	sequence  uint64
	bytes     int
	completed bool
	disabled  bool
}

// devStartupProgressEvent is a bounded record that excludes commands, environment, and error output.
type devStartupProgressEvent struct {
	SchemaVersion int    `json:"schema_version"`
	Session       string `json:"session"`
	Sequence      uint64 `json:"sequence"`
	Phase         string `json:"phase"`
	Label         string `json:"label"`
	Status        string `json:"status"`
	ElapsedMS     int64  `json:"elapsed_ms"`
}

// openDevStartupProgress requires an empty, caller-created file so telemetry cannot overwrite project files.
func openDevStartupProgress(path, session string) (*devStartupProgress, error) {
	if path == "" && session == "" {
		return nil, nil
	}
	if !filepath.IsAbs(path) || !validDevProgressSession(session) {
		return nil, errors.New("startup progress requires an absolute file and a bounded session identifier")
	}
	parent := filepath.Dir(path)
	canonical, err := filepath.EvalSymlinks(parent)
	if err != nil || canonical != parent {
		return nil, errors.New("startup progress directory must be canonical")
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return nil, fmt.Errorf("open startup progress directory: %w", err)
	}
	defer root.Close()
	info, err := root.Lstat(filepath.Base(path))
	if err != nil || !info.Mode().IsRegular() || info.Size() != 0 || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
		return nil, errors.New("startup progress file must be empty, regular, and private (0600)")
	}
	file, err := root.OpenFile(filepath.Base(path), os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return nil, fmt.Errorf("open startup progress stream: %w", err)
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		file.Close()
		return nil, errors.New("startup progress file changed during admission")
	}
	return &devStartupProgress{file: file, session: session}, nil
}

// runDevInitialSPABuildsWithProgress observes the frontend phase without altering subprocess output ownership.
func runDevInitialSPABuildsWithProgress(config *project.Config, stdout, stderr io.Writer, progress *devStartupProgress) (built bool, resultErr error) {
	finish := progress.phase("frontend.build", "Build frontend")
	defer func() { finish(resultErr == nil) }()
	return runDevInitialSPABuilds(config, stdout, stderr)
}

// validDevProgressSession excludes whitespace, paths, and unbounded correlation values.
func validDevProgressSession(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_') {
			return false
		}
	}
	return true
}

// firstDevStartupProgress preserves existing internal callers when progress is not requested.
func firstDevStartupProgress(values []*devStartupProgress) *devStartupProgress {
	if len(values) > 0 {
		return values[0]
	}
	return nil
}

// phase records one optional startup span without copying subprocess error text.
func (progress *devStartupProgress) phase(phase, label string) func(bool) {
	if progress == nil {
		return func(bool) {}
	}
	started := time.Now()
	progress.emit(phase, label, "started", 0)
	return func(success bool) {
		status := "failed"
		if success {
			status = "completed"
		}
		progress.emit(phase, label, status, time.Since(started).Milliseconds())
	}
}

// emit bounds diagnostic writes and disables a broken consumer without failing development work.
func (progress *devStartupProgress) emit(phase, label, status string, elapsed int64) {
	if progress == nil {
		return
	}
	progress.mu.Lock()
	defer progress.mu.Unlock()
	if progress.disabled || progress.completed || progress.sequence >= 256 {
		return
	}
	label = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, label)
	if strings.TrimSpace(label) == "" || len(label) > 256 {
		label = "Development task"
	}
	progress.sequence++
	encoded, err := json.Marshal(devStartupProgressEvent{1, progress.session, progress.sequence, phase, label, status, elapsed})
	if err != nil || progress.bytes+len(encoded)+1 > 32*1024 {
		progress.disabled = true
		return
	}
	encoded = append(encoded, '\n')
	written, err := progress.file.Write(encoded)
	progress.bytes += written
	if err != nil || written != len(encoded) {
		progress.disabled = true
	}
	if phase == "startup" && status != "started" {
		progress.completed = true
	}
}

// close retires only the optional stream descriptor; its caller owns file cleanup.
func (progress *devStartupProgress) close() {
	if progress != nil {
		_ = progress.file.Close()
	}
}

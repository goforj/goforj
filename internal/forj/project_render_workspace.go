package forj

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/goforj/goforj/project"
)

// projectRenderWorkspace anchors one renderer invocation without changing process-wide working-directory state.
type projectRenderWorkspace struct {
	root string
}

// projectRenderLogicalError preserves boundary context while exposing only project-relative path details.
type projectRenderLogicalError struct {
	message string
	cause   error
}

// Error returns the original boundary message with its physical project path normalized.
func (e *projectRenderLogicalError) Error() string {
	return e.message
}

// Unwrap retains the normalized cause so errors.Is and errors.As keep their standard behavior.
func (e *projectRenderLogicalError) Unwrap() error {
	return e.cause
}

// resolveProjectRenderWorkspace normalizes the invocation root once so every boundary observes the same project.
func resolveProjectRenderWorkspace(root string) (projectRenderWorkspace, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return projectRenderWorkspace{}, fmt.Errorf("resolve project render root %q: %w", root, err)
	}
	return projectRenderWorkspace{root: filepath.Clean(absolute)}, nil
}

// path applies the invocation root only at filesystem, process, and external-library boundaries.
func (w projectRenderWorkspace) path(parts ...string) string {
	if w.root == "" {
		panic("project render workspace must be resolved before use")
	}
	logical := filepath.Join(parts...)
	if logical == "" {
		logical = "."
	}
	if filepath.IsAbs(logical) {
		return logical
	}
	if logical == "." {
		return w.root
	}
	return filepath.Join(w.root, logical)
}

// logicalLabel rewrites physical paths under the invocation root while leaving external paths intact.
func (w projectRenderWorkspace) logicalLabel(path string) string {
	if relative, err := filepath.Rel(w.path(), path); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return relative
	}
	return path
}

// logicalError rewrites filesystem path labels back to project-relative names while preserving operation and cause.
func (w projectRenderWorkspace) logicalError(err error) error {
	if err == nil {
		return nil
	}
	switch typed := err.(type) {
	case *os.LinkError:
		return &os.LinkError{
			Op:  typed.Op,
			Old: w.logicalLabel(typed.Old),
			New: w.logicalLabel(typed.New),
			Err: typed.Err,
		}
	case *os.PathError:
		return &os.PathError{Op: typed.Op, Path: w.logicalLabel(typed.Path), Err: typed.Err}
	case interface{ Unwrap() error }:
		cause := typed.Unwrap()
		if cause == nil {
			return err
		}
		normalized := w.logicalError(cause)
		if normalized.Error() == cause.Error() {
			return err
		}
		message := strings.Replace(err.Error(), cause.Error(), normalized.Error(), 1)
		return &projectRenderLogicalError{message: message, cause: normalized}
	}
	return err
}

// readFile reads one logical project file without exposing the physical invocation root to renderer logic.
func (w projectRenderWorkspace) readFile(parts ...string) ([]byte, error) {
	data, err := os.ReadFile(w.path(parts...))
	return data, w.logicalError(err)
}

// writeFile publishes one logical project file while keeping generated paths relative everywhere else.
func (w projectRenderWorkspace) writeFile(path string, data []byte, mode fs.FileMode) error {
	return w.logicalError(os.WriteFile(w.path(path), data, mode))
}

// writeFileAtomically protects renderer-owned contracts from becoming visible before a complete replacement exists.
func (w projectRenderWorkspace) writeFileAtomically(path string, data []byte, mode fs.FileMode) error {
	return w.logicalError(writeFileAtomically(w.path(path), data, mode))
}

// loadProjectConfig keeps configuration discovery on the invocation root and reports its conventional filename.
func (w projectRenderWorkspace) loadProjectConfig() (*project.Config, error) {
	config, err := project.LoadProjectConfigAt(w.path())
	return config, w.logicalError(err)
}

// writeProjectConfig publishes the renderer-owned configuration through the workspace error boundary.
func (w projectRenderWorkspace) writeProjectConfig(config *project.Config) error {
	return w.logicalError(writeProjectConfig(w.path(".goforj.yml"), config))
}

// writeEnvironmentExample redacts and publishes the generated example without leaking its physical root in errors.
func (w projectRenderWorkspace) writeEnvironmentExample(source []byte, mode fs.FileMode) error {
	return w.logicalError(WriteEnvironmentExampleAtomic(w.path(".env.example"), source, mode))
}

// ensureGitignoreEnvironmentRules preserves owner-authored rules while reporting the logical project filename.
func (w projectRenderWorkspace) ensureGitignoreEnvironmentRules() error {
	return w.logicalError(ensureGitignoreEnvironmentRules(w.path(".gitignore")))
}

// exists distinguishes a missing logical project path from filesystem failures that must stop rendering.
func (w projectRenderWorkspace) exists(parts ...string) (bool, error) {
	_, err := os.Stat(w.path(parts...))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, w.logicalError(err)
}

// stat retains file metadata only for renderer decisions that cannot be expressed as presence checks.
func (w projectRenderWorkspace) stat(parts ...string) (fs.FileInfo, error) {
	info, err := os.Stat(w.path(parts...))
	return info, w.logicalError(err)
}

// readDir returns logical project directory contents without making callers assemble rooted paths.
func (w projectRenderWorkspace) readDir(parts ...string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(w.path(parts...))
	return entries, w.logicalError(err)
}

// ensureDir creates a logical project directory before renderer publication or owner migration.
func (w projectRenderWorkspace) ensureDir(parts ...string) error {
	return w.logicalError(os.MkdirAll(w.path(parts...), 0o755))
}

// removeTree removes a framework-owned logical directory and all generated descendants.
func (w projectRenderWorkspace) removeTree(parts ...string) error {
	return w.logicalError(os.RemoveAll(w.path(parts...)))
}

// removeFileIfExists reports whether a conventional generated file was present.
func (w projectRenderWorkspace) removeFileIfExists(parts ...string) (bool, error) {
	if err := os.Remove(w.path(parts...)); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, w.logicalError(err)
	}
	return true, nil
}

// removeTreeIfExists reports whether a conventional generated directory was present before recursive removal.
func (w projectRenderWorkspace) removeTreeIfExists(parts ...string) (bool, error) {
	exists, err := w.exists(parts...)
	if err != nil || !exists {
		return false, err
	}
	if err := w.removeTree(parts...); err != nil {
		return false, err
	}
	remaining, err := w.exists(parts...)
	if err != nil {
		return false, err
	}
	if remaining {
		return false, fmt.Errorf("remove directory %s: still exists after removal", filepath.Join(parts...))
	}
	return true, nil
}

// removeEmptyDir removes a generated shell only when owner files do not keep it populated.
func (w projectRenderWorkspace) removeEmptyDir(parts ...string) (bool, error) {
	if err := os.Remove(w.path(parts...)); err != nil {
		if os.IsNotExist(err) || errors.Is(err, syscall.ENOTEMPTY) {
			return false, nil
		}
		return false, w.logicalError(err)
	}
	return true, nil
}

// move renames one logical project path without exposing physical workspace paths to migration plans.
func (w projectRenderWorkspace) move(source string, target string) error {
	return w.logicalError(os.Rename(w.path(source), w.path(target)))
}

// discoveryRoot supplies the resolved invocation root to project layout discovery.
func (w projectRenderWorkspace) discoveryRoot() string {
	return w.path()
}

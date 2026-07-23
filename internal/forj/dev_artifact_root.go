package forj

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/project"
)

// devArtifactRootEnv keeps development-only generated artifacts outside a project checkout when requested.
const devArtifactRootEnv = "FORJ_DEV_ARTIFACT_ROOT"

// devArtifactRoot resolves the inherited development artifact root and rejects paths that could still write into the checkout.
func devArtifactRoot() (string, error) {
	value := strings.TrimSpace(os.Getenv(devArtifactRootEnv))
	if value == "" {
		return "", nil
	}
	if !filepath.IsAbs(value) {
		return "", fmt.Errorf("%s must be an absolute path", devArtifactRootEnv)
	}
	root, err := filepath.Abs(".")
	if err != nil {
		return "", fmt.Errorf("resolve project checkout: %w", err)
	}
	root, err = resolveDevArtifactPath(root)
	if err != nil {
		return "", fmt.Errorf("resolve project checkout: %w", err)
	}
	artifactRoot, err := resolveDevArtifactPath(filepath.Clean(value))
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", devArtifactRootEnv, err)
	}
	if artifactRoot == root || pathWithin(artifactRoot, root) {
		return "", fmt.Errorf("%s must be outside the project checkout", devArtifactRootEnv)
	}
	return artifactRoot, nil
}

// resolveDevArtifactPath follows existing path components so a symlink cannot place artifacts back inside the checkout.
func resolveDevArtifactPath(path string) (string, error) {
	missing := make([]string, 0)
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			return filepath.Join(append([]string{resolved}, missing...)...), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		missing = append([]string{filepath.Base(path)}, missing...)
		path = parent
	}
}

// pathWithin reports whether path is nested beneath parent without confusing common lexical prefixes.
func pathWithin(path string, parent string) bool {
	relative, err := filepath.Rel(parent, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// devRuntimeExecutable preserves the historic project-relative command spelling unless an external artifact root is configured.
func devRuntimeExecutable(app project.App) string {
	root, err := devArtifactRoot()
	if err != nil || root == "" {
		return projectlayout.RuntimeExecutable(".", app)
	}
	return filepath.Join(root, "bin", projectlayout.NormalizeApp(app).Name)
}

// devRuntimeShellExecutable returns the runtime executable in a form safe to interpolate into a POSIX shell command.
func devRuntimeShellExecutable(app project.App) string {
	return devArtifactShellPath(filepath.ToSlash(devRuntimeExecutable(app)))
}

// devArtifactShellPath preserves historic command text unless an external artifact root supplies a path to interpolate.
func devArtifactShellPath(path string) string {
	root, err := devArtifactRoot()
	if err != nil || root == "" {
		return path
	}
	return shellSingleQuote(path)
}

// devRuntimeBinary returns the filesystem artifact path used for publication and cleanup.
func devRuntimeBinary(app project.App) string {
	root, err := devArtifactRoot()
	if err != nil || root == "" {
		return projectlayout.RuntimeBinary(".", app)
	}
	return filepath.Join(root, "bin", projectlayout.NormalizeApp(app).Name)
}

// devRuntimeReadyStamp keeps readiness publication beside the selected runtime artifact.
func devRuntimeReadyStamp(app project.App) string {
	root, err := devArtifactRoot()
	if err != nil || root == "" {
		return projectlayout.RuntimeReadyStamp(".", app)
	}
	return filepath.Join(root, "bin", "."+projectlayout.NormalizeApp(app).Name+".ready")
}

// devArtifactBuildEnvironment directs Go's derived build cache outside the checkout with the other dev artifacts.
func devArtifactBuildEnvironment(env map[string]string) map[string]string {
	root, err := devArtifactRoot()
	if err != nil || root == "" {
		return env
	}
	result := copyDevWatchEnv(env)
	result["GOCACHE"] = filepath.Join(root, ".gocache")
	return result
}

// ensureDevArtifactDir creates the selected runtime output directory before development starts.
func ensureDevArtifactDir() error {
	root, err := devArtifactRoot()
	if err != nil {
		return err
	}
	path := "bin"
	if root != "" {
		path = filepath.Join(root, "bin")
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("ensure dev artifact directory: %w", err)
	}
	return nil
}

// Package envcontract owns safe synchronization between local, example, and testing dotenv files.
package envcontract

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/crypt"
	"github.com/goforj/goforj/internal/envfile"
)

const (
	localEnvironmentName   = ".env"
	exampleEnvironmentName = ".env.example"
	testingEnvironmentName = ".env.testing"
)

// ErrExampleMissing reports that a local environment cannot be initialized without its committed contract.
var ErrExampleMissing = errors.New(".env.example is missing")

// SyncResult reports which committed contracts changed without exposing their contents.
type SyncResult struct {
	ExampleChanged bool
	TestingChanged bool
}

// Changed reports whether synchronization published either committed contract.
func (r SyncResult) Changed() bool {
	return r.ExampleChanged || r.TestingChanged
}

// Sync updates the safe example and runnable testing contracts from the local environment when it exists.
func Sync(root string) (SyncResult, error) {
	root, err := absoluteRoot(root)
	if err != nil {
		return SyncResult{}, err
	}
	local, err := os.ReadFile(filepath.Join(root, localEnvironmentName))
	if os.IsNotExist(err) {
		return SyncResult{}, nil
	}
	if err != nil {
		return SyncResult{}, fmt.Errorf("read %s: %w", localEnvironmentName, err)
	}
	example, testing, err := expectedContracts(root, local)
	if err != nil {
		return SyncResult{}, err
	}
	result := SyncResult{}
	result.ExampleChanged, err = writeIfChanged(filepath.Join(root, exampleEnvironmentName), example, 0o644, true)
	if err != nil {
		return SyncResult{}, fmt.Errorf("write %s: %w", exampleEnvironmentName, err)
	}
	result.TestingChanged, err = writeIfChanged(filepath.Join(root, testingEnvironmentName), testing, 0o644, true)
	if err != nil {
		return SyncResult{}, fmt.Errorf("write %s: %w", testingEnvironmentName, err)
	}
	return result, nil
}

// Check verifies that committed contracts match the output synchronization would publish.
func Check(root string) error {
	root, err := absoluteRoot(root)
	if err != nil {
		return err
	}
	local, localErr := os.ReadFile(filepath.Join(root, localEnvironmentName))
	if localErr != nil && !os.IsNotExist(localErr) {
		return fmt.Errorf("read %s: %w", localEnvironmentName, localErr)
	}
	if os.IsNotExist(localErr) {
		local, err = os.ReadFile(filepath.Join(root, exampleEnvironmentName))
		if err != nil {
			return fmt.Errorf("read %s: %w", exampleEnvironmentName, err)
		}
	}
	example, testing, err := expectedContracts(root, local)
	if err != nil {
		return err
	}
	stale := make([]string, 0, 2)
	if current, readErr := os.ReadFile(filepath.Join(root, exampleEnvironmentName)); readErr != nil || !bytes.Equal(current, example) {
		stale = append(stale, exampleEnvironmentName)
	}
	if current, readErr := os.ReadFile(filepath.Join(root, testingEnvironmentName)); readErr != nil || !bytes.Equal(current, testing) {
		stale = append(stale, testingEnvironmentName)
	}
	if len(stale) > 0 {
		return fmt.Errorf("environment contract is stale: %s; run forj generate", strings.Join(stale, ", "))
	}
	return nil
}

// Initialize creates a private local environment from the committed example and generates framework-owned secrets.
func Initialize(root string) (bool, error) {
	root, err := absoluteRoot(root)
	if err != nil {
		return false, err
	}
	localPath := filepath.Join(root, localEnvironmentName)
	if _, err := os.Stat(localPath); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("inspect %s: %w", localEnvironmentName, err)
	}
	example, err := os.ReadFile(filepath.Join(root, exampleEnvironmentName))
	if os.IsNotExist(err) {
		return false, ErrExampleMissing
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", exampleEnvironmentName, err)
	}
	local, err := materializeLocal(example)
	if err != nil {
		return false, err
	}
	file, err := os.OpenFile(localPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create %s: %w", localEnvironmentName, err)
	}
	removeOnFailure := true
	defer func() {
		_ = file.Close()
		if removeOnFailure {
			_ = os.Remove(localPath)
		}
	}()
	if _, err := file.Write(local); err != nil {
		return false, fmt.Errorf("write %s: %w", localEnvironmentName, err)
	}
	if err := file.Sync(); err != nil {
		return false, fmt.Errorf("sync %s: %w", localEnvironmentName, err)
	}
	if err := file.Close(); err != nil {
		return false, fmt.Errorf("close %s: %w", localEnvironmentName, err)
	}
	removeOnFailure = false
	return true, nil
}

// SetLocal stores one prompted local value privately so the next generation lifecycle can publish its safe contract entry.
func SetLocal(root string, key string, value string) error {
	key = strings.TrimSpace(key)
	if !envfile.IsValidKey(key) {
		return fmt.Errorf("environment key %q must contain only letters, digits, and underscores and cannot start with a digit", key)
	}
	if value == "" {
		return fmt.Errorf("environment value for %s cannot be empty", key)
	}
	if _, err := Initialize(root); err != nil {
		return err
	}
	root, err := absoluteRoot(root)
	if err != nil {
		return err
	}
	path := filepath.Join(root, localEnvironmentName)
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", localEnvironmentName, err)
	}
	lines := strings.Split(string(content), "\n")
	lines = envfile.SetFinal(lines, key, envfile.EncodeValue(value))
	updated := []byte(strings.Join(lines, "\n"))
	if err := writeAtomic(path, updated, 0o600, false); err != nil {
		return fmt.Errorf("write %s: %w", localEnvironmentName, err)
	}
	return nil
}

// expectedContracts derives both safe outputs while preserving their current project-owned additions.
func expectedContracts(root string, local []byte) ([]byte, []byte, error) {
	existingExample, err := readOptional(filepath.Join(root, exampleEnvironmentName))
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", exampleEnvironmentName, err)
	}
	example := envfile.MergeExample(existingExample, local)
	existingTesting, err := readOptional(filepath.Join(root, testingEnvironmentName))
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", testingEnvironmentName, err)
	}
	return example, envfile.MergeTesting(existingTesting, example), nil
}

// materializeLocal fills only framework-owned secret blanks and leaves application credentials for the developer.
func materializeLocal(example []byte) ([]byte, error) {
	lines := strings.Split(string(example), "\n")
	generators := map[string]func() (string, error){
		"APP_KEY": crypt.GenerateAppKey,
		"APP_DIAG_TOKEN": func() (string, error) {
			return randomToken(24)
		},
		"LIGHTHOUSE_SECRET": func() (string, error) {
			return randomToken(24)
		},
		"API_JWT_SECRET_KEY": func() (string, error) {
			return randomToken(36)
		},
	}
	for key, generate := range generators {
		value, found := envfile.Lookup(lines, key)
		if !found || strings.TrimSpace(value) != "" {
			continue
		}
		generated, err := generate()
		if err != nil {
			return nil, fmt.Errorf("generate local %s: %w", key, err)
		}
		lines = envfile.SetFinal(lines, key, envfile.EncodeValue(generated))
	}
	return []byte(strings.Join(lines, "\n")), nil
}

// randomToken returns URL-safe random bytes without modulo bias or shell-significant characters.
func randomToken(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// absoluteRoot normalizes every contract path through the platform filepath implementation.
func absoluteRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve environment root: %w", err)
	}
	return absolute, nil
}

// readOptional returns an absent contract as empty input without hiding other filesystem failures.
func readOptional(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return content, err
}

// writeIfChanged avoids timestamp churn when the committed contract is already current.
func writeIfChanged(path string, content []byte, mode fs.FileMode, preserveMode bool) (bool, error) {
	current, err := os.ReadFile(path)
	if err == nil && bytes.Equal(current, content) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if err := writeAtomic(path, content, mode, preserveMode); err != nil {
		return false, err
	}
	return true, nil
}

// writeAtomic publishes a complete same-directory replacement and optionally preserves owner-selected permissions.
func writeAtomic(path string, content []byte, defaultMode fs.FileMode, preserveMode bool) error {
	mode := defaultMode.Perm()
	if preserveMode {
		if info, err := os.Stat(path); err == nil {
			mode = info.Mode().Perm()
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

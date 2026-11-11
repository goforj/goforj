package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/goforj/goforj/internal/devwatch"
)

// fileState fingerprints public files without opening private layers or symlink targets.
type fileState struct {
	SHA256  string `json:"sha256,omitempty"`
	Mode    uint32 `json:"mode,omitempty"`
	Private bool   `json:"private,omitempty"`
	Symlink string `json:"symlink,omitempty"`
}

// inventory records source changes without following links or generated dependency trees.
func inventory(root string) (map[string]fileState, error) {
	return inventoryTree(root, true)
}

// inventoryTree includes tracked dependency trees when verifying reference inspection purity.
func inventoryTree(root string, skipGenerated bool) (map[string]fileState, error) {
	result := map[string]fileState{}
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == root {
			return nil
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if strings.HasPrefix(strings.ToLower(entry.Name()), ".env") {
			result[relative] = fileState{Private: true}
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if skipGenerated && (entry.Name() == ".git" || entry.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(name)
			if err != nil {
				return err
			}
			result[relative] = fileState{Symlink: target}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		digest, err := digestFile(name)
		if err != nil {
			return err
		}
		result[relative] = fileState{SHA256: digest, Mode: uint32(info.Mode().Perm())}
		return nil
	})
	return result, err
}

// digestFile fingerprints a local input without exporting its contents.
func digestFile(name string) (string, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// copyFile preserves the requested permissions independently of the process umask.
func copyFile(source, destination string, mode fs.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	modeErr := output.Chmod(mode.Perm())
	return errors.Join(copyErr, modeErr, output.Close())
}

// writeJSON keeps retained evidence readable and newline terminated.
func writeJSON(name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(name, append(data, '\n'), 0o600)
}

// copyCommittedApp exports only committed regular files into a disposable tree.
func copyCommittedApp(ctx context.Context, source, destination, revision string) error {
	var archive, stderr bytes.Buffer
	code, err := runCommand(ctx, source, []string{"git", "archive", "--format=tar", revision}, trialEnvironment(), &archive, &stderr)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("git archive failed with exit code %d: %s", code, stderr.String())
	}
	return extractCommittedArchive(bytes.NewReader(archive.Bytes()), destination)
}

// extractCommittedArchive excludes private layers and refuses paths escaping the destination.
func extractCommittedArchive(source io.Reader, destination string) error {
	entries := tar.NewReader(source)
	for {
		entry, err := entries.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.Typeflag != tar.TypeReg && entry.Typeflag != tar.TypeRegA {
			continue
		}
		parts := strings.Split(entry.Name, "/")
		private := false
		for _, part := range parts {
			if strings.HasPrefix(strings.ToLower(part), ".env") {
				private = true
			}
			if part == ".." {
				return fmt.Errorf("archive contains unsafe path %q", entry.Name)
			}
		}
		if path.IsAbs(entry.Name) || !filepath.IsLocal(filepath.FromSlash(entry.Name)) || strings.Contains(entry.Name, "\\") {
			return fmt.Errorf("archive contains unsafe path %q", entry.Name)
		}
		if private {
			continue
		}
		target := filepath.Join(destination, filepath.FromSlash(entry.Name))
		// OpenRoot prevents existing destination symlinks from escaping the disposable tree.
		if err := os.MkdirAll(destination, 0o755); err != nil {
			return err
		}
		root, err := os.OpenRoot(destination)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(destination, target)
		if err == nil {
			err = root.MkdirAll(filepath.Dir(relative), 0o755)
		}
		if err != nil {
			root.Close()
			return err
		}
		output, err := root.OpenFile(relative, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fs.FileMode(entry.Mode)&0o777)
		if err != nil {
			root.Close()
			return err
		}
		_, copyErr := io.Copy(output, entries)
		modeErr := output.Chmod(fs.FileMode(entry.Mode) & 0o777)
		closeErr := output.Close()
		rootErr := root.Close()
		if err := errors.Join(copyErr, modeErr, closeErr, rootErr); err != nil {
			return err
		}
	}
}

// trialEnvironment limits validation to public test settings and required tool locations.
func trialEnvironment() []string {
	values := map[string]string{}
	for _, key := range []string{"PATH", "HOME", "GOCACHE", "GOMODCACHE", "SYSTEMROOT", "USERPROFILE"} {
		if value, ok := os.LookupEnv(key); ok {
			values[key] = value
		}
	}
	values["TMPDIR"] = "/tmp"
	values["GOWORK"] = "off"
	values["APP_ENV"] = "testing"
	values["DB_DRIVER"] = "sqlite"
	values["API_JWT_SECRET_KEY"] = "public-agent-upgrade-test-key-0123456789"
	if values["GOCACHE"] == "" {
		values["GOCACHE"] = "/tmp/gocache"
	}
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	sort.Strings(result)
	return result
}

// testResult retains test identities and package failures separately from command success.
type testResult struct {
	ExitCode        int      `json:"exit_code"`
	Passed          []string `json:"passed"`
	Failed          []string `json:"failed"`
	Skipped         []string `json:"skipped"`
	PackageFailures []string `json:"package_failures"`
	Packages        []string `json:"packages"`
}

// parseTests retains failures even when the surrounding command reports success.
func parseTests(source io.Reader, exitCode int) (testResult, error) {
	sets := make([]map[string]bool, 5)
	for i := range sets {
		sets[i] = map[string]bool{}
	}
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var event struct{ Package, ImportPath, Action, Test string }
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		pkg := event.Package
		if pkg == "" {
			pkg = event.ImportPath
		}
		if pkg == "" {
			pkg = "unknown"
		}
		switch event.Action {
		case "start", "pass", "fail", "skip", "build-fail":
			if pkg != "unknown" {
				sets[4][pkg] = true
			}
		}
		if event.Test != "" {
			index := -1
			switch event.Action {
			case "pass":
				index = 0
			case "fail":
				index = 1
			case "skip":
				index = 2
			}
			if index >= 0 {
				sets[index][pkg+"::"+event.Test] = true
			}
		} else if event.Action == "fail" || event.Action == "build-fail" {
			sets[3][pkg] = true
		}
	}
	lists := make([][]string, 5)
	for i, set := range sets {
		lists[i] = []string{}
		for value := range set {
			lists[i] = append(lists[i], value)
		}
		sort.Strings(lists[i])
	}
	return testResult{ExitCode: exitCode, Passed: lists[0], Failed: lists[1], Skipped: lists[2], PackageFailures: lists[3], Packages: lists[4]}, scanner.Err()
}

// runGo retains complete command evidence and bounds each validation invocation.
func runGo(ctx context.Context, app, evaluation, name string, args []string) (testResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	log, err := os.Create(filepath.Join(evaluation, name+".json"))
	if err != nil {
		return testResult{}, err
	}
	code, runErr := runCommand(ctx, app, append([]string{"go"}, args...), trialEnvironment(), log, log)
	if errors.Is(runErr, context.DeadlineExceeded) {
		code = 124
		runErr = nil
	}
	closeErr := log.Close()
	if err := errors.Join(runErr, closeErr); err != nil {
		return testResult{}, err
	}
	input, err := os.Open(filepath.Join(evaluation, name+".json"))
	if err != nil {
		return testResult{}, err
	}
	defer input.Close()
	return parseTests(input, code)
}

// runCommand owns the native process tree through cancellation and normal leader exit.
func runCommand(ctx context.Context, dir string, args, env []string, stdout, stderr io.Writer) (int, error) {
	environment := map[string]string{}
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			environment[key] = value
		}
	}
	supervisor := devwatch.NewSupervisor(devwatch.SupervisorOptions{StopTimeout: 100 * time.Millisecond})
	defer supervisor.Close()
	exit, err := supervisor.Run(ctx, "upgrade-spike", devwatch.Command{Args: args, Dir: dir, Env: environment, ReplaceEnv: true, Stdout: stdout, Stderr: stderr})
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && !exit.Intentional() {
			return exit.ExitCode, nil
		}
		return exit.ExitCode, err
	}
	return exit.ExitCode, nil
}
